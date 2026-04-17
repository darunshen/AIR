# AI Agent 选型与接入方案

本文档用于回答两个问题：

1. AIR 当前应该先接哪类 AI agent
2. 这套 agent 的接入环境应该如何准备

## 1. 当前结论

截至 2026-04-16，AIR 的执行后端已经具备，并且 OpenAI / DeepSeek planner 都已经完成第一版接入。

当前建议的第一阶段方案是：

- 保持 AIR 作为隔离执行后端
- 先接一个外部 LLM 作为 planner / decision maker
- 第一个接入目标优先选 OpenAI Responses API
- 默认模型先用 `gpt-5.4-mini`
- 复杂任务升级到 `gpt-5.4`
- 保持 provider 抽象，后续再补 Anthropic 和 Gemini

补充说明：

- OpenAI 仍是默认接入 provider
- DeepSeek 已可作为低成本替代 planner

这不是在说 Anthropic 或 Gemini 不行，而是在 AIR 当前阶段，最重要的是先把“真实 agent 工作流”跑通，而不是同时做三套模型适配。

## 2. 选型标准

AIR 当前不是在做聊天产品，而是在做 coding agent 的执行底座。因此首批模型选型标准应是：

- 对 coding / agentic workflow 友好
- 支持稳定的结构化输出
- 支持工具调用
- API 与 SDK 成熟，便于快速落地
- 能做分层模型策略：默认便宜模型，必要时升级强模型
- 后续可切换供应商，避免把 AIR 和单一厂商强绑定

## 3. 候选方案对比

### 3.1 OpenAI

官方文档当前明确写到：

- 如果不确定从哪个模型开始，复杂推理和 coding 优先用 `gpt-5.4`
- 如果要兼顾时延和成本，可选 `gpt-5.4-mini` 或 `gpt-5.4-nano`
- 最新模型支持 Responses API、函数调用、结构化输出、Web search、File search、Computer use

这和 AIR 当前阶段高度匹配，因为 AIR 最需要的是：

- 一个能稳定决定“下一步执行什么命令”的 planner
- 一个好接、好控、好做结构化输出的 API
- 一条能快速接到 reference agent 上的最短路径

因此 OpenAI 适合作为 AIR 的第一接入目标。

### 3.2 Anthropic

Anthropic 官方文档当前显示：

- `Claude Sonnet 4.6` 是速度与智能较平衡的模型
- `Claude Opus 4.6` 是更强的高端模型
- Claude 的 Messages API 原生支持 tool use
- Claude 也提供 computer use、text editor、code execution 等能力

Anthropic 同样非常适合 coding agent，尤其是复杂工具调用场景。

但对 AIR 当前阶段来说，Anthropic 更适合作为第二接入目标，而不是第一目标，原因是：

- 我们现在更需要先定一条最短接入路径
- `examples/agent-runner` 还处在 reference agent 阶段
- 同时接多家 provider 会分散 P0 精力

### 3.3 Gemini

Google 官方文档当前显示：

- `Gemini 2.5 Pro` 面向复杂推理与代码分析
- `Gemini 2.5 Flash` 主打价格性能比
- `Gemini 2.5 Flash-Lite` 主打低成本高吞吐
- Gemini API 支持 structured outputs、function calling、code execution、长上下文

Gemini 在长上下文和成本性能比方面很有吸引力。

但 AIR 当前的首要目标不是长文档理解，而是：

- 先把 agent -> AIR -> 结果 -> 下一步决策 的闭环跑通

所以 Gemini 也建议放在第二批适配。

## 4. 推荐决策

### 4.1 第一阶段

先接：

- Provider: OpenAI
- API: Responses API
- Default model: `gpt-5.4-mini`
- Escalation model: `gpt-5.4`

原因：

- 官方当前推荐 `gpt-5.4` 用于复杂推理和 coding
- `gpt-5.4-mini` 更适合先做低成本 reference agent
- 同一供应商内做默认模型与升级模型切换最简单
- Responses API 比较适合后续扩展 structured outputs 和工具调用

### 4.2 第二阶段

补第二家 provider：

