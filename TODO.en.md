# TODO

[中文](TODO.md)

This file is the English companion of the working TODO and should track the same product state as the Chinese version.

## Done

- Firecracker host-side runtime integration is now in place
  - runtime providers are split into `local` and `firecracker`
  - Firecracker API socket, pid, console, metrics, vsock, and config artifacts are persisted
  - Firecracker preflight errors are now classified more clearly
  - real `Start()` / `Stop()` on a KVM-capable machine have been validated
  - real kernel boot logs are now captured in `console.log`

- Firecracker asset preparation is now simpler
  - official Firecracker release binaries can be downloaded
  - official demo `hello-vmlinux.bin` can be downloaded
  - official demo `hello-rootfs.ext4` can be downloaded
  - repo script: `scripts/fetch-firecracker-demo-assets.sh`
  - when `assets/firecracker/` exists in the repo, the `firecracker` provider can auto-discover those assets

- Base operations and deployment documentation is now available
  - `docs/operations-manual.md`
  - `docs/firecracker-deployment-guide.md`
  - `air doctor` install and troubleshooting steps are documented
  - `air init firecracker` interactive setup is documented

- Basic debugging and runtime inspection paths are now available
  - `air session list`
  - `air session inspect <id>`
  - `air session console <id>`
  - `air session console <id> --follow`
  - `air session events <id>`
  - `air session events <id> --follow`
  - `air init firecracker`
  - `air doctor --provider firecracker`
  - runtime inspect returns provider, pid, console, socket, vsock, config, and related paths
  - `list` and `inspect` refresh session status from runtime reality
  - older sessions missing `provider` are auto-backfilled

- CLI-level provider selection is now available
  - `air session create --provider local`
  - `air session create --provider firecracker`

- Guest agent and `vsock exec` are now wired through
  - `cmd/air-agent` exists
  - the guest listens on `virtio-vsock`
  - the host/guest protocol is implemented
  - `internal/vm/firecracker.go` now uses real `vsock` transport for `Exec()`
  - real `session create -> exec -> delete` has been validated

- Firecracker guest rootfs injection is now available
  - repo script: `scripts/prepare-firecracker-rootfs.sh`
  - it injects `air-agent` into the rootfs
  - it wires the boot path into the default runlevel
  - it produces the auto-discoverable `assets/firecracker/hello-rootfs-air.ext4`

- Firecracker now uses a per-session writable root disk
  - startup copies the base rootfs into a session-private `rootfs.ext4`
  - without workspace injection, Firecracker mounts the session-private writable rootfs
  - with workspace injection, Firecracker mounts the session-private rootfs read-only
  - session deletion cleans up the session rootfs

- Runtime observability is now part of the execution path
  - session lifecycle events are recorded
  - exec `request_id` is recorded
  - exec duration is recorded
  - event logs are written to `events.jsonl`

- One-shot agent execution is now available
  - `air run` exists
  - it reuses `session create -> exec -> delete`
  - `--provider` is supported
  - `--timeout` is supported
  - `--memory-mib` is supported
  - `--vcpu-count` is supported
  - structured JSON is the default output
  - `--human` is available for manual debugging
  - non-zero execution returns stable `exec_error`

- The first reference agent is now in place
  - `examples/agent-runner` exists
  - it covers one-shot `run-smoke`
  - it covers multi-step `session-workflow`
  - it covers failure recovery `session-recovery`
  - the basic loop of “execute -> inspect result -> decide next step” is validated

- Agent selection has converged
  - `docs/agent-selection.md` was added
  - OpenAI is the first provider choice
  - `gpt-5.4-mini` is the default model
  - `gpt-5.4` is used for harder tasks
  - Anthropic and Gemini are later providers
  - `.env.example` was added as the environment template

- The first OpenAI planner is now in place
  - `internal/llm` was added
  - `llm.Provider` abstraction exists
  - the OpenAI Responses API adapter is implemented
  - `examples/agent-runner` can now plan real next steps through OpenAI
  - the scripted planner remains as the offline regression fallback

- The first DeepSeek planner is now in place
  - the DeepSeek Chat Completions adapter is implemented
  - `examples/agent-runner` can now plan next steps through DeepSeek
  - `run-smoke` has been validated
  - `session-workflow` has been validated

- AI agent usage documentation is now available
  - `docs/ai-agent-usage.md` was added
  - it explains planner / runner / AIR layering
  - it explains OpenAI / DeepSeek / scripted usage
  - it explains one-shot / session / recovery / test-and-fix invocation

