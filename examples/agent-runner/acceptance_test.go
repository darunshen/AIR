package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/llm"
	"github.com/darunshen/AIR/internal/session"
)

type acceptanceCase struct {
	name   string
	assert func(*testing.T, taskReport)
}

func TestScriptedAgentWorkflowAcceptance(t *testing.T) {
	t.Helper()

	for _, tt := range acceptanceCases() {
		t.Run(tt.name, func(t *testing.T) {
			r := newScriptedAcceptanceRunner(t)
			runAndAssertAcceptanceTask(t, r, tt)
		})
	}
}

func TestRealLLMAgentWorkflowAcceptance(t *testing.T) {
	t.Helper()

	if os.Getenv("AIR_LLM_ACCEPTANCE") != "1" {
		t.Skip("set AIR_LLM_ACCEPTANCE=1 to run real LLM acceptance test")
	}

	cfg := resolveAcceptancePlannerConfig(t)
	provider := getenvDefault("AIR_AGENT_RUNTIME_PROVIDER", "local")
	r := newAcceptanceRunner(t, cfg, provider)

	selected := resolveAcceptanceTasks(t)
	result := report{
		StartedAt:       time.Now().UTC(),
		Planner:         r.plannerName,
		Model:           plannerModel(r.plannerName, r.model),
		EscalationModel: plannerModel(r.plannerName, r.escalationModel),
		PlannerRetries:  r.plannerRetries,
		Provider:        resolvedProvider(provider),
		Task:            strings.Join(selected, ","),
		Success:         true,
	}
	defer func() {
		writeAcceptanceArtifact(t, result)
	}()

	available := map[string]acceptanceCase{}
	for _, tt := range acceptanceCases() {
		available[tt.name] = tt
	}

	for _, name := range selected {
		tt, ok := available[name]
		if !ok {
			t.Fatalf("unsupported acceptance task %q", name)
		}
		passed := t.Run(tt.name, func(t *testing.T) {
			taskResult, err := runAcceptanceTask(r, tt.name)
			result.Tasks = append(result.Tasks, taskResult)
			if err != nil {
				t.Fatal(err)
			}
			if !taskResult.Success {
				t.Fatalf("task %s failed: %+v", tt.name, taskResult)
			}
			tt.assert(t, taskResult)
		})
		if !passed {
			result.Success = false
		}
	}
	if len(result.Tasks) != len(selected) {
		result.Success = false
	}
}

func newScriptedAcceptanceRunner(t *testing.T) *runner {
	t.Helper()

	return newAcceptanceRunner(t, llm.Config{Provider: "scripted"}, "")
}

func newAcceptanceRunner(t *testing.T, cfg llm.Config, provider string) *runner {
	t.Helper()

	root := t.TempDir()
	manager, err := session.NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	_, resolvedPlanner, err := newRunnerPlanner(cfg)
	if err != nil {
		t.Fatalf("resolve planner: %v", err)
	}

	plannerRetries := resolvePlannerRetries(-1)
	r := &runner{
		manager:         manager,
		provider:        provider,
		plannerName:     resolvedPlanner,
		plannerFactory:  newRunnerPlanner,
		model:           cfg.Model,
		escalationModel: strings.TrimSpace(os.Getenv("AIR_AGENT_ESCALATION_MODEL")),
		plannerRetries:  plannerRetries,
		commandTimeout:  5 * time.Second,
		traceEnabled:    envBool("AIR_AGENT_TRACE"),
	}
	cfg.Logger = r.tracef
	r.plannerConfig = cfg
	return r
}

