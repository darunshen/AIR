# agent-runner

[中文](README.md)

`examples/agent-runner` is the minimal reference agent for AIR.

It currently supports:

- `openai`
- `deepseek`
- `scripted`

The goal is to validate that a real planner can use AIR as an isolated execution backend for one-shot commands, multi-step sessions, recovery flows, and repo-level bug fixing.

## Running

Examples:

```bash
go run ./examples/agent-runner --task all
go run ./examples/agent-runner --planner scripted --task all
go run ./examples/agent-runner --planner deepseek --model deepseek-chat --task repo-bugfix
```

## Built-in Tasks

- `run-smoke`
- `session-workflow`
- `session-recovery`
- `test-and-fix`
- `repo-bugfix`

## Output

The runner prints structured JSON with planner metadata, per-step evidence, and task-level `final_summary` for coding tasks.