- `test-and-fix` workflow is now available
  - the reference agent supports `test-and-fix`
  - the scripted planner path is validated
  - the DeepSeek planner path is validated
  - an extra verification test run happens after `finish`

- `repo-bugfix` workflow is now available
  - the reference agent supports multi-file repo repair tasks
  - the scripted planner is included in acceptance coverage
  - the DeepSeek planner path is validated
  - the task checks repo files, runs repo tests, repairs implementation, and re-validates
  - this is closer to a real coding-agent repo repair loop than single-file `test-and-fix`

- LLM acceptance rerun entrypoints are now available
  - `scripts/run-agent-acceptance.sh` was added
  - OpenAI / DeepSeek / scripted acceptance can run from env vars or key files
  - each run writes `command.txt`, `metadata.txt`, and `result.json`
  - `docs/llm-acceptance-results.md` was added for recorded acceptance snapshots

- Real LLM acceptance tests are now wired in
  - gated `go test` entry `TestRealLLMAgentWorkflowAcceptance` was added
  - it is skipped by default and does not affect normal `go test ./...`
  - it can be enabled with `AIR_LLM_ACCEPTANCE=1` and related env vars
  - a manually triggered GitHub Actions workflow `llm-acceptance` was added

- GitHub Actions now connect normal tests, LLM acceptance, and release
  - `.github/workflows/ci.yml` was added
  - `ci` runs `go test ./...` first
  - if the `DEEPSEEK_API_KEY` secret exists, it automatically reuses `llm-acceptance`
  - `release` now runs normal tests and optional LLM acceptance before packaging
  - `llm-acceptance` uploads standard artifacts including logs and `result.json`
  - `llm-acceptance` in `firecracker` mode now includes KVM preflight and demo asset auto-setup
  - agent trace is enabled by default so GitHub logs show planner / exec / session activity

- Release and packaging baseline is now available
  - `air version` is supported
  - `air-agent --version` is supported
  - release packaging scripts were added
  - GitHub Release archive output is supported
  - Firecracker official bundle archive output is supported
  - `.deb` packaging is supported
  - apt-style repo directory output is supported
  - the GitHub Actions release workflow was added
  - `docs/release-distribution.md` was added

- OpenClaude inside AIR is now substantially wired through
  - `air agent openclaude start/status/stop` exists
  - `air agent openclaude forward` exists
  - session metadata and pid/log lifecycle management are implemented
  - provider env passthrough for OpenAI-compatible / DeepSeek-compatible guest startup is implemented
  - Firecracker guest startup now injects writable `HOME` / `CLAUDE_CONFIG_DIR` for OpenClaude
  - `scripts/prepare-openclaude-alpine-rootfs.sh` now builds the recommended Alpine-based OpenClaude guest image
  - the Alpine guest image now includes Bun-compatible runtime dependencies (`libgcc`, `libstdc++`)
  - the OpenClaude-in-Firecracker real acceptance workflow `scripts/run-openclaude-firecracker-acceptance.sh` now exists
  - the real Firecracker acceptance path has been validated end-to-end
  - guest `air-agent` proxy handling was hardened so a single failed proxy stream no longer tears down the entire control service

## Priority Rule

- The priority rule is: prove the real AI-agent workflow first, then continue deeper infrastructure work
- “Real workflow” here means:
  - create an isolated environment
  - execute commands or tasks
  - preserve or destroy state
  - inspect stdout / stderr / events / failure reasons
  - set basic execution boundaries such as timeout and resource limits
- Items that do not directly improve agent usability, such as a true block-level COW overlay, snapshots, or a warm pool, stay behind that priority

## P0: AI Agent Workflow Loop

### P0 Goal

- Build a real reference agent that uses AIR, not a full agent platform first
- That reference agent must at least:
  - accept a task
  - call `air run` or `air session ...`
  - read stdout / stderr / exit code / events
  - decide the next step from the result
  - surface a stable and explainable failure reason

### P0 Minimum Deliverables

- A small but real task set
  - read files
  - write files
  - run tests
  - continue from the previous result

- A stable execution result structure
  - stdout
  - stderr
  - exit_code
  - request_id
  - duration_ms
  - timeout
  - error_type
  - error_message

## P1: Debuggability And Runtime Stability

Keep improving:

- observability
- runtime integration stability
- tests
- Firecracker guest networking and policy controls
- OpenClaude forwarding and acceptance robustness

## P2: Lifecycle And Image System

Keep improving:

- session cleanup and reclaim
- rootfs and image layering
- image lifecycle management
- reproducibility of prepared guest images

## P3: Performance And Open Source Polish

Later priorities include:

- snapshot and restore
- startup performance improvements
- open-source packaging and documentation polish
