# AI Callback 规则与使用指南

**概念**

`Config` 里有三类路由槽位：

- `OriginalAICallback` 原始回调，主要用于异步任务或兜底调用，不占用质量/速度优先通道。
- `QualityPriorityAICallback` 质量优先回调，面向高质量模型（intelligent tier）。
- `SpeedPriorityAICallback` 速度优先回调，面向轻量快速模型（lightweight tier）。

从外部看它们仍是 `AICallbackType`（`func(cfg, req) (*AIResponse, error)`）。但 `Config` 内部存储的是
`AICallbackFactory`（`func(model *ypb.AIModelConfig) (AICallbackType, error)`）而不是回调实例，getter
每次调用时生成。这样做的原因是：

- 质量模型的上下文预算（prompt 组装时读取）和实际请求的模型必须是同一个配置。
- 如果 Config 在构造时就固定回调，probe/组装 prompt 之后发生 hot-patch 或 tier 切换，就会出现
  “按 A 的预算裁剪，却发给 B”的 TOCTOU 竞态。
- 把“生成时机”交给调用方：主线 ReAct 每轮生成一次绑定并持有，其它调用方按需生成，天然对齐。

**每轮绑定（推荐主线用法）**

`Config.GeneratePrimaryAICallback(model)` 用同一个模型快照生成当轮绑定：

- `binding.Callback`：主调用目标，质量 → 速度 → 原始，与 `CallAI` 回退顺序一致。
- `binding.Quality`：压缩/溢出恢复目标，始终钉在生成时的质量模型。
- 质量回调在生成时即完成绑定（provider、model、thinking level、model context key），
  之后全局 tier 再变化也不会重定向。

ReAct 循环在 `captureLoopPromptBuildInput` 中生成绑定，并保存到 `requestAIBinding`。绑定里的
`ModelConfig` 就是当轮 prompt 预算、image 路由和溢出重试使用的快照，`Callback` / `Quality` 由同一
快照生成；同一轮内所有重试、prompt 重建、上下文压缩都复用它。速度/视觉等辅助 tier 仍走配置级
getter，不随轮次钉住。

自定义的固定回调（`WithQualityPriorityAICallback` 等）本身不依赖 tier 配置，生成时原样返回，
因此不会被快照重定向，这是符合预期的。

**配置入口与规则**

- `WithAutoTieredAICallback(cb)`
  推荐入口。如果启用了 tiered 配置，会尝试 `WithTieredAICallback` 安装质量/速度/视觉生成器，同时把
  `OriginalAICallback` 设为 `cb` 以保证异步任务可用。如果未启用 tiered 配置，等价于“单一回调”，
  四类槽位都设为固定生成器。
- `WithTieredAICallback(singleModelMode...)`
  从全局 tiered 配置读取 intelligent/lightweight/vision 模型并安装生成器。只设置质量/速度/视觉槽位，
  不设置 `OriginalAICallback`。
- `WithInheritTieredAICallback(parentConfig, force)`
  子 invoker 继承父级的回调生成器和质量上下文绑定。生成器按原样复制，因此在子 Config 上生成回调时
  仍会读取“当前”全局 tier（继承的是决策逻辑，不是一次性快照）；`force=false` 时父级没有任何 tier
  回调则跳过继承。
- `WithAICallbacks(callbacks)`
  直接安装一组生成器。测试里可用 `FixedAICallbackFactory(cb)` 把固定回调适配成生成器。
  LiteForge 也通过它把协议绑定包在生成器外层。
- `WithAICallback(cb)`
  粗粒度入口，仅建议用于测试或单一模型模块（例如知识库蒸馏）。会把固定生成器装到四个槽位。
- `WithFastAICallback(cb)`
  快速/纯设置入口，不做 wrapper。只设置 `OriginalAICallback`，并清空质量/速度/视觉槽位。适合主线无关的
  LiteForge 调用。
- `WithQualityPriorityAICallback(cb)`、`WithSpeedPriorityAICallback(cb)` 与 `WithVisionPriorityAICallback(cb)`
  仅设置对应优先级的固定生成器。

**调用顺序**

- `CallAI`：质量优先 → 速度优先 → 原始回调。
- `CallQualityPriorityAI`：质量优先 → 原始回调。
- `CallSpeedPriorityAI`：速度优先 → 原始回调。
- `CallOriginalAI`：仅原始回调。
- `Config.InvokeLiteForge`：优先使用速度回调，若无则使用质量回调。

**推荐用法**

- 主线 ReAct/Coordinator：优先使用 `WithAutoTieredAICallback`。
- 父子调用链需要保持同一组回调时：使用 `WithInheritTieredAICallback`。
- 需要强制单一模型的模块：使用 `WithAICallback`，但仅限测试或功能非常固定的场景。
- LiteForge 或非主线的小任务：使用 `WithFastAICallback`。
- 需要“预算与路由对齐”的一次性调用：`GeneratePrimaryAICallback(snapshot)` 后使用
  `binding.Callback` / `binding.Quality`，不要直接保存 getter 结果跨轮次使用。

**Config 初始化说明（重点）**

- `newConfig` / `NewConfig` 不再保证自动注入可用 AI 回调。
- 也就是说，直接 `NewConfig(ctx)` 在未额外传入 callback option 的情况下，可能出现三类回调都为空。
- 因此在业务初始化时，建议明确传入：
  - `WithAutoTieredAICallback(cb)`（推荐，主线场景）
  - 或 `WithAICallback(cb)`（仅测试/固定单模型场景）
- 如果你依赖 tiered 配置，请确保 aiconfig 中 intelligent/lightweight/vision 至少各有一个可用模型；
  否则对应槽位生成时仍可能为空。

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

示例（每轮绑定）：

```go
snapshot := cfg.SnapshotPrimaryModelConfig()
binding, err := cfg.GeneratePrimaryAICallback(snapshot)
if err != nil {
    return err
}
// 本轮主调用
resp, err := binding.Callback(cfg, req)
// 本轮上下文压缩
resp, err = binding.Quality(cfg, compressReq)
```

**注意事项**

- 若三类回调都为空，`CallAI/CallQualityPriorityAI/CallSpeedPriorityAI` 会直接返回错误。
- 生成器返回 nil 表示该槽位未配置。`GeneratePrimaryAICallback` 遇到生成错误会返回错误，不会静默换成其它 tier，
  避免预算与路由悄悄错位；配置级 getter 为保持历史行为会返回 nil，由 `CallAI` 继续回退。
- 如果启用了 tiered 配置但未配置对应模型，质量/速度回调可能为空，需要在 aiconfig 中补齐。
