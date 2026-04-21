# AIR Project Plan

[中文](project-plan.md)

## 1. Project Overview

AIR is an isolated execution runtime for AI agents. The project goal is to provide a safe, disposable, and reproducible VM boundary for untrusted code.

## 2. Business Value

- reduce host risk for AI-generated code execution
- provide a reusable sandbox backend for agent products
- lower the engineering cost of building isolation in-house
- prepare for later API, multi-tenant, and scheduling expansion

## 3. Milestones

### Phase 1: MVP

Deliver `air session create`, `air session exec`, and `air session delete` on a single machine with local JSON state and default no-network behavior.

### Phase 2: V1

Add guest agent communication over `virtio-serial` or `vsock`, HTTP APIs, overlay-based rootfs management, timeout handling, logs, and garbage collection.

### Phase 3: V2

Add snapshot and restore, warm VM pools, streaming output, whitelist networking, and baseline monitoring.

## 4. Suggested Work Split

- virtualization and low-level runtime
- control-plane backend
- platform engineering, build, release, and observability
- product definition and acceptance criteria

## 5. Major Risks

- unstable host/guest communication
- leaked or unrecoverable session state
- high cold-start latency
- guest agent crashes making sessions unusable

## 6. Acceptance Criteria

- sessions can be created, executed, and deleted
- state persists across exec calls in the same session
- VMs have no network by default
- timeouts and abnormal exits surface clearly
- idle sessions can be reclaimed
