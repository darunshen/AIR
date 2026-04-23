# AIR 操作手册

本文档描述当前仓库版本的实际操作方式，重点覆盖：

- 如何在本地启动和验证 AIR
- 如何在 `local` 与 `firecracker` 两种 runtime provider 间切换
- 如何查看运行产物和排查常见问题
- 当前版本已经实现和暂未实现的能力边界

## 1. 当前状态

截至当前代码版本：

- `air run`
- `air session create`
- `air session list`
- `air session inspect`
- `air session console`
- `air session exec`
- `air session delete`

已经可以通过 `local` provider 完整使用。

`air run` 已经具备：

- 一次性创建隔离环境
- 执行单条命令
- 返回结构化结果
- 自动销毁临时 session
- 支持 `--timeout`
- 支持 `--memory-mib`
- 支持 `--vcpu-count`

仓库还提供了一个最小 reference agent：

- `examples/agent-runner`
- 用于验证 AIR 是否适合 agent 消费
- 当前已支持 OpenAI planner
- 当前已支持 DeepSeek planner
- 当前也保留 `scripted` planner 作为离线 fallback
- 当前内置 one-shot、session 多步执行、失败恢复三类任务

关于外部 LLM agent 的选型与环境建议，见：

- [AI Agent 选型与接入方案](agent-selection.md)
- [通过 AI Agent 使用 AIR](ai-agent-usage.md)

关于安装包、GitHub Release 和 `.deb` 交付，见：

- [发布与安装包交付](release-distribution.md)

`firecracker` provider 已经具备：

- 启动前环境预检
- Firecracker 进程启动
- Host 侧 API 配置
- guest rootfs 注入脚本
- guest `air-agent` 启动链
- Host/Guest `vsock exec`
- 运行目录和日志产物落盘
- `Stop()` 清理

因此，当前如果切到 `firecracker` provider，并且 rootfs 已通过仓库脚本注入 `air-agent`，`Start()`、`Exec()` 和 `Stop()` 都可以真实工作。

当前调试方式：

- 用 `air session list` 查看现有 session
- 用 `air session inspect <id>` 查看 session 与 runtime 状态
- 用 `air session console <id>` 查看串口日志
- 用 `air session console <id> --follow` 持续跟随串口日志
- 用 `air session events <id>` 查看结构化事件日志
- 用 `air session events <id> --follow` 跟随生命周期和 exec 事件

注意：

- 这里的 `console` 目前是日志查看，不是交互式 guest shell
- `list` / `inspect` 会根据 runtime 实况刷新 session 状态，不只是读取 JSON 中的旧值

## 2. 环境要求

### 2.1 基础开发环境

- Go 1.22 或更新版本
- Linux、macOS 或 Windows 可用于本地 `local` provider 开发

### 2.2 Firecracker 实验环境

`firecracker` provider 需要：

- Linux
- KVM 可用
- 可访问 `/dev/kvm`
- 已安装 Firecracker 二进制
- 可启动的 Linux kernel 镜像
- 可启动的 rootfs 镜像

如果缺少这些条件，AIR 会在启动前直接报错。

建议先执行：

```bash
air init firecracker
air doctor --provider firecracker --human
```

如果你使用的是 `.deb` 安装包，需要注意当前包只安装 `air` / `air-agent` CLI，不会附带 Firecracker、`vmlinux`、`rootfs.ext4`。

更细的宿主机准备步骤见：

- [Firecracker 真机部署指南](firecracker-deployment-guide.md)

## 3. 仓库准备

在仓库根目录执行：

```bash
go test ./...
```

如果只想生成 CLI：

```bash
go build ./cmd/air
```

也可以直接使用：

```bash
go run ./cmd/air run -- echo hello
go run ./cmd/air run --memory-mib 512 --vcpu-count 2 -- echo hello
go run ./cmd/air init firecracker --source custom
go run ./cmd/air doctor --provider firecracker --human
go run ./cmd/air run --timeout 5s -- sh -c 'echo hello && exit 3'
go run ./examples/agent-runner --task all
go run ./examples/agent-runner --planner deepseek --model deepseek-chat --task all
go run ./examples/agent-runner --planner scripted --task all
go run ./cmd/air session create
go run ./cmd/air session create --provider local
go run ./cmd/air session create --provider firecracker
```

## 4. 本地模式操作

`local` provider 是默认模式，不需要额外配置。

### 4.1 一次性执行

```bash
go run ./cmd/air run -- echo hello
go run ./cmd/air run --memory-mib 512 --vcpu-count 2 -- echo hello
go run ./cmd/air run --timeout 5s -- sh -c 'echo hello && exit 3'
```

其中：

- `--timeout` 控制单次执行超时
- `--memory-mib` 控制 Firecracker VM 内存上限
- `--vcpu-count` 控制 Firecracker VM vCPU 数
- `local` provider 当前会接受这两个参数，但不会真的做 CPU/内存隔离

默认输出为结构化 JSON，字段包括：

