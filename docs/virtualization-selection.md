# AIR 虚拟化技术选型

## 1. 选型目标

AIR 的核心场景不是“运行普通业务应用”，而是“为 AI Agent 执行不可信代码提供默认安全的边界”。因此虚拟化方案必须优先满足以下要求：

- 强隔离，适合多租户或不可信代码执行
- 启动快，适合短任务与高频调度
- 设备模型简洁，攻击面小
- 支持程序化控制，便于 Go 控制面接入
- 支持 Guest/Host 通信
- 后续支持 snapshot / restore

## 2. 候选方案

本次重点比较三类开源方案：

- Firecracker
- Cloud Hypervisor
- QEMU/KVM

## 3. 方案对比

### 3.1 Firecracker

特点：

- 面向 microVM，专注容器和函数类工作负载
- 基于 KVM
- 极简设备模型，内存开销低，启动快
- 提供 REST API
- 支持 `virtio-net`、`virtio-block`、`virtio-vsock`
- 支持 snapshot
- 提供 `jailer`，适合生产隔离

优点：

- 最贴合 AIR 的“短任务 + 强隔离 + 快启动”目标
- 设备少，攻击面更小
- `vsock` 很适合 Host/Guest 命令通道
- API 清晰，适合控制面编排
- 社区成熟，已有 Go SDK

缺点：

- 功能有意收敛，不适合复杂通用虚拟机场景
- 生态灵活性不如 QEMU
- 对 rootfs、kernel、启动链路要求更明确

### 3.2 Cloud Hypervisor

特点：

- Rust 实现，面向现代云工作负载
- 也强调最小设备模型
- 支持 HTTP API
- 支持 Linux 与 Windows guest
- 支持热插拔、迁移等更丰富能力

优点：

- 现代化，API 友好
- 比 QEMU 更精简
- 平台能力更丰富

缺点：

- 对 AIR 当前阶段来说能力偏重
- 对“极简 microVM 沙箱”这个目标，不如 Firecracker 聚焦
- 社区认知度和“AI 沙箱”场景绑定度目前弱于 Firecracker

### 3.3 QEMU/KVM

特点：

- 通用虚拟化事实标准
- 功能非常完整
- 支持 QMP、virtio、vsock、snapshot 等能力

优点：

- 最成熟，功能覆盖最广
- 调试工具多
- 适合作为兼容兜底方案

缺点：

- 功能太多，复杂度和攻击面都更大
- 对 AIR 的目标来说过重
- 启动开销和工程控制复杂度通常高于 Firecracker

## 4. 结论

### 4.1 最终选择

AIR 当前阶段推荐使用：

```text
Firecracker + KVM + vsock + raw rootfs + Guest Agent
```

### 4.2 选择理由

Firecracker 是当前最符合 AIR 目标的开源虚拟化方案，原因如下：

1. 它本身就是为安全、高密度、快速启动的 microVM 设计的。
2. 设备模型极简，更适合做默认安全边界。
3. 它原生支持 `virtio-vsock`，非常适合实现 Host/Guest 控制通道。
4. 它有明确的程序化 API，适合 AIR 的 Go 控制面接入。
5. 它支持 snapshot，满足后续性能路线。
6. 它有 `jailer`，可以作为生产环境的第二层防护。

## 5. AIR 的推荐技术栈

### 5.1 虚拟化层

- VMM：Firecracker
- 硬件虚拟化：KVM
- 开发态控制：Firecracker REST API
- 生产态隔离：Firecracker `jailer`

### 5.2 Guest 通信

- 第一优先：`virtio-vsock`
- 调试兜底：串口控制台
- 不推荐作为长期方案：共享文件轮询

### 5.3 磁盘与镜像

- kernel：固定 Linux kernel
- rootfs：只读基础镜像
- session 写层：每 session 独立 writable 层
- 后续：snapshot / restore

## 6. 分阶段落地建议

### 阶段 A：Firecracker 接入

- 能通过 Go 控制 Firecracker 启动一个 microVM
- 能挂载 kernel 和 rootfs
- 能获取串口日志

### 阶段 B：Guest Agent 通信

- 在 guest 内启动 `air-agent`
- 通过 `vsock` 接收执行请求
- 返回 stdout、stderr、exit_code

### 阶段 C：Session 化

- 每个 session 对应一个 Firecracker microVM
- session 生命周期由控制面管理
- 支持多次 `exec`

### 阶段 D：性能优化

- rootfs 写层优化
- snapshot / restore
- 预热池

## 7. 不选其他方案作为主方案的原因

### 不选 QEMU 作为主方案

不是因为 QEMU 不行，而是因为 AIR 当前目标非常明确：优先做一个“面向 AI 不可信代码执行”的轻量 runtime，而不是覆盖所有虚拟化需求。QEMU 更适合作为调试兜底和兼容路径，而不是默认主路径。

### 不选 Cloud Hypervisor 作为主方案

Cloud Hypervisor 很强，也很现代，但对 AIR 当前阶段来说，它更像是“云 VM 平台”的基础设施，而 Firecracker 更像是“microVM 沙箱”的基础设施。AIR 当前更需要后者。

## 8. 选型结论摘要

```text
主方案：Firecracker
控制方式：REST API / Go SDK
通信方式：virtio-vsock
生产隔离：jailer
兜底方案：QEMU/KVM
```

## 9. 参考来源

以下结论基于官方文档与项目主页整理，检索时间为 2026-04-13：

- Firecracker 官方站点：https://firecracker-microvm.github.io/
- Firecracker GitHub：https://github.com/firecracker-microvm/firecracker
- Firecracker Go SDK：https://github.com/firecracker-microvm/firecracker-go-sdk
- Cloud Hypervisor 官方站点：https://www.cloudhypervisor.org/
- QEMU 官方文档：https://www.qemu.org/docs/master/
