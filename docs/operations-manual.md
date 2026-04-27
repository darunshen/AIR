# AIR 操作手册

[English](operations-manual.en.md)

本文档只描述当前版本 AIR 的实际使用、排障和运行产物。

边界说明：

- 宿主机安装、KVM 检查、Firecracker 资产准备，见 [Firecracker 真机部署指南](firecracker-deployment-guide.md)
- 产品定位、能力边界、路线图，见根目录 `README.md`、`ROADMAP.md`、`TODO.md`
- 本文不重复展开部署指南中的环境准备步骤

## 1. 当前可用能力

当前 CLI 入口：

- `air version`
- `air run`
- `air session create`
- `air session list`
- `air session inspect`
- `air session console`
- `air session events`
- `air session exec`
- `air session export-workspace`
- `air session delete`
- `air init firecracker`
- `air doctor --provider firecracker`

当前 runtime provider：

- `local`
- `firecracker`

当前 Firecracker 路径已经打通：

- 宿主机预检
- Firecracker microVM 启动
- guest `air-agent` 启动链
- Host/Guest `vsock` exec
- 每 session 独立 `rootfs.ext4`
- 可选只读 `workspace.ext4` + 可写 `workspace-upper.ext4`
- `air session export-workspace` 导出 guest 当前 merged `/workspace`

## 2. 快速开始

### 2.1 本地模式

```bash
air run -- echo hello
air session create --provider local
air session exec <session_id> "echo hello > a.txt"
air session exec <session_id> "cat a.txt"
air session delete <session_id>
```

### 2.2 Firecracker 模式

先完成宿主机准备，再执行：

```bash
air init firecracker
air doctor --provider firecracker --human
air session create --provider firecracker
air session exec <session_id> "uname -a"
air session inspect <session_id>
air session console <session_id> --follow
air session events <session_id> --follow
air session delete <session_id>
```

## 3. `air run`

一次性执行示例：

```bash
air run -- echo hello
air run --timeout 5s -- sh -c 'echo hello && exit 3'
air run --memory-mib 512 --vcpu-count 2 -- echo hello
air run --provider firecracker -- echo hello
```

默认输出为结构化 JSON，重点字段包括：

- `provider`
- `session_id`
- `request_id`
- `stdout`
- `stderr`
- `exit_code`
- `duration_ms`
- `timeout`
- `error_type`
- `error_message`

当前稳定的 `error_type` 主要包括：

- `invalid_argument`
- `startup_error`
- `transport_error`
- `exec_error`
- `timeout`
- `cleanup_error`

如果需要更适合人工查看的输出：

```bash
air run --human -- echo hello
```

## 4. Session 生命周期

### 4.1 创建

```bash
air session create
air session create --provider local
air session create --provider firecracker
air session create --provider firecracker --workspace /absolute/path/to/repo
```

### 4.2 列表与详情

```bash
air session list
air session inspect <session_id>
```

说明：

- `list` / `inspect` 会结合当前 runtime 实况刷新状态
- 它们不只是读取 `data/sessions.json` 的旧值

### 4.3 执行命令

```bash
air session exec <session_id> "pwd"
air session exec <session_id> "ls -la"
air session exec <session_id> "go test ./..."
```

### 4.4 查看日志

```bash
air session console <session_id>
air session console <session_id> --follow
air session console <session_id> --tail=100
air session events <session_id>
air session events <session_id> --follow
```

说明：

- `console` 是串口日志查看，不是交互式 shell
- `events` 是结构化事件流，适合看生命周期、exec、错误语义

### 4.5 导出工作区

如果 session 启动时附带了 `--workspace`，可以导出当前 guest 里的 merged `/workspace` 视图：

```bash
air session export-workspace <session_id> /tmp/air-export
air session export-workspace <session_id> /tmp/air-export --force
```

说明：

- 导出的不是原始只读 `workspace.ext4`
- 导出的是当前 merged overlay 结果
- 默认要求输出目录不存在或为空
- `--force` 会清空已有输出目录后再导出

### 4.6 删除

```bash
air session delete <session_id>
```

## 5. 运行目录与日志

### 5.1 Local provider

```text
runtime/sessions/local/<session_id>/
  workspace/
  task/
```

### 5.2 Firecracker provider

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
```

重点文件：

- `rootfs.ext4`：每 session 私有根盘
- `workspace.ext4`：只读工作区镜像
- `workspace-upper.ext4`：可写 overlay 上层
- `console.log`：guest 串口日志
- `events.jsonl`：结构化事件日志
- `config/*.json`：AIR 实际下发给 Firecracker 的配置快照

## 6. 常见排障路径

### 6.1 `firecracker binary not found`

说明：

- Firecracker 不在 `PATH`
- 或 `AIR_FIRECRACKER_BIN` 指向错误

处理：

- 先跑 `air doctor --provider firecracker --human`
- 检查二进制位置与执行权限

### 6.2 `AIR_FIRECRACKER_KERNEL is required`

说明：

- 没有配置 kernel 资产

处理：

- 按部署指南准备 `vmlinux`

### 6.3 `AIR_FIRECRACKER_ROOTFS is required`

说明：

- 没有配置 rootfs 资产

处理：

- 按部署指南准备 `rootfs.ext4`

### 6.4 `kvm device is unavailable for firecracker runtime`

说明：

- `/dev/kvm` 不存在
- 或当前用户没有权限

处理：

- 回到部署指南检查 KVM 与权限

### 6.5 `guest agent is not ready`

说明：

- guest 启动了，但 `air-agent` 没有在预期时间内就绪

排查顺序：

1. 看 `console.log`
2. 看 `events.jsonl`
3. 看 `config/*.json` 是否指向了正确 rootfs / kernel / boot args
4. 确认 rootfs 是否已通过仓库脚本注入 `air-agent`

## 7. 推荐验证顺序

建议按下面顺序验证一台机器：

1. `air run -- echo hello`
2. `air session create --provider local`
3. `air session exec` / `delete`
4. `air init firecracker`
5. `air doctor --provider firecracker --human`
6. `air session create --provider firecracker`
7. `air session exec`
8. `air session console --follow`
9. `air session export-workspace`

## 8. AI Agent 相关入口

如果要从 AI Agent 侧使用 AIR，继续看：

- [AI Agent 使用说明](ai-agent-usage.md)
- [OpenClaude 接入方案](openclaude-integration.md)
- [LLM 验收结果](llm-acceptance-results.md)
