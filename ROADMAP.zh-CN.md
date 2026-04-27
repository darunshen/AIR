# AIR 路线图

[English](ROADMAP.md)

## 愿景

通过轻量虚拟机隔离，为 coding agent 提供更安全的默认执行边界，同时保住它们真实需要的 repo 任务闭环。

## 当前基线

AIR 已经不再处于“能不能先做一个最小 session MVP”的阶段。

当前基线已经包含：

- `air run`
- 有状态 session
- `local` 与 `firecracker` provider
- 基于 `vsock` 的 guest 执行
- Firecracker 运行产物与调试路径
- workspace 注入与导出
- OpenAI / DeepSeek reference agent 接入
- Firecracker guest 内 OpenClaude 启动与 forward
- LLM 驱动与 OpenClaude 驱动链路的真实验收工作流

## 下一阶段：运行时加固

目标：让当前工作流更稳定、更可观察、更接近生产可用。

重点：

- 稳定 Firecracker guest 网络与策略能力
- 改进清理与生命周期保证
- 继续细化 host/guest 错误分类与恢复行为
- 持续补强 OpenClaude 与 agent-facing 工作流稳定性
- 改进发布打包与安装体验

成功标准：

- Firecracker 驱动的 agent 工作流失败更少、失败原因更清晰
- guest 运行产物更易于检查与支持
- 部署和安装路径更可预测

## 后续阶段：镜像与状态系统

目标：改善 guest 镜像管理的可复现性与运维成本。

重点：

- 镜像生命周期管理
- 更清晰的 rootfs / workspace layering 契约
- prepared guest image 的可复现构建
- runtime 产物的清理与回收
- 更扎实的存储架构文档与工具

成功标准：

- prepared guest image 更容易重建和理解
- runtime 状态更容易安全回收
- rootfs 和 workspace 行为更容易被用户理解

## 更后阶段：性能优化

目标：降低冷启动成本，改善重复任务体验。

重点：

- snapshot / restore
- 更快的 guest 启动
- 预热池或类似复用机制
- 流式输出与长任务体验优化

成功标准：

- 启动延迟显著下降
- 重复 agent 工作流的操作体验更好

## 生态与社区

目标：让外部用户更容易采用 AIR。

重点：

- 更好的示例与教程
- 贡献者 onboarding
- 公开架构说明
- 更清晰的打包与发布预期

## 当前优先级

当前第一优先级不再是“先做一个基础 MVP”，而是把已经打通的 AI agent 工作流继续加固，让 Firecracker 驱动的真实使用场景更稳定、更容易运维。