- `stdout`
- `stderr`
- `exit_code`
- `request_id`
- `duration_ms`
- `timeout`
- `error_type`
- `error_message`

当前 `error_type` 的稳定取值包括：

- `invalid_argument`
- `startup_error`
- `transport_error`
- `exec_error`
- `timeout`
- `cleanup_error`

其中：

- 命令非零退出时会返回 `exec_error`
- 超时时会返回 `timeout`
- Firecracker 启动前环境问题通常返回 `startup_error`

如果想使用更适合人工查看的输出，可加：

```bash
go run ./cmd/air run --human -- echo hello
```

### 4.2 创建 session

```bash
go run ./cmd/air session create
```

输出为一个 session ID，例如：

```text
sess_xxx
```

### 4.3 执行命令

```bash
go run ./cmd/air session exec <session_id> "echo hello > a.txt"
go run ./cmd/air session exec <session_id> "cat a.txt"
```

在 `local` provider 下，命令会在该 session 独立 workspace 中执行，文件状态会保留到 session 删除为止。

### 4.4 删除 session

```bash
go run ./cmd/air session delete <session_id>
```

删除后会同时移除对应运行目录，并从 `data/sessions.json` 中清除记录。

### 4.5 查看 session 列表

```bash
go run ./cmd/air session list
```

### 4.6 查看 session 详情

```bash
go run ./cmd/air session inspect <session_id>
```

### 4.6 查看控制台日志

```bash
go run ./cmd/air session console <session_id>
go run ./cmd/air session console <session_id> --follow
go run ./cmd/air session console <session_id> --tail=100
go run ./cmd/air session events <session_id>
go run ./cmd/air session events <session_id> --follow
```

## 5. Firecracker 模式操作

### 5.1 环境变量

如果想先确认当前机器是否具备 Firecracker 运行条件，先执行：

```bash
go run ./cmd/air init firecracker
go run ./cmd/air doctor --provider firecracker --human
```

`air init firecracker` 会先询问：

- 是否下载 AIR 官方镜像包
- 还是用户自己部署 Firecracker 资产

如果选择 AIR 官方镜像，命令会下载当前 AIR 版本对应的官方 bundle。

切换到 `firecracker` provider 前，至少需要设置：

```bash
export AIR_VM_RUNTIME=firecracker
export AIR_FIRECRACKER_BIN=firecracker
export AIR_FIRECRACKER_KERNEL=/path/to/vmlinux
export AIR_FIRECRACKER_ROOTFS=/path/to/rootfs.ext4
export AIR_KVM_DEVICE=/dev/kvm
```

其中：

- `AIR_VM_RUNTIME` 默认为 `local`
- `AIR_FIRECRACKER_BIN` 默认为 `firecracker`
- `AIR_KVM_DEVICE` 默认为 `/dev/kvm`

如果你还没有准备好 Firecracker 二进制、kernel 和 rootfs，先看：

- [Firecracker 真机部署指南](firecracker-deployment-guide.md)

如果只是先把 AIR 当前的 Firecracker 生命周期跑通，优先执行：

```bash
scripts/fetch-firecracker-demo-assets.sh
scripts/prepare-firecracker-rootfs.sh
```

它会下载：

- 官方 release 的 `firecracker`
- 官方 demo `hello-vmlinux.bin`
- 官方 demo `hello-rootfs.ext4`

并生成：

- 注入了 `air-agent` 的 `hello-rootfs-air.ext4`

### 5.2 创建 session

```bash
go run ./cmd/air session create
```

如果环境满足要求，AIR 会：

- 启动 Firecracker 进程
- 创建 API socket
- 写入 machine、boot、drive、vsock 配置
- 启动 microVM

如果你不想依赖当前 shell 的默认 provider，也可以显式指定：

```bash
go run ./cmd/air session create --provider firecracker
```

### 5.3 执行命令

```bash
go run ./cmd/air session exec <session_id> "uname -a"
```

前提是当前使用的 rootfs 已经过 `scripts/prepare-firecracker-rootfs.sh` 处理，或者 `AIR_FIRECRACKER_ROOTFS` 指向等价的已注入镜像。

### 5.4 删除 session

```bash
go run ./cmd/air session delete <session_id>
```

该操作会尝试：

- 读取 `firecracker.pid`
- 杀掉 Firecracker 进程
- 清理整個 session 运行目录

### 5.5 查看 Firecracker 状态和日志

```bash
go run ./cmd/air session inspect <session_id>
go run ./cmd/air session console <session_id>
go run ./cmd/air session console <session_id> --follow
```

## 6. 运行目录说明

### 6.1 本地模式

`local` provider 运行目录：

```text
runtime/sessions/local/<session_id>/
  workspace/
  task/
```

说明：

- `workspace/`：命令执行工作目录
- `task/cmd.sh`：最近一次执行命令记录
- `task/result.txt`：最近一次标准输出
- `task/stderr.txt`：最近一次标准错误

### 6.2 Firecracker 模式

