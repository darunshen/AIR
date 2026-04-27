# Firecracker 真机部署指南

[English](firecracker-deployment-guide.en.md)

本文档只描述 AIR 在真机上使用 Firecracker 所需的宿主机准备、资产准备和基础验证。

边界说明：

- 命令使用、session 生命周期、日志查看，见 [操作手册](operations-manual.md)
- 本文不重复描述 AI Agent 工作流

## 1. 适用范围

本文适用于：

- Linux 宿主机
- CPU 与内核支持 KVM
- 需要本地或 self-hosted CI 运行 Firecracker provider

本文不适用于：

- macOS / Windows 直接运行 Firecracker
- GitHub-hosted runner 直接跑 KVM
- 生产级多租户加固细节

GitHub Actions 说明：

- `local` provider 可在 GitHub-hosted runner 使用
- `firecracker` provider 需要 self-hosted Linux runner
- 该 runner 需要可读写 `/dev/kvm`

## 2. 宿主机前提

至少需要：

- Linux
- `x86_64` 或 `aarch64`
- `/dev/kvm`
- 当前用户对 `/dev/kvm` 有读写权限
- Firecracker 二进制
- `vmlinux`
- `rootfs.ext4`

AIR 提供两个统一入口：

```bash
air init firecracker
air doctor --provider firecracker --human
```

说明：

- `air init firecracker` 会交互提示用户选择“下载 AIR 官方镜像包”还是“自己部署”
- 如果选择 AIR 官方镜像，命令会下载与当前 AIR 版本匹配的官方 bundle
- `air doctor` 用于检查当前环境是否已经完整

## 3. 检查 KVM

### 3.1 检查模块

```bash
lsmod | grep kvm
```

### 3.2 检查设备

```bash
ls -l /dev/kvm
```

### 3.3 检查权限

```bash
[ -r /dev/kvm ] && [ -w /dev/kvm ] && echo "OK" || echo "FAIL"
```

如果权限不足，常见做法有两种。

方式一：加入 `kvm` 组

```bash
sudo usermod -aG kvm "${USER}"
```

方式二：临时 ACL 授权

```bash
sudo setfacl -m u:${USER}:rw /dev/kvm
```

## 4. Firecracker 资产策略

当前推荐策略非常明确：

1. Firecracker 二进制先用官方 release
2. `vmlinux` / `rootfs.ext4` 先用官方 demo / CI 资产跑通
3. 再用 AIR 仓库脚本生成适配当前运行链路的 rootfs

这对应两类脚本：

```bash
scripts/fetch-firecracker-demo-assets.sh
scripts/prepare-firecracker-rootfs.sh
```

如果要跑 OpenClaude guest，还需要：

```bash
scripts/prepare-openclaude-alpine-rootfs.sh
```

## 5. 获取官方资产

### 5.1 Firecracker 二进制

可直接使用官方 release，或用仓库脚本统一下载：

```bash
scripts/fetch-firecracker-demo-assets.sh
```

默认会把文件放到：

```text
assets/firecracker/
```

包括：

- `firecracker`
- `hello-vmlinux.bin`
- `hello-rootfs.ext4`

## 6. 准备 AIR 可用的 rootfs

### 6.1 基础 guest

把 `air-agent` 注入官方 demo rootfs：

```bash
scripts/prepare-firecracker-rootfs.sh
```

产物通常包括：

- `assets/firecracker/hello-rootfs-air.ext4`

### 6.2 OpenClaude guest

如果要在 guest 内运行 OpenClaude：

```bash
scripts/prepare-openclaude-alpine-rootfs.sh
```

该脚本会准备一份适合 OpenClaude + Bun + `air-agent` 的 guest rootfs。

## 7. AIR 资产发现方式

AIR 会优先从环境变量读取：

```bash
export AIR_VM_RUNTIME=firecracker
export AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker
export AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux
export AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4
export AIR_KVM_DEVICE=/dev/kvm
```

如果不显式配置，AIR 也会尝试从这些目录自动发现资产：

- `assets/firecracker/`
- `/usr/lib/air/firecracker/`
- `/usr/local/lib/air/firecracker/`
- `~/.local/share/air/firecracker/`

## 8. 基础验证

### 8.1 预检

```bash
air init firecracker
air doctor --provider firecracker --human
```

### 8.2 生命周期验证

```bash
air session create --provider firecracker
air session inspect <session_id>
air session exec <session_id> "uname -a"
air session console <session_id> --follow
air session delete <session_id>
```

### 8.3 工作区验证

```bash
air session create --provider firecracker --workspace /absolute/path/to/repo
air session export-workspace <session_id> /tmp/air-export
```

这里验证的是：

- Firecracker 能启动
- guest `air-agent` 能就绪
- `vsock exec` 可用
- `/workspace` overlay 可挂载
- merged workspace 可导出

## 9. 集成测试

仓库内置了一条真机生命周期测试：

```bash
AIR_FIRECRACKER_INTEGRATION=1 \
AIR_VM_RUNTIME=firecracker \
AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker \
AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux \
AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4 \
AIR_KVM_DEVICE=/dev/kvm \
go test ./internal/vm -run TestFirecrackerIntegrationLifecycle -v
```

## 10. 常见问题

### 10.1 `firecracker binary not found`

处理：

- 检查 `PATH`
- 或显式设置 `AIR_FIRECRACKER_BIN`

### 10.2 `AIR_FIRECRACKER_KERNEL is required`

处理：

- 配置 `AIR_FIRECRACKER_KERNEL`

### 10.3 `AIR_FIRECRACKER_ROOTFS is required`

处理：

- 配置 `AIR_FIRECRACKER_ROOTFS`

### 10.4 `kvm device is unavailable for firecracker runtime`

处理：

- 回到第 3 节检查 `/dev/kvm`

### 10.5 `guest agent is not ready`

处理：

- 看 session 目录下的 `console.log`
- 看 `events.jsonl`
- 确认 rootfs 经过了仓库脚本准备，而不是直接拿原始 demo rootfs

## 11. 当前建议

建议按这个顺序推进：

1. 官方 release `firecracker`
2. 官方 demo `vmlinux` / `rootfs.ext4`
3. `prepare-firecracker-rootfs.sh`
4. `air doctor`
5. `air session create/exec/delete`
6. `--workspace` / `export-workspace`
7. OpenClaude guest 与真实 agent 工作流
