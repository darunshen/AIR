package main

import (
	"context"
	"fmt"
	"strings"
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
	SummaryCommand     string
}

func repoBugfixTaskSpec() taskSpec {
	return taskSpec{
		Name:               "repo-bugfix",
		Mode:               "session",
		Goal:               "In one persistent session, fix the demo repo under demo-repo so that `cd demo-repo && sh tests/test.sh` exits 0 and prints TEST PASSED. The repo already contains README.md, src/lib.sh, src/message.sh, and tests/test.sh. Inspect the repo, understand the failure, update the implementation, rerun the tests, and finish successfully only after the repo test passes.",
		AllowedActionTypes: []string{"session_exec", "finish"},
		MaxSteps:           10,
		SetupFn:            setupRepoBugfixFixture,
		FinishCheckCommand: "cd demo-repo && sh tests/test.sh",
		SummaryCommand:     `cd demo-repo && printf 'Updated file:\n- src/lib.sh\n\nCurrent src/lib.sh:\n' && sed -n '1,120p' src/lib.sh && printf '\nCurrent demo output:\n' && sh src/message.sh && printf '\nExpected verification:\n- sh tests/test.sh -> TEST PASSED\n'`,
	}
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
				SummaryCommand:     `printf 'Updated file:\n- app.sh\n\nCurrent app.sh:\n' && sed -n '1,120p' app.sh && printf '\nExpected verification:\n- sh test.sh -> TEST PASSED\n'`,
			}),
			r.runTask(ctx, repoBugfixTaskSpec()),
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
			SummaryCommand:     `printf 'Updated file:\n- app.sh\n\nCurrent app.sh:\n' && sed -n '1,120p' app.sh && printf '\nExpected verification:\n- sh test.sh -> TEST PASSED\n'`,
		})}, nil
	case "repo-bugfix":
		return []taskReport{r.runTask(ctx, repoBugfixTaskSpec())}, nil
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
	r.tracef("[agent/task] start name=%s mode=%s provider=%s max_steps=%d", spec.Name, spec.Mode, resolvedProvider(r.provider), spec.MaxSteps)

	var (
		sessionID string
		history   []llm.StepObservation
	)

	if spec.Mode == "session" {
		s, err := r.manager.CreateWithProvider(r.provider)
		if err != nil {
			r.tracef("[agent/session] create failed task=%s provider=%s error=%s", spec.Name, resolvedProvider(r.provider), err)
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
		r.tracef("[agent/session] created task=%s session_id=%s provider=%s", spec.Name, s.ID, s.Provider)
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
			r.tracef("[agent/task] setup start task=%s session_id=%s", spec.Name, s.ID)
			setupSteps, setupHistory, err := spec.SetupFn(r, s.ID)
			result.Steps = append(result.Steps, setupSteps...)
			history = append(history, setupHistory...)
			if err != nil {
				r.tracef("[agent/task] setup failed task=%s session_id=%s error=%s", spec.Name, s.ID, err)
				result.Success = false
				result.ErrorMessage = firstError(result.ErrorMessage, err.Error())
				return result
			}
			r.tracef("[agent/task] setup done task=%s session_id=%s", spec.Name, s.ID)
		}
	}

	for stepNum := 1; stepNum <= spec.MaxSteps; stepNum++ {
		request := llm.PlanRequest{
			TaskName:           spec.Name,
			Goal:               spec.Goal,
			Mode:               spec.Mode,
			AllowedActionTypes: spec.AllowedActionTypes,
			Provider:           resolvedProvider(r.provider),
			SessionID:          sessionID,
			Step:               stepNum,
			MaxSteps:           spec.MaxSteps,
			History:            history,
		}
		action, plannerSteps, err := r.nextPlannerAction(ctx, request)
		result.Steps = append(result.Steps, plannerSteps...)
		if err != nil {
			r.tracef("[agent/plan] error task=%s step=%d session_id=%s error=%s", spec.Name, stepNum, sessionID, err)
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
		r.tracef("[agent/plan] action task=%s step=%d type=%s session_id=%s command=%q reason=%q", spec.Name, stepNum, action.Type, sessionID, action.Command, action.Reason)

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
			r.tracef("[agent/plan] finish task=%s step=%d success=%t summary=%q", spec.Name, stepNum, action.FinishSuccess, action.FinishSummary)
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
			if spec.SummaryCommand != "" && spec.Mode == "session" {
				summaryStep := r.runSessionExec(sessionID, &llm.PlanAction{
					Type:    "session_exec",
					Command: spec.SummaryCommand,
					Reason:  "capture a delivery-style final summary for the completed task",
				})
				summaryStep.Name = "final_summary"
				summaryStep.Kind = "final_summary"
				result.Steps = append(result.Steps, summaryStep)
				if text := summaryText(summaryStep); text != "" {
					result.FinalSummary = text
				}
			}
			r.tracef("[agent/task] done name=%s success=%t session_id=%s", spec.Name, result.Success, sessionID)
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
	r.tracef("[agent/task] max-steps-exceeded name=%s session_id=%s", spec.Name, sessionID)
	return result
}

func (r *runner) nextPlannerAction(ctx context.Context, req llm.PlanRequest) (*llm.PlanAction, []stepReport, error) {
	models := plannerModelCandidates(r.plannerName, r.model, r.escalationModel)
	if len(models) == 0 {
		return nil, nil, fmt.Errorf("no planner model configured")
	}

	attemptsPerModel := r.plannerRetries + 1
	if attemptsPerModel <= 0 {
		attemptsPerModel = 1
	}

	var plannerSteps []stepReport
	var lastErr error
	globalAttempt := 0

	for modelIndex, model := range models {
		cfg := r.plannerConfig
		if model != "" {
			cfg.Model = model
		}
		planner, _, err := r.plannerFactory(cfg)
		if err != nil {
			return nil, plannerSteps, err
		}

		for attempt := 1; attempt <= attemptsPerModel; attempt++ {
			if modelIndex > 0 && attempt == 1 {
				r.tracef("[agent/plan] escalate task=%s step=%d from=%s to=%s", req.TaskName, req.Step, models[modelIndex-1], model)
				plannerSteps = append(plannerSteps, stepReport{
					Name:           fmt.Sprintf("plan_%02d_escalation", req.Step),
					Kind:           "planner_escalation",
					SessionID:      req.SessionID,
					PlannerModel:   model,
					PlannerAttempt: globalAttempt + 1,
					Success:        true,
					Note:           fmt.Sprintf("escalate planner model from %s to %s", models[modelIndex-1], model),
				})
			}
			globalAttempt++
			r.tracef("[agent/plan] request task=%s step=%d attempt=%d model=%s", req.TaskName, req.Step, globalAttempt, model)
			action, err := planner.NextAction(ctx, req)
			if err == nil {
				r.tracef("[agent/plan] response task=%s step=%d attempt=%d model=%s type=%s command=%q", req.TaskName, req.Step, globalAttempt, model, action.Type, action.Command)
				if globalAttempt > 1 {
					plannerSteps = append(plannerSteps, stepReport{
						Name:           fmt.Sprintf("plan_%02d_recovered", req.Step),
						Kind:           "planner_recovered",
						SessionID:      req.SessionID,
						PlannerModel:   model,
						PlannerAttempt: globalAttempt,
						Success:        true,
						Note:           "planner returned a valid action after retry/escalation",
					})
				}
				return action, plannerSteps, nil
			}

			lastErr = err
			r.tracef("[agent/plan] retry task=%s step=%d attempt=%d model=%s error=%s", req.TaskName, req.Step, globalAttempt, model, err)
			stepKind := "planner_retry"
			note := "retry the same planner model"
			if model == "" {
				note = strings.TrimSpace("retry scripted planner")
			}
			plannerSteps = append(plannerSteps, stepReport{
				Name:           fmt.Sprintf("plan_%02d_attempt_%02d", req.Step, globalAttempt),
				Kind:           stepKind,
				SessionID:      req.SessionID,
				PlannerModel:   model,
				PlannerAttempt: globalAttempt,
				Success:        false,
				ErrorMessage:   err.Error(),
				Note:           note,
			})
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("planner did not return an action")
	}
	return nil, plannerSteps, fmt.Errorf("planner failed after %d attempt(s): %w", globalAttempt, lastErr)
}

func (r *runner) runOneShot(action *llm.PlanAction, timeout time.Duration) stepReport {
	if timeout <= 0 {
		timeout = r.commandTimeout
	}

	r.tracef("[agent/run] start provider=%s timeout=%s command=%q", resolvedProvider(r.provider), timeout, action.Command)
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
		r.tracef("[agent/run] error provider=%s session_id=%s request_id=%s error=%s", resolvedProvider(r.provider), step.SessionID, step.RequestID, err)
		return step
	}
	r.tracef("[agent/run] done provider=%s session_id=%s request_id=%s exit_code=%d duration_ms=%d timeout=%t", resolvedProvider(r.provider), step.SessionID, step.RequestID, step.ExitCode, step.DurationMS, step.Timeout)
	r.tracef("[agent/run] stdout=%q stderr=%q", previewText(step.Stdout, 400), previewText(step.Stderr, 400))
	return step
}

func (r *runner) runSessionExec(sessionID string, action *llm.PlanAction) stepReport {
	r.tracef("[agent/exec] start session_id=%s command=%q", sessionID, action.Command)
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
		r.tracef("[agent/exec] error session_id=%s request_id=%s error=%s", sessionID, step.RequestID, err)
		return step
	}
	r.tracef("[agent/exec] done session_id=%s request_id=%s exit_code=%d duration_ms=%d timeout=%t", sessionID, step.RequestID, step.ExitCode, step.DurationMS, step.Timeout)
	r.tracef("[agent/exec] stdout=%q stderr=%q", previewText(step.Stdout, 400), previewText(step.Stderr, 400))
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

func summaryText(step stepReport) string {
	if text := strings.TrimSpace(step.Stdout); text != "" {
		return text
	}
	return strings.TrimSpace(step.Stderr)
}

func (r *runner) appendDeleteStep(result *taskReport, sessionID string) {
	r.tracef("[agent/session] delete start session_id=%s", sessionID)
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
		r.tracef("[agent/session] delete failed session_id=%s error=%s", sessionID, err)
	}
	result.Steps = append(result.Steps, step)
	if err == nil {
		r.tracef("[agent/session] delete done session_id=%s", sessionID)
	}
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

	r.tracef("[agent/setup] fixture start session_id=%s", sessionID)
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
		r.tracef("[agent/setup] fixture error session_id=%s request_id=%s error=%s", sessionID, step.RequestID, err)
		return []stepReport{step}, []llm.StepObservation{observationFromStep(step)}, err
	}
	if result != nil && result.ExitCode != 0 {
		step.Success = false
		step.ErrorMessage = fmt.Sprintf("fixture setup failed with exit code %d", result.ExitCode)
		r.tracef("[agent/setup] fixture nonzero session_id=%s request_id=%s exit_code=%d", sessionID, step.RequestID, result.ExitCode)
		return []stepReport{step}, []llm.StepObservation{observationFromStep(step)}, fmt.Errorf("fixture setup failed with exit code %d", result.ExitCode)
	}
	r.tracef("[agent/setup] fixture done session_id=%s request_id=%s", sessionID, step.RequestID)

	observation := llm.StepObservation{
		Name:    step.Name,
		Kind:    step.Kind,
		Success: true,
		Note:    "Fixture ready: app.sh currently prints helo; test.sh passes only if app.sh prints hello exactly.",
	}
	return []stepReport{step}, []llm.StepObservation{observation}, nil
}

