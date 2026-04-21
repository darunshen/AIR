package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/darunshen/AIR/internal/llm"
	"github.com/darunshen/AIR/internal/session"
)

type report struct {
	StartedAt       time.Time    `json:"started_at"`
	Planner         string       `json:"planner"`
	Model           string       `json:"model,omitempty"`
	EscalationModel string       `json:"escalation_model,omitempty"`
	PlannerRetries  int          `json:"planner_retries,omitempty"`
	Provider        string       `json:"provider"`
	Task            string       `json:"task"`
	Success         bool         `json:"success"`
	Tasks           []taskReport `json:"tasks"`
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
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Command        string `json:"command,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	PlannerModel   string `json:"planner_model,omitempty"`
	PlannerAttempt int    `json:"planner_attempt,omitempty"`
	Stdout         string `json:"stdout,omitempty"`
	Stderr         string `json:"stderr,omitempty"`
	ExitCode       int    `json:"exit_code,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	Timeout        bool   `json:"timeout,omitempty"`
	Success        bool   `json:"success"`
	ErrorMessage   string `json:"error_message,omitempty"`
	Note           string `json:"note,omitempty"`
}

type runner struct {
	manager         *session.Manager
	provider        string
	plannerName     string
	plannerConfig   llm.Config
	plannerFactory  plannerFactory
	model           string
	escalationModel string
	plannerRetries  int
	commandTimeout  time.Duration
	traceEnabled    bool
}

func main() {
	task := flag.String("task", "all", "task to run: all, run-smoke, session-workflow, session-recovery, test-and-fix, repo-bugfix")
	provider := flag.String("provider", "", "runtime provider: local or firecracker")
	plannerName := flag.String("planner", "", "planner backend: openai or scripted")
	model := flag.String("model", "", "planner model override")
	escalationModel := flag.String("escalation-model", "", "planner escalation model override")
	plannerRetries := flag.Int("planner-retries", -1, "planner retries before escalation")
	reasoning := flag.String("reasoning", "", "planner reasoning override")
	flag.Parse()

	cfg := llm.ResolveConfigFromEnv()
	if *plannerName != "" {
		cfg.Provider = *plannerName
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *escalationModel == "" {
		*escalationModel = os.Getenv("AIR_AGENT_ESCALATION_MODEL")
	}
	if *reasoning != "" {
		cfg.Reasoning = *reasoning
	}
	cfg = llm.NormalizeConfig(cfg)

	_, resolvedPlanner, err := newRunnerPlanner(cfg)
	if err != nil {
		exitJSONError(err)
	}

	manager, err := session.NewManager()
	if err != nil {
		exitJSONError(err)
	}

	r := &runner{
		manager:         manager,
		provider:        *provider,
		plannerName:     resolvedPlanner,
		plannerFactory:  newRunnerPlanner,
		model:           cfg.Model,
		escalationModel: strings.TrimSpace(*escalationModel),
		plannerRetries:  resolvePlannerRetries(*plannerRetries),
		commandTimeout:  10 * time.Second,
		traceEnabled:    envBool("AIR_AGENT_TRACE"),
	}
	cfg.Logger = r.tracef
	r.plannerConfig = cfg

	ctx := context.Background()
	result := report{
		StartedAt:       time.Now().UTC(),
		Planner:         resolvedPlanner,
		Model:           plannerModel(resolvedPlanner, cfg.Model),
		EscalationModel: plannerModel(resolvedPlanner, strings.TrimSpace(*escalationModel)),
		PlannerRetries:  resolvePlannerRetries(*plannerRetries),
		Provider:        resolvedProvider(*provider),
		Task:            *task,
		Success:         true,
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

func resolvePlannerRetries(value int) int {
	if value >= 0 {
		return value
	}
	if raw := os.Getenv("AIR_AGENT_PLANNER_RETRIES"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return 1
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

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (r *runner) tracef(format string, args ...any) {
	if !r.traceEnabled {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...(truncated)"
}
