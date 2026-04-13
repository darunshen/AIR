# AIR Roadmap

## Vision

Build an open-source runtime that gives AI agents a safer default execution boundary through lightweight VM isolation.

## Phase 1: MVP

Target: a minimal local runtime that proves the session model works.

Scope:

- `air session create`
- `air session exec`
- `air session delete`
- local `sessions.json`
- single-node runtime
- file-based Host/Guest communication
- default no-network mode

Success criteria:

- session can be created
- commands can be executed in the same VM
- file state persists across `exec`
- session can be deleted cleanly

## Phase 2: Engineering Baseline

Target: turn the MVP into a stable developer-facing foundation.

Scope:

- guest agent inside VM
- `virtio-serial` or `vsock`
- HTTP API
- `base image + overlay`
- timeout handling
- session GC
- structured logs

Success criteria:

- communication is more reliable than file polling
- session lifecycle is observable
- host and guest boundaries are cleaner

## Phase 3: Performance and Platform

Target: reduce cold start cost and improve runtime usability.

Scope:

- snapshot / restore
- warm VM pool
- streaming output
- network whitelist mode
- metrics and observability

Success criteria:

- startup latency is significantly reduced
- long-running tasks have better UX
- operational visibility is available

## Phase 4: Community and Ecosystem

Target: make AIR usable and attractive for external developers.

Scope:

- examples and demos
- starter issues
- public architecture discussions
- contributor onboarding
- benchmark and comparison material

## Current Priority

The immediate priority is Phase 1: get a working session-based MVP into the repository.
