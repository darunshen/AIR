# LLM 验收结果

本文档记录 `examples/agent-runner` 在真实模型上的一次验收结果，以及后续复跑方式。

这些结果是仓库内 reference agent 的验收快照，不代表长期 SLA。请始终结合当前 commit、模型额度、API 网关状态一起判断。

## 1. 复跑入口

统一使用仓库脚本：

```bash
scripts/run-agent-acceptance.sh --planner scripted --task all
```

也支持受环境变量控制的 `go test` 入口：

```bash
AIR_LLM_ACCEPTANCE=1 \
AIR_AGENT_PROVIDER=deepseek \
AIR_AGENT_MODEL=deepseek-chat \
AIR_AGENT_ESCALATION_MODEL=deepseek-reasoner \
AIR_AGENT_ACCEPTANCE_TASKS=run-smoke,session-workflow,test-and-fix \
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
go test ./examples/agent-runner -run TestRealLLMAgentWorkflowAcceptance -v
```

真实模型可用：

```bash
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
scripts/run-agent-acceptance.sh \
  --planner deepseek \
  --model deepseek-chat \
  --escalation-model deepseek-reasoner \
  --planner-retries 1 \
  --task all
```

```bash
OPENAI_API_KEY_FILE=~/tmp/openai.api \
scripts/run-agent-acceptance.sh \
  --planner openai \
  --model gpt-5.4-mini \
  --escalation-model gpt-5.4 \
  --planner-retries 1 \
  --task all
```

每次执行都会在 `artifacts/agent-acceptance/` 下生成单独目录，至少包含：

- `command.txt`
- `metadata.txt`
- `result.json`

仓库还提供了一套复用式 GitHub Actions：

- `.github/workflows/llm-acceptance.yml`
  - 既支持手动触发，也支持被其他 workflow 复用
- `.github/workflows/ci.yml`
  - 常规 `go test ./...`
  - 如果仓库配置了 `DEEPSEEK_API_KEY` secret，会自动串上 `llm-acceptance`
- `.github/workflows/release.yml`
  - 先跑常规测试
  - 如果仓库配置了 `DEEPSEEK_API_KEY` secret，会复用同一份 `llm-acceptance`
  - 通过后才构建并上传 release 产物

## 2. 2026-04-20 验收快照

验收环境：

- 仓库：`github.com/darunshen/AIR`
- planner runner：`examples/agent-runner`
- runtime provider：`local`
- 重试策略：先同模型重试，再升级到 `--escalation-model`

### 2.1 DeepSeek

执行配置：

```bash
DEEPSEEK_API_KEY_FILE=~/tmp/deepseek.api \
scripts/run-agent-acceptance.sh \
  --planner deepseek \
  --model deepseek-chat \
  --escalation-model deepseek-reasoner \
  --planner-retries 1 \
  --task all
```

结果：

- `run-smoke`：通过
- `session-workflow`：通过
- `test-and-fix`：通过

备注：

- 这次通过没有明显触发升级模型
- 说明当前 DeepSeek 接入已足够覆盖 one-shot、多步 session、test-and-fix 三类主流程

### 2.2 OpenAI

执行配置：

```bash
OPENAI_API_KEY_FILE=~/tmp/openai.api \
scripts/run-agent-acceptance.sh \
  --planner openai \
  --model gpt-5.4-mini \
  --escalation-model gpt-5.4 \
  --planner-retries 1 \
  --task run-smoke
```

结果：

- `run-smoke`：失败
- HTTP 状态码：`429`
- 错误类型：`insufficient_quota`

备注：

- 这次失败不是 runner 协议错误，而是账户额度不足
- 但它验证了当前重试与升级链路已可观测
- 事件里已出现 `planner_retry`、`planner_escalation`、最终 `planner_error`

## 3. 如何解读结果

建议优先看 `result.json` 中这几类信号：

- `success`
- `tasks[].steps[].kind`
- `tasks[].steps[].planner_attempt`
- `tasks[].steps[].planner_model`
- `tasks[].steps[].request_id`
- `tasks[].steps[].error_message`

判定顺序建议是：

1. 先看是否是模型侧错误，例如 `429`、认证失败、网关超时
2. 再看是否是 AIR 执行侧错误，例如 `startup_error`、`transport_error`
3. 最后再看任务本身是否未收敛，例如 `test-and-fix` 未修复成功

## 4. 当前结论

基于 2026-04-20 这次验收快照，当前状态可以表述为：

- `scripted` 已可作为稳定离线回归基线
- `deepseek` 已跑通 reference agent 主流程
- `openai` adapter 已接通，但这次真实验收被额度限制阻断
- planner 重试与升级链路已进入可复跑、可排障状态
