# 单模型配置与辅助调度

## 边界

- `aicommon.Config` 的单模型选项只用于辅助任务 Skip / Run 判断，并通过现有 `ConvertConfigToOptions` 继承给子 Config。
- Config 不改写自定义 callback，不延迟 callback option，也不维护模式初始化或继承状态。
- `WithTieredAICallback(singleModelMode ...bool)` 在创建 tier callbacks 时接收可选模式。未传值时读取当前 Config 的有效模式。
- 脚本 `ai` 包、gateway、aispec 保持底层原有行为，不增加单模型拦截。
- 禁止通过 Context 绑定或传递模式变量。

## Config 行为

有效调度模式为：

```go
config.singleAIModelMode || consts.IsSingleAIModelMode()
```

单模型下辅助注册表只区分：

- Skip：不构建 Prompt，不调用 AI。
- Run：调用当前 Config 已配置的 Speed callback。

自定义 callback 完全遵守 option 顺序和自身参数。单模型模式不会强制包装或替换它。

子 Config 构建时，如果父 Config 的有效模式为 true，则追加 `WithSingleAIModelMode(true)`。Raw callback 仍按原有机制继承。

## Tier callback 初始化

`WithTieredAICallback` 创建三个 callback：

| 请求角色 | 普通模式 | 单模型模式 |
| --- | --- | --- |
| Quality | Intelligent | Intelligent |
| Speed | Lightweight | Intelligent + LiteCall |
| Vision | Vision | Intelligent |

可选 bool 只影响此次 callback 初始化：

```go
WithTieredAICallback(true)  // 按单模型构造
WithTieredAICallback(false) // 按普通 tier 构造
WithTieredAICallback()      // 读取 Config 当前有效模式
```

callback option 本身仍立即执行，后面的 option 覆盖前面的 option。

## Tier callback 调用

普通 tier getter 共用 `newTierAIModelCallback(tier, mode...)`：

1. 判断普通 tier 配置是否启用；显式单模型模式允许只配置 Intelligent。
2. 创建时确定目标角色和模式。
3. 每次调用读取对应 tier 最新的 global model config。
4. `resolveTierModelConfig` 克隆模型配置，避免修改 global config。
5. 单模型 Speed 将临时配置的 `Provider.ReasoningEffort` 设置为 `none`。
6. 使用现有的 `CreateCallbackFromConfigWithExtraOpts` 把临时配置构造成 service callback，并透传用户 UsageCallback。
7. 通过标准 `AICallbackType` 调用链执行请求，不在 tier callback 中重复实现 `ai.Chat` 参数拼装。

因此模型、Provider、地址、凭据更新会在下一次调用生效；callback 的角色不会随全局模式变化重新选择。

旧的 provider/model 精确选择接口保留独立实现，不参与单模型 tier 初始化，也不进入普通 getter 的公共路径。

## 已删除的错误尝试

- Context 模式绑定与继承。
- gateway / aispec 单模型拦截。
- 脚本 AI 导出替换。
- manager 的多层 `BindModelConfig / BindRequestOptions / NewChatCallback`。
- Config callback 延迟构建、初始化状态和三态继承字段。
- Config 层对所有 Speed callback 的 LiteCall 包装。
- AIRequest profile 和逐任务 LiteCall 参数注入。

## 配置入口

- `AIGlobalConfig.SingleModelMode`：全局调度模式。
- `AIStartParams.SingleModelMode`：本次会话调度模式。
- 主模型始终来自 global Intelligent 第一项，不增加独立单模型配置。
- 已废弃的 `AIService / AIModelName` 不参与启动选模。
