package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/darunshen/AIR/internal/llm"
	"github.com/darunshen/AIR/internal/session"
)

type report struct {
	StartedAt time.Time    `json:"started_at"`
	Planner   string       `json:"planner"`
	Model     string       `json:"model,omitempty"`
	Provider  string       `json:"provider"`
	Task      string       `json:"task"`
	Success   bool         `json:"success"`
	Tasks     []taskReport `json:"tasks"`
}

type taskReport struct {
	Name         string       `json:"name"`
	Provider     string       `json:"provider"`
	SessionID    string       `json:"session_id,omitempty"`
	Success      bool         `json:"success"`
	ErrorMessage string       `json:"error_message,omitempty"`
	Steps        []stepReport `json:"steps"`
}

type stepReport struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Command      string `json:"command,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	Stdout       string `json:"stdout,omitempty"`
	Stderr       string `json:"stderr,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
	Timeout      bool   `json:"timeout,omitempty"`
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	Note         string `json:"note,omitempty"`
}

type runner struct {
	manager        *session.Manager
	provider       string
	plannerName    string
	planner        llm.Planner
	model          string
	commandTimeout time.Duration
}

func main() {
	task := flag.String("task", "all", "task to run: all, run-smoke, session-workflow, session-recovery")
	provider := flag.String("provider", "", "runtime provider: local or firecracker")
	plannerName := flag.String("planner", "", "planner backend: openai or scripted")
	model := flag.String("model", "", "planner model override")
	reasoning := flag.String("reasoning", "", "planner reasoning override")
	flag.Parse()

	cfg := llm.ResolveConfigFromEnv()
	if *plannerName != "" {
		cfg.Provider = *plannerName
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *reasoning != "" {
		cfg.Reasoning = *reasoning
	}
	cfg = llm.NormalizeConfig(cfg)

	planner, resolvedPlanner, err := newRunnerPlanner(cfg)
	if err != nil {
		exitJSONError(err)
	}

	manager, err := session.NewManager()
	if err != nil {
		exitJSONError(err)
	}

	r := &runner{
		manager:        manager,
		provider:       *provider,
		plannerName:    resolvedPlanner,
		planner:        planner,
		model:          cfg.Model,
		commandTimeout: 10 * time.Second,
	}

	ctx := context.Background()
	result := report{
		StartedAt: time.Now().UTC(),
		Planner:   resolvedPlanner,
		Model:     plannerModel(resolvedPlanner, cfg.Model),
		Provider:  resolvedProvider(*provider),
		Task:      *task,
		Success:   true,
	}

	tasks, err := r.runSelected(ctx, *task)
	if err != nil {
		exitJSONError(err)
	}
	result.Tasks = tasks
	for _, taskResult := range tasks {
		if !taskResult.Success {
			result.Success = false
			break
		}
	}

	printJSON(result)
	if !result.Success {
		os.Exit(1)
	}
}

func resolvedProvider(provider string) string {
	if provider == "" {
		return "local"
	}
	return provider
}

func plannerModel(plannerName, model string) string {
	if plannerName == "scripted" {
		return ""
	}
	return model
}

func firstError(existing, fallback string) string {
	if existing != "" {
		return existing
	}
	return fallback
}

func printJSON(v any) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		exitJSONError(err)
	}
	fmt.Println(string(body))
}

func exitJSONError(err error) {
	printJSON(map[string]any{
		"success":       false,
		"error_message": err.Error(),
	})
	os.Exit(1)
}
