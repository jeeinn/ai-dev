# TODO: LLM Provider 可选增强

> 状态：未完成（可选 / P2）  
> 创建日期：2026-07-14  
> 来源：从 [archived/20260710-LLMProvider模型选择与Token配置扩展方案.md](archived/20260710-LLMProvider模型选择与Token配置扩展方案.md) 拆出  
> 说明：主方案 Phase 1/2 与 Phase 3.1–3.3（usage 记录、成本估算、任务页展示）已完成并归档；本文仅跟踪剩余可选能力。

---

## 未完成项

| # | 任务 | 优先级 | 建议位置 | 说明 |
|---|------|--------|----------|------|
| 3.4 | 精确 tokenizer | P2 | `internal/agent/context.go` | 引入 tiktoken（或同类）替代 CJK 估算 |
| 3.5 | 语义摘要 | P2 | `internal/agent/summarize.go` | 对超长历史对话做摘要以省入窗 |
| 3.6 | 成本预算控制 | P2 | `internal/agents/*` | 单任务预算上限；超限中止或拒绝 |

---

## 完成标准（可选）

- [ ] 3.4：可选开启精确计数；默认仍可用现行估算；有单测对比误差  
- [ ] 3.5：超长 Session 可配置摘要策略，不影响短对话路径  
- [ ] 3.6：可配置 per-task 成本上限，超限写回可读错误  

**非阻塞**：不完成不影响现有模型下拉、动态发现、自适应 token、usage/成本展示。

---

## 参考

- 完整设计与已完成进度：[archived/20260710-LLMProvider…](archived/20260710-LLMProvider模型选择与Token配置扩展方案.md) §九 / §十三  
- Token 截断与预算（已有上下文 truncation，非本条目「美元成本预算」）
