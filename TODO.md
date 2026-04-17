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
  - `air session events <id>`
  - `air session events <id> --follow`
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

- Firecracker 每 session 独立可写根盘已接入
  - 启动时会从基础 rootfs 复制出 `overlay.ext4`
  - Firecracker 实际挂载的是 session 自己的 `overlay.ext4`
  - session 删除时会一起清理

- 可观测性已补到运行链路
  - 已记录 session 生命周期事件
  - 已记录 exec `request_id`
  - 已记录 exec 耗时
  - 事件日志落到 `events.jsonl`

- 一次性 agent 执行入口已接入
  - 已有 `air run`
  - 内部复用 `session create -> exec -> delete`
  - 已支持 `--provider`
  - 已支持 `--timeout`
  - 默认返回结构化 JSON
  - 已支持 `--human` 便于人工调试

- reference agent 初版已接入
  - 已有 `examples/agent-runner`
  - 已覆盖 one-shot `run-smoke`
  - 已覆盖多步 `session-workflow`
  - 已覆盖失败恢复 `session-recovery`
  - 已验证“执行 -> 读取结果 -> 决策下一步”基础模式

- agent 选型决策已收敛
  - 已新增 `docs/agent-selection.md`
  - 第一接入 provider 定为 OpenAI
  - 默认模型定为 `gpt-5.4-mini`
  - 复杂任务升级到 `gpt-5.4`
  - Anthropic / Gemini 作为后续 provider
  - 已新增 `.env.example` 作为环境模板

- OpenAI planner 第一版已接入
  - 已新增 `internal/llm`
  - 已有 `llm.Provider` 抽象
  - 已实现 OpenAI Responses API adapter
  - `examples/agent-runner` 已可通过真实 OpenAI planner 决定下一步命令
  - `scripted` planner 仍保留作离线回归 fallback

- DeepSeek planner 第一版已接入
  - 已实现 DeepSeek Chat Completions adapter
  - `examples/agent-runner` 已可通过真实 DeepSeek planner 决定下一步命令
  - 已验证 `run-smoke`
  - 已验证 `session-workflow`

- AI agent 使用说明已补齐
  - 已新增 `docs/ai-agent-usage.md`
  - 已说明 planner / runner / AIR 三层关系
  - 已说明 OpenAI / DeepSeek / scripted 使用方式
  - 已说明 one-shot / session / recovery / test-and-fix 的调用方式

- `test-and-fix` 工作流已接入
  - reference agent 已支持 `test-and-fix`
  - scripted planner 已验证通过
  - DeepSeek planner 已验证通过
  - finish 后会做一次额外测试校验

- 发布与安装包基线已接入
  - 已支持 `air version`
  - 已支持 `air-agent --version`
  - 已新增 release 打包脚本
  - 已支持 GitHub Release 归档产物
  - 已支持 `.deb` 打包
  - 已支持 apt repo 目录产物构建
  - 已新增 GitHub Actions release workflow
  - 已新增 `docs/release-distribution.md`

## Priority Rule

- 优先级判断标准调整为：先打通 AI agent 真实工作流，再补底层工程化
- 所谓“真实工作流”指：
  - 能创建隔离环境
  - 能执行命令或任务
  - 能保留或销毁状态
  - 能获取 stdout / stderr / 事件 / 失败原因
  - 能设置最基本的执行边界，如 timeout / 资源限制
- 不能直接提升 agent 可用性的事项，如真正的 COW overlay、snapshot、预热池，暂时后置

## P0: AI Agent 工作流闭环

### P0 目标定义

- 先做一个能真实使用 AIR 的 reference agent，而不是先做完整 agent 平台
- 这个 reference agent 至少要能完成：
  - 接收一个任务
  - 调用 `air run` 或 `air session ...`
  - 读取 stdout / stderr / exit code / events
  - 根据结果继续下一步
  - 在失败时给出可判定的错误原因

### P0 最小交付物

- 一套最小任务集
  - 读文件
  - 写文件
  - 运行测试
  - 根据上一步结果继续执行下一条命令

- 一套稳定的执行结果结构
  - stdout
  - stderr
  - exit_code
  - request_id
  - duration_ms
  - timeout
  - error_type
  - error_message

### P0 具体实现方法

- 继续补强 `air run`
  - 面向一次性 agent 任务
  - 当前已复用 `session create -> exec -> delete`
  - 后续重点是将其作为 reference agent 的稳定执行接口

