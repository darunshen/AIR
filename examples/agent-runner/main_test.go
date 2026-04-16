package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

	r := &runner{
		manager:        manager,
		plannerName:    "scripted",
		planner:        scriptedPlanner{},
		commandTimeout: 5 * time.Second,
	}

	results, err := r.runSelected(context.Background(), "all")
	if err != nil {
		t.Fatalf("run selected: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 task results, got %d", len(results))
	}
	for _, result := range results {
		if !result.Success {
			t.Fatalf("task %s failed: %+v", result.Name, result)
		}
		if len(result.Steps) == 0 {
			t.Fatalf("task %s returned no steps", result.Name)
		}
	}
}
