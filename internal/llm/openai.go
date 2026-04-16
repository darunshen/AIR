package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAIPlanner struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
	client  *http.Client
	effort  string
}

type openAIResponsesRequest struct {
	Model     string                 `json:"model"`
	Input     []openAIInputMessage   `json:"input"`
	Reasoning *openAIReasoningConfig `json:"reasoning,omitempty"`
	Text      openAITextConfig       `json:"text"`
}

type openAIInputMessage struct {
	Role    string            `json:"role"`
	Content []openAIInputText `json:"content"`
}

type openAIInputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIReasoningConfig struct {
	Effort string `json:"effort"`
}

type openAITextConfig struct {
	Format openAITextFormat `json:"format"`
}

type openAITextFormat struct {
	Type        string         `json:"type"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type openAIResponsesResponse struct {
	Error      *openAIAPIError        `json:"error"`
	OutputText string                 `json:"output_text"`
	Output     []openAIResponseOutput `json:"output"`
}

type openAIResponseOutput struct {
	Type    string                  `json:"type"`
	Content []openAIResponseContent `json:"content"`
}

type openAIResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIAPIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func NewOpenAIPlanner(cfg Config) (Planner, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("OPENAI_API_KEY is required for openai planner")
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-5.4-mini"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.Reasoning == "" {
		cfg.Reasoning = "medium"
	}

	return &openAIPlanner{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		timeout: cfg.Timeout,
		client:  &http.Client{Timeout: cfg.Timeout},
		effort:  cfg.Reasoning,
	}, nil
}

func (p *openAIPlanner) NextAction(ctx context.Context, req PlanRequest) (*PlanAction, error) {
	payload, err := p.newRequest(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai responses api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded openAIResponsesResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("openai api error (%s): %s", decoded.Error.Type, decoded.Error.Message)
	}

	text := strings.TrimSpace(decoded.OutputText)
	if text == "" {
		text = strings.TrimSpace(extractOutputText(decoded.Output))
	}
	if text == "" {
		return nil, errors.New("openai response did not contain output_text")
	}

	var action PlanAction
	if err := json.Unmarshal([]byte(text), &action); err != nil {
		return nil, fmt.Errorf("decode openai planner action: %w", err)
	}
	if err := validatePlanAction(action, req); err != nil {
		return nil, err
	}
	return &action, nil
}

func (p *openAIPlanner) newRequest(req PlanRequest) (*openAIResponsesRequest, error) {
	requestJSON, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return nil, err
	}

	developerPrompt := strings.TrimSpace(`
You are the planner for AIR, an isolated execution runtime for AI agents.

You must decide exactly one next action.

Rules:
- Return JSON only, matching the required schema exactly.
- Use only the allowed action types listed in the request.
- For "run" tasks, prefer a single deterministic shell command.
- For "session" tasks, choose the next shell command based on prior step outputs.
- Use concise POSIX shell commands. Do not use markdown fences.
- Do not rely on network access.
- When the task goal is satisfied, return type "finish" with finish_success=true and a short finish_summary.
- If the task is blocked or has failed irrecoverably, return type "finish" with finish_success=false and explain why in finish_summary.
- Fill the reason field with a short explanation of why this is the right next action.
- For non-finish actions, leave finish_summary empty.
`)

	userPrompt := "Plan the next action for this AIR task.\n\n" + string(requestJSON)

	payload := &openAIResponsesRequest{
		Model: p.model,
		Input: []openAIInputMessage{
			{
				Role: "developer",
				Content: []openAIInputText{
					{Type: "input_text", Text: developerPrompt},
				},
			},
			{
				Role: "user",
				Content: []openAIInputText{
					{Type: "input_text", Text: userPrompt},
				},
			},
		},
		Text: openAITextConfig{
			Format: openAITextFormat{
				Type:        "json_schema",
				Name:        "air_next_action",
				Description: "Planner action for the next AIR execution step",
				Strict:      true,
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{
							"type": "string",
							"enum": []string{"run", "session_exec", "finish"},
						},
						"command": map[string]any{
							"type": "string",
						},
						"reason": map[string]any{
							"type": "string",
						},
						"finish_success": map[string]any{
							"type": "boolean",
						},
						"finish_summary": map[string]any{
							"type": "string",
						},
					},
					"required":             []string{"type", "command", "reason", "finish_success", "finish_summary"},
					"additionalProperties": false,
				},
			},
		},
	}
	if p.effort != "" {
		payload.Reasoning = &openAIReasoningConfig{Effort: p.effort}
	}
	return payload, nil
}

func extractOutputText(items []openAIResponseOutput) string {
	for _, item := range items {
		for _, content := range item.Content {
			if content.Type == "output_text" && content.Text != "" {
				return content.Text
			}
		}
	}
	return ""
}

func validatePlanAction(action PlanAction, req PlanRequest) error {
	allowed := make(map[string]struct{}, len(req.AllowedActionTypes))
	for _, item := range req.AllowedActionTypes {
		allowed[item] = struct{}{}
	}
	if _, ok := allowed[action.Type]; !ok {
		return fmt.Errorf("planner returned disallowed action type %q", action.Type)
	}
	if strings.TrimSpace(action.Reason) == "" {
		return errors.New("planner returned empty reason")
	}
	switch action.Type {
	case "run", "session_exec":
		if strings.TrimSpace(action.Command) == "" {
			return fmt.Errorf("planner returned empty command for %s action", action.Type)
		}
	case "finish":
		if strings.TrimSpace(action.FinishSummary) == "" {
			return errors.New("planner returned empty finish_summary")
		}
	}
	return nil
}
