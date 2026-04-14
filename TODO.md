# TODO

## P0: Firecracker 真机跑通

- 准备可用宿主机环境
  - 提供 `/dev/kvm`
  - 确认当前用户具备访问 KVM 的权限
  - 安装 Firecracker 二进制

- 准备最小启动资产
  - 获取或构建可启动的 Linux kernel
  - 获取或构建最小 rootfs
  - 明确 `AIR_FIRECRACKER_KERNEL` 和 `AIR_FIRECRACKER_ROOTFS` 的目录规范

- 在真实环境验证最小启动链路
  - `Start()` 能真正启动 microVM
  - `Stop()` 能正常销毁 microVM
  - 产出实际 console 日志样例

## P0: Guest Agent 与通信

- 设计并实现最小 `air-agent`
  - guest 启动后自动运行
  - 监听 `virtio-vsock`
  - 接收 `exec` 请求
  - 返回 `stdout/stderr/exit_code`

- 完成 Host/Guest 协议
  - 请求结构
  - 响应结构
  - 超时与错误语义
  - request_id 关联机制

- 将 `internal/vm/firecracker.go` 的 `Exec()` 从占位实现替换为真实 `vsock` 通信

## P1: Runtime 集成

- 将 session provider 从默认 `local` 平滑切换到可配置 `firecracker`
- 为 `vm` provider 增加显式配置入口
  - 环境变量说明
  - CLI/配置文件方案

- 增加 Firecracker 运行目录规范
  - socket
  - pid
  - console
  - metrics
  - vsock

- 增加更清晰的错误信息
  - 缺少 `/dev/kvm`
  - 找不到 Firecracker 二进制
  - kernel/rootfs 缺失
  - guest agent 未就绪

## P1: Rootfs 与镜像体系

- 确定 guest rootfs 技术路线
  - Ubuntu 最小镜像
  - Yocto 自定义镜像

- 将 `air-agent`、必要 shell、基础工具打进 rootfs

- 设计只读基础镜像 + 每 session 独立写层
- 明确 overlay 方案与目录布局

## P1: 可观测性

- 记录 Firecracker console 日志
- 记录启动失败原因
- 记录 exec 耗时
- 记录 session 生命周期事件

## P1: 测试

- 增加 Firecracker 集成测试
  - 仅在具备 `/dev/kvm` 的环境执行
  - 跳过条件明确

- 增加 guest agent 通信测试
- 增加 `session -> firecracker runtime -> exec -> delete` 端到端测试

## P2: CLI 与产品能力

- 增加 `air run`
- 增加 `air session list`
- 增加 `air session inspect`
- 增加 timeout 配置能力
- 增加资源限制配置能力

## P2: 生命周期与回收

- 增加 session 状态机细化
  - created
  - booting
  - running
  - idle
  - stopped
  - error
  - deleted

- 增加 GC
  - 超时回收
  - 异常 session 清理
  - 运行目录残留清理

## P2: Snapshot 与性能

- 调研并接入 Firecracker snapshot / restore
- 设计预热 VM 池
- 评估启动时延
- 评估 session 恢复时延

## P3: 开源与文档

- 补充 Firecracker 本地开发指南
- 补充 guest rootfs 构建说明
- 补充真实环境运行示例
- 补充集成测试说明
