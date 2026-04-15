# Firecracker 真机部署指南

本文档用于准备 AIR 的 Firecracker 实验环境。

目标不是生产级宿主机加固，而是让当前仓库中的 `firecracker` provider 能完成：

- 启动前环境预检
- `session create`
- `session exec`
- `session delete`
- `TestFirecrackerIntegrationLifecycle`

## 1. 适用范围

本指南适用于：

- Linux 宿主机
- CPU 支持硬件虚拟化
- 内核已启用 KVM
- 需要本地验证 Firecracker 生命周期

不适用于：

- macOS 或 Windows 直接运行 Firecracker
- 生产环境多租户隔离基线
- 已经要求 `jailer`、只读镜像、overlay、snapshot 的场景

## 2. 宿主机前提

根据 Firecracker 官方 Getting Started 文档，Firecracker 需要 Linux 宿主机和可读写的 `/dev/kvm`。官方文档也明确将 `jailer` 视为生产建议，但示例允许先不使用 `jailer` 做基础验证。

建议先确认以下条件：

- 宿主机为 Linux
- `uname -m` 为 `x86_64` 或 `aarch64`
- 当前内核已加载 KVM 模块
- 当前用户可以读写 `/dev/kvm`

## 3. 检查 KVM

### 3.1 确认模块已加载

```bash
lsmod | grep kvm
```

常见输出示例：

```text
kvm_intel             ...
kvm                   ...
```

如果没有输出，说明 KVM 模块尚未加载，或宿主机/虚拟化环境不支持。

### 3.2 确认 `/dev/kvm` 存在

```bash
ls -l /dev/kvm
```

### 3.3 确认当前用户有权限

```bash
[ -r /dev/kvm ] && [ -w /dev/kvm ] && echo "OK" || echo "FAIL"
```

如果输出 `FAIL`，可以用两种常见方式授权。

方式一：把当前用户加入 `kvm` 组

```bash
getent group kvm
groups
sudo usermod -aG kvm "${USER}"
```

执行后通常需要重新登录，再次执行权限检查。

方式二：使用 ACL 临时授权

```bash
sudo setfacl -m u:${USER}:rw /dev/kvm
```

如果宿主机没有 `setfacl`，需要先安装对应发行版的 ACL 工具包。

## 4. 获取 Firecracker 二进制

有两种常见方式。

### 4.1 直接使用官方 release

优先建议从 Firecracker 官方 release 获取二进制，并放到固定目录，例如：

```text
~/bin/firecracker
```

如果二进制已经在 `PATH` 中，AIR 可直接通过默认配置找到它；否则显式设置：

```bash
export AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker
```

如果你想直接按 AIR 当前推荐路径准备整套实验资产，可以直接运行仓库脚本：

```bash
scripts/fetch-firecracker-demo-assets.sh
```

默认会把以下文件放到：

```text
assets/firecracker/
```

包括：

- 官方 release 的 `firecracker`
- 官方 demo `hello-vmlinux.bin`
- 官方 demo `hello-rootfs.ext4`

### 4.2 从源码构建

如果你需要自行构建，可按 Firecracker 官方流程使用其开发工具链构建。该路径通常要求：

- `git`
- `bash`
- `docker`

构建完成后，把生成的 `firecracker` 二进制复制到固定路径，再通过 `AIR_FIRECRACKER_BIN` 指向它。

## 5. 准备 kernel 和 rootfs

AIR 当前的 Firecracker provider 依赖两个宿主机文件：

- 一个未压缩的 Linux kernel 映像
- 一个 ext4 格式 rootfs 映像

环境变量分别为：

```bash
export AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux
export AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4
```

### 5.0 当前推荐做法

当前 AIR 建议：

1. Firecracker 二进制先使用官方 release
2. `vmlinux` / `rootfs.ext4` 先使用 Firecracker 官方 Getting Started 中引用的 demo 资产
3. 再用仓库脚本把 `air-agent` 注入到 demo rootfs 中

仓库脚本已经把这条路径固化好了：

```bash
scripts/fetch-firecracker-demo-assets.sh
scripts/prepare-firecracker-rootfs.sh
```

脚本流程基于 Firecracker 官方 Getting Started，但省略了 SSH key 注入，因为 AIR 当前并不通过 SSH 进入 guest。

### 5.1 最低要求

kernel 需要满足：

- 宿主机架构匹配
- 可被 Firecracker 直接引导

rootfs 需要满足：

- 是合法 ext4 文件系统映像
- 至少能完成 guest 启动

### 5.2 当前阶段的建议

当前仓库已经内建 demo rootfs 重打包流程。建议先用仓库脚本生成 `hello-rootfs-air.ext4`，优先验证：

- `session create`
- `session exec`
- `session delete`

### 5.3 目录约定

建议统一放到宿主机固定目录，例如：

```text
~/air-assets/firecracker/
  firecracker
  vmlinux
  rootfs.ext4
```

然后导出环境变量：

```bash
export AIR_FIRECRACKER_BIN=~/air-assets/firecracker/firecracker
export AIR_FIRECRACKER_KERNEL=~/air-assets/firecracker/vmlinux
export AIR_FIRECRACKER_ROOTFS=~/air-assets/firecracker/rootfs.ext4
export AIR_KVM_DEVICE=/dev/kvm
```

如果你的 shell 不会自动展开 `~`，请改用绝对路径。

如果使用仓库脚本，建议直接采用脚本默认目录，不要再手工移动文件：

