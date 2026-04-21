# 通过 AI Agent 使用 AIR

本文档说明如何把 AIR 当作 AI agent 的隔离执行后端来使用。

目标不是把 AIR 变成模型服务，而是让外部 planner 或 reference agent 能安全地：

- 决定下一步动作
- 在隔离环境里执行命令
- 读取 stdout / stderr / exit_code / timeout / request_id
- 可选传入 `--memory-mib` / `--vcpu-count`
- 根据结果继续下一步

## 1. 使用模式

当前推荐分成三层：

```text
LLM Planner
   |
   v
Agent Runner
   |
   v
AIR
   |
   v
Isolated Runtime
```

职责分别是：

- LLM Planner
  - 负责决定下一步命令
  - 不直接执行代码

- Agent Runner
  - 维护任务状态
  - 把运行结果喂回给 planner
  - 调用 `air run` 或 `air session`

- AIR
  - 只负责隔离执行和返回结果

## 2. 当前已支持的 planner

`examples/agent-runner` 当前支持：

- `openai`
  - 通过 OpenAI Responses API 规划下一步动作

- `deepseek`
  - 通过 DeepSeek Chat Completions API 规划下一步动作

- `scripted`
  - 不走外部模型
  - 用于离线回归和最小验证

## 3. 环境准备

### 3.1 AIR 执行环境

最小本地模式只需要：

- Go 1.22+
- 当前仓库代码

直接验证：

```bash
go test ./...
go run ./cmd/air run -- echo hello
```

### 3.2 OpenAI planner

```bash
export AIR_AGENT_PROVIDER=openai
export AIR_AGENT_MODEL=gpt-5.4-mini
export AIR_AGENT_REASONING=medium
export OPENAI_API_KEY=...
```

### 3.3 DeepSeek planner

```bash
export AIR_AGENT_PROVIDER=deepseek
export AIR_AGENT_MODEL=deepseek-chat
export DEEPSEEK_API_KEY=...
```

如果你使用自定义网关，也可以配置：

```bash
export OPENAI_BASE_URL=...
export DEEPSEEK_BASE_URL=...
```

## 4. 直接运行 reference agent

### 4.1 默认任务集合

```bash
go run ./examples/agent-runner --task all
```

### 4.2 使用 OpenAI

```bash
OPENAI_API_KEY=... \
go run ./examples/agent-runner --planner openai --model gpt-5.4-mini --task all
```

### 4.3 使用 DeepSeek

```bash
DEEPSEEK_API_KEY=... \
go run ./examples/agent-runner --planner deepseek --model deepseek-chat --task all
```

### 4.4 离线 scripted fallback

```bash
go run ./examples/agent-runner --planner scripted --task all
```

### 4.5 使用统一验收脚本

```bash
scripts/run-agent-acceptance.sh --planner scripted --task all
```

如果要跑真实模型，优先用这个脚本，而不是手工拼长命令。它支持从 `OPENAI_API_KEY_FILE` / `DEEPSEEK_API_KEY_FILE` 读取 key，并把结果落到 `artifacts/agent-acceptance/`。

真实模型验收快照见 `docs/llm-acceptance-results.md`。

### 4.6 使用 gated `go test`

如果你想把真实模型验收接到本地自动化或 CI，可以直接跑：

```bash
AIR_LLM_ACCEPTANCE=1 \
AIR_AGENT_PROVIDER=deepseek \
AIR_AGENT_MODEL=deepseek-chat \
AIR_AGENT_ESCALATION_MODEL=deepseek-reasoner \
AIR_AGENT_ACCEPTANCE_TASKS=run-smoke,session-workflow,test-and-fix,repo-bugfix \
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
go test ./examples/agent-runner -run TestRealLLMAgentWorkflowAcceptance -v
```

默认情况下这个测试会跳过，不会影响常规 `go test ./...`。

## 5. 当前内置任务

### 5.1 `run-smoke`

验证 one-shot 工作流：

- planner 决定单条命令
- runner 调用 `air run`
- AIR 创建临时隔离环境、执行、返回结果、销毁

### 5.2 `session-workflow`

验证多步状态保留：

- 创建 session
- 写文件
- 读文件
- 根据前一步结果继续执行
- 删除 session

### 5.3 `session-recovery`

验证失败恢复：

- 先执行一个预期失败的命令
- 读取失败信号
- 再执行恢复动作
- 验证恢复结果

### 5.4 `test-and-fix`

验证更接近 coding agent 的闭环：

- 预置一个带 bug 的 `app.sh`
- 预置一个会失败的 `test.sh`
- planner 先检查文件或跑测试
- 根据失败结果修复 `app.sh`
- 再次执行 `sh test.sh`
- runner 在 planner `finish` 之后再做一次最终校验

### 5.5 `repo-bugfix`

验证更贴近真实 repo 修复任务的闭环：

- 预置一个多文件 demo repo
- 包含 `README.md`、`src/lib.sh`、`src/message.sh`、`tests/test.sh`
- planner 先检查 repo 文件和测试脚本
- 运行 repo 级测试
- 修改实现并再次执行 `cd demo-repo && sh tests/test.sh`
- runner 在 planner `finish` 之后再做一次 repo 级最终校验
- 成功时会输出一个 delivery-style `final_summary`，概括修改文件和最终状态

## 6. 输出结构

`examples/agent-runner` 输出结构化 JSON，包含：

- `planner`
- `model`
- `task`
- `success`
- `tasks[].final_summary`
- `tasks[].steps[]`

每一步会包含：

- `command`
- `stdout`
- `stderr`
- `exit_code`
- `request_id`
- `duration_ms`
- `timeout`
- `success`
- `note`

其中 `tasks[].final_summary` 适合直接展示给人看，尤其是 repo 修复类任务；`tasks[].steps[]` 则保留完整的 agent 执行证据链。

这意味着你可以把 `examples/agent-runner` 当成：

- 最小 reference agent
- 回归测试器
- 后续产品级 agent 的行为样例

## 7. 如果你要自己接入 AIR

如果不是直接用 `examples/agent-runner`，而是要把 AIR 接到你自己的 agent，推荐顺序是：

1. planner 决定下一步 shell command
2. 如果是一次性任务，调用 `air run`
   必要时附带 `--timeout`、`--memory-mib`、`--vcpu-count`
3. 如果是多步任务，调用 `air session create`
4. 多次调用 `air session exec`
5. 读取结构化结果
6. 完成后调用 `air session delete`

## 8. 当前边界

当前已经具备：

- 真正的 LLM planner
- 真正的隔离执行
- one-shot / session / recovery / test-and-fix / repo-bugfix
- `air run` 已支持稳定 `error_type`
- OpenAI / DeepSeek / scripted 三类 planner
- `examples/agent-runner` 已支持 planner 重试和模型升级

当前还没有：

- 多 agent 编排
- Anthropic / Gemini adapter
- HTTP API 形式的 planner service
- 更大规模的 repo 修复任务集

## 9. 当前推荐

如果你现在就要通过 AI agent 使用 AIR，建议：

1. 本地先用 `scripted` 跑通
2. 然后切 `deepseek` 或 `openai`
3. 先跑 `run-smoke`
4. 再跑 `session-workflow`
5. 最后跑 `test-and-fix` 或 `repo-bugfix`

这样最容易定位问题出在：

- planner
- runner
- AIR runtime
- 还是模型额度 / API 配置
