# LLM Acceptance Results

[中文](llm-acceptance-results.md)

This document records dated acceptance snapshots for `examples/agent-runner` against real models, plus the replay entry points used to rerun those checks.

Important:

- this is operational evidence and historical snapshot material, not a product SLA
- always interpret the results together with the exact commit, runtime provider, model quota, and upstream API availability

## 1. Replay Entry Points

Use the repository script for standard replay:

```bash
scripts/run-agent-acceptance.sh --planner scripted --task all
```

There is also a gated `go test` path:

```bash
AIR_LLM_ACCEPTANCE=1 \
AIR_AGENT_PROVIDER=deepseek \
AIR_AGENT_MODEL=deepseek-chat \
AIR_AGENT_ESCALATION_MODEL=deepseek-reasoner \
AIR_AGENT_ACCEPTANCE_TASKS=run-smoke,session-workflow,test-and-fix,repo-bugfix \
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
go test ./examples/agent-runner -run TestRealLLMAgentWorkflowAcceptance -v
```

Real-model examples:

```bash
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
scripts/run-agent-acceptance.sh \
  --planner deepseek \
  --model deepseek-chat \
  --escalation-model deepseek-reasoner \
  --planner-retries 1 \
  --task all
```

```bash
OPENAI_API_KEY_FILE=~/tmp/openai.api \
scripts/run-agent-acceptance.sh \
  --planner openai \
  --model gpt-5.4-mini \
  --escalation-model gpt-5.4 \
  --planner-retries 1 \
  --task all
```

Each run creates a dedicated directory under `artifacts/agent-acceptance/`, typically including:

- `command.txt`
- `metadata.txt`
- `result.json`

GitHub Actions support:

- `.github/workflows/llm-acceptance.yml`
  - supports both manual dispatch and workflow reuse
  - uploads standard acceptance artifacts
  - currently includes at least `llm-acceptance.log` and `result.json`
  - enables `AIR_AGENT_TRACE=1` by default so planner, session, exec, and API flow are visible in CI logs
  - performs a `/dev/kvm` preflight when `runtime_provider=firecracker`
  - requires a self-hosted Linux runner for Firecracker because GitHub-hosted runners usually do not expose KVM
- `.github/workflows/ci.yml`
  - runs normal `go test ./...`
  - chains `llm-acceptance` when a `DEEPSEEK_API_KEY` secret is configured
- `.github/workflows/release.yml`
  - runs normal tests first
  - reuses the same `llm-acceptance` workflow when `DEEPSEEK_API_KEY` is configured
  - builds and uploads release artifacts only after those checks pass

## 2. Snapshot On 2026-04-20

Acceptance environment:

- repository: `github.com/darunshen/AIR`
- planner runner: `examples/agent-runner`
- runtime provider: `local`
- retry policy: retry on the same planner first, then escalate to `--escalation-model`

### 2.1 DeepSeek

Run:

```bash
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
scripts/run-agent-acceptance.sh \
  --planner deepseek \
  --model deepseek-chat \
  --escalation-model deepseek-reasoner \
  --planner-retries 1 \
  --task all
```

Result:

- `run-smoke`: passed
- `session-workflow`: passed
- `test-and-fix`: passed
- `repo-bugfix`: passed

Notes:

- no obvious escalation was needed in that run
- this showed that the current DeepSeek integration could cover one-shot, multi-step session, test-and-fix, and repo-bugfix flows

### 2.2 OpenAI

Run:

```bash
OPENAI_API_KEY_FILE=~/tmp/openai.api \
scripts/run-agent-acceptance.sh \
  --planner openai \
  --model gpt-5.4-mini \
  --escalation-model gpt-5.4 \
  --planner-retries 1 \
  --task run-smoke
```

Result:

- `run-smoke`: failed
- HTTP status: `429`
- error type: `insufficient_quota`

Notes:

- this was not a runner protocol failure
- it did confirm that the retry and escalation path was observable
- events already showed `planner_retry`, `planner_escalation`, and the final `planner_error`

## 3. How To Read The Results

Check these signals in `result.json` first:

- `success`
- `tasks[].steps[].kind`
- `tasks[].steps[].planner_attempt`
- `tasks[].steps[].planner_model`
- `tasks[].steps[].request_id`
- `tasks[].steps[].error_message`

Recommended interpretation order:

1. determine whether the failure is model-side, such as `429`, auth failure, or upstream timeout
2. then determine whether it is an AIR execution-side failure, such as `startup_error` or `transport_error`
3. only then decide whether the task itself failed to converge

## 4. Current Conclusion

Based on the 2026-04-20 snapshot:

- `scripted` remains the stable offline regression baseline
- `deepseek` has completed the main reference-agent workflow
- the `openai` adapter is wired, but that run was blocked by account quota
- planner retry and escalation are now replayable and observable
