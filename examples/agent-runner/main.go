package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/darunshen/AIR/internal/session"
)

type report struct {
	StartedAt time.Time    `json:"started_at"`
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
	manager  *session.Manager
	provider string
}

func main() {
	task := flag.String("task", "all", "task to run: all, run-smoke, session-workflow, session-recovery")
	provider := flag.String("provider", "", "runtime provider: local or firecracker")
	flag.Parse()

	manager, err := session.NewManager()
	if err != nil {
		exitJSONError(err)
	}

	r := &runner{
		manager:  manager,
		provider: *provider,
	}

	result := report{
		StartedAt: time.Now().UTC(),
		Provider:  resolvedProvider(*provider),
		Task:      *task,
		Success:   true,
	}

	tasks, err := r.runSelected(*task)
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

func (r *runner) runSelected(name string) ([]taskReport, error) {
	switch name {
	case "all":
		return []taskReport{
			r.runSmokeTask(),
			r.runSessionWorkflowTask(),
			r.runSessionRecoveryTask(),
		}, nil
	case "run-smoke":
		return []taskReport{r.runSmokeTask()}, nil
	case "session-workflow":
		return []taskReport{r.runSessionWorkflowTask()}, nil
	case "session-recovery":
		return []taskReport{r.runSessionRecoveryTask()}, nil
	default:
		return nil, fmt.Errorf("unsupported task %q", name)
	}
}

func (r *runner) runSmokeTask() taskReport {
	result := taskReport{
		Name:     "run-smoke",
		Provider: resolvedProvider(r.provider),
		Success:  true,
	}

	runResult, err := r.manager.Run("printf hello", session.RunOptions{
		Provider: r.provider,
		Timeout:  5 * time.Second,
	})
	step := stepReport{
		Name:       "run_printf_hello",
		Kind:       "run",
		Command:    "printf hello",
		SessionID:  runResult.SessionID,
		RequestID:  runResult.RequestID,
		Stdout:     runResult.Stdout,
		Stderr:     runResult.Stderr,
		ExitCode:   runResult.ExitCode,
		DurationMS: runResult.DurationMS,
		Timeout:    runResult.Timeout,
		Success:    err == nil && strings.TrimSpace(runResult.Stdout) == "hello" && runResult.ExitCode == 0,
	}
	if err != nil {
		step.ErrorMessage = err.Error()
	}
	result.Provider = runResult.Provider
	result.Steps = append(result.Steps, step)
	if !step.Success {
		result.Success = false
		result.ErrorMessage = firstError(step.ErrorMessage, "expected stdout to equal hello")
	}
	return result
}

func (r *runner) runSessionWorkflowTask() (result taskReport) {
	result = taskReport{
		Name:     "session-workflow",
		Provider: resolvedProvider(r.provider),
		Success:  true,
	}

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
	result.Provider = s.Provider
	result.SessionID = s.ID
	result.Steps = append(result.Steps, stepReport{
		Name:      "create_session",
		Kind:      "session_create",
		SessionID: s.ID,
		Success:   true,
	})
	defer r.appendDeleteStep(&result, s.ID)

	writeStep, writeOK := r.execExpectExit(s.ID, "write_note", "printf hello > note.txt", 0)
	result.Steps = append(result.Steps, writeStep)
	if !writeOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, writeStep.ErrorMessage)
		return result
	}

	readStep, readOK := r.execExpectStdout(s.ID, "read_note", "cat note.txt", "hello")
	result.Steps = append(result.Steps, readStep)
	if !readOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, readStep.ErrorMessage)
		return result
	}

	statusValue := "mismatch"
	statusNote := "note content mismatch, mark session as mismatch"
	if strings.TrimSpace(readStep.Stdout) == "hello" {
		statusValue = "verified"
		statusNote = "note content matched, continue with verified marker"
	}

	markStep, markOK := r.execExpectExit(s.ID, "mark_status", "printf "+statusValue+" > status.txt", 0)
	markStep.Note = statusNote
	result.Steps = append(result.Steps, markStep)
	if !markOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, markStep.ErrorMessage)
		return result
	}

	statusStep, statusOK := r.execExpectStdout(s.ID, "read_status", "cat status.txt", "verified")
	result.Steps = append(result.Steps, statusStep)
	if !statusOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, statusStep.ErrorMessage)
	}
	return result
}

