# AIR 路线图

[English](ROADMAP.md)

## 愿景

通过轻量虚拟机隔离，为 AI Agent 提供更安全的默认执行边界。

## 第一阶段：MVP

目标：交付一个最小可运行的本地 session runtime。

范围：

- `air session create`
- `air session exec`
- `air session delete`
- 本地 `sessions.json`
- 单机运行时
- Host/Guest 文件通信
- 默认无网络

成功标准：

- session 可以创建
- 同一 VM 中可以连续执行命令
- `exec` 之间文件状态可保留
- session 可以被干净删除

## 第二阶段：工程化基础

目标：把 MVP 打造成稳定的开发者基础设施。

范围：

- Firecracker 运行时集成
- guest agent
- `virtio-vsock`
- HTTP API
- `base image + overlay`
- timeout 处理
- session GC
- 结构化日志

## 第三阶段：性能与平台能力

目标：降低冷启动成本并改善运行时体验。

范围：

- snapshot / restore
- 预热 VM 池
- 流式输出
- 网络白名单模式
- 指标与可观测性

## 第四阶段：社区与生态

目标：让外部开发者更容易使用和参与 AIR。

范围：

- 示例和 demo
- starter issue
- 公开架构讨论
- 贡献者 onboarding
- benchmark 与对比材料

## 当前优先级

当前第一优先级仍然是：先把基于 session 的可用 MVP 和真实 AI agent 工作流闭环打通。
