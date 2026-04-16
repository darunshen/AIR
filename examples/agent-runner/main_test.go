package main

import (
	"path/filepath"
	"testing"

	"github.com/darunshen/AIR/internal/session"
)

func TestRunnerTasks(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := session.NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	r := &runner{manager: manager}

	for _, tc := range []struct {
		name string
		run  func() taskReport
	}{
		{name: "run-smoke", run: r.runSmokeTask},
		{name: "session-workflow", run: r.runSessionWorkflowTask},
		{name: "session-recovery", run: r.runSessionRecoveryTask},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.run()
			if !result.Success {
				t.Fatalf("task %s failed: %+v", tc.name, result)
			}
			if len(result.Steps) == 0 {
				t.Fatalf("task %s returned no steps", tc.name)
			}
		})
	}
}
