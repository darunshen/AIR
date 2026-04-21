package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/darunshen/AIR/internal/llm"
)

type plannerFactory func(llm.Config) (llm.Planner, string, error)

func newRunnerPlanner(cfg llm.Config) (llm.Planner, string, error) {
	switch cfg.Provider {
	case "", "openai":
		planner, err := llm.New(cfg)
		return planner, "openai", err
	case "deepseek":
		planner, err := llm.New(cfg)
		return planner, "deepseek", err
	case "scripted":
		return scriptedPlanner{}, "scripted", nil
	default:
		return nil, "", fmt.Errorf("unsupported planner: %s", cfg.Provider)
	}
}

func plannerModelCandidates(plannerName, model, escalationModel string) []string {
	if plannerName == "scripted" {
		return []string{""}
	}

	candidates := make([]string, 0, 2)
	seen := map[string]struct{}{}
	appendCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	appendCandidate(model)
	appendCandidate(escalationModel)
	if len(candidates) == 0 {
		candidates = append(candidates, model)
	}
	return candidates
}

type scriptedPlanner struct{}

func (scriptedPlanner) NextAction(_ context.Context, req llm.PlanRequest) (*llm.PlanAction, error) {
	switch req.TaskName {
	case "run-smoke":
		if len(req.History) == 0 {
			return &llm.PlanAction{
				Type:    "run",
				Command: "printf hello",
				Reason:  "print hello to stdout in one shot",
			}, nil
		}
		return &llm.PlanAction{
			Type:          "finish",
			Reason:        "the one-shot command has completed",
			FinishSuccess: req.History[len(req.History)-1].Success,
			FinishSummary: "one-shot task completed",
		}, nil
	case "session-workflow":
		switch len(req.History) {
		case 0:
			return &llm.PlanAction{Type: "session_exec", Command: "printf hello > note.txt", Reason: "create note.txt with hello"}, nil
		case 1:
			return &llm.PlanAction{Type: "session_exec", Command: "cat note.txt", Reason: "read note.txt to verify its contents"}, nil
		case 2:
			if req.History[1].Success {
				return &llm.PlanAction{Type: "session_exec", Command: "printf verified > status.txt", Reason: "write verified marker after successful read"}, nil
			}
			return &llm.PlanAction{Type: "finish", Reason: "the read step failed", FinishSuccess: false, FinishSummary: "could not verify note.txt"}, nil
		case 3:
			return &llm.PlanAction{Type: "session_exec", Command: "cat status.txt", Reason: "read status.txt to verify the marker"}, nil
		default:
			return &llm.PlanAction{Type: "finish", Reason: "all session workflow steps have completed", FinishSuccess: req.History[len(req.History)-1].Success, FinishSummary: "session workflow completed"}, nil
		}
	case "session-recovery":
		switch len(req.History) {
		case 0:
			return &llm.PlanAction{Type: "session_exec", Command: "sh -c 'echo boom >&2; exit 7'", Reason: "run an expected failing command to exercise recovery"}, nil
		case 1:
			if req.History[0].ExitCode != 7 {
				return &llm.PlanAction{Type: "finish", Reason: "the expected failure pattern did not happen", FinishSuccess: false, FinishSummary: "expected exit 7 with stderr boom"}, nil
			}
			return &llm.PlanAction{Type: "session_exec", Command: "printf recovered > recovery.txt", Reason: "write recovery marker after the expected failure"}, nil
		case 2:
			return &llm.PlanAction{Type: "session_exec", Command: "cat recovery.txt", Reason: "read recovery marker to confirm recovery"}, nil
		default:
			return &llm.PlanAction{Type: "finish", Reason: "recovery task steps have completed", FinishSuccess: req.History[len(req.History)-1].Success, FinishSummary: "session recovery completed"}, nil
		}
	case "test-and-fix":
		switch len(req.History) {
		case 1:
			return &llm.PlanAction{Type: "session_exec", Command: "cat app.sh", Reason: "inspect the buggy program before changing it"}, nil
		case 2:
			return &llm.PlanAction{Type: "session_exec", Command: "cat test.sh", Reason: "inspect the test to understand the expected output"}, nil
		case 3:
			return &llm.PlanAction{Type: "session_exec", Command: "sh test.sh", Reason: "run the failing test to observe the current failure"}, nil
		case 4:
			return &llm.PlanAction{Type: "session_exec", Command: "cat > app.sh <<'EOF'\n#!/bin/sh\necho hello\nEOF\nchmod +x app.sh", Reason: "rewrite app.sh so it prints the expected hello output"}, nil
		case 5:
			return &llm.PlanAction{Type: "session_exec", Command: "sh test.sh", Reason: "rerun the test after fixing app.sh"}, nil
		default:
			last := req.History[len(req.History)-1]
			if last.ExitCode == 0 {
				return &llm.PlanAction{Type: "finish", Reason: "the test now passes", FinishSuccess: true, FinishSummary: "test-and-fix task completed successfully"}, nil
			}
			return &llm.PlanAction{Type: "finish", Reason: "the test still fails after the attempted fix", FinishSuccess: false, FinishSummary: "test-and-fix task did not converge"}, nil
		}
	case "repo-bugfix":
		switch len(req.History) {
		case 1:
			return &llm.PlanAction{Type: "session_exec", Command: "cd demo-repo && cat README.md", Reason: "inspect the repo readme to understand the expected behavior"}, nil
		case 2:
			return &llm.PlanAction{Type: "session_exec", Command: "cd demo-repo && cat src/lib.sh", Reason: "inspect the shared helper that likely builds the greeting"}, nil
		case 3:
			return &llm.PlanAction{Type: "session_exec", Command: "cd demo-repo && cat src/message.sh", Reason: "inspect the repo entrypoint that uses the helper"}, nil
		case 4:
			return &llm.PlanAction{Type: "session_exec", Command: "cd demo-repo && cat tests/test.sh", Reason: "inspect the repo test suite to confirm the expected output"}, nil
		case 5:
			return &llm.PlanAction{Type: "session_exec", Command: "cd demo-repo && sh tests/test.sh", Reason: "run the repo test suite to observe the current failure"}, nil
		case 6:
			return &llm.PlanAction{Type: "session_exec", Command: `cd demo-repo && cat > src/lib.sh <<'EOF'
#!/bin/sh

build_greeting() {
  name="$1"
  printf 'hello %s\n' "$name"
}
EOF
chmod +x src/lib.sh`, Reason: "rewrite the helper so both the library and entrypoint produce hello air"}, nil
		case 7:
			return &llm.PlanAction{Type: "session_exec", Command: "cd demo-repo && sh tests/test.sh", Reason: "rerun the repo test suite after fixing the helper"}, nil
		default:
			last := req.History[len(req.History)-1]
			if last.ExitCode == 0 {
				return &llm.PlanAction{Type: "finish", Reason: "the repo test suite now passes", FinishSuccess: true, FinishSummary: "repo-bugfix task completed successfully"}, nil
			}
			return &llm.PlanAction{Type: "finish", Reason: "the repo test suite still fails after the attempted fix", FinishSuccess: false, FinishSummary: "repo-bugfix task did not converge"}, nil
		}
	default:
		return nil, fmt.Errorf("unsupported task %q", req.TaskName)
	}
}
