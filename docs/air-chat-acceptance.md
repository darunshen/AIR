# `air chat` 真机验收记录

[English](air-chat-acceptance.en.md)

本文档记录一次真实 Linux/KVM 机器上的 `.deb -> air chat -> 自动下载 -> 进入对话` 验收过程。

这不是长期 SLA 承诺，而是一次带问题发现和修复闭环的工程验收快照。解读时应同时参考：

- 当前 commit
- 当前 GitHub Release 产物
- 当前外网和 GitHub 下载状态

## 1. 验收目标

目标不是只验证某个子命令能运行，而是验证终端用户安装 `.deb` 后，能否通过一条命令进入可用的 AI 对话入口：

1. 安装 `air` `.deb`
2. 执行 `air chat`
3. 自动提示并下载缺失的 Firecracker 资产
4. 自动提示并下载缺失的 OpenClaude guest rootfs
5. 自动准备 OpenClaude host runtime
6. 完成模型配置录入
7. 进入 `AIR OpenClaude Chat` 提示符

## 2. 验收环境

- 日期：2026-05-08
- 宿主架构：`x86_64`
- `/dev/kvm`：可读可写
- 包类型：`air_0.1.1-dev3_amd64.deb`
- provider：`firecracker`
- 模拟用户环境：全新临时 `HOME`
- 模拟官方源：本地 HTTP 镜像

说明：

- 首次尝试直接访问 GitHub Release 时，Firecracker bundle 下载遇到过一次 `TLS handshake timeout`
- 为了继续验证首次交互状态机和自动下载逻辑，后续使用了本地 HTTP 镜像承载同版本 release 产物
- 这次验收验证的是产品逻辑闭环，不是 GitHub 外网稳定性

## 3. 最终结果

最终已成功进入：

```text
AIR OpenClaude Chat
provider=firecracker session=sess_748ef6b0f627bd88 workdir=/workspace
Connected to 127.0.0.1:50052. Type /exit to quit.
air:openclaude@sess_748ef6b0f627bd88>
```

并成功生成 transcript：

```text
/tmp/air-chat-home5/workspace/runtime/sessions/firecracker/sess_748ef6b0f627bd88/openclaude-chat-transcript.jsonl
```

## 4. 验收过程中暴露并修复的问题

### 4.1 `.deb` 产物过旧，不包含 `air chat`

现象：

- 早期 `dist/air_0.1.1_amd64.deb` 解包后仍是旧 CLI
- 直接执行只显示旧版 usage，没有 `air chat`

处理：

- 重新基于当前源码构建 release 产物
- 确认新包内 `air version` 与当前 commit 对齐

### 4.2 OpenClaude host bundle 解包失败

现象：

- 官方 OpenClaude host bundle 初次解包失败
- 错误类型先后暴露为 tar entry type `49`、`50`

根因：

- bundle 中包含 hard link 与 symlink
- `extractTarGz` 初始只支持目录与普通文件

处理：

- 为 `extractTarGz` 增加 `tar.TypeLink` 支持
- 为 `extractTarGz` 增加 `tar.TypeSymlink` 支持
- 增加对应测试覆盖

### 4.3 host bundle 失败后的源码回退路径不完整

现象：

- host bundle 失败后，`bun install` 在半成品目录里执行
- 会出现“没有 `package.json`”之类的错误

处理：

- 在回退到 `git clone + bun install` 前，先清理自动管理目录中的半成品内容

### 4.4 `air chat` 交互下载完成后仍使用旧 Firecracker 配置

现象：

- `air chat` 中 Firecracker bundle 已下载成功
- `air doctor` 也显示 ready
- 但真正进入 session 创建时仍报：
  - `firecracker binary not found: firecracker`

根因：

- `main()` 在程序启动早期就创建了 `session.Manager`
- `air chat` 后续虽然完成了交互式下载，但真正执行 session 时仍持有旧 `vm.Config`

处理：

- 在 `air chat` 完成交互准备后，重新创建 `session.Manager`

### 4.5 官方 OpenClaude host bundle 能解包，但仍被误判为无效 repo

现象：

- bundle 已成功下载并落盘
- 但后续仍报：
  - `stat .../scripts/start-grpc.ts: no such file or directory`

根因：

- 官方 host bundle 发布结构是：
  - `<install_dir>/openclaude/...`
  - `<install_dir>/bin/bun`
- 初始校验逻辑误把 `<install_dir>` 当成源码 repo 根目录

处理：

- 官方 host bundle 安装完成后，统一通过 `resolveOpenClaudeRepo()` 解析
- 不再直接用源码目录结构去校验 bundle 根目录

## 5. 与本次验收直接相关的提交

- `ef1ac0c` `Add OpenClaude guest bundle setup to air chat`
- `fc293d3` `Verify OpenClaude guest release artifacts`
- `c4f1365` `Fix air chat first-run bundle setup`
- `c6fef95` `Isolate runtime session state`

## 6. 当前结论

基于 2026-05-08 这次真机验收，当前可以确认：

- `.deb` 安装后的 `air chat` 已具备首次交互入口形态
- `firecracker` 模式下，已能按顺序引导下载关键运行资产
- OpenClaude guest rootfs 自动准备链路已可用
- OpenClaude host runtime 自动准备链路已可用
- session 启动后已可进入真实 `AIR OpenClaude Chat` 提示符

当前仍建议继续做一轮更贴近正式发布环境的复验：

1. 使用正式 GitHub Release，而不是本地 HTTP 镜像
2. 使用真实可用的模型 key
3. 从进入提示符推进到真正的一次对话请求成功返回