```bash
export AIR_FIRECRACKER_BIN="$(pwd)/assets/firecracker/firecracker"
export AIR_FIRECRACKER_KERNEL="$(pwd)/assets/firecracker/hello-vmlinux.bin"
export AIR_FIRECRACKER_ROOTFS="$(pwd)/assets/firecracker/hello-rootfs-air.ext4"
export AIR_KVM_DEVICE=/dev/kvm
```

## 6. 验证文件是否可用

### 6.1 检查二进制

```bash
"${AIR_FIRECRACKER_BIN}" --help >/dev/null
echo $?
```

返回 `0` 一般表示可执行。

### 6.2 检查 kernel 文件

```bash
test -f "${AIR_FIRECRACKER_KERNEL}" && echo "kernel OK" || echo "kernel FAIL"
```

### 6.3 检查 rootfs 文件

```bash
test -f "${AIR_FIRECRACKER_ROOTFS}" && echo "rootfs OK" || echo "rootfs FAIL"
```

如果宿主机有 `e2fsck`，还可以做只读校验：

```bash
e2fsck -fn "${AIR_FIRECRACKER_ROOTFS}"
```

## 7. 为 AIR 设置环境变量

推荐在当前 shell 会话中导出：

```bash
export AIR_VM_RUNTIME=firecracker
export AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker
export AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux
export AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4
export AIR_KVM_DEVICE=/dev/kvm
```

确认配置：

```bash
printf '%s\n' \
  "AIR_VM_RUNTIME=${AIR_VM_RUNTIME}" \
  "AIR_FIRECRACKER_BIN=${AIR_FIRECRACKER_BIN}" \
  "AIR_FIRECRACKER_KERNEL=${AIR_FIRECRACKER_KERNEL}" \
  "AIR_FIRECRACKER_ROOTFS=${AIR_FIRECRACKER_ROOTFS}" \
  "AIR_KVM_DEVICE=${AIR_KVM_DEVICE}"
```

## 8. 用 AIR 验证生命周期

### 8.1 创建 session

```bash
go run ./cmd/air session create
```

如果成功，会输出一个 session ID。

### 8.2 查看运行目录

创建后检查：

```text
runtime/sessions/firecracker/<session_id>/
```

关键文件包括：

- `firecracker.sock`
- `firecracker.pid`
- `firecracker.vsock`
- `console.log`
- `metrics.log`
- `config/*.json`

### 8.3 删除 session

```bash
go run ./cmd/air session delete <session_id>
```

删除后，对应目录应被清理。

## 9. 运行集成测试

AIR 当前内置了一条真机生命周期测试：

```bash
AIR_FIRECRACKER_INTEGRATION=1 \
AIR_VM_RUNTIME=firecracker \
AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker \
AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux \
AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4 \
AIR_KVM_DEVICE=/dev/kvm \
go test ./internal/vm -run TestFirecrackerIntegrationLifecycle -v
```

该测试目前验证：

- Firecracker 可以启动
- API socket 和 pid 文件存在
- `console.log` 和 `metrics.log` 已创建
- `Stop()` 能清理目录

## 10. 读日志和产物

### 10.1 看 console 日志

```bash
cat runtime/sessions/firecracker/<session_id>/console.log
```

如果 guest 成功进入 early boot，这里通常会有内核启动输出。

### 10.2 看配置快照

```bash
ls runtime/sessions/firecracker/<session_id>/config
cat runtime/sessions/firecracker/<session_id>/config/boot-source.json
```

这些文件反映 AIR 实际下发给 Firecracker 的配置，可用于核对：

- kernel 路径
- rootfs 路径
- boot args
- vsock CID

## 11. 常见问题

### 11.1 `firecracker binary not found`

原因：

- 二进制路径错误
- 没有执行权限
- 没有加入 `PATH`

处理：

- 用绝对路径设置 `AIR_FIRECRACKER_BIN`
- `chmod +x` 二进制

### 11.2 `AIR_FIRECRACKER_KERNEL is required`

原因：

- 没有设置 kernel 环境变量

处理：

- 设置 `AIR_FIRECRACKER_KERNEL`

### 11.3 `firecracker kernel image not found`

原因：

- 路径存在拼写问题
- 文件不在宿主机上

### 11.4 `AIR_FIRECRACKER_ROOTFS is required`

原因：

- 没有设置 rootfs 环境变量

处理：

- 设置 `AIR_FIRECRACKER_ROOTFS`

### 11.5 `kvm device is unavailable for firecracker runtime`

原因：

- `/dev/kvm` 不存在
- 没有读写权限
- 当前环境本身不支持 KVM

处理：

- 重做第 3 节检查

### 11.6 `guest agent is not ready`

这是当前阶段的预期限制，不代表 Firecracker 启动链路一定失败。

如果你已经看到：

- session 创建成功
- `firecracker.pid` 存在
- `console.log` 有内容

那么宿主机部署通常已经基本成立。

## 12. 当前不建议做的事

在 AIR 现阶段，不建议把下面这些问题和 Firecracker 宿主机准备混在一起处理：

- 配置 TAP 网络
- 配置 NAT 和 iptables
- 用 SSH 登录 guest
- 在 guest 内安装软件包
- 做生产级 jailer 或 seccomp 加固

这些事情后续都会需要，但它们不是当前 AIR 仓库验证 `Start()` / `Stop()` 的必要条件。

## 13. 下一步建议

真机部署通过后，建议按下面顺序继续：

1. 固化 kernel 和 rootfs 资产目录
2. 记录一份成功启动后的 `console.log` 样例
3. 设计并实现 guest `air-agent`
4. 打通 `vsock` 协议
5. 将 `firecracker.Exec()` 从当前错误返回替换为真实通信
