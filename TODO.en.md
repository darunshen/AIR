# TODO

[中文](TODO.md)

This file is the English companion of the working TODO.

## Done

Major completed areas already include:

- baseline Firecracker host integration
- official Firecracker binary and demo asset download scripts
- Firecracker deployment and operations documentation
- debugging commands such as `session list`, `inspect`, `console`, and `events`
- guest agent integration over `vsock`
- per-session writable rootfs overlays
- runtime observability and structured event logs
- one-shot `air run`
- reference agent tasks including `run-smoke`, `session-workflow`, `session-recovery`, `test-and-fix`, and `repo-bugfix`
- OpenAI and DeepSeek planner adapters
- real LLM acceptance tests and GitHub Actions workflows
- release packaging including archives, Firecracker bundles, `.deb`, and apt-style outputs

## Priority Rule

Prioritize a real AI-agent workflow before deeper infrastructure optimization.

## P0: Real AI Agent Loop

The top goal is a usable reference agent that can:

- accept a task
- call `air run` or `air session ...`
- read stdout, stderr, exit code, and events
- decide the next step from the result
- surface stable failure reasons

## OpenClaude Integration

- document a zero-intrusion OpenClaude path first
- run the OpenClaude gRPC server inside an AIR session / VM
- `air agent openclaude start/status/stop` now exists
- basic long-running process management, pid/log metadata, and cleanup on session delete are now implemented
- `air agent openclaude forward` now provides a host local-port bridge to the session's OpenClaude TCP endpoint
- `scripts/prepare-openclaude-firecracker-rootfs.sh` now bakes Bun + OpenClaude into a Firecracker guest rootfs
- `scripts/prepare-openclaude-alpine-rootfs.sh` now provides a newer Alpine-based Bun/OpenClaude guest rootfs path
- the official demo rootfs remains useful for AIR base validation, but it is no longer the recommended OpenClaude guest baseline
- `air session create --provider firecracker --workspace ...` now builds a read-only `workspace.ext4` and exposes `/workspace` inside the guest through overlayfs
- session-private writes now go into `workspace-upper.ext4`, so guest changes do not mutate the original host repo
- the `/workspace` overlayfs flow has now been validated in a real Firecracker guest
- `air session export-workspace <id> <output-dir>` now exports the current merged workspace result
- Firecracker guests can now reach provider APIs through a host-side HTTP CONNECT relay, with prepared guest images injecting `HTTP_PROXY` / `HTTPS_PROXY`
- real OpenClaude task validation still needs to be added

## P1: Debuggability And Runtime Stability

Continue improving:

- observability
- runtime integration stability
- tests

## P2: Lifecycle And Image System

Continue improving:

- session cleanup and reclaim
- rootfs and image layering

## P3: Performance And Open Source Polish

Later priorities include:

- snapshot and restore
- startup performance improvements
- open-source packaging and documentation polish