- 候选一：Anthropic `claude-sonnet-4-6`
- 候选二：Google `gemini-2.5-flash`

原则：

- AIR 内部保留统一的 planner 接口
- 不让业务层直接依赖某一家厂商的消息格式
- 任何 provider 切换都不应影响 AIR 的执行接口

## 5. 推荐架构

第一阶段不要把 LLM 和 AIR 强耦合在一起，建议保持三层结构：

```text
Task Input
   |
   v
Planner LLM
   |
   v
Agent Runner
   |
   v
AIR CLI / AIR API
   |
   v
Isolated Runtime
```

职责拆分：

- Planner LLM
  - 决定下一步动作
  - 生成结构化 action

- Agent Runner
  - 维护对话状态
  - 调用 AIR
  - 读取 stdout / stderr / exit code / timeout / request_id
  - 把结果回填给 LLM

- AIR
  - 只负责隔离执行
  - 不负责模型推理

这样做的好处是：

- AIR 仍然保持“执行底座”定位
- 后续更换模型供应商成本更低
- 参考 agent 和生产 agent 能共用一套 AIR 接口

## 6. 环境准备结论

### 6.1 已经搭起来的部分

AIR 执行环境已经基本具备：

- `local` provider 可用
- `air run` 可用
- `session create / exec / delete` 可用
- `examples/agent-runner` 固定策略版可用
- Firecracker 实验链路可用

### 6.2 当前仍未完成的部分

第一版 OpenAI / DeepSeek planner 已经接上，但还没有全部做完：

- 还没有 Anthropic / Gemini adapter
- 还没有更复杂的“跑测试 -> 解析失败 -> 修复”任务
- 已支持基础模型升级策略，但还没有 prompt 自动升级策略
- 还没有把 planner 抽成单独的 API 服务

所以准确说法是：

- AIR 执行环境：已搭起来
- OpenAI / DeepSeek planner 接入环境：已搭起来第一版
- 多 provider agent 平台：还没有完成

## 7. 第一阶段环境变量建议

建议先统一成如下配置：

```bash
AIR_AGENT_PROVIDER=openai
AIR_AGENT_MODEL=gpt-5.4-mini
AIR_AGENT_ESCALATION_MODEL=gpt-5.4
AIR_AGENT_PLANNER_RETRIES=1
AIR_AGENT_REASONING=medium
OPENAI_API_KEY=...
```

后续扩展时再加：

```bash
ANTHROPIC_API_KEY=...
ANTHROPIC_MODEL=claude-sonnet-4-6

GEMINI_API_KEY=...
GEMINI_MODEL=gemini-2.5-flash
```

## 8. 落地顺序

建议按这个顺序做：

1. 已定义 `llm.Provider` 抽象
2. 已实现 OpenAI Responses API adapter
3. 已让 `examples/agent-runner` 支持 OpenAI / DeepSeek planner
4. 已支持基础 `test-and-fix` 闭环
5. 再补 Anthropic / Gemini adapter
6. 最后再考虑独立 planner service

## 9. 不建议现在做的事

- 不建议一开始就同时接 OpenAI、Anthropic、Gemini 三家
- 不建议一开始就做复杂多 agent 架构
- 不建议一开始就做长期记忆、RAG、Browser agent
- 不建议把模型 prompt 和 AIR runtime 代码写死耦合

## 10. 当前建议

如果只做一个明确决策，当前建议是：

- 先选 OpenAI 作为第一家接入 provider
- 默认用 `gpt-5.4-mini`
- 难任务升级到 `gpt-5.4`
- AIR 保持 provider-agnostic 的 planner 抽象
- 下一步直接做更复杂的 planner task，并扩展多 provider

## 参考资料

- OpenAI Models: https://developers.openai.com/api/docs/models
- OpenAI GPT-5.4 / Responses API 相关模型页: https://platform.openai.com/docs/models
- Anthropic Models Overview: https://docs.anthropic.com/en/docs/about-claude/models/overview
- Anthropic Tool Use: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/overview
- Gemini Models: https://ai.google.dev/models/gemini
- Gemini API Models 文档: https://ai.google.dev/gemini-api/docs/models/gemini-v2
