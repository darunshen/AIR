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

- 基础调试能力已接入
  - `air session list`
  - `air session inspect <id>`
  - `air session console <id>`
  - `air session console <id> --follow`
  - runtime inspect 已返回 provider、pid、console、socket、vsock、config 等路径
  - `list` / `inspect` 会按 runtime 实况刷新 session 状态
  - 旧 session 缺失 `provider` 时可自动补全

- CLI 级 provider 选择能力已接入
  - `air session create --provider local`
  - `air session create --provider firecracker`

- Guest Agent 与 `vsock exec` 已打通
  - 已有 `cmd/air-agent`
  - guest 侧已支持监听 `virtio-vsock`
  - Host/Guest 协议已打通
  - `internal/vm/firecracker.go` 的 `Exec()` 已替换为真实 `vsock` 通信
  - 真机已验证 `session create -> exec -> delete`

- Firecracker guest rootfs 注入链路已接入
  - 仓库脚本：`scripts/prepare-firecracker-rootfs.sh`
  - 会将 `air-agent` 打进 rootfs
  - 会把 `local` service 接进 default runlevel
  - 会生成可自动发现的 `assets/firecracker/hello-rootfs-air.ext4`

## P0: Guest Agent 与通信

- guest ready 判定已接入
  - `Start()` 已等待 guest `ready` 握手
  - 串口里仍可看到 `[air-agent] boot hook start`

- 补强 Host/Guest 协议
  - request_id 的可观测性
  - 版本兼容字段
  - 更细的 transport / guest 错误分类

## P1: Runtime 集成

- 为 `vm` provider 增加更直接的启动入口
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

## P1: Rootfs 与镜像体系

- 确定 guest rootfs 技术路线
  - 已先基于官方 demo rootfs 跑通启动与 `exec`
  - 再切到自维护 rootfs

- 设计只读基础镜像 + 每 session 独立写层
- 明确 overlay 方案与目录布局
- 评估 rootfs 构建时如何保留 root ownership / device 节点

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
- 增加 timeout 配置能力
- 增加资源限制配置能力
- 将 `air session console` 从日志查看升级为更强的调试入口
  - 明确串口 attach 方案
  - 评估可交互控制台能力
  - 区分“console 查看”与“guest exec”

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
