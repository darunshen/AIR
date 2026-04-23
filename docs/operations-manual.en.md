# AIR Operations Manual

[中文](operations-manual.md)

## 1. Current Status

AIR already supports local execution, Firecracker-backed sessions, guest-agent exec, runtime inspection, and release packaging baselines.

## 2. Environment Requirements

- Go toolchain for development
- optional Firecracker plus KVM for VM-backed execution
- required runtime assets such as the Firecracker binary, kernel, and rootfs

## 3. Repository Preparation

Clone the repo, install dependencies, and prepare local or Firecracker assets depending on the provider you want to use.

## 4. Local Mode Operations

The manual covers:

- one-shot execution
- session creation
- command execution inside a session
- session deletion
- session listing and inspection
- console access

## 5. Firecracker Mode Operations

The Firecracker path adds environment variables, runtime validation, session lifecycle checks, and access to console and event logs.

When `--workspace` is used, AIR also prepares:

- a session-private `rootfs.ext4`
- a read-only `workspace.ext4`
- a writable `workspace-upper.ext4`

Inside the guest, `workspace.ext4` and `workspace-upper.ext4` are mounted as an overlayfs-backed `/workspace`.

When a session has a workspace attached, you can export the current merged workspace view:

```bash
air session export-workspace <session-id> /tmp/air-export
air session export-workspace <session-id> /tmp/air-export --force
```

This exports the current merged `/workspace` view, not the original read-only `workspace.ext4`. By default the output directory must be empty or absent; `--force` recreates it.

## 6. Runtime Directory Layout

AIR persists runtime state on the host so operators can inspect sessions, config files, sockets, metrics, and logs.

Typical Firecracker layout:

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
  config/
```

`rootfs.ext4` is the per-session root disk copied from the base rootfs. `workspace.ext4` and `workspace-upper.ext4` are only present when a workspace is attached.

## 7. Real-Machine Lifecycle Validation

Use create, exec, inspect, console, events, and delete as the standard validation loop.

The current real-machine validation baseline is:

- `Start()` succeeds
- `Exec()` works over `vsock`
- per-session `rootfs.ext4` wiring works
- `/workspace` overlayfs works when a workspace is attached
- `air session export-workspace` exports the current merged workspace

## 8. Common Failures

The document covers missing Firecracker assets, unavailable KVM, and guest-agent readiness failures.

## 9. State Files

Session metadata and runtime artifacts are intentionally visible and inspectable for debugging and cleanup.

## 10. Recommended Workflow

Start local, move to Firecracker with official assets, validate the lifecycle, then layer on agent workflows and release packaging.
