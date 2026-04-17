# AIR

[中文](#中文简介) | [English](#english)

**AIR (Agent Isolation Runtime)** is an open-source runtime for executing untrusted AI-generated code inside isolated lightweight VMs.

> AI-generated code should not run in the host environment by default.

AIR is designed for coding agents, sandboxed tools, and automated development workflows that need a stronger execution boundary than shared-kernel containers.

## English

### What AIR Is

AIR provides a VM-based execution boundary for untrusted code. The project is focused on building a safe, disposable, reproducible runtime for AI agents that generate and execute code autonomously.

### Why AIR

Modern AI agents do more than generate code. They write, execute, iterate, and return results. That changes the infrastructure requirement:

- The code is often untrusted
- The environment should be disposable
- Resource access should be controlled
- State should be reproducible when needed

### Core Goals

- Run untrusted code inside an isolated VM
- Support both one-shot tasks and stateful sessions
- Disable network access by default
- Enforce CPU, memory, and timeout limits
- Evolve toward `overlay + snapshot + fast restore`

### Product Direction

#### 1. One-shot execution

```bash
air run hello.py
```

```text
Create VM -> Load environment -> Execute -> Return output -> Destroy VM
```

#### 2. Stateful session execution

```bash
air init firecracker
air doctor --provider firecracker --human
air session create
air session create --provider local
air session create --provider firecracker
air session list
air session inspect <id>
air session console <id> --follow
air session exec <id> "echo hello > a.txt"
air session exec <id> "cat a.txt"
air session delete <id>
```

```text
Create VM -> Keep state -> Execute multiple commands -> Destroy session
```

### Architecture Overview

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

### Roadmap

#### Phase 1: MVP
- `air session create`
- `air session exec`
- `air session delete`
- Local JSON state store
- File-based Host/Guest communication
- Single-node runtime

#### Phase 2: Engineering baseline
- Guest agent
- `virtio-serial` or `vsock`
- HTTP API
- `base image + overlay`
- Timeout, GC, logging

#### Phase 3: Performance and platform
- Snapshot / restore
- Warm VM pool
- Streaming output
- Network whitelist mode
- Metrics and observability

### Documentation

- [Project Plan](docs/project-plan.md)
- [Product Requirement Document](docs/prd.md)
- [Technical Architecture](docs/technical-architecture.md)
- [API Design](docs/api-design.md)
- [Data Model](docs/data-model.md)
- [Virtualization Selection](docs/virtualization-selection.md)
- [VM Runtime Design](docs/vm-runtime-design.md)
- [AI Agent Selection](docs/agent-selection.md)
- [Using AIR From AI Agents](docs/ai-agent-usage.md)
- [Release And Distribution](docs/release-distribution.md)
- [Firecracker Deployment Guide](docs/firecracker-deployment-guide.md)
- [Operations Manual](docs/operations-manual.md)
- [Repository Guidelines](AGENTS.md)
- [Roadmap](ROADMAP.md)
- [Contributing Guide](CONTRIBUTING.md)

### Community

We want AIR to be built in the open. Contributions are welcome across:

- VM-based AI sandboxing
- Agent runtime design
- Guest/Host communication
- Snapshot and fast restore
- Go control plane services
- Documentation and developer experience

## 中文简介

### AIR 是什么

AIR（Agent Isolation Runtime）是一个面向 AI Agent 的开源隔离运行时，用轻量虚拟机而不是共享内核容器来执行不可信代码。

它解决的不是“怎么运行代码”，而是“怎么安全地运行 AI 生成的代码”。

### 为什么要做 AIR

AI Agent 已经不只是生成代码，而是开始自己执行代码、修改文件、跑测试、返回结果。这意味着底层基础设施必须变化：

- 执行代码默认不可信
- 执行环境需要可销毁
- 资源访问需要被限制
- 执行状态需要可复现

### 核心目标

- 在独立 VM 中执行不可信代码
- 同时支持一次性执行和有状态 Session
- 默认关闭网络
- 支持 CPU、内存、超时限制
- 后续演进到 `overlay + snapshot + fast restore`

### 产品方向

#### 1. 一次性执行

```bash
air run hello.py
```

```text
创建 VM -> 加载环境 -> 执行 -> 返回结果 -> 销毁 VM
```

#### 2. 有状态 Session

```bash
air session create
air session create --provider local
air session create --provider firecracker
air session list
air session inspect <id>
air session console <id> --follow
air session exec <id> "echo hello > a.txt"
air session exec <id> "cat a.txt"
air session delete <id>
```

```text
创建 VM -> 保留状态 -> 多次执行 -> 销毁 Session
```

### 技术架构概览

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

### 路线图

#### 第一阶段：MVP
- `air session create`
- `air session exec`
- `air session delete`
- 本地 JSON 状态存储
- Host/Guest 文件通信
- 单机运行时

#### 第二阶段：工程化基础
- Guest Agent
- `virtio-serial` 或 `vsock`
- HTTP API
- `base image + overlay`
- timeout、GC、日志

#### 第三阶段：性能与平台能力
- Snapshot / Restore
- 预热 VM 池
- 流式输出
- 白名单网络模式
- 指标与可观测性

### 文档

- [项目计划](docs/project-plan.md)
- [产品说明书](docs/prd.md)
- [技术架构](docs/technical-architecture.md)
- [接口设计](docs/api-design.md)
- [数据模型](docs/data-model.md)
- [虚拟化技术选型](docs/virtualization-selection.md)
- [VM Runtime 设计](docs/vm-runtime-design.md)
- [AI Agent 选型](docs/agent-selection.md)
- [通过 AI Agent 使用 AIR](docs/ai-agent-usage.md)
- [发布与安装包交付](docs/release-distribution.md)
- [Firecracker 真机部署指南](docs/firecracker-deployment-guide.md)
- [操作手册](docs/operations-manual.md)
- [仓库协作指南](AGENTS.md)
- [项目路线图](ROADMAP.md)
- [贡献指南](CONTRIBUTING.md)

如果你是通过 `.deb` 安装 AIR，需要注意当前安装包只包含 CLI，不包含 Firecracker 二进制、`vmlinux`、`rootfs.ext4`。要启用 `firecracker` provider，先执行：

```bash
air init firecracker
air doctor --provider firecracker --human
```

`air init firecracker` 会交互询问你是下载 AIR 官方镜像包，还是自己部署 Firecracker 资产。

### 社区建设

AIR 的目标是面向社区持续建设。欢迎以下方向的开发者加入：

- 虚拟化与 VM 管理
- Guest / Host 通信
- Go 控制面与 API
- Snapshot 与快速恢复
- AI 安全执行基础设施
- 文档、示例和开发者体验

## Status

AIR is in the early design and bootstrap stage. The current focus is turning the architecture into a working open-source MVP.

Current implementation note:

- Phase 1 has started with a minimal Go CLI skeleton
- `air run` and `session create / exec / delete` are now available as the first executable workflow
- The `vm` layer now supports a configurable provider with `local` as the default and `firecracker` as the experimental VM-backed path
- Firecracker bootstrapping, guest `air-agent`, and host/guest `vsock exec` are wired end to end
- Firecracker now uses a per-session writable rootfs image copied from the configured base rootfs
- `air session list` / `inspect` / `console` / `events` are available for basic debugging
- `air run` supports `--provider`, `--timeout`, `--memory-mib`, `--vcpu-count`, and structured JSON output for agent consumption
- `examples/agent-runner` now supports OpenAI and DeepSeek planners, with `scripted` as an offline fallback
- `docs/agent-selection.md` now records the first external LLM integration decision and environment template
- `scripts/prepare-firecracker-rootfs.sh` rebuilds the demo rootfs with `air-agent` baked in and enabled through OpenRC `local.d`
- Release packaging now supports GitHub Release archives, `.deb` packages, and an initial apt repository directory bundle

Distribution:

- Build release artifacts locally with `./scripts/build-release-artifacts.sh dist`
- The repository includes a GitHub Actions workflow at `.github/workflows/release.yml`
- Packaging details are documented in `docs/release-distribution.md`
- The current `.deb` package contains only `air` and `air-agent`; Firecracker runtime assets must be installed separately
- Official releases now also publish `air_firecracker_linux_<arch>.tar.gz` for `air init firecracker`

Runtime configuration:

- `AIR_VM_RUNTIME`: choose `local` or `firecracker`
- `AIR_FIRECRACKER_BIN`: Firecracker binary path, default `firecracker`
- `AIR_FIRECRACKER_KERNEL`: kernel image path required by the `firecracker` provider
- `AIR_FIRECRACKER_ROOTFS`: rootfs image path required by the `firecracker` provider
- `AIR_KVM_DEVICE`: KVM device path, default `/dev/kvm`

Startup shortcut:

- After running `scripts/fetch-firecracker-demo-assets.sh` and `scripts/prepare-firecracker-rootfs.sh`, you can usually start the Firecracker provider from the repository root with only `AIR_VM_RUNTIME=firecracker`
- If `assets/firecracker/firecracker`, `assets/firecracker/hello-vmlinux.bin`, and `assets/firecracker/hello-rootfs-air.ext4` exist, AIR will auto-discover them
- AIR also auto-discovers the same files under `/usr/lib/air/firecracker` and `/usr/local/lib/air/firecracker`
- You can also bypass the default provider and create a session explicitly with `air session create --provider firecracker`

Firecracker runtime layout:

- `runtime/sessions/firecracker/<session_id>/overlay.ext4`
- `runtime/sessions/firecracker/<session_id>/firecracker.sock`
- `runtime/sessions/firecracker/<session_id>/firecracker.pid`
- `runtime/sessions/firecracker/<session_id>/console.log`
- `runtime/sessions/firecracker/<session_id>/events.jsonl`
- `runtime/sessions/firecracker/<session_id>/metrics.log`
- `runtime/sessions/firecracker/<session_id>/firecracker.vsock`
- `runtime/sessions/firecracker/<session_id>/config/*.json`

Real-environment lifecycle test:

- `AIR_FIRECRACKER_INTEGRATION=1 go test ./internal/vm -run TestFirecrackerIntegrationLifecycle`
- The test validates `start -> exec -> stop`, non-empty console output, and session overlay wiring
- The test is skipped unless Linux, `/dev/kvm`, Firecracker, kernel, and rootfs are all available

Debugging commands:

- `air init firecracker`
- `air doctor --provider firecracker --human`
- `air run [--provider ...] [--timeout 30s] [--memory-mib 256] [--vcpu-count 1] -- <command>`
- `go run ./examples/agent-runner --task all`
- `go run ./examples/agent-runner --planner deepseek --model deepseek-chat --task all`
- `go run ./examples/agent-runner --planner scripted --task all`
- `air session list`
- `air session inspect <id>`
- `air session console <id> [--tail=N]`
- `air session console <id> --follow [--tail=N]`
- `air session events <id> [--tail=N]`
- `air session events <id> --follow [--tail=N]`

Current console limitation:

- `air session console` currently shows the serial console log file
- `air session events` shows structured lifecycle / exec events including `request_id` and duration
- It is useful for boot diagnostics, but it is not an interactive guest shell yet

Current status behavior:

- `air session list` and `air session inspect` refresh session status from the runtime before printing
- If the runtime directory still exists but the VM process has exited, the session status is reported as `stopped`
