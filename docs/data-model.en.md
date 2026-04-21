# AIR Data Model

[中文](data-model.md)

## 1. Goals

Define stable runtime objects for sessions, VMs, requests, results, and local persistence.

## 2. Core Objects

- `Session`: session identity, provider, lifecycle state, timestamps, runtime metadata
- `VM`: hypervisor-level runtime state and artifact paths
- `ExecRequest`: command, timeout, and execution options
- `ExecResult`: `stdout`, `stderr`, `exit_code`, duration, timeout flag, and request id
- `RunRequest`: one-shot wrapper around a disposable session lifecycle

## 3. Local Storage

The host persists state in a local store such as `sessions.json` plus per-session runtime directories.

## 4. Guest Communication Format

Request and response payloads need a stable schema so the host can distinguish protocol errors from command failures.

## 5. Lifecycle State Machines

Define explicit states for session and VM progression so cleanup, recovery, and operator debugging are deterministic.

## 6. Recommended Directory Layout

Keep host runtime artifacts grouped by session to simplify inspection and garbage collection.

## 7. Future Evolution

Later versions can add snapshots, versioned protocol envelopes, and richer execution metadata without changing the core model.
