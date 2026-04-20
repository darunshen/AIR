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

type deepSeekPlanner struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
	client  *http.Client
	logger  func(format string, args ...any)
}

type deepSeekChatCompletionRequest struct {
	Model          string                  `json:"model"`
	Messages       []deepSeekChatMessage   `json:"messages"`
	ResponseFormat deepSeekResponseFormat  `json:"response_format"`
	MaxTokens      int                     `json:"max_tokens,omitempty"`
	Thinking       *deepSeekThinkingConfig `json:"thinking,omitempty"`
}

type deepSeekChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekResponseFormat struct {
	Type string `json:"type"`
}

type deepSeekThinkingConfig struct {
	Type string `json:"type"`
}

type deepSeekChatCompletionResponse struct {
	Error   *deepSeekAPIError        `json:"error"`
	Choices []deepSeekChoiceResponse `json:"choices"`
}

type deepSeekChoiceResponse struct {
	Message deepSeekChoiceMessage `json:"message"`
}

type deepSeekChoiceMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type deepSeekAPIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func NewDeepSeekPlanner(cfg Config) (Planner, error) {
	cfg = NormalizeConfig(cfg)
	if cfg.APIKey == "" {
		return nil, errors.New("DEEPSEEK_API_KEY is required for deepseek planner")
	}
	return &deepSeekPlanner{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		timeout: cfg.Timeout,
		client:  &http.Client{Timeout: cfg.Timeout},
		logger:  cfg.Logger,
	}, nil
}

func (p *deepSeekPlanner) NextAction(ctx context.Context, req PlanRequest) (*PlanAction, error) {
	payload, err := p.newRequest(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if p.logger != nil {
		p.logger("[llm/deepseek] request model=%s task=%s step=%d max_steps=%d", p.model, req.TaskName, req.Step, req.MaxSteps)
		p.logger("[llm/deepseek] request body=%s", previewText(string(body), 1200))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
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
	if p.logger != nil {
		p.logger("[llm/deepseek] response status=%d body=%s", resp.StatusCode, previewText(string(respBody), 1200))
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("deepseek chat completions api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded deepSeekChatCompletionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("deepseek api error (%s/%s): %s", decoded.Error.Type, decoded.Error.Code, decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return nil, errors.New("deepseek response did not contain choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return nil, errors.New("deepseek response returned empty content")
	}

	var action PlanAction
	if err := json.Unmarshal([]byte(content), &action); err != nil {
		return nil, fmt.Errorf("decode deepseek planner action: %w", err)
	}
	if err := validatePlanAction(action, req); err != nil {
		return nil, err
	}
	return &action, nil
}

func (p *deepSeekPlanner) newRequest(req PlanRequest) (*deepSeekChatCompletionRequest, error) {
	requestJSON, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.TrimSpace(`
You are the planner for AIR, an isolated execution runtime for AI agents.

You must output valid json only.

Rules:
- Use only the allowed action types listed in the request.
- Return exactly one next action as json.
- For "run" tasks, prefer a single deterministic shell command.
- For "session" tasks, choose the next shell command based on prior step outputs.
- Use concise POSIX shell commands. Do not use markdown fences.
- Do not rely on network access.
- When the task goal is satisfied, return type "finish" with finish_success=true and a short finish_summary.
- If the task is blocked or has failed irrecoverably, return type "finish" with finish_success=false and explain why in finish_summary.

The expected json shape is:
{
  "type": "run | session_exec | finish",
  "command": "shell command or empty string for finish",
  "reason": "why this is the right next action",
  "finish_success": false,
  "finish_summary": ""
}
`)

	userPrompt := "Plan the next action for this AIR task and return json only.\n\n" + string(requestJSON)

	payload := &deepSeekChatCompletionRequest{
		Model: p.model,
		Messages: []deepSeekChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: deepSeekResponseFormat{Type: "json_object"},
		MaxTokens:      1024,
	}
	if p.model == "deepseek-reasoner" {
		payload.Thinking = &deepSeekThinkingConfig{Type: "enabled"}
	}
	return payload, nil
}