func (r *runner) runSessionRecoveryTask() (result taskReport) {
	result = taskReport{
		Name:     "session-recovery",
		Provider: resolvedProvider(r.provider),
		Success:  true,
	}

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
	result.Provider = s.Provider
	result.SessionID = s.ID
	result.Steps = append(result.Steps, stepReport{
		Name:      "create_session",
		Kind:      "session_create",
		SessionID: s.ID,
		Success:   true,
	})
	defer r.appendDeleteStep(&result, s.ID)

	failStep, failOK := r.execExpectExit(s.ID, "run_failing_command", "sh -c 'echo boom >&2; exit 7'", 7)
	failStep.Note = "expect non-zero exit and continue with recovery"
	result.Steps = append(result.Steps, failStep)
	if !failOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, failStep.ErrorMessage)
		return result
	}

	recoveryStep, recoveryOK := r.execExpectExit(s.ID, "write_recovery_marker", "printf recovered > recovery.txt", 0)
	recoveryStep.Note = "previous step failed as expected, write recovery marker"
	result.Steps = append(result.Steps, recoveryStep)
	if !recoveryOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, recoveryStep.ErrorMessage)
		return result
	}

	verifyStep, verifyOK := r.execExpectStdout(s.ID, "read_recovery_marker", "cat recovery.txt", "recovered")
	result.Steps = append(result.Steps, verifyStep)
	if !verifyOK {
		result.Success = false
		result.ErrorMessage = firstError(result.ErrorMessage, verifyStep.ErrorMessage)
	}
	return result
}

func (r *runner) execExpectExit(sessionID, name, command string, wantExit int) (stepReport, bool) {
	result, err := r.manager.Exec(sessionID, command)
	step := stepReport{
		Name:      name,
		Kind:      "session_exec",
		Command:   command,
		SessionID: sessionID,
	}
	if result != nil {
		step.RequestID = result.RequestID
		step.Stdout = result.Stdout
		step.Stderr = result.Stderr
		step.ExitCode = result.ExitCode
		step.DurationMS = result.Duration.Milliseconds()
		step.Timeout = result.TimedOut
		step.Success = err == nil && result.ExitCode == wantExit
	}
	if err != nil {
		step.ErrorMessage = err.Error()
		return step, false
	}
	if result.ExitCode != wantExit {
		step.ErrorMessage = fmt.Sprintf("unexpected exit code: got %d want %d", result.ExitCode, wantExit)
		return step, false
	}
	return step, true
}

func (r *runner) execExpectStdout(sessionID, name, command, want string) (stepReport, bool) {
	result, err := r.manager.Exec(sessionID, command)
	step := stepReport{
		Name:      name,
		Kind:      "session_exec",
		Command:   command,
		SessionID: sessionID,
	}
	if result != nil {
		step.RequestID = result.RequestID
		step.Stdout = result.Stdout
		step.Stderr = result.Stderr
		step.ExitCode = result.ExitCode
		step.DurationMS = result.Duration.Milliseconds()
		step.Timeout = result.TimedOut
		step.Success = err == nil && result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == want
	}
	if err != nil {
		step.ErrorMessage = err.Error()
		return step, false
	}
	if result.ExitCode != 0 {
		step.ErrorMessage = fmt.Sprintf("unexpected exit code: got %d want 0", result.ExitCode)
		return step, false
	}
	if strings.TrimSpace(result.Stdout) != want {
		step.ErrorMessage = fmt.Sprintf("unexpected stdout: got %q want %q", strings.TrimSpace(result.Stdout), want)
		return step, false
	}
	return step, true
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

func resolvedProvider(provider string) string {
	if provider == "" {
		return "local"
	}
	return provider
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
