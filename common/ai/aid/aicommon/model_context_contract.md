# 模型上下文行为契约

## 数据归属与单位

所有数值以 token 计，UI 的 1K = 1024 tokens。`provider + model` 共同确定模型身份；不同供应商的同名模型、质量模型与轻量模型互相隔离。数据保存在 profile 数据库独立的 `ai-model-context:*` 元数据中，不改动路由配置或 API 参数。

- 未知模型：容量与压缩阈值均为 500K。
- 已知模型：新会话、恢复会话使用该模型保存的阈值。前端启动参数传 `RuntimeContextSizeLimit=0`，不再从全局设置复制 500K 或其他数值。
- 容量是模型的能力证据；阈值是压缩触发点。用户将阈值设成 600K 不证明模型容量达到 600K。
- 旧客户端显式传入非零启动阈值仅作为未知模型的兼容初始预算；一旦存在模型记录，该记录优先于旧会话上限；新 UI 仅通过带模型身份的热更新修改模型设置。SDK 内部 `WithTimelineContentLimit` 的小预算不属于用户输入。

## 手动边界

全局手动下限为 `100 * 1024` tokens。存储 API、运行时热更新、会话启动入口均验证下限；负数及 `(0,100K)` 被拒绝。0 是“跟随模型记录”的控制值，不是 0 token，也不清空已学习容量。被拒绝的热更新不改运行时、模型元数据或恢复参数，并发送包含 `error` 的当前状态。

UI 只在拖动结束时提交，指针拖动和键盘操作都不能提交低于 100K 的数值。服务端发现真实容量不足 100K 时可以自动使用更小阈值并告警，不能为通过手动输入校验而伪造容量。超过 500K 允许保存，UI 显示“过大上下文可能降低模型表现、增加延迟和费用”。

## 自动恢复状态机

1. 仅明确的上下文超限错误进入校准。鉴权、限流、普通 API 错误、取消不改变容量。
2. 报告最大容量 M：候选阈值为 `0.8M`；未报告：候选阈值为失败 prompt 估算 token 数的 `0.6`。候选不扩大当前有效阈值；自动恢复保留 4K 的内部最小预算（内部显式预算更小时遵循更小预算）。
3. 在内存为失败模型保存 pending 证据，立即发 `model_context`，包含变更前后数值。此时不得落库。
4. 优先用最近成功的历史边界压缩。没有成功边界、或该压缩请求也被拒绝时缩短历史前缀；边界之外的新历史保留。单轮最多 2 次重建重试；跨回合最多累计 9 次超限拒绝。压缩维护请求成功不重置拒绝预算，只有正常业务回合成功后才重置，避免“压缩成功 → 业务超限”的无限循环。
5. 只有实际成功响应的 provider/model 能确认对应 pending 证据。身份来自本次响应，不能读取可能已切换的会话配置。质量模型代替轻量模型压缩成功，不会确认轻量模型的容量。匿名 SDK 回调只能在当前 Config 内恢复，不能持久化到其他模型。
6. 未报告最大容量时，成功 prompt 的 token 数作为可用预算的下界，修正过低估计；它是保守的可用预算，不是测得的物理最大容量。
7. 确认成功后落库并发 confirmed 事件。并发已学习结果只能收紧；每次手动设置生成版本，旧版本的迟到校准不得覆盖新设置。新手动设置之后发生的真实超限仍可重新校准。
8. 持续失败、取消和其他模型成功不落库。后续会话只读取上次成功确认或用户手动保存的结果。

## 前端事件契约

`model_context` 包含 `provider/model/prompt_tokens/context_tokens/threshold_tokens/source/pending/too_small/minimum_threshold_tokens`。`change` 包含唯一 `id`、`reason`（overflow/confirmed/manual）及 `previous_threshold_tokens/threshold_tokens`。

- `overflow`：即时提示阈值变化、正在压缩重试；阈值线使用同一个事件中的当前阈值。
- `confirmed`：提示成功并已保存，后续会话复用；未知上限的成功请求可修正阈值线。
- 状态定期重复时 `change.id` 不变；客户端按会话去重，历史同步 `IsSync` 只恢复状态、不弹通知。
- 模型面板持续展示最新自动变更说明，用户关闭弹窗时也会收到通知。手动失败展示错误并保留后端数值。拖动中后端更新到达会取消旧的本地拖动预览。

## 可执行验收

| 行为 | 回归测试 |
| --- | --- |
| 启动入口拒绝非法阈值 | `sessionruntime/TestModelContextContractRejectsInvalidSessionStart` |
| 100K 边界、600K 可保存、失败无状态副作用 | `TestModelContextContractManualFloorCannotBeBypassed` |
| 两个供应商同名模型分别 pending/确认 | `TestModelContextContractIndependentPendingModels` |
| 取消不学习 | `TestModelContextContractCanceledRecoveryDoesNotLearn` |
| 另一个会话手动更新胜过迟到探测 | `TestModelContextContractLateProbeCannotOverwriteManualEdit` |
| 新会话、0 控制值复用模型；另一模型仍为 500K | `TestModelContextContractZeroFollowsSavedModelAcrossSessions` |
| pending/confirmed 数值、低容量警告、稳定事件 ID | `TestModelContextContractEmitsPendingAndConfirmedChanges` |
| 第一轮超限，前缀压缩保留新历史，其他模型成功不确认 | `TestReAct_FirstOverflowShrinksPrefixAndLearnsAfterSuccess` |
| 历史成功边界恢复 | `TestReAct_OverflowCompressionUsesLastFittingBoundary` |
| 压缩成功但业务持续超限，仍按预算退出 | `TestReAct_CompressionSuccessCannotResetOverflowBudget` |
| 永久超限在预算内终止且不落库 | `TestReAct_PermanentOverflowIsBoundedAndNeverLearns` |
| 有/无上限证据、未知上限成功修正、无进展及 4K 边界 | `prompt_overflow_limit_test.go` |
| 供应商错误识别、普通错误不压缩 | `prompt_overflow_test.go`、`reactloopstests/prompt_overflow_retry_test.go` |

前端组件测试另约束：拖动松开提交、取消不提交、键盘下限、超过 500K 警告、后端更新移动阈值线、通知去重、历史回放静默、模型身份随热更新发送、旧会话默认设置入口迁移。

这些测试使用可控供应商回调与实际 ReAct/Timeline/数据库路径，验证恢复机制与路由隔离；不能代替真实模型长任务的答案质量评测。