func acceptanceCases() []acceptanceCase {
	return []acceptanceCase{
		{
			name: "run-smoke",
			assert: func(t *testing.T, result taskReport) {
				t.Helper()

				runStep := findStepByKind(result.Steps, "run")
				if runStep == nil {
					t.Fatal("expected run step")
				}
				if strings.TrimSpace(runStep.Stdout) != "hello" {
					t.Fatalf("unexpected run stdout: %q", runStep.Stdout)
				}
				if runStep.ExitCode != 0 {
					t.Fatalf("unexpected run exit code: %d", runStep.ExitCode)
				}
			},
		},
		{
			name: "session-workflow",
			assert: func(t *testing.T, result taskReport) {
				t.Helper()

				if result.SessionID == "" {
					t.Fatal("expected session id")
				}
				if findStepByKind(result.Steps, "session_create") == nil {
					t.Fatal("expected session_create step")
				}
				if findStepByKind(result.Steps, "session_delete") == nil {
					t.Fatal("expected session_delete step")
				}
				if !hasStepStdout(result.Steps, "verified") {
					t.Fatalf("expected verification stdout in steps: %+v", result.Steps)
				}
			},
		},
		{
			name: "session-recovery",
			assert: func(t *testing.T, result taskReport) {
				t.Helper()

				failStep := findStepWithExitCode(result.Steps, 7)
				if failStep == nil {
					t.Fatal("expected a failing probe step with exit 7")
				}
				if !strings.Contains(failStep.Stderr, "boom") {
					t.Fatalf("unexpected failure stderr: %q", failStep.Stderr)
				}
				if !hasStepStdout(result.Steps, "recovered") {
					t.Fatalf("expected recovery stdout in steps: %+v", result.Steps)
				}
			},
		},
		{
			name: "test-and-fix",
			assert: func(t *testing.T, result taskReport) {
				t.Helper()

				setupStep := findStepByKind(result.Steps, "task_setup")
				if setupStep == nil || !setupStep.Success {
					t.Fatalf("expected successful setup step, got %+v", setupStep)
				}

				verifyStep := findStepByKind(result.Steps, "finish_verification")
				if verifyStep == nil {
					t.Fatal("expected finish verification step")
				}
				if verifyStep.ExitCode != 0 {
					t.Fatalf("unexpected finish verification exit code: %d", verifyStep.ExitCode)
				}
				if !strings.Contains(verifyStep.Stdout, "TEST PASSED") {
					t.Fatalf("unexpected finish verification stdout: %q", verifyStep.Stdout)
				}
				if !strings.Contains(result.FinalSummary, "app.sh") {
					t.Fatalf("expected final summary to mention app.sh, got: %q", result.FinalSummary)
				}
				if !strings.Contains(result.FinalSummary, "hello") {
					t.Fatalf("expected final summary to mention fixed hello output, got: %q", result.FinalSummary)
				}
			},
		},
		{
			name: "repo-bugfix",
			assert: func(t *testing.T, result taskReport) {
				t.Helper()

				setupStep := findStepByKind(result.Steps, "task_setup")
				if setupStep == nil || !setupStep.Success {
					t.Fatalf("expected successful repo setup step, got %+v", setupStep)
				}

				if !hasStepStdout(result.Steps, "TEST PASSED") {
					t.Fatalf("expected repo test success stdout in steps: %+v", result.Steps)
				}

				verifyStep := findStepByKind(result.Steps, "finish_verification")
				if verifyStep == nil {
					t.Fatal("expected repo finish verification step")
				}
				if verifyStep.ExitCode != 0 {
					t.Fatalf("unexpected repo finish verification exit code: %d", verifyStep.ExitCode)
				}
				if !strings.Contains(result.FinalSummary, "src/lib.sh") {
					t.Fatalf("expected final summary to mention src/lib.sh, got: %q", result.FinalSummary)
				}
				if !strings.Contains(result.FinalSummary, "hello air") {
					t.Fatalf("expected final summary to mention hello air, got: %q", result.FinalSummary)
				}
			},
		},
	}
}

func runAndAssertAcceptanceTask(t *testing.T, r *runner, tt acceptanceCase) {
	t.Helper()

	result, err := runAcceptanceTask(r, tt.name)
	if err != nil {
		t.Fatalf("run selected %s: %v", tt.name, err)
	}
	if !result.Success {
		t.Fatalf("task %s failed: %+v", tt.name, result)
	}

	tt.assert(t, result)
}

