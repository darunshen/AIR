# AIR VM Runtime Design (Firecracker)

[中文](vm-runtime-design.md)

## 1. Goal

Describe the Firecracker-based runtime design used to create isolated AIR sessions.

## 2. Architecture

The host control plane manages session lifecycle, prepares runtime artifacts, starts Firecracker, and communicates with an in-guest agent over `vsock`.

```mermaid
flowchart TD
    C[CLI / API] --> SM[Session Manager]
    SM --> RT[VM Runtime: Firecracker]
    RT --> API[Firecracker API]
    RT --> J[Jailer]
    RT --> A[Kernel / Rootfs / Socket / Logs]
    RT --> VM[MicroVM]
    VM --> LG[Linux Guest]
    LG --> AG[air-agent]
    LG --> WS[Workspace]
```

## 3. Components

- Session Manager
- VM Runtime
- Guest Agent

## 4. Host Runtime Directory

Each session should own its own runtime directory, including config, sockets, logs, metrics, and writable disk state.

## 5. Boot Flow

```mermaid
flowchart TD
    A[Create session metadata] --> B[Prepare runtime directory]
    B --> C[Prepare kernel / rootfs / overlay]
    C --> D[Start Firecracker process]
    D --> E[Configure machine / boot / drives / vsock]
    E --> F[Boot guest]
    F --> G[Wait for guest agent readiness]
    G --> H[Return session handle]
```

## 6. Guest Communication

`vsock` is the preferred transport because it cleanly matches host/guest boundaries and avoids ad-hoc polling.

```mermaid
sequenceDiagram
    participant Host as Host VM Runtime
    participant VS as vsock
    participant Agent as air-agent
    participant Shell as Guest Shell

    Host->>VS: exec request JSON
    VS->>Agent: deliver request
    Agent->>Shell: run command with timeout
    Shell-->>Agent: stdout / stderr / exit_code
    Agent-->>VS: result JSON
    VS-->>Host: structured ExecResult
```

## 7. Rootfs Design

Use a stable base rootfs and derive a writable per-session disk. This keeps sessions isolated while preserving startup simplicity.

## 8. Guest Startup Chain

The guest should boot directly into the minimal service path required to bring up `air-agent`.

```mermaid
flowchart LR
    A[kernel] --> B[init]
    B --> C[mount proc / sys / dev]
    C --> D[start air-agent]
    D --> E[ready]
```

## 9. Go Control Plane Integration

The Go side should expose stable create, exec, inspect, and delete behavior and normalize runtime errors for agent consumers.

## 10. Security

Default no-network behavior, process isolation, and bounded resources remain core controls.

## 11. Failure Handling

Handle boot failures, guest-agent readiness failures, and exec failures as separate classes.

## 12. Implementation Order

Start from boot and transport, then integrate guest agent, writable rootfs, lifecycle tooling, and error handling.

## 13. Current Replacement Strategy

The design already replaces mock execution with real Firecracker plus real guest-agent communication where available.

## 14. Conclusion

The Firecracker path is now the concrete runtime direction for AIR.
