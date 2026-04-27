# AIR

[中文](#中文简介) | [English](#english)

AIR (Agent Isolation Runtime) is an open-source runtime for executing AI-agent work inside isolated lightweight VMs.

The product goal is not “run code somehow”. The product goal is to give coding agents a safer default execution boundary, while still preserving the core repo-task loop: read files, execute commands, modify code, inspect logs, and continue.

## English

### What AIR Is

AIR provides a VM-backed execution boundary for agent workflows that should not run directly on the host by default.

Today AIR already supports:

- one-shot execution through `air run`
- stateful sessions through `air session ...`
- `local` and `firecracker` runtime providers
- guest execution over `vsock`
- Firecracker console, events, and runtime inspection
- workspace injection and merged workspace export
- OpenClaude process management inside AIR sessions
- host-to-guest OpenClaude TCP forwarding for Firecracker sessions
- real LLM acceptance workflows for the reference agent

### Why AIR

Modern coding agents do more than generate text. They inspect repositories, run commands, modify files, rerun tests, and keep iterating. That changes the runtime requirement:

- the executed code is often untrusted
- the execution environment should be disposable
- resource access should be explicit and constrained
- task state and artifacts should be inspectable
- the host should not be the default blast radius

### Current Product Shape

#### 1. One-shot execution

```bash
air run -- echo hello
air run --provider firecracker --timeout 30s --workspace /path/to/repo -- make test
```

#### 2. Stateful sessions

```bash
air session create --provider firecracker --workspace /path/to/repo
air session list
air session inspect <id>
air session console <id> --follow
air session events <id> --follow
air session exec <id> "pwd && ls"
air session export-workspace <id> ./out --force
air session delete <id>
```

#### 3. Firecracker environment setup

```bash
air init firecracker
air doctor --provider firecracker --human
```

#### 4. OpenClaude inside AIR

```bash
air agent openclaude start --provider firecracker --guest-repo /opt/openclaude
air agent openclaude status <session-id>
air agent openclaude forward <session-id> --listen 127.0.0.1:50052
air agent openclaude stop <session-id>
```

### Current Firecracker Architecture

```text
CLI
  |
  v
Session Manager
  |
  +-- local runtime
  +-- firecracker runtime
          |
          +-- Firecracker VM
          +-- guest air-agent over vsock
          +-- rootfs.ext4 (session-private)
          +-- workspace.ext4 (read-only lower)
          +-- workspace-upper.ext4 (writable upper)
          +-- events / console / metrics / config artifacts
```

### Current OpenClaude Path

The current OpenClaude integration path is:

- run the OpenClaude gRPC server inside an AIR session or Firecracker guest
- keep OpenClaude installed in the guest at `/opt/openclaude`
- run repo tasks against `/workspace`
- expose the guest OpenClaude TCP endpoint on the host with `air agent openclaude forward`

The real Firecracker acceptance path is now implemented and validated through:

```bash
scripts/run-openclaude-firecracker-acceptance.sh
```

That workflow now covers:

- guest startup
- OpenClaude start and readiness
- host forwarding
- real model task execution
- workspace export and output verification

### Firecracker Asset Strategy

The current recommended asset strategy is:

- Firecracker binary: official Firecracker release
- kernel and demo rootfs baseline: official Firecracker demo assets
- AIR guest agent injection: `scripts/prepare-firecracker-rootfs.sh`
- OpenClaude guest image: `scripts/prepare-openclaude-alpine-rootfs.sh`

If you installed AIR from `.deb`, the package contains the CLI, but not the Firecracker binary, kernel, or rootfs assets. Run:

```bash
air init firecracker
air doctor --provider firecracker --human
```

### What Is Already Done

- Firecracker host-side lifecycle and runtime artifacts
- guest execution over `vsock`
- one-shot `air run`
- session inspection, console, events, and debugging paths
- workspace overlay mount inside the guest
- merged workspace export
- OpenAI and DeepSeek planner adapters in the reference agent
- gated real-LLM acceptance workflows in GitHub Actions
- `.deb`, release archive, and apt-style packaging output
- OpenClaude guest startup, forwarding, and real Firecracker acceptance path

### What Is Still Next

The next major product work is no longer “build a basic MVP”. The next work is product hardening:

- stabilize and extend the Firecracker guest networking and policy surface
- improve image lifecycle, cleanup, and reproducibility
- add snapshot / fast restore / startup performance work
- keep tightening the agent-facing workflow and packaging quality

### Documentation

- [Documentation Index](docs/README.en.md) / [文档索引](docs/README.md)
- [Using AIR From AI Agents](docs/ai-agent-usage.en.md) / [中文](docs/ai-agent-usage.md)
- [Firecracker Deployment Guide](docs/firecracker-deployment-guide.en.md) / [中文](docs/firecracker-deployment-guide.md)
- [Operations Manual](docs/operations-manual.en.md) / [中文](docs/operations-manual.md)
- [Rootfs Management Architecture](docs/rootfs-management-architecture.en.md) / [中文](docs/rootfs-management-architecture.md)
- [OpenClaude Integration](docs/openclaude-integration.en.md) / [中文](docs/openclaude-integration.md)
- [Release And Distribution](docs/release-distribution.en.md) / [中文](docs/release-distribution.md)
- [Roadmap](ROADMAP.md) / [中文](ROADMAP.zh-CN.md)
- [Repository Guidelines](AGENTS.md)
- [Contributing Guide](CONTRIBUTING.md) / [中文](CONTRIBUTING.zh-CN.md)

## 中文简介

### AIR 是什么

AIR（Agent Isolation Runtime）是一个面向 AI Agent 的开源隔离运行时，用轻量虚拟机而不是共享内核容器来承载 agent 的真实执行流程。

它解决的不是“怎么运行代码”，而是“怎么让 coding agent 在默认更安全的边界里运行，同时还能保住读文件、改代码、跑测试、看日志、继续迭代这些核心闭环”。

### 为什么要做 AIR

现代 coding agent 已经不只是输出文本，而是在真实仓库里：

- 读文件
- 执行命令
- 修改代码
- 重跑测试
- 根据结果继续下一步

这意味着底层运行时必须变化：

- 执行代码默认不可信
- 环境需要可销毁
- 资源访问需要显式受限
- 状态与产物需要可检查
- 宿主机不该成为默认爆炸半径

### 当前产品形态

#### 1. 一次性执行

```bash
air run -- echo hello
air run --provider firecracker --timeout 30s --workspace /path/to/repo -- make test
```

#### 2. 有状态 Session

```bash
air session create --provider firecracker --workspace /path/to/repo
air session list
air session inspect <id>
air session console <id> --follow
air session events <id> --follow
air session exec <id> "pwd && ls"
air session export-workspace <id> ./out --force
air session delete <id>
```

#### 3. Firecracker 环境准备

```bash
air init firecracker
air doctor --provider firecracker --human
```

#### 4. 在 AIR 中运行 OpenClaude

```bash
air agent openclaude start --provider firecracker --guest-repo /opt/openclaude
air agent openclaude status <session-id>
air agent openclaude forward <session-id> --listen 127.0.0.1:50052
air agent openclaude stop <session-id>
```

### 当前 Firecracker 架构

```text
CLI
  |
  v
Session Manager
  |
  +-- local runtime
  +-- firecracker runtime
          |
          +-- Firecracker VM
          +-- guest air-agent over vsock
          +-- rootfs.ext4（session 私有）
          +-- workspace.ext4（只读 lower）
          +-- workspace-upper.ext4（可写 upper）
          +-- events / console / metrics / config 等运行产物
```

### 当前 OpenClaude 路径

当前 OpenClaude 接入路径是：

- 把 OpenClaude gRPC server 跑在 AIR session / Firecracker guest 内
- guest 内程序目录固定在 `/opt/openclaude`
- repo 任务工作目录固定面向 `/workspace`
- 通过 `air agent openclaude forward` 把 guest 内 OpenClaude TCP endpoint 暴露到 host

真实 Firecracker 验收链路已经实现并验证，入口是：

```bash
scripts/run-openclaude-firecracker-acceptance.sh
```

这条链路当前已经覆盖：

- guest 启动
- OpenClaude 启动与探活
- host 侧 forward
- 真实模型任务执行
- workspace 导出与结果校验

### Firecracker 资产策略

当前推荐资产策略是：

- Firecracker 二进制：官方 release
- kernel / demo rootfs 基线：官方 demo 资产
- AIR guest agent 注入：`scripts/prepare-firecracker-rootfs.sh`
- OpenClaude guest 镜像：`scripts/prepare-openclaude-alpine-rootfs.sh`

如果你是通过 `.deb` 安装 AIR，需要注意安装包只包含 CLI，不包含 Firecracker 二进制、kernel 或 rootfs 资产。启用 `firecracker` provider 前先执行：

```bash
air init firecracker
air doctor --provider firecracker --human
```

### 当前已经完成

- Firecracker host 侧生命周期与运行产物管理
- 基于 `vsock` 的 guest 执行
- 一次性入口 `air run`
- session inspect / console / events 等调试路径
- guest 内 workspace overlay 挂载
- merged workspace 导出
- reference agent 的 OpenAI / DeepSeek planner
- GitHub Actions 中的 gated 真实 LLM 验收链路
- `.deb`、release archive、apt 风格目录产物
- OpenClaude 的 guest 启动、host forward 与 Firecracker 真机验收链路

### 当前下一步

当前下一阶段已经不再是“先做一个最小 MVP”，而是产品化加固：

- 继续增强 Firecracker guest 网络与策略能力
- 改进镜像生命周期、清理与可复现性
- 增加 snapshot / fast restore / 启动性能优化
- 持续补强面向 agent 的工作流与发布质量

### 文档

- [文档索引](docs/README.md) / [Documentation Index](docs/README.en.md)
- [AI Agent 使用说明](docs/ai-agent-usage.md) / [English](docs/ai-agent-usage.en.md)
- [Firecracker 真机部署指南](docs/firecracker-deployment-guide.md) / [English](docs/firecracker-deployment-guide.en.md)
- [操作手册](docs/operations-manual.md) / [English](docs/operations-manual.en.md)
- [根文件系统管理架构](docs/rootfs-management-architecture.md) / [English](docs/rootfs-management-architecture.en.md)
- [OpenClaude 接入方案](docs/openclaude-integration.md) / [English](docs/openclaude-integration.en.md)
- [发布与安装包交付](docs/release-distribution.md) / [English](docs/release-distribution.en.md)
- [路线图](ROADMAP.zh-CN.md) / [English](ROADMAP.md)
- [仓库协作指南](AGENTS.md)
- [贡献指南](CONTRIBUTING.zh-CN.md) / [English](CONTRIBUTING.md)
