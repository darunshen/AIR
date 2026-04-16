# agent-runner

`examples/agent-runner` 是 AIR 的最小 reference agent。

它不是一个完整的 LLM agent 产品，而是一个固定策略的执行器，用来验证：

- AIR 的一次性执行接口是否足够给 agent 使用
- AIR 的 session 工作流是否足够支持多步任务
- stdout / stderr / exit code / request_id / timeout 是否足够做下一步决策

## 运行方式

默认使用 `local` provider：

```bash
go run ./examples/agent-runner --task all
```

只跑 one-shot smoke：

```bash
go run ./examples/agent-runner --task run-smoke
```

只跑多步 session workflow：

```bash
go run ./examples/agent-runner --task session-workflow
```

切到 Firecracker：

```bash
go run ./examples/agent-runner --provider firecracker --task all
```

## 内置任务

- `run-smoke`
  - 调用 `air run` 等价路径
  - 验证 one-shot 执行与结构化结果

- `session-workflow`
  - 创建 session
  - 写入文件
  - 读取文件
  - 根据读取结果决定下一条命令
  - 删除 session

- `session-recovery`
  - 创建 session
  - 执行一个预期失败的命令
  - 根据失败结果执行恢复动作
  - 验证恢复结果
  - 删除 session

## 输出

输出为结构化 JSON，便于后续接入外部 agent 编排层或测试脚本。
