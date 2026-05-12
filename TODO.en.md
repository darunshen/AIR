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
  - official getting-started / CI `vmlinux.bin` can be downloaded
  - official getting-started / CI `ubuntu-rootfs.ext4` can be downloaded
  - repo script: `scripts/fetch-firecracker-ubuntu-assets.sh`
  - when `assets/firecracker/` exists in the repo, the `firecracker` provider can auto-discover those assets

- Base operations and deployment documentation is now available
  - `docs/operations-manual.md`
  - `docs/firecracker-deployment-guide.md`
  - `air doctor` install and troubleshooting steps are documented
  - `air init firecracker` interactive setup is documented

- Basic debugging and runtime inspection paths are now available
  - `air session list`
  - `air session gc`
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
  - repo script: `scripts/prepare-firecracker-ubuntu-rootfs.sh`
  - it injects `air-agent` into the rootfs
  - it takes over guest `init` to launch `air-agent`
  - it produces the auto-discoverable `assets/firecracker/ubuntu-rootfs-air.ext4`

- Firecracker now uses a per-session writable root disk
  - startup copies the base rootfs into a session-private `rootfs.ext4`
  - without workspace injection, Firecracker mounts the session-private writable rootfs
  - with workspace injection, Firecracker mounts the session-private rootfs read-only
  - session deletion cleans up the session rootfs

- The first Firecracker `--network full` path is now in place
  - it uses `virtio-net + TAP + host NAT (MASQUERADE)`
  - the guest has been validated to receive a static IP, a default route, and working internet egress
  - current product gap: when a session is created by root, later `session exec/inspect/delete` also require root
  - this should be refactored into a privileged network helper plus an unprivileged session control path so users do not need `sudo` for the whole workflow

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

- The `air chat` first-run entrypoint is now stronger
  - `air chat` is supported
  - `air chat --reconfigure` is supported
  - first-run model settings can be entered interactively and persisted to `~/.config/air/chat.json`
  - in `firecracker` mode it can now interactively offer the AIR official Firecracker bundle
  - in `firecracker` mode it can now interactively offer the AIR official OpenClaude guest rootfs bundle
  - on `linux/amd64` it prefers the AIR official OpenClaude host bundle
  - if the host bundle path is unavailable, it falls back to source download plus `bun install`

- Official OpenClaude release artifacts are now expanded
  - `air_openclaude_linux_amd64.tar.gz` is supported
  - `air_openclaude_firecracker_linux_amd64.tar.gz` is supported
  - `air_openclaude_firecracker_linux_arm64.tar.gz` is supported
  - the release workflow now builds the OpenClaude Firecracker guest bundle
  - `scripts/prepare-openclaude-ubuntu-rootfs.sh` now also injects the minimal guest tool dependencies: `bash`, `ripgrep`, `curl`, `git`, and `ca-certificates`

## Product Gaps

## Product Experience Priorities

### P0

- The command surface has too many parameters and the interaction model is still behind
  - many common flows still require users to manually assemble `provider`, `network`, `memory`, `storage`, `workspace`, `repo`, `guest-repo`, and `timeout`
  - this directly increases first-run friction, cognitive load, and operator error
  - AIR needs profiles, presets, default policies, and guided flows so common cases collapse into short commands

- `air chat` needs to become a unified chat + shell entrypoint
  - the user wants one entry that supports both natural conversation and direct terminal control similar to `ssh`
  - today `air chat` is conversation-oriented while `air session exec` is command-oriented, which creates an obvious split
  - AIR should converge on one session model that supports agent conversation, PTY handoff, exit, and return to chat mode

- Firecracker workspace still uses the image-plus-overlay model
  - the current backend cannot simply switch to `virtio-fs`
  - the VM feel is still too strong and the host/guest workspace split remains obvious
  - reducing that friction requires a host/guest workspace sync mode, or a new runtime provider that supports a shared filesystem

- Tool execution remains opaque inside `air chat`
  - users can currently see `Tool Call` and the final `Tool Result` only
  - stdout / stderr produced while a Bash tool is still running are not shown live
  - this needs OpenClaude gRPC progress passthrough plus AIR chat-side rendering

### P1

- Effective config and auth sources are still opaque
  - it is hard to tell which key, provider, or config file is actually active
  - debugging `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` conflicts is expensive
  - AIR needs an effective-config plus config-source view

- AIR and host OpenClaude configuration models are still fragmented
  - AIR config, OpenClaude user settings, and environment variables can still contaminate one another
  - a single config entrypoint and stricter provider/profile validation are needed

- Guest base environment checks are still incomplete
  - missing `PATH`, `sh`, `rg`, `cargo`, `rustup`, and related tools usually fail mid-run
  - startup or doctor output should make these checks explicit

- Timeout policy is still inconsistent across execution paths
  - `session exec` already supports `--timeout`
  - internal `air chat` tool execution and longer OpenClaude paths still need a unified timeout model

- Host / guest OpenClaude and `air-agent` versions still lack a handshake
  - after a host upgrade, the guest rootfs may still run an older agent
  - there is no protocol-version negotiation or explicit incompatibility error
  - startup should detect and report this directly instead of relying on symptom debugging

- Approval denial does not yet terminate the whole task cleanly
  - entering `n` at `Approve Bash? (y/n)` only rejects that one tool call and does not cancel the current task
  - AIR needs an explicit "deny and stop task" path

- Resource cleanup UX is still not product-grade
  - this recovery round exposed multiple object models: sessions, chat processes, orphan runtimes, orphan processes, and multi-root runtime trees
  - users currently need to understand the differences between `session gc`, `chat gc`, `--force`, `--root`, and `gc --all`, which means the cleanup model is still implementation-shaped
  - a mature product should self-clean by default and offer one unified resource overview plus one-shot recovery instead of forcing users to decide whether they need to kill a session, a chat process, a directory, or a process tree

- Resource ownership and lifecycle remain opaque
  - users can see many processes in `ps`, but the system does not directly explain which runtime root or session they belong to, or whether they are still store-managed
  - the ownership and exit ordering across chat parents, Bun child processes, Firecracker VMs, and egress proxies are not surfaced clearly
  - AIR needs one unified status view that directly labels resources as active, stale, orphaned, or root-owned

### P2

- Log levels still need finer separation
  - transport byte-stream logs should not appear as part of normal `AIR_LOG_LEVEL=debug` usage
  - major flow logs and transport-level logs need separate controls

- There is still no system-level status overview
  - provider, network, memory, storage, guest agent version, OpenClaude config source, and auth source are not shown in one place
  - AIR needs a stronger top-level status and diagnostics surface

- Full-network sessions created by root still require root for later operations
  - `session exec/inspect/delete` still require `sudo` in the current full-network mode
  - this is still a visible experience break
  - this should be split into a privileged network helper plus ordinary session control

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