func runAcceptanceTask(r *runner, name string) (taskReport, error) {
	results, err := r.runSelected(context.Background(), name)
	if err != nil {
		return taskReport{
			Name:         name,
			Provider:     resolvedProvider(r.provider),
			Success:      false,
			ErrorMessage: err.Error(),
		}, fmt.Errorf("run selected %s: %w", name, err)
	}
	if len(results) != 1 {
		return taskReport{
			Name:         name,
			Provider:     resolvedProvider(r.provider),
			Success:      false,
			ErrorMessage: fmt.Sprintf("expected 1 task result, got %d", len(results)),
		}, fmt.Errorf("expected 1 task result, got %d", len(results))
	}

	result := results[0]
	if len(result.Steps) == 0 {
		result.Success = false
		if result.ErrorMessage == "" {
			result.ErrorMessage = "task returned no steps"
		}
		return result, fmt.Errorf("task %s returned no steps", name)
	}
	return result, nil
}

func resolveAcceptancePlannerConfig(t *testing.T) llm.Config {
	t.Helper()

	cfg := llm.ResolveConfigFromEnv()
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	if cfg.Provider == "" || cfg.Provider == "scripted" {
		t.Fatalf("AIR_AGENT_PROVIDER must be openai or deepseek for real LLM acceptance, got %q", cfg.Provider)
	}

	loadAPIKeyFromFile(t, cfg.Provider)
	cfg = llm.NormalizeConfig(cfg)
	if cfg.APIKey == "" {
		t.Fatalf("planner %q requires API key", cfg.Provider)
	}
	return cfg
}

func resolveAcceptanceTasks(t *testing.T) []string {
	t.Helper()

	raw := getenvDefault("AIR_AGENT_ACCEPTANCE_TASKS", "run-smoke,session-workflow,test-and-fix,repo-bugfix")
	parts := strings.Split(raw, ",")
	selected := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if slices.Contains(selected, part) {
			continue
		}
		selected = append(selected, part)
	}
	if len(selected) == 0 {
		t.Fatal("AIR_AGENT_ACCEPTANCE_TASKS resolved to empty task set")
	}
	return selected
}

func loadAPIKeyFromFile(t *testing.T, planner string) {
	t.Helper()

	var envName string
	var fileEnvName string
	switch planner {
	case "deepseek":
		envName = "DEEPSEEK_API_KEY"
		fileEnvName = "DEEPSEEK_API_KEY_FILE"
	case "openai":
		envName = "OPENAI_API_KEY"
		fileEnvName = "OPENAI_API_KEY_FILE"
	default:
		t.Fatalf("unsupported planner %q", planner)
	}

	if strings.TrimSpace(os.Getenv(envName)) != "" {
		return
	}

	filePath := strings.TrimSpace(os.Getenv(fileEnvName))
	if filePath == "" {
		return
	}

	body, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", fileEnvName, err)
	}
	t.Setenv(envName, strings.TrimSpace(string(body)))
}

func hasStepStdout(steps []stepReport, want string) bool {
	for _, step := range steps {
		if strings.TrimSpace(step.Stdout) == want {
			return true
		}
	}
	return false
}

func findStepByKind(steps []stepReport, kind string) *stepReport {
	for i := range steps {
		if steps[i].Kind == kind {
			return &steps[i]
		}
	}
	return nil
}

func findStepWithExitCode(steps []stepReport, exitCode int) *stepReport {
	for i := range steps {
		if steps[i].ExitCode == exitCode {
			return &steps[i]
		}
	}
	return nil
}

func writeAcceptanceArtifact(t *testing.T, result report) {
	t.Helper()

	dir := strings.TrimSpace(os.Getenv("AIR_AGENT_ACCEPTANCE_ARTIFACT_DIR"))
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir acceptance artifact dir: %v", err)
	}

	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal acceptance result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), body, 0o644); err != nil {
		t.Fatalf("write acceptance result: %v", err)
	}
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (tt acceptanceCase) String() string {
	return fmt.Sprintf("acceptanceCase(%s)", tt.name)
}
