package main

import (
	"context"
	"fmt"
	"time"

	"github.com/darunshen/AIR/internal/llm"
	"github.com/darunshen/AIR/internal/session"
)

type taskSpec struct {
	Name               string
	Mode               string
	Goal               string
	AllowedActionTypes []string
	MaxSteps           int
	RunTimeout         time.Duration
	SetupFn            func(*runner, string) ([]stepReport, []llm.StepObservation, error)
	FinishCheckCommand string
}

func (r *runner) runSelected(ctx context.Context, name string) ([]taskReport, error) {
	switch name {
	case "all":
		return []taskReport{
			r.runTask(ctx, taskSpec{
				Name:               "run-smoke",
				Mode:               "run",
				Goal:               "Use one one-shot isolated command to print hello to stdout, then finish successfully.",
				AllowedActionTypes: []string{"run", "finish"},
				MaxSteps:           3,
				RunTimeout:         5 * time.Second,
			}),
			r.runTask(ctx, taskSpec{
				Name:               "session-workflow",
				Mode:               "session",
				Goal:               "In one persistent session, create note.txt with hello, read it back, then write verified to status.txt and read it back before finishing successfully.",
				AllowedActionTypes: []string{"session_exec", "finish"},
				MaxSteps:           6,
			}),
			r.runTask(ctx, taskSpec{
				Name:               "session-recovery",
				Mode:               "session",
				Goal:               "In one persistent session, deliberately run a command that prints boom to stderr and exits 7, then recover by writing recovery.txt with recovered, read it back, and finish successfully.",
				AllowedActionTypes: []string{"session_exec", "finish"},
				MaxSteps:           6,
			}),
			r.runTask(ctx, taskSpec{
				Name:               "test-and-fix",
				Mode:               "session",
				Goal:               "In one persistent session, fix the existing app.sh so that `sh test.sh` exits 0 and prints TEST PASSED. Files already exist in the workspace: app.sh currently prints the wrong output, and test.sh checks that app.sh prints exactly hello. You may inspect files, run tests, and overwrite app.sh. Finish successfully only after the test passes.",
				AllowedActionTypes: []string{"session_exec", "finish"},
				MaxSteps:           8,
				SetupFn:            setupTestAndFixFixture,
				FinishCheckCommand: "sh test.sh",
			}),
		}, nil
	case "run-smoke":
		return []taskReport{r.runTask(ctx, taskSpec{
			Name:               "run-smoke",
			Mode:               "run",
			Goal:               "Use one one-shot isolated command to print hello to stdout, then finish successfully.",
			AllowedActionTypes: []string{"run", "finish"},
			MaxSteps:           3,
			RunTimeout:         5 * time.Second,
		})}, nil
	case "session-workflow":
		return []taskReport{r.runTask(ctx, taskSpec{
			Name:               "session-workflow",
			Mode:               "session",
			Goal:               "In one persistent session, create note.txt with hello, read it back, then write verified to status.txt and read it back before finishing successfully.",
			AllowedActionTypes: []string{"session_exec", "finish"},
			MaxSteps:           6,
		})}, nil
	case "session-recovery":
		return []taskReport{r.runTask(ctx, taskSpec{
			Name:               "session-recovery",
			Mode:               "session",
			Goal:               "In one persistent session, deliberately run a command that prints boom to stderr and exits 7, then recover by writing recovery.txt with recovered, read it back, and finish successfully.",
			AllowedActionTypes: []string{"session_exec", "finish"},
			MaxSteps:           6,
		})}, nil
	case "test-and-fix":
		return []taskReport{r.runTask(ctx, taskSpec{
			Name:               "test-and-fix",
			Mode:               "session",
			Goal:               "In one persistent session, fix the existing app.sh so that `sh test.sh` exits 0 and prints TEST PASSED. Files already exist in the workspace: app.sh currently prints the wrong output, and test.sh checks that app.sh prints exactly hello. You may inspect files, run tests, and overwrite app.sh. Finish successfully only after the test passes.",
			AllowedActionTypes: []string{"session_exec", "finish"},
			MaxSteps:           8,
			SetupFn:            setupTestAndFixFixture,
			FinishCheckCommand: "sh test.sh",
		})}, nil
	default:
		return nil, fmt.Errorf("unsupported task %q", name)
	}
}

