# TODO

## Done

- Firecracker provider 已完成 host 侧基础接入
  - runtime provider 已拆分为 `local` / `firecracker`
  - Firecracker API socket、pid、console、metrics、vsock、config 产物已落盘
  - Firecracker 预检错误已细化
  - 真机 `Start()` / `Stop()` 已验证通过
  - 已拿到真实 `console.log` 内核启动日志

- Firecracker 实验资产准备已简化
  - 支持下载官方 release 的 `firecracker`
  - 支持下载官方 demo `hello-vmlinux.bin`
  - 支持下载官方 demo `hello-rootfs.ext4`
  - 仓库脚本：`scripts/fetch-firecracker-demo-assets.sh`
  - 当仓库根目录存在 `assets/firecracker/` 时，`firecracker` provider 可自动发现这些资产

- 文档基础已补齐
  - `docs/operations-manual.md`
  - `docs/firecracker-deployment-guide.md`

## P0: Guest Agent 与通信

- 设计最小 `air-agent`
  - guest 启动后自动运行
  - 监听 `virtio-vsock`
  - 接收 `exec` 请求
  - 返回 `stdout/stderr/exit_code`

- 完成 Host/Guest `vsock` 协议
  - 请求结构
  - 响应结构
  - 超时与错误语义
  - request_id 关联机制

- 将 `internal/vm/firecracker.go` 的 `Exec()` 从“guest 未就绪”错误替换为真实 `vsock` 通信

## P1: Runtime 集成

- 为 `vm` provider 增加更直接的启动入口
  - CLI 级 provider 选择能力
  - 配置文件方案
  - “使用仓库内 demo 资产”的一键入口

- 将 Firecracker 相关配置从 `Start()` 中继续拆分
  - machine config
  - boot source
  - drives
  - vsock
  - startup action

- 处理 Firecracker API 向后兼容细节
  - `vsock_id` deprecated 警告
  - 不同版本 API 路径和字段差异

- 在成功启动后增加“guest ready”判定
  - 先通过 console 日志判断
  - 后续切换到 `air-agent` ready 信号

## P1: Rootfs 与镜像体系

- 确定 guest rootfs 技术路线
  - 先基于官方 demo rootfs 跑通启动
  - 再切到自维护 rootfs

- 将 `air-agent`、必要 shell、基础工具打进 rootfs

- 设计只读基础镜像 + 每 session 独立写层
- 明确 overlay 方案与目录布局

## P1: 可观测性

- 记录启动失败原因到更清晰的错误链
- 记录 exec 耗时
- 记录 session 生命周期事件

## P1: 测试

- 扩大 Firecracker 集成测试覆盖
  - 校验 `console.log` 非空
  - 校验真实 `delete` 后目录清理
  - 校验 demo 资产自动发现路径

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

- 补充 guest rootfs 构建说明
- 补充真实环境运行示例
- 补充集成测试说明