- 补齐执行边界
  - 已先支持 timeout 配置能力
  - 增加资源限制配置能力
  - 明确超时、OOM、guest 异常退出时的返回语义
  - 实现方式：
    - 先支持 `--timeout`
    - 再支持基础资源参数，如 `--memory-mib`、`--vcpu-count`
    - Host 侧统一把超时、信号中断、guest 失联转换成稳定错误类型
    - 先定义错误枚举，再回填具体实现

- 补强 Host/Guest 协议
  - 版本兼容字段
  - 更细的 transport / guest 错误分类
  - 为 agent 侧保留稳定的错误码和失败原因
  - 实现方式：
    - 为 request/response 增加 `version`
    - response 中明确区分协议错误、transport 错误、命令执行错误
    - guest 返回退出码与 stderr，host 负责补齐 transport 维度错误
    - 错误结构固定，避免后续 agent 接口反复变更

- 为 `vm` provider 增加更直接的启动入口
  - 配置文件方案
  - “使用仓库内 demo 资产”的一键入口
  - 降低 agent 或外部编排系统接入成本
  - 实现方式：
    - 支持 `air run --provider firecracker`
    - 支持从配置文件或环境变量读取 provider 参数
    - 支持一个“demo 配置预设”，减少 agent runner 的启动负担
    - 保证 reference agent 不需要拼大量环境变量

- 增加更完整的 exec 失败细节
  - 区分 host transport 失败、guest 执行失败、超时失败
  - 明确 stderr、退出码、request_id、duration 的对外呈现
  - 实现方式：
    - 统一 `ExecResult` / `RunResult` 数据结构
    - CLI JSON 输出与内部结构保持一致
    - `events.jsonl` 中记录相同 `request_id`，便于串联排障
    - 文档中明确每类失败的判定方式

- 继续补强 `examples/agent-runner`
  - 当前已有 OpenAI planner 版 reference agent
  - 下一步重点：
    - 增加 prompt / model 升级策略
    - 用它持续反向校验 `air run`、session API、错误结构、timeout、事件日志是否足够

- 增加 OpenAI planner 接入
  - 已完成第一版
  - 下一步：
    - 增加更复杂的 task prompt
    - 增加 planner 失败重试与模型升级策略
    - 评估是否接 Anthropic / Gemini
    - 第一版仍只做单 agent planner，不做多 agent 编排

- 增加一套 agent workflow 验收用例
  - 实现方式：
    - 准备 3 到 5 个标准任务样例
    - 每个任务都要求 runner 能完成“执行 -> 读取结果 -> 决策下一步”
    - 将这些样例纳入集成测试或 `examples/` 下的验收脚本
    - 后续 P1/P2 的改动都以这些任务不退化为准

## P1: 调试与可观测性

- guest ready 判定已接入
  - `Start()` 已等待 guest `ready` 握手
  - 串口里仍可看到 `[air-agent] boot hook start`

- 记录启动失败原因到更清晰的错误链
- 增加事件等级与分类
- 将 `air session console` 从日志查看升级为更强的调试入口
  - 明确串口 attach 方案
  - 评估可交互控制台能力
  - 增加按事件类型筛选

## P1: Runtime 集成与稳定性

- 将 Firecracker 相关配置从 `Start()` 中继续拆分
  - machine config
  - boot source
  - drives
  - vsock
  - startup action

- 处理 Firecracker API 向后兼容细节
  - `vsock_id` deprecated 警告
  - 不同版本 API 路径和字段差异

## P1: 测试

- 扩大 Firecracker 集成测试覆盖
  - 校验 `console.log` 非空
  - 校验 session overlay 生效
  - 校验事件日志包含 ready / exec
  - 校验真实 `delete` 后目录清理
  - 校验 demo 资产自动发现路径
  - 增加 `air run` 场景覆盖
  - 增加 timeout / 失败语义覆盖

- 增加 guest agent 通信测试

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

## P2: Rootfs 与镜像体系

- 确定 guest rootfs 技术路线
  - 已先基于官方 demo rootfs 跑通启动与 `exec`
  - 再切到自维护 rootfs

- 将当前“复制基础 rootfs 到 `overlay.ext4`”演进为真正的 COW overlay
- 明确 overlay 方案与目录布局
- 评估 rootfs 构建时如何保留 root ownership / device 节点

## P3: Snapshot 与性能

- 调研并接入 Firecracker snapshot / restore
- 设计预热 VM 池
- 评估启动时延
- 评估 session 恢复时延

## P3: 开源与文档

- 补充 guest rootfs 构建说明
- 补充真实环境运行示例
- 补充集成测试说明
