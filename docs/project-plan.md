# AIR 项目计划（历史归档）

[English](project-plan.en.md)

本文档保留 AIR 早期阶段的计划快照，用于说明项目最初如何拆分 MVP、V1、V2。

注意：

- 这不是当前能力基线
- 当前产品状态、优先级和路线请以根目录 `README.md`、`ROADMAP.md`、`TODO.md` 为准
- 文中对“尚未实现”的描述可能已经过时

## 1. 早期项目定位

AIR（Agent Isolation Runtime）最初被定义为一个面向 AI Agent 的隔离执行运行时，目标是为不可信代码提供默认安全、可复现、可销毁的执行环境。产品重点不是通用容器编排，而是为 AI 生成代码提供独立 VM 级执行边界。

## 2. 早期目标

- 提供一次性执行能力 `air run`
- 提供有状态执行能力 `air session create/exec/delete`
- 默认无网络、可限制 CPU/内存/超时
- 通过基础镜像、overlay、snapshot 实现可复现与快速恢复

## 3. 早期里程碑划分

### 阶段一：MVP

当时的目标是完成最小可跑版本，范围包括：

- `air session create`
- `air session exec <id> "<cmd>"`
- `air session delete <id>`
- 单机部署
- 本地 JSON 存储 session 状态
- Host 与 VM 通过文件方式通信
- 默认无网络

### 阶段二：V1

当时规划的工程化能力包括：

- 引入 Guest Agent
- 通信升级为 `virtio-serial` 或 `vsock`
- 增加 HTTP API
- rootfs 改为 `base image + overlay`
- 增加 timeout、日志、GC

### 阶段三：V2

当时规划的性能与产品化能力包括：

- Snapshot / Restore
- 预热 VM 池
- 流式输出
- 白名单网络策略
- 资源配额与监控

## 4. 仍然有参考价值的内容

尽管这是历史文档，其中几类判断依然有参考意义：

- AIR 的核心边界是“为 agent 提供隔离执行底座”，不是通用 PaaS
- Host/Guest 通信、状态回收、启动耗时始终是关键工程点
- 生命周期、镜像管理、可观测性需要长期持续增强
