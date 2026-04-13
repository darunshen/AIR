# AIR 项目计划

## 1. 项目概述

### 1.1 项目名称

AIR（Agent Isolation Runtime）

### 1.2 项目定位

AIR 是一个面向 AI Agent 的隔离执行运行时，目标是为不可信代码提供默认安全、可复现、可销毁的执行环境。产品重点不是通用容器编排，而是为 AI 生成代码提供独立 VM 级执行边界。

### 1.3 项目目标

- 提供一次性执行能力 `air run`
- 提供有状态执行能力 `air session create/exec/delete`
- 默认无网络、可限制 CPU/内存/超时
- 通过基础镜像、overlay、snapshot 实现可复现与快速恢复

## 2. 业务价值

- 解决 AI 自动生成代码执行时的宿主机安全风险
- 为 Agent 平台提供统一的沙箱执行底座
- 降低接入方自建隔离环境的工程成本
- 支持后续扩展为 API 服务、多租户平台与调度系统

## 3. 里程碑规划

### 阶段一：MVP（1-2 周）

目标：完成最小可跑版本。

范围：

- `air session create`
- `air session exec <id> "<cmd>"`
- `air session delete <id>`
- 单机部署
- 本地 JSON 存储 session 状态
- Host 与 VM 通过文件方式通信
- 默认无网络

交付：

- CLI 工具
- Session Manager
- VM 启动/销毁封装
- Guest 内轮询执行脚本

### 阶段二：V1（2-4 周）

目标：完成工程化基础能力。

范围：

- 引入 Guest Agent
- 通信升级为 `virtio-serial` 或 `vsock`
- 增加 HTTP API
- rootfs 改为 `base image + overlay`
- 增加 timeout、日志、GC

交付：

- API Server
- Overlay 文件系统管理
- 生命周期管理模块
- 闲置 session 自动回收

### 阶段三：V2（4-8 周）

目标：提升性能与产品可用性。

范围：

- Snapshot / Restore
- 预热 VM 池
- 流式输出
- 白名单网络策略
- 资源配额与监控

交付：

- Snapshot Engine
- VM Pool
- 基础监控指标与告警

## 4. 团队分工建议

- 虚拟化/底层：Hypervisor、VM 启动、镜像、snapshot
- 控制面后端：Session Manager、API、调度、状态存储
- 平台工程：构建、测试、发布、日志与监控
- 产品/方案：需求收敛、场景定义、验收标准

## 5. 关键风险与对策

- Host/Guest 通信不稳定
  对策：MVP 先文件通信，V1 尽快切到 `virtio-serial`
- Session 状态泄漏或无法回收
  对策：设计明确状态机与 GC 机制
- 启动耗时高
  对策：V2 引入预热池与 snapshot
- Guest Agent 崩溃导致 session 不可用
  对策：增加健康检查、超时与重建机制

## 6. 验收标准

- 可以创建、执行、销毁 session
- 同一 session 多次执行能保留文件状态
- 默认情况下 VM 无网络访问能力
- 支持超时控制与异常返回
- 闲置 session 可被自动回收