func setupRepoBugfixFixture(r *runner, sessionID string) ([]stepReport, []llm.StepObservation, error) {
	setupCommand := `mkdir -p demo-repo/src demo-repo/tests
cat > demo-repo/README.md <<'EOF'
# Demo Repo

src/message.sh should print exactly hello air.

The implementation currently uses src/lib.sh and the repo test suite is:

sh tests/test.sh
EOF
cat > demo-repo/src/lib.sh <<'EOF'
#!/bin/sh

build_greeting() {
  name="$1"
  printf 'helo %s\n' "$name"
}
EOF
chmod +x demo-repo/src/lib.sh
cat > demo-repo/src/message.sh <<'EOF'
#!/bin/sh
. "$(dirname "$0")/lib.sh"
build_greeting air
EOF
chmod +x demo-repo/src/message.sh
cat > demo-repo/tests/test.sh <<'EOF'
#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
. ./src/lib.sh

lib_output=$(build_greeting air)
[ "$lib_output" = "hello air" ] || { echo "lib expected hello air, got: $lib_output" >&2; exit 1; }

script_output=$(sh ./src/message.sh)
[ "$script_output" = "hello air" ] || { echo "script expected hello air, got: $script_output" >&2; exit 1; }

echo TEST PASSED
EOF
chmod +x demo-repo/tests/test.sh`

	r.tracef("[agent/setup] repo fixture start session_id=%s", sessionID)
	result, err := r.manager.Exec(sessionID, setupCommand)
	step := stepReport{
		Name:      "setup_repo_bugfix_fixture",
		Kind:      "task_setup",
		Command:   setupCommand,
		SessionID: sessionID,
		Note:      "fixture created: demo-repo has a broken greeting implementation and a repo-level test suite",
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
		r.tracef("[agent/setup] repo fixture error session_id=%s request_id=%s error=%s", sessionID, step.RequestID, err)
		return []stepReport{step}, []llm.StepObservation{observationFromStep(step)}, err
	}
	if result != nil && result.ExitCode != 0 {
		step.Success = false
		step.ErrorMessage = fmt.Sprintf("repo fixture setup failed with exit code %d", result.ExitCode)
		r.tracef("[agent/setup] repo fixture nonzero session_id=%s request_id=%s exit_code=%d", sessionID, step.RequestID, result.ExitCode)
		return []stepReport{step}, []llm.StepObservation{observationFromStep(step)}, fmt.Errorf("repo fixture setup failed with exit code %d", result.ExitCode)
	}
	r.tracef("[agent/setup] repo fixture done session_id=%s request_id=%s", sessionID, step.RequestID)

	observation := llm.StepObservation{
		Name:    step.Name,
		Kind:    step.Kind,
		Success: true,
		Note:    "Fixture ready: demo-repo is a small multi-file shell repo. src/lib.sh returns the wrong greeting, src/message.sh uses that helper, and tests/test.sh expects hello air from both paths.",
	}
	return []stepReport{step}, []llm.StepObservation{observation}, nil
}
