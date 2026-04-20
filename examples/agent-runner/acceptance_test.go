package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/llm"
	"github.com/darunshen/AIR/internal/session"
)

func TestScriptedAgentWorkflowAcceptance(t *testing.T) {
	t.Helper()

	cases := []struct {
		name   string
		assert func(*testing.T, taskReport)
	}{
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
				statusStep := findStepByCommand(result.Steps, "cat status.txt")
				if statusStep == nil {
					t.Fatal("expected status verification step")
				}
				if strings.TrimSpace(statusStep.Stdout) != "verified" {
					t.Fatalf("unexpected status stdout: %q", statusStep.Stdout)
				}
				deleteStep := findStepByKind(result.Steps, "session_delete")
				if deleteStep == nil || !deleteStep.Success {
					t.Fatalf("expected successful delete step, got %+v", deleteStep)
				}
			},
		},
		{
			name: "session-recovery",
			assert: func(t *testing.T, result taskReport) {
				t.Helper()

				failStep := findStepByCommand(result.Steps, "sh -c 'echo boom >&2; exit 7'")
				if failStep == nil {
					t.Fatal("expected failure probe step")
				}
				if failStep.ExitCode != 7 {
					t.Fatalf("unexpected failure exit code: %d", failStep.ExitCode)
				}
				if !strings.Contains(failStep.Stderr, "boom") {
					t.Fatalf("unexpected failure stderr: %q", failStep.Stderr)
				}

				recoveryStep := findStepByCommand(result.Steps, "cat recovery.txt")
				if recoveryStep == nil {
					t.Fatal("expected recovery verification step")
				}
				if strings.TrimSpace(recoveryStep.Stdout) != "recovered" {
					t.Fatalf("unexpected recovery stdout: %q", recoveryStep.Stdout)
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
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := newScriptedAcceptanceRunner(t)
			results, err := r.runSelected(context.Background(), tt.name)
			if err != nil {
				t.Fatalf("run selected %s: %v", tt.name, err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 task result, got %d", len(results))
			}

			result := results[0]
			if !result.Success {
				t.Fatalf("task %s failed: %+v", tt.name, result)
			}
			if len(result.Steps) == 0 {
				t.Fatalf("task %s returned no steps", tt.name)
			}

			tt.assert(t, result)
		})
	}
}

func newScriptedAcceptanceRunner(t *testing.T) *runner {
	t.Helper()

	root := t.TempDir()
	manager, err := session.NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	return &runner{
		manager:        manager,
		plannerName:    "scripted",
		plannerConfig:  llm.Config{Provider: "scripted"},
		plannerFactory: newRunnerPlanner,
		commandTimeout: 5 * time.Second,
	}
}

func findStepByKind(steps []stepReport, kind string) *stepReport {
	for i := range steps {
		if steps[i].Kind == kind {
			return &steps[i]
		}
	}
	return nil
}

func findStepByCommand(steps []stepReport, command string) *stepReport {
	for i := range steps {
		if steps[i].Command == command {
			return &steps[i]
		}
	}
	return nil
}
