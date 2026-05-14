# AIR

[English](README.md)

AIR 是给 coding agent 用的 VM 隔离运行时。最常用入口是：

```bash
air chat
```

`air chat` 会启动交互式 OpenClaude 会话，并在首次使用时按需准备
Firecracker 资产、OpenClaude guest 镜像、host runtime 和模型配置。

## 快速开始

启动聊天：

```bash
air chat
```

常用 Firecracker PTY 模式：

```bash
sudo -E env PATH="$PATH" HOME="$HOME" air chat \
  --provider firecracker \
  --network full \
  --pty \
  --auto-approve \
  --workspace /path/to/repo
```

如果 workspace 很大，并且需要频繁重启，可以复用 workspace 快照：

```bash
sudo -E env PATH="$PATH" HOME="$HOME" air chat \
  --provider firecracker \
  --network full \
  --pty \
  --auto-approve \
  --workspace /path/to/repo \
  --workspace-cache
```

## 常用命令

在隔离运行时里执行一次命令：

```bash
air run -- echo hello
air run --provider firecracker --workspace /path/to/repo -- make test
```

创建并使用有状态 session：

```bash
air session create --provider firecracker --workspace /path/to/repo
air session exec <session-id> "go test ./..."
air session events <session-id> --tail=100
air session console <session-id> --tail=100
air session export-workspace <session-id> ./out --force
air session delete <session-id>
```

检查或准备 Firecracker：

```bash
air doctor --provider firecracker --human
air init firecracker
```

清理残留 session：

```bash
air gc --all
```

## 安装

使用最新 GitHub Release：

```text
https://github.com/darunshen/AIR/releases/latest
```

安装 release 版本后，如果缺少 Firecracker 或 OpenClaude 官方 bundle，
`air chat` 会按当前 AIR 版本自动下载匹配资源。

## 从源码构建

```bash
go build -o /tmp/air ./cmd/air
/tmp/air version
```

## 注意事项

- `air chat` 是推荐入口。
- Firecracker 模式需要访问 `/dev/kvm`，并且创建 TAP/network 资源通常需要
  `sudo`，除非当前用户已经具备对应权限。
- `--workspace-cache` 会复用 workspace 快照来加速重复启动；如果宿主
  workspace 内容变了，需要刷新缓存后 guest 才能看到新内容。
- PTY 模式会透传 OpenClaude 输出，包括终端 UI 和标题更新。

## 文档

- [文档索引](docs/README.md) / [English](docs/README.en.md)
- [OpenClaude 接入方案](docs/openclaude-integration.md) / [English](docs/openclaude-integration.en.md)
- [Firecracker 部署指南](docs/firecracker-deployment-guide.md) / [English](docs/firecracker-deployment-guide.en.md)
- [发布与安装包交付](docs/release-distribution.md) / [English](docs/release-distribution.en.md)
