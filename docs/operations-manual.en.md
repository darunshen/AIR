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
air run --provider firecracker -- echo hello
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

### 4.3 Exec

```bash
air session exec <session_id> "pwd"
air session exec <session_id> "ls -la"
air session exec <session_id> "go test ./..."
```

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
- `console.log`: guest serial log
- `events.jsonl`: structured event log
- `config/*.json`: configuration snapshots actually sent to Firecracker

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