`firecracker` provider 运行目录：

```text
runtime/sessions/firecracker/<session_id>/
  rootfs.ext4
  workspace.ext4
  workspace-upper.ext4
  firecracker.sock
  firecracker.pid
  firecracker.vsock
  console.log
  events.jsonl
  metrics.log
  config/
    machine-config.json
    boot-source.json
    rootfs-drive.json
    vsock.json
```

说明：

- `rootfs.ext4`：从基础 rootfs 复制出的每 session 私有根盘
- `workspace.ext4`：当使用 `--workspace` 时生成的 host repo 只读镜像
- `workspace-upper.ext4`：当使用 `--workspace` 时生成的 guest workspace 写层
- `firecracker.sock`：Host 访问 Firecracker API 的 Unix socket
- `firecracker.pid`：Firecracker 进程 PID
- `firecracker.vsock`：预留给 Host/Guest `vsock` 通信的 Unix socket 路径
- `console.log`：串口输出日志
- `events.jsonl`：结构化生命周期 / exec 事件日志
- `metrics.log`：当前仅预创建，后续用于 Firecracker metrics
- `config/*.json`：启动时实际下发的配置快照

当 session 带 `--workspace` 时，可导出当前工作区结果：

```bash
air session export-workspace <session-id> /tmp/air-export
air session export-workspace <session-id> /tmp/air-export --force
```

说明：

- 导出的是当前 merged `/workspace` 视图，而不是原始只读 `workspace.ext4`
- 默认要求输出目录为空或不存在
- `--force` 会清空已有输出目录后再导出
- 当前要求 session 仍处于运行中

## 7. 真机生命周期验证

仓库内已经提供一个默认跳过的集成测试，用于验证：

- `Start()` 能启动 Firecracker
- `Exec()` 能通过 `vsock` 真实执行命令
- 每 session 独立 `rootfs.ext4` 生效
- `air session export-workspace` 能导出当前 merged workspace
- `Stop()` 能正常清理
- 运行目录产物完整

执行方式：

```bash
AIR_FIRECRACKER_INTEGRATION=1 \
AIR_VM_RUNTIME=firecracker \
AIR_FIRECRACKER_BIN=firecracker \
AIR_FIRECRACKER_KERNEL=/path/to/vmlinux \
AIR_FIRECRACKER_ROOTFS=/path/to/rootfs.ext4 \
AIR_KVM_DEVICE=/dev/kvm \
go test ./internal/vm -run TestFirecrackerIntegrationLifecycle -v
```

测试会在以下情况下直接跳过：

- 不是 Linux
- 未设置 `AIR_FIRECRACKER_INTEGRATION=1`
- 未提供 kernel 或 rootfs

## 8. 常见故障

### 8.1 `firecracker binary not found`

说明：

- Firecracker 二进制不存在，或者不在 `PATH` 中

处理：

- 安装 Firecracker
- 或显式设置 `AIR_FIRECRACKER_BIN`

### 8.2 `AIR_FIRECRACKER_KERNEL is required`

说明：

- 没有设置 kernel 镜像路径

处理：

- 设置 `AIR_FIRECRACKER_KERNEL=/path/to/vmlinux`

### 8.3 `firecracker kernel image not found`

说明：

- 设置了路径，但文件不存在

处理：

- 检查 kernel 文件路径

### 8.4 `AIR_FIRECRACKER_ROOTFS is required`

说明：

- 没有设置 rootfs 镜像路径

处理：

- 设置 `AIR_FIRECRACKER_ROOTFS=/path/to/rootfs.ext4`

### 8.5 `kvm device is unavailable for firecracker runtime`

说明：

- `/dev/kvm` 不存在，或当前用户没有权限

处理：

- 确认宿主机启用了 KVM
- 确认当前用户具备 `/dev/kvm` 访问权限
- 如有需要，调整 `AIR_KVM_DEVICE`

### 8.6 `guest agent is not ready`

说明：

- 这是当前版本的预期行为之一
- 表示 Firecracker VM 已进入启动链路，但 guest 内命令执行通道尚未完成

处理：

- 当前阶段仅验证 `create` / `delete` 生命周期
- 不要将 `firecracker` provider 用于真实 `exec`

## 9. 状态文件

Session 元数据保存在：

```text
data/sessions.json
```

其中包含：

- session ID
- VM ID
- 状态
- 创建时间
- 最后使用时间

如果只是想清空本地实验状态，可以删除：

```text
data/sessions.json
runtime/sessions/
```

删除前需要先确认没有还在使用的 session。

## 10. 推荐使用顺序

当前建议按下面顺序使用仓库：

1. 先用 `local` provider 验证 `create -> exec -> delete`
2. 运行 `scripts/fetch-firecracker-demo-assets.sh`
3. 运行 `scripts/prepare-firecracker-rootfs.sh`
4. 切到 `firecracker` provider 验证 `create -> exec -> delete`
5. 用 `console.log`、`config/*.json` 和集成测试确认 Firecracker 启动链路
