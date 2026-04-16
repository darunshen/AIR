package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeepSeekPlannerNextAction(t *testing.T) {
	t.Helper()

	var seenAuth string
	var seenReq deepSeekChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{"type":"run","command":"printf hello","reason":"print hello","finish_success":false,"finish_summary":""}`,
					},
				},
			},
		})
	}))
	defer server.Close()

	planner, err := NewDeepSeekPlanner(Config{
		Provider: "deepseek",
		APIKey:   "test-key",
		Model:    "deepseek-chat",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	action, err := planner.NextAction(context.Background(), PlanRequest{
		TaskName:           "run-smoke",
		Goal:               "print hello",
		Mode:               "run",
		AllowedActionTypes: []string{"run", "finish"},
		Provider:           "local",
		Step:               1,
		MaxSteps:           3,
	})
	if err != nil {
		t.Fatalf("next action: %v", err)
	}
	if seenAuth != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", seenAuth)
	}
	if seenReq.Model != "deepseek-chat" {
		t.Fatalf("unexpected model: %q", seenReq.Model)
	}
	if seenReq.ResponseFormat.Type != "json_object" {
		t.Fatalf("expected json_object response format, got %q", seenReq.ResponseFormat.Type)
	}
	if action.Type != "run" || action.Command != "printf hello" {
		t.Fatalf("unexpected action: %+v", action)
	}
}
