# AIR Operations Manual

[中文](operations-manual.md)

This document focuses on how to use, inspect, and debug the current AIR build.

Scope boundary:

- host installation, KVM checks, and Firecracker asset preparation belong in the [Firecracker Deployment Guide](firecracker-deployment-guide.en.md)
- product direction and priorities belong in root `README.md`, `ROADMAP.md`, and `TODO.md`
- this manual does not duplicate the deployment guide's environment setup section

## 1. Current Available Capabilities

Current CLI entry points:

- `air version`
- `air run`
- `air session create`
- `air session list`
- `air session gc`
- `air session inspect`
- `air session console`
- `air session events`
- `air session exec`
- `air session export-workspace`
- `air session delete`
- `air init firecracker`
- `air doctor --provider firecracker`

Current runtime providers:

- `local`
- `firecracker`

The Firecracker path already covers:

- host-side preflight validation
- Firecracker microVM startup
- guest `air-agent` startup
- host/guest `vsock` exec
- per-session `rootfs.ext4`
- optional read-only `workspace.ext4` plus writable `workspace-upper.ext4`
- `air session export-workspace` exporting the current merged guest `/workspace`
- Firecracker `virtio-net + TAP + host NAT` in `--network full`

## 2. Quick Start

### 2.1 Local Mode

```bash
air run -- echo hello
air session create --provider local
air session exec <session_id> "echo hello > a.txt"
air session exec <session_id> "cat a.txt"
air session delete <session_id>
```

### 2.2 Firecracker Mode

Prepare the host first, then run:

```bash
air init firecracker
air doctor --provider firecracker --human
air session create --provider firecracker
air session create --provider firecracker --network full
air session exec <session_id> "uname -a"
air session inspect <session_id>
air session console <session_id> --follow
air session events <session_id> --follow
air session delete <session_id>
```

## 3. `air run`

One-shot examples:

```bash
air run -- echo hello
air run --timeout 5s -- sh -c 'echo hello && exit 3'
air run --memory-mib 512 --vcpu-count 2 -- echo hello
air run --memory-mib 2048 --storage-mib 4096 -- echo hello
air run --provider firecracker -- echo hello
air run --provider firecracker --network full -- curl -I https://api.deepseek.com/anthropic
```

Structured output fields to watch:

- `provider`
- `session_id`
- `request_id`
- `stdout`
- `stderr`
- `exit_code`
- `duration_ms`
- `timeout`
- `error_type`
- `error_message`

Current stable `error_type` values mainly include:

- `invalid_argument`
- `startup_error`
- `transport_error`
- `exec_error`
- `timeout`
- `cleanup_error`

For a human-readable view:

```bash
air run --human -- echo hello
```

## 4. Session Lifecycle

### 4.1 Create

```bash
air session create
air session create --provider local
air session create --provider firecracker
air session create --provider firecracker --network full
air session create --provider firecracker --memory-mib 2048 --storage-mib 4096
air session create --provider firecracker --workspace /absolute/path/to/repo
```

### 4.2 List And Inspect

```bash
air session list
air session inspect <session_id>
```

Notes:

- `list` and `inspect` refresh status from the live runtime when possible
- they do not rely only on stale JSON state
- `inspect.session.network` shows the current session network mode

### 4.3 Exec

```bash
air session exec <session_id> "pwd"
air session exec <session_id> "ls -la"
air session exec <session_id> "go test ./..."
```

Notes:

- `session exec` now streams stdout/stderr by default
- output is shown while the guest command is still running instead of being delayed until completion
- this makes long-running compile, download, and install jobs directly observable

Capacity notes:

- `--memory-mib` controls VM memory size for higher-memory tasks such as `rustup`, builds, and larger toolchains
- `--storage-mib` controls per-session storage capacity
- for Firecracker sessions, `--storage-mib` expands the per-session `rootfs.ext4`
- when `--workspace` is attached, AIR also enlarges the writable `workspace-upper.ext4`
- the current default is `1024`

### 4.4 Logs

```bash
air session console <session_id>
air session console <session_id> --follow
air session console <session_id> --tail=100
air session events <session_id>
air session events <session_id> --follow
```

Notes:

- `console` is serial-log viewing, not an interactive guest shell
- `events` is the structured event stream for lifecycle, exec, and failure analysis
- the default log level is `debug`
- that means AIR prints the full execution-side status flow by default
- you can tune it with `AIR_LOG_LEVEL`:
  - `debug`: default, fully verbose
  - `info`: keep major flow logs, reduce low-level noise
  - `quiet`: minimal output

### 4.5 Export Workspace

When a session was created with `--workspace`, you can export the current merged guest `/workspace` view:

```bash
air session export-workspace <session_id> /tmp/air-export
air session export-workspace <session_id> /tmp/air-export --force
```

Notes:

- AIR exports the current merged overlay view, not the original read-only `workspace.ext4`
- the output directory must be empty or absent by default
- `--force` recreates the output directory before export

### 4.6 Delete

```bash
air session delete <session_id>
```

### 4.7 Clean Up Stale Sessions

When `runtime/sessions/store.json` still contains stopped sessions, missing runtime directories, or older leftover records, run:

```bash
air session gc --dry-run
air session gc
air session gc --force
air session gc --root /home/bigrain/tmp/runtime/sessions --force
```

Notes:

- `--dry-run` previews cleanup without deleting anything
- `--force` also removes running sessions
- `--force` also tries to clean orphan runtimes under the current runtime root even when they are no longer present in the store
- even if the session directory is already gone, `--force` now attempts to detect and clean leftover `firecracker` / `egress-proxy` processes that still reference that runtime root
- `--root` lets you target a different runtime root such as `/home/bigrain/tmp/runtime/sessions`
- the default behavior only cleans non-running sessions
- `running` sessions are skipped so active VMs are not removed accidentally
- stale records whose runtime directories are already gone are removed directly from the store
- stopped sessions that still have runtime artifacts are cleaned up and then removed from the store

## 5. Runtime Directories And Logs

### 5.1 Local Provider

```text
runtime/sessions/local/<session_id>/
  workspace/
  task/
```

### 5.2 Firecracker Provider

```text
runtime/sessions/firecracker/<session_id>/
  rootfs.ext4
  workspace.ext4
  workspace-upper.ext4
  firecracker.sock
  firecracker.pid
  firecracker.vsock
  console.log
  events.jsonl
  metrics.log
  config/
```

Key files:

- `rootfs.ext4`: per-session private root disk
- `workspace.ext4`: read-only workspace image
- `workspace-upper.ext4`: writable overlay upper layer
- `rootfs.ext4` and `workspace-upper.ext4` capacity can be adjusted with `--storage-mib`
- `console.log`: guest serial log
- `events.jsonl`: structured event log
- `config/*.json`: configuration snapshots actually sent to Firecracker
- `config/network-interface-eth0.json`: NIC config when `--network full` is enabled
- `config/network-host.env`: host-side TAP, NAT, and address metadata
- `config/network-guest.env`: guest static network metadata

### 5.3 Network Modes

Currently supported:

- `none`
  - default mode
  - no general guest network device is attached
  - OpenClaude can still reach model APIs through the host-side HTTP CONNECT relay
- `full`
  - attaches Firecracker `virtio-net`
  - creates a dedicated TAP on the host
  - configures host-side `iptables MASQUERADE`
  - configures static IP, default route, and `resolv.conf` inside the guest

Examples:

```bash
air chat --provider firecracker --network full
air session create --provider firecracker --network full
air agent openclaude run --provider firecracker --network full --workspace /path/to/repo --repo /path/to/openclaude
```

## 6. Common Troubleshooting

### 6.1 `firecracker binary not found`

Meaning:

- Firecracker is not in `PATH`
- or `AIR_FIRECRACKER_BIN` points to the wrong place

Action:

- run `air doctor --provider firecracker --human`
- verify binary location and execute permission

### 6.2 `AIR_FIRECRACKER_KERNEL is required`

Meaning:

- no kernel asset is configured

Action:

- prepare `vmlinux` via the deployment guide

### 6.3 `AIR_FIRECRACKER_ROOTFS is required`

Meaning:

- no rootfs asset is configured

Action:

- prepare `rootfs.ext4` via the deployment guide

### 6.4 `kvm device is unavailable for firecracker runtime`

Meaning:

- `/dev/kvm` is missing
- or the current user lacks permission

Action:

- go back to the deployment guide and verify KVM plus permissions

### 6.5 `guest agent is not ready`

Meaning:

- the guest booted, but `air-agent` did not become ready in time

Recommended check order:

1. inspect `console.log`
2. inspect `events.jsonl`
3. inspect `config/*.json` for rootfs, kernel, and boot-arg mismatches
4. verify that the rootfs was prepared with the repository scripts and includes `air-agent`

## 7. Recommended Validation Order

Validate a machine in this order:

1. `air run -- echo hello`
2. `air session create --provider local`
3. `air session exec` and `delete`
4. `air init firecracker`
5. `air doctor --provider firecracker --human`
6. `air session create --provider firecracker`
7. `air session exec`
8. `air session console --follow`
9. `air session export-workspace`

## 8. AI Agent Entry Points

For agent-facing usage, continue with:

- [Using AIR From AI Agents](ai-agent-usage.en.md)
- [OpenClaude Integration](openclaude-integration.en.md)
- [LLM Acceptance Results](llm-acceptance-results.en.md)

The recommended first-use entry point is:

```bash
air chat
```

It interactively prepares the missing runtime dependencies and configuration, then enters chat directly.
On `firecracker`, this step can prompt for the AIR official Firecracker bundle, the AIR official OpenClaude Firecracker guest rootfs bundle, and the AIR official OpenClaude host bundle (currently `linux/amd64` only).
The first saved model settings are persisted to `~/.config/air/chat.json`.
To force model setup again, run `air chat --reconfigure`.
`air chat` now uses an ephemeral host loopback port by default instead of relying on a fixed `127.0.0.1:50052`.
It also prints stage timing so you can tell whether the delay is in preflight checks or in Firecracker / OpenClaude cold start.

If you already prepared the OpenClaude runtime manually and only want the old direct path, the shortest host-side text entry point is:

```bash
AIR_OPENCLAUDE_REPO=~/Documents/code/openclaude \
air agent openclaude chat <session_id>
```

If you want to keep using the older one-command direct path, use:

```bash
AIR_OPENCLAUDE_REPO=~/Documents/code/openclaude \
air agent openclaude run --provider firecracker --workspace /path/to/repo --guest-repo /opt/openclaude
```

If you use DeepSeek or another OpenAI-compatible provider on `firecracker`, you no longer need to set `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` manually; AIR injects the guest proxy defaults automatically.

Once started, AIR shows:

- an `air:openclaude@<session_id>` prompt
- the current `provider/session/workdir` header
- the transcript output path

The current transcript file is:

```text
runtime/sessions/<provider>/<session_id>/openclaude-chat-transcript.jsonl
```

You can replay it directly with:

```bash
air agent openclaude replay <session_id>
```
