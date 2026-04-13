# AIR

**AIR (Agent Isolation Runtime)** is an open-source runtime for executing untrusted AI-generated code inside isolated lightweight VMs.

The project is built around a simple premise:

> AI-generated code should not run in the host environment by default.

AIR aims to provide a safer execution boundary for coding agents, sandboxed tools, and automated development workflows by using VM-based isolation instead of shared-kernel containers.

## Why AIR

Modern AI agents do more than generate code. They write, execute, iterate, and return results. That changes the infrastructure requirement:

- The code is often untrusted
- The environment should be disposable
- Resource access should be controlled
- State should be reproducible when needed

AIR is designed for that model.

## Core Goals

- Run untrusted code inside an isolated VM
- Support disposable one-shot execution and stateful sessions
- Disable network access by default
- Enforce CPU, memory, and timeout limits
- Evolve toward `overlay + snapshot + fast restore`

## Product Direction

AIR is planned around two execution modes:

### 1. One-shot execution

```bash
air run hello.py
```

Flow:

```text
Create VM -> Load environment -> Execute -> Return output -> Destroy VM
```

### 2. Stateful session execution

```bash
air session create
air session exec <id> "echo hello > a.txt"
air session exec <id> "cat a.txt"
air session delete <id>
```

Flow:

```text
Create VM -> Keep state -> Execute multiple commands -> Destroy session
```

## Architecture Overview

AIR follows a control-plane / execution-plane design:

```text
CLI / HTTP API
      |
      v
Orchestrator
      |
      +-- Session Manager
      +-- VM Manager
      +-- Isolation Controller
      +-- Snapshot Engine
      |
      v
Hypervisor + Guest Agent + Rootfs
```

## Roadmap

### Phase 1: MVP
- `air session create`
- `air session exec`
- `air session delete`
- Local JSON state store
- File-based Host/Guest communication
- Single-node runtime

### Phase 2: Engineering baseline
- Guest agent
- `virtio-serial` or `vsock`
- HTTP API
- `base image + overlay`
- Timeout, GC, logging

### Phase 3: Performance and platform
- Snapshot / restore
- Warm VM pool
- Streaming output
- Network whitelist mode
- Metrics and observability

## Open Source Direction

AIR is intended to be built in the open.

We want to grow a developer community around:

- VM-based AI sandboxing
- Agent runtime design
- Guest/Host communication
- Snapshot and fast restore
- Security-first execution infrastructure

Contributions are welcome across architecture, virtualization, Go services, guest agent design, and documentation.

## Current Documentation

- [Project Plan](docs/project-plan.md)
- [Product Requirement Document](docs/prd.md)
- [Technical Architecture](docs/technical-architecture.md)
- [API Design](docs/api-design.md)
- [Data Model](docs/data-model.md)
- [Repository Guidelines](AGENTS.md)

## Suggested MVP Structure

```text
cmd/air/
internal/session/
internal/vm/
internal/store/
data/
docs/
```

## Who This Is For

AIR is relevant if you are building:

- AI coding agents
- secure execution sandboxes
- cloud IDE backends
- automated code evaluation systems
- infrastructure for untrusted task execution

## Community

If you are interested in building AIR, you can contribute by:

- opening issues
- proposing architecture improvements
- implementing the MVP runtime
- improving isolation and communication design
- helping shape the public roadmap

## Status

AIR is at the early design and bootstrap stage. The current focus is to turn the architecture into a working open-source MVP.

