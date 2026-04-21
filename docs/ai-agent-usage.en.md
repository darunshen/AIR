# Using AIR From AI Agents

[中文](ai-agent-usage.md)

## 1. Usage Model

AIR should be treated as the isolated execution backend behind an agent workflow:

`LLM Planner -> Agent Runner -> AIR -> Isolated Runtime`

The planner decides what to do next, the runner manages state and history, and AIR only executes inside the isolation boundary.

## 2. Supported Planners

- OpenAI
- DeepSeek
- scripted fallback for offline regression

## 3. Environment Setup

Prepare the AIR runtime locally, then export planner-specific API keys and model settings.

## 4. Running The Reference Agent

The repository provides `examples/agent-runner` with:

- default task set via `--task all`
- OpenAI mode
- DeepSeek mode
- scripted mode
- acceptance script support
- gated real-LLM `go test`

## 5. Built-in Tasks

- `run-smoke`
- `session-workflow`
- `session-recovery`
- `test-and-fix`
- `repo-bugfix`

These tasks validate one-shot execution, persistent state, recovery, and repo-level bug fixing.

## 6. Output Structure

Structured JSON includes planner metadata, task success state, step traces, and task-level `final_summary` for delivery-style reporting.

## 7. Integrating AIR Into Your Own Agent

Use `air run` for one-shot work and `air session ...` for multi-step work. Feed structured results back into the planner for the next decision.

## 8. Current Boundaries

The current reference agent is intentionally minimal. AIR is the runtime, not a full hosted agent platform.

## 9. Current Recommendation

Focus on a real end-to-end workflow where an agent can inspect, run, fix, verify, and summarize a repo task inside an isolated environment.