func (r *runner) runTask(ctx context.Context, spec taskSpec) (result taskReport) {
	result = taskReport{
		Name:     spec.Name,
		Provider: resolvedProvider(r.provider),
		Success:  true,
	}

	var (
		sessionID string
		history   []llm.StepObservation
	)

	if spec.Mode == "session" {
		s, err := r.manager.CreateWithProvider(r.provider)
		if err != nil {
			result.Success = false
			result.ErrorMessage = err.Error()
			result.Steps = append(result.Steps, stepReport{
				Name:         "create_session",
				Kind:         "session_create",
				Success:      false,
				ErrorMessage: err.Error(),
			})
			return result
		}
		sessionID = s.ID
		result.Provider = s.Provider
		result.SessionID = s.ID
		result.Steps = append(result.Steps, stepReport{
			Name:      "create_session",
			Kind:      "session_create",
			SessionID: s.ID,
			Success:   true,
		})
		defer r.appendDeleteStep(&result, s.ID)

		if spec.SetupFn != nil {
			setupSteps, setupHistory, err := spec.SetupFn(r, s.ID)
			result.Steps = append(result.Steps, setupSteps...)
			history = append(history, setupHistory...)
			if err != nil {
				result.Success = false
				result.ErrorMessage = firstError(result.ErrorMessage, err.Error())
				return result
			}
		}
	}

	for stepNum := 1; stepNum <= spec.MaxSteps; stepNum++ {
		action, err := r.planner.NextAction(ctx, llm.PlanRequest{
			TaskName:           spec.Name,
			Goal:               spec.Goal,
			Mode:               spec.Mode,
			AllowedActionTypes: spec.AllowedActionTypes,
			Provider:           resolvedProvider(r.provider),
			SessionID:          sessionID,
			Step:               stepNum,
			MaxSteps:           spec.MaxSteps,
			History:            history,
		})
		if err != nil {
			result.Success = false
			result.ErrorMessage = err.Error()
			result.Steps = append(result.Steps, stepReport{
				Name:         fmt.Sprintf("plan_%02d", stepNum),
				Kind:         "planner_error",
				SessionID:    sessionID,
				Success:      false,
				ErrorMessage: err.Error(),
			})
			return result
		}

		switch action.Type {
		case "finish":
			step := stepReport{
				Name:      fmt.Sprintf("finish_%02d", stepNum),
				Kind:      "planner_finish",
				SessionID: sessionID,
				Success:   action.FinishSuccess,
				Note:      action.FinishSummary,
			}
			result.Steps = append(result.Steps, step)
			if !action.FinishSuccess {
				result.Success = false
				result.ErrorMessage = firstError(result.ErrorMessage, action.FinishSummary)
				return result
			}
			if spec.FinishCheckCommand != "" && spec.Mode == "session" {
				verifyStep := r.runSessionExec(sessionID, &llm.PlanAction{
					Type:    "session_exec",
					Command: spec.FinishCheckCommand,
					Reason:  "verify the task-specific finish condition",
				})
				verifyStep.Name = "finish_verification"
				verifyStep.Kind = "finish_verification"
				result.Steps = append(result.Steps, verifyStep)
				if !verifyStep.Success || verifyStep.ExitCode != 0 {
					result.Success = false
					result.ErrorMessage = firstError(result.ErrorMessage, "finish verification failed")
				}
			}
			return result
		case "run":
			step := r.runOneShot(action, spec.RunTimeout)
			result.Steps = append(result.Steps, step)
			history = append(history, observationFromStep(step))
			if !step.Success {
				result.Success = false
				result.ErrorMessage = firstError(result.ErrorMessage, step.ErrorMessage)
			}
		case "session_exec":
			step := r.runSessionExec(sessionID, action)
			result.Steps = append(result.Steps, step)
			history = append(history, observationFromStep(step))
			if !step.Success {
				result.Success = false
				result.ErrorMessage = firstError(result.ErrorMessage, step.ErrorMessage)
			}
		default:
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("planner returned unsupported action type %q", action.Type)
			result.Steps = append(result.Steps, stepReport{
				Name:         fmt.Sprintf("plan_%02d", stepNum),
				Kind:         "planner_error",
				SessionID:    sessionID,
				Success:      false,
				ErrorMessage: result.ErrorMessage,
			})
			return result
		}
	}

	result.Success = false
	result.ErrorMessage = firstError(result.ErrorMessage, "planner exceeded max steps")
	result.Steps = append(result.Steps, stepReport{
		Name:         "max_steps_exceeded",
		Kind:         "planner_finish",
		SessionID:    sessionID,
		Success:      false,
		ErrorMessage: "planner exceeded max steps",
	})
	return result
}

