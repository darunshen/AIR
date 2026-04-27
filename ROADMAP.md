# AIR Roadmap

[中文](ROADMAP.zh-CN.md)

## Vision

Build an open-source runtime that gives coding agents a safer default execution boundary through lightweight VM isolation, while preserving the practical repo-task loop those agents need.

## Current Baseline

AIR is already past the “can we build a minimal session MVP” stage.

The current baseline already includes:

- `air run`
- stateful sessions
- `local` and `firecracker` providers
- guest execution over `vsock`
- Firecracker runtime artifacts and debugging paths
- workspace injection and export
- OpenAI / DeepSeek reference-agent integrations
- OpenClaude startup and forwarding inside Firecracker guests
- real acceptance workflows for LLM-driven and OpenClaude-driven paths

## Next Stage: Runtime Hardening

Target: make the current workflow more reliable, inspectable, and production-friendly.

Focus:

- stabilize Firecracker guest networking and policy controls
- improve cleanup and lifecycle guarantees
- tighten host/guest error classification and recovery behavior
- continue strengthening OpenClaude and agent-facing workflow reliability
- improve release packaging and installation UX

Success criteria:

- Firecracker-backed agent workflows fail less often and fail more clearly
- guest runtime artifacts are easier to inspect and support
- deployment and installation paths are predictable

## Following Stage: Image And State System

Target: improve reproducibility and reduce operational friction in guest image management.

Focus:

- image lifecycle management
- clearer rootfs / workspace layering contracts
- reproducible prepared guest images
- cleanup and reclaim of runtime artifacts
- stronger storage architecture documentation and tooling

Success criteria:

- prepared guest images are easier to rebuild and reason about
- runtime state is easier to reclaim safely
- rootfs and workspace behavior are easier for users to understand

## Later Stage: Performance

Target: reduce cold-start cost and improve repeat-task ergonomics.

Focus:

- snapshot / restore
- faster guest startup
- warm pools or similar reuse strategies
- streaming and long-running task UX improvements

Success criteria:

- lower startup latency
- better operator experience for repeated agent workflows

## Ecosystem And Community

Target: make AIR easier to adopt externally.

Focus:

- better examples and tutorials
- contributor onboarding
- public architecture guidance
- clearer packaging and release expectations

## Current Priority

The immediate priority is not rebuilding a basic MVP. The immediate priority is hardening the already working AI-agent workflow so that Firecracker-backed real-world usage becomes more stable and easier to operate.
