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