func (r *runner) runOneShot(action *llm.PlanAction, timeout time.Duration) stepReport {
	if timeout <= 0 {
		timeout = r.commandTimeout
	}

	result, err := r.manager.Run(action.Command, session.RunOptions{
		Provider: r.provider,
		Timeout:  timeout,
	})
	step := stepReport{
		Name:    "run",
		Kind:    "run",
		Command: action.Command,
		Note:    action.Reason,
		Success: err == nil,
	}
	if result != nil {
		step.SessionID = result.SessionID
		step.RequestID = result.RequestID
		step.Stdout = result.Stdout
		step.Stderr = result.Stderr
		step.ExitCode = result.ExitCode
		step.DurationMS = result.DurationMS
		step.Timeout = result.Timeout
	}
	if err != nil {
		step.ErrorMessage = err.Error()
		return step
	}
	return step
}

func (r *runner) runSessionExec(sessionID string, action *llm.PlanAction) stepReport {
	result, err := r.manager.Exec(sessionID, action.Command)
	step := stepReport{
		Name:      "session_exec",
		Kind:      "session_exec",
		Command:   action.Command,
		SessionID: sessionID,
		Note:      action.Reason,
	}
	if result != nil {
		step.RequestID = result.RequestID
		step.Stdout = result.Stdout
		step.Stderr = result.Stderr
		step.ExitCode = result.ExitCode
		step.DurationMS = result.Duration.Milliseconds()
		step.Timeout = result.TimedOut
		step.Success = err == nil
	}
	if err != nil {
		step.ErrorMessage = err.Error()
		return step
	}
	return step
}

func observationFromStep(step stepReport) llm.StepObservation {
	return llm.StepObservation{
		Name:         step.Name,
		Kind:         step.Kind,
		Command:      step.Command,
		RequestID:    step.RequestID,
		Stdout:       step.Stdout,
		Stderr:       step.Stderr,
		ExitCode:     step.ExitCode,
		DurationMS:   step.DurationMS,
		Timeout:      step.Timeout,
		Success:      step.Success,
		ErrorMessage: step.ErrorMessage,
		Note:         step.Note,
	}
}

func (r *runner) appendDeleteStep(result *taskReport, sessionID string) {
	err := r.manager.Delete(sessionID)
	step := stepReport{
		Name:      "delete_session",
		Kind:      "session_delete",
		SessionID: sessionID,
		Success:   err == nil,
	}
	if err != nil {
		step.ErrorMessage = err.Error()
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, err.Error())
	}
	result.Steps = append(result.Steps, step)
}

func setupTestAndFixFixture(r *runner, sessionID string) ([]stepReport, []llm.StepObservation, error) {
	setupCommand := `cat > app.sh <<'EOF'
#!/bin/sh
echo helo
EOF
chmod +x app.sh
cat > test.sh <<'EOF'
#!/bin/sh
output=$(sh app.sh)
[ "$output" = "hello" ] || { echo "expected hello, got: $output" >&2; exit 1; }
echo TEST PASSED
EOF
chmod +x test.sh`

	result, err := r.manager.Exec(sessionID, setupCommand)
	step := stepReport{
		Name:      "setup_test_and_fix_fixture",
		Kind:      "task_setup",
		Command:   setupCommand,
		SessionID: sessionID,
		Note:      "fixture created: app.sh prints helo; test.sh expects hello",
		Success:   err == nil,
	}
	if result != nil {
		step.RequestID = result.RequestID
		step.Stdout = result.Stdout
		step.Stderr = result.Stderr
		step.ExitCode = result.ExitCode
		step.DurationMS = result.Duration.Milliseconds()
		step.Timeout = result.TimedOut
	}
	if err != nil {
		step.ErrorMessage = err.Error()
		return []stepReport{step}, []llm.StepObservation{observationFromStep(step)}, err
	}
	if result != nil && result.ExitCode != 0 {
		step.Success = false
		step.ErrorMessage = fmt.Sprintf("fixture setup failed with exit code %d", result.ExitCode)
		return []stepReport{step}, []llm.StepObservation{observationFromStep(step)}, fmt.Errorf("fixture setup failed with exit code %d", result.ExitCode)
	}

	observation := llm.StepObservation{
		Name:    step.Name,
		Kind:    step.Kind,
		Success: true,
		Note:    "Fixture ready: app.sh currently prints helo; test.sh passes only if app.sh prints hello exactly.",
	}
	return []stepReport{step}, []llm.StepObservation{observation}, nil
}
