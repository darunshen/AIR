# agent-runner

`examples/agent-runner` 是 AIR 的最小 reference agent。

当前它已经支持两种 planner：

- `openai`
  - 通过 OpenAI Responses API 规划下一步动作
- `deepseek`
  - 通过 DeepSeek Chat Completions API 规划下一步动作
- `scripted`
  - 作为离线回归和无 API key 场景下的固定策略 fallback

它不是一个完整的多 agent 平台，但已经是一个真实的 LLM planner + AIR executor 组合，用来验证：

- AIR 的一次性执行接口是否足够给 agent 使用
- AIR 的 session 工作流是否足够支持多步任务
- stdout / stderr / exit code / request_id / timeout 是否足够做下一步决策
- planner 失败时的重试和模型升级策略是否足够稳定

## 运行方式

默认使用 OpenAI planner 和 `local` provider：

```bash
go run ./examples/agent-runner --task all
```

离线回归可切到 scripted planner：

```bash
go run ./examples/agent-runner --planner scripted --task all
```

使用 DeepSeek：

```bash
export DEEPSEEK_API_KEY=...
go run ./examples/agent-runner --planner deepseek --model deepseek-chat --task all
```

只跑 one-shot smoke：

```bash
go run ./examples/agent-runner --task run-smoke
```

只跑多步 session workflow：

```bash
go run ./examples/agent-runner --task session-workflow
```

只跑 `test-and-fix`：

```bash
go run ./examples/agent-runner --task test-and-fix
```

切到 Firecracker：

```bash
go run ./examples/agent-runner --provider firecracker --task all
```

显式指定模型：

```bash
go run ./examples/agent-runner --model gpt-5.4 --task all
```

显式指定升级模型和重试次数：

```bash
go run ./examples/agent-runner --model gpt-5.4-mini --escalation-model gpt-5.4 --planner-retries 1 --task all
```

运行前需要准备：

```bash
export OPENAI_API_KEY=...
export AIR_AGENT_PROVIDER=openai
export AIR_AGENT_MODEL=gpt-5.4-mini
export AIR_AGENT_ESCALATION_MODEL=gpt-5.4
export AIR_AGENT_PLANNER_RETRIES=1
export AIR_AGENT_REASONING=medium
```

## 内置任务

- `run-smoke`
  - planner 先决定 one-shot shell 命令
  - runner 再调用 `air run` 等价路径执行
  - 验证 one-shot 执行与结构化结果

- `session-workflow`
  - 创建 session
  - planner 根据历史结果逐步决定写入、读取、验证命令
  - 删除 session

- `session-recovery`
  - 创建 session
  - planner 先执行一个预期失败的命令
  - 再根据失败结果执行恢复动作并验证
  - 删除 session

- `test-and-fix`
  - 创建 session
  - 预置一个带 bug 的 `app.sh` 和会失败的 `test.sh`
  - planner 自己检查文件、跑测试、修复、复测
  - finish 后 runner 再做一次最终校验
  - 删除 session

## 输出

输出为结构化 JSON，便于后续接入外部 agent 编排层或测试脚本。

其中会包含：

- planner 类型
- planner 模型
- planner 升级模型
- planner 重试次数
- 每一步的命令
- stdout / stderr / exit_code / request_id / timeout
- planner 最终的 `finish` 判定
