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

## 6. Runtime Directory Layout

AIR persists runtime state on the host so operators can inspect sessions, config files, sockets, metrics, and logs.

## 7. Real-Machine Lifecycle Validation

Use create, exec, inspect, console, events, and delete as the standard validation loop.

## 8. Common Failures

The document covers missing Firecracker assets, unavailable KVM, and guest-agent readiness failures.

## 9. State Files

Session metadata and runtime artifacts are intentionally visible and inspectable for debugging and cleanup.

## 10. Recommended Workflow

Start local, move to Firecracker with official assets, validate the lifecycle, then layer on agent workflows and release packaging.
