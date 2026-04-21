package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/llm"
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
		plannerConfig:  llm.Config{Provider: "scripted"},
		plannerFactory: newRunnerPlanner,
		commandTimeout: 5 * time.Second,
	}

	results, err := r.runSelected(context.Background(), "all")
	if err != nil {
		t.Fatalf("run selected: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 task results, got %d", len(results))
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

func TestPlannerModelCandidates(t *testing.T) {
	t.Helper()

	got := plannerModelCandidates("openai", "gpt-5.4-mini", "gpt-5.4")
	if len(got) != 2 || got[0] != "gpt-5.4-mini" || got[1] != "gpt-5.4" {
		t.Fatalf("unexpected planner candidates: %#v", got)
	}

	scripted := plannerModelCandidates("scripted", "", "gpt-5.4")
	if len(scripted) != 1 || scripted[0] != "" {
		t.Fatalf("unexpected scripted candidates: %#v", scripted)
	}
}

func TestRunnerPlannerRetrySucceedsWithoutEscalation(t *testing.T) {
	t.Helper()

	factory := &fakePlannerFactory{
		perModel: map[string][]fakePlannerResponse{
			"gpt-5.4-mini": {
				{err: context.DeadlineExceeded},
				{action: &llm.PlanAction{Type: "finish", Reason: "done", FinishSuccess: true, FinishSummary: "ok"}},
			},
		},
	}

	r := &runner{
		plannerName:    "openai",
		plannerConfig:  llm.Config{Provider: "openai", Model: "gpt-5.4-mini"},
		plannerFactory: factory.build,
		model:          "gpt-5.4-mini",
		plannerRetries: 1,
	}

	action, steps, err := r.nextPlannerAction(context.Background(), llm.PlanRequest{
		TaskName:           "run-smoke",
		AllowedActionTypes: []string{"finish"},
		Step:               1,
		MaxSteps:           1,
	})
	if err != nil {
		t.Fatalf("next planner action: %v", err)
	}
	if action == nil || action.Type != "finish" {
		t.Fatalf("unexpected planner action: %+v", action)
	}
	if len(steps) != 2 {
		t.Fatalf("expected retry trace steps, got %d", len(steps))
	}
	if steps[0].Kind != "planner_retry" {
		t.Fatalf("expected planner_retry, got %+v", steps[0])
	}
	if steps[1].Kind != "planner_recovered" {
		t.Fatalf("expected planner_recovered, got %+v", steps[1])
	}
}

func TestRunnerPlannerEscalatesModelAfterRetries(t *testing.T) {
	t.Helper()

	factory := &fakePlannerFactory{
		perModel: map[string][]fakePlannerResponse{
			"gpt-5.4-mini": {
				{err: context.DeadlineExceeded},
				{err: context.DeadlineExceeded},
			},
			"gpt-5.4": {
				{action: &llm.PlanAction{Type: "finish", Reason: "done", FinishSuccess: true, FinishSummary: "ok"}},
			},
		},
	}

	r := &runner{
		plannerName:     "openai",
		plannerConfig:   llm.Config{Provider: "openai", Model: "gpt-5.4-mini"},
		plannerFactory:  factory.build,
		model:           "gpt-5.4-mini",
		escalationModel: "gpt-5.4",
		plannerRetries:  1,
	}

	action, steps, err := r.nextPlannerAction(context.Background(), llm.PlanRequest{
		TaskName:           "run-smoke",
		AllowedActionTypes: []string{"finish"},
		Step:               1,
		MaxSteps:           1,
	})
	if err != nil {
		t.Fatalf("next planner action: %v", err)
	}
	if action == nil || action.Type != "finish" {
		t.Fatalf("unexpected planner action: %+v", action)
	}
	if len(steps) != 4 {
		t.Fatalf("expected escalation trace steps, got %d", len(steps))
	}
	if steps[2].Kind != "planner_escalation" {
		t.Fatalf("expected planner_escalation, got %+v", steps[2])
	}
	if steps[2].PlannerModel != "gpt-5.4" {
		t.Fatalf("expected escalated model gpt-5.4, got %+v", steps[2])
	}
	if steps[3].Kind != "planner_recovered" {
		t.Fatalf("expected planner_recovered, got %+v", steps[3])
	}
}

type fakePlannerFactory struct {
	perModel map[string][]fakePlannerResponse
}

func (f *fakePlannerFactory) build(cfg llm.Config) (llm.Planner, string, error) {
	responses := append([]fakePlannerResponse(nil), f.perModel[cfg.Model]...)
	return &fakePlanner{responses: responses}, cfg.Provider, nil
}

type fakePlanner struct {
	responses []fakePlannerResponse
}

func (p *fakePlanner) NextAction(_ context.Context, _ llm.PlanRequest) (*llm.PlanAction, error) {
	if len(p.responses) == 0 {
		return nil, context.DeadlineExceeded
	}
	next := p.responses[0]
	p.responses = p.responses[1:]
	if next.err != nil {
		return nil, next.err
	}
	return next.action, nil
}

type fakePlannerResponse struct {
	action *llm.PlanAction
	err    error
}
