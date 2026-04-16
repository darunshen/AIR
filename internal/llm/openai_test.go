package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIPlannerNextAction(t *testing.T) {
	t.Helper()

	var seenAuth string
	var seenReq openAIResponsesRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&seenReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"output_text": `{"type":"session_exec","command":"cat note.txt","reason":"read the note file","finish_success":false,"finish_summary":""}`,
		})
	}))
	defer server.Close()

	planner, err := NewOpenAIPlanner(Config{
		APIKey:    "test-key",
		Model:     "gpt-5.4-mini",
		Reasoning: "medium",
		BaseURL:   server.URL,
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	action, err := planner.NextAction(context.Background(), PlanRequest{
		TaskName:           "session-workflow",
		Goal:               "read note.txt",
		Mode:               "session",
		AllowedActionTypes: []string{"session_exec", "finish"},
		Provider:           "local",
		SessionID:          "sess_123",
		Step:               2,
		MaxSteps:           6,
	})
	if err != nil {
		t.Fatalf("next action: %v", err)
	}
	if seenAuth != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", seenAuth)
	}
	if seenReq.Model != "gpt-5.4-mini" {
		t.Fatalf("unexpected model: %q", seenReq.Model)
	}
	if seenReq.Text.Format.Type != "json_schema" {
		t.Fatalf("expected json_schema format, got %q", seenReq.Text.Format.Type)
	}
	if action.Type != "session_exec" || action.Command != "cat note.txt" {
		t.Fatalf("unexpected action: %+v", action)
	}
}
