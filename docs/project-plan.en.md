# AIR Project Plan (Historical Archive)

[中文](project-plan.md)

This document preserves an early planning snapshot for AIR. It is useful for understanding the original MVP/V1/V2 breakdown, but it is not the current product baseline.

Important:

- do not treat this document as the current capability matrix
- use root `README.md`, `ROADMAP.md`, and `TODO.md` for current status and priorities
- statements here about "not yet implemented" may already be outdated

## 1. Early Project Positioning

AIR (Agent Isolation Runtime) was originally framed as an isolated execution runtime for AI agents, intended to provide a safe, disposable, and reproducible VM boundary for untrusted code.

## 2. Early Goals

- one-shot execution through `air run`
- stateful execution through `air session create/exec/delete`
- default no-network execution with CPU, memory, and timeout controls
- reproducibility and fast recovery through base images, overlays, and snapshots

## 3. Early Milestone Breakdown

### Phase 1: MVP

The original MVP scope included:

- `air session create`
- `air session exec <id> "<cmd>"`
- `air session delete <id>`
- single-machine deployment
- local JSON session state
- file-based host/guest communication
- default no-network behavior

### Phase 2: V1

The original V1 plan included:

- a guest agent
- `virtio-serial` or `vsock` communication
- an HTTP API
- `base image + overlay` rootfs management
- timeouts, logs, and garbage collection

### Phase 3: V2

The original V2 plan included:

- snapshot / restore
- warm VM pools
- streaming output
- allowlisted networking
- quotas and monitoring

## 4. What Still Matters

Even as a historical document, a few themes still matter:

- AIR is an isolated runtime for agent workflows, not a general-purpose container platform
- host/guest communication, state cleanup, and startup latency remain core engineering constraints
- lifecycle management, image management, and observability still need continuous hardening
