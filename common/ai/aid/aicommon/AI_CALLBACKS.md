# AI Callback 规则与使用指南

**概念**

`Config` 里有三类回调：

- `OriginalAICallback` 原始回调，主要用于异步任务或兜底调用，不占用质量/速度优先通道。
- `QualityPriorityAICallback` 质量优先回调，面向高质量模型（intelligent tier）。
- `SpeedPriorityAICallback` 速度优先回调，面向轻量快速模型（lightweight tier）。

**配置入口与规则**

- `WithAutoTieredAICallback(cb)`
  推荐入口。如果启用了 tiered 配置，会尝试 `WithTieredAICallback` 填充质量/速度回调，同时把 `OriginalAICallback` 设为 `cb` 以保证异步任务可用。如果未启用 tiered 配置，等价于“单一回调”，会把 `cb` 设为 `OriginalAICallback`，并为质量/速度回调做 wrapper。
- `WithTieredAICallback()`
  从全局 tiered 配置读取 intelligent/lightweight 模型回调。只设置质量/速度回调，不设置 `OriginalAICallback`。
- `WithInheritTieredAICallback(parentConfig, force)`
  子 invoker 继承父级回调的入口。会先继承父级的 `OriginalAICallback`；`force=true` 时直接沿用父级的质量/速度回调，`force=false` 且启用了 tiered 配置时，会按当前 tiered 配置重新生成质量/速度回调，但仍保留父级原始回调用于异步兜底。
- `WithAICallback(cb)`
  粗粒度入口，仅建议用于测试或单一模型模块（例如知识库蒸馏）。会把 `cb` 设为 `OriginalAICallback`，并为质量/速度回调做 wrapper。
- `WithFastAICallback(cb)`
  快速/纯设置入口，不做 wrapper。只设置 `OriginalAICallback`，并清空质量/速度回调。适合主线无关的 LiteForge 调用。
- `WithQualityPriorityAICallback(cb)` 与 `WithSpeedPriorityAICallback(cb)`
  仅设置对应优先级回调，并自动 wrapper。

**调用顺序**

- `CallAI`：质量优先 → 速度优先 → 原始回调。
- `CallQualityPriorityAI`：质量优先 → 原始回调。
- `CallSpeedPriorityAI`：速度优先 → 原始回调。
- `CallOriginalAI`：仅原始回调。
- `Config.InvokeLiteForge`：优先使用速度回调，若无则使用质量回调，并通过 `WithFastAICallback` 传入。

**推荐用法**

- 主线 ReAct/Coordinator：优先使用 `WithAutoTieredAICallback`。
- 父子调用链需要保持同一组回调时：使用 `WithInheritTieredAICallback`。
- 需要强制单一模型的模块：使用 `WithAICallback`，但仅限测试或功能非常固定的场景。
- LiteForge 或非主线的小任务：使用 `WithFastAICallback`。

**Config 初始化说明（重点）**

- `newConfig` / `NewConfig` 不再保证自动注入可用 AI 回调。
- 也就是说，直接 `NewConfig(ctx)` 在未额外传入 callback option 的情况下，可能出现三类回调都为空。
- 因此在业务初始化时，建议明确传入：
  - `WithAutoTieredAICallback(cb)`（推荐，主线场景）
  - 或 `WithAICallback(cb)`（仅测试/固定单模型场景）
- 如果你依赖 tiered 配置，请确保 aiconfig 中 intelligent/lightweight 至少各有一个可用模型；否则质量/速度回调仍可能为空。

示例（推荐）：

```go
cfg := aicommon.NewConfig(ctx,
    aicommon.WithAutoTieredAICallback(myDefaultCb),
)
```

示例（快速子任务/LiteForge）：

```go
cfg := aicommon.NewConfig(ctx,
    aicommon.WithFastAICallback(myCb),
)
```

**注意事项**

- 若三类回调都为空，`CallAI/CallQualityPriorityAI/CallSpeedPriorityAI` 会直接返回错误。
- 如果启用了 tiered 配置但未配置对应模型，质量/速度回调可能为空，需要在 aiconfig 中补齐。
