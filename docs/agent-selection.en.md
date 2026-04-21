# AI Agent Selection And Integration Plan

[中文](agent-selection.md)

## 1. Current Conclusion

The first provider path is OpenAI, with `gpt-5.4-mini` as the default planning model and `gpt-5.4` as the escalation model for harder tasks. DeepSeek is already integrated as another practical provider.

## 2. Selection Criteria

The comparison focuses on toolability, planning stability, model quality, operational simplicity, and fit for coding-agent workflows.

## 3. Candidates

- OpenAI
- Anthropic
- Gemini

## 4. Recommended Decision

### Phase 1

Adopt OpenAI first, keep DeepSeek available, and delay adding more providers until the core workflow is stable.

### Phase 2

Consider Anthropic or Gemini only after the reference agent and product loop are clearly validated.

## 5. Recommended Architecture

Use a provider abstraction in `internal/llm`, keep planner logic in the reference agent, and keep AIR itself focused on isolated execution.

## 6. Environment Preparation Status

Some planner and key-loading paths are already in place; remaining work is mostly around smoother operational integration.

## 7. Suggested Environment Variables

Keep explicit variables for provider, model, escalation model, reasoning level, and API keys or key files.

## 8. Rollout Order

Build the reference agent first, prove end-to-end task execution, then expand providers.

## 9. What Not To Do Yet

Do not overbuild a general multi-agent platform before the single-agent execution loop is solid.

## 10. Current Recommendation

Prioritize the most pragmatic planner stack that can complete real repo tasks through AIR.
