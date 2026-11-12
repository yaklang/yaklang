# 模型上下文行为契约

## 数据归属与单位

所有数值以 token 计，UI 的 1K = 1,000 tokens，1M = 1,000,000 tokens。`provider + model` 共同确定模型身份；不同供应商的同名模型、质量模型与轻量模型互相隔离。数据保存在 profile 数据库独立的 `ai-model-context:*` 元数据中，不改动路由配置或 API 参数。

- 未知模型：容量与压缩阈值均为 500K（500,000 tokens）。
- 升级单位换算时不迁移或缩放已存储的整数 token 数；旧手动阈值、已学习阈值及会话恢复参数保留原值。例如旧的 512,000 tokens 现在显示 512K，只有主动恢复默认才改成 500,000。
- 已知模型：新会话、恢复会话使用该模型保存的阈值。前端启动参数传 `RuntimeContextSizeLimit=0`，不再从全局设置复制 500K 或其他数值。
- 容量是模型的能力证据；阈值是压缩触发点。用户将阈值设成 600K 不证明模型容量达到 600K。
- 旧客户端显式传入非零启动阈值仅作为未知模型的兼容初始预算；一旦存在模型记录，该记录优先于旧会话上限。新 UI 在会话运行时使用带模型身份的热更新；会话开始前通过 `GetAIModelContext` / `SetAIModelContext` 读取、保存同一份模型元数据。SDK 内部 `WithTimelineContentLimit` 的小预算不属于用户输入。

## 手动边界

全局手动下限为 `100 * 1000` tokens。存储 API、运行时热更新、会话启动入口均验证下限；负数及 `(0,100K)` 被拒绝。0 是“跟随模型记录”的控制值，不是 0 token，也不清空已学习容量。被拒绝的热更新不改运行时、模型元数据或恢复参数，并发送包含 `error` 的当前状态。

UI 只在拖动结束时提交，指针拖动和键盘操作都不能提交低于 100K 的数值。服务端发现真实容量不足 100K 时可以自动使用更小阈值并告警，不能为通过手动输入校验而伪造容量。手动输入和滑条使用相同上限（当前默认右端为 1M），回车或失焦提交均不能越过该上限；接口、运行时热更新及手动持久化路径也拒绝越界，不允许一次非法设置扩大自己的上限。超过 500K 允许保存；超过 750K 时 UI 显示不含具体边界数值的“过大上下文”提示，并保持面板高度不变。

新建会话页的模型选择器旁始终显示上下文调整入口。独立设置接口必须携带非空 provider/model，不创建会话，不调用模型，也不修改全局路由。`SetAIModelContext` 的阈值编辑只接受至少 100K 的明确阈值（包括 0 在内的更小值均拒绝）；仅切换模式时可传 0 并显式携带 `DisableAutoAdjust`，此时保留原阈值，包括已学习的不足 100K 阈值；保存阈值不扩大模型容量。查询可返回已学习的不足 100K 容量及对应告警。接口失败时 UI 保留已确认值，不能把错误显示成新的 500K 默认值。

## 自动调整开关

开关按 `provider + model` 保存，默认开启，旧记录缺少字段时也视为开启。关闭后仍按手动阈值执行正常 timeline 压缩，但模型一旦拒绝上下文过长，本次任务立即报错停止：不得自适应收缩、不得进入压缩重试或外层循环重试、不得改写阈值。关闭期间的迟到探测成功不得学习；重新开启后允许新的自适应恢复。

开关始终展示在压缩阈值滑条下方，不能被“大上下文”警告替换。保存静默，失败或超时恢复已确认状态；阈值编辑不能隐式重置开关。`DisableContextAutoAdjust` 仅用于带模型身份的运行时热更新，不复制为每个会话的独立设置。

## 自动恢复状态机

1. 仅明确的上下文超限错误进入校准。鉴权、限流、普通 API 错误、取消不改变容量。
2. 报告最大容量 M：候选阈值为 `0.8M`；未报告：候选阈值为失败 prompt 估算 token 数的 `0.6`。候选不扩大当前有效阈值；自动恢复保留 4K 的内部最小预算（内部显式预算更小时遵循更小预算）。
3. 在内存为失败模型保存 pending 证据，立即发 `model_context`，包含变更前后数值。此时不得落库。
4. 优先用最近成功的历史边界压缩。没有成功边界、或该压缩请求也被拒绝时缩短历史前缀；边界之外的新历史保留。单轮最多 2 次重建重试；跨回合最多累计 9 次超限拒绝。压缩维护请求成功不重置拒绝预算，只有正常业务回合成功后才重置，避免“压缩成功 → 业务超限”的无限循环。
5. 只有实际成功响应的 provider/model 能确认对应 pending 证据。身份来自本次响应，不能读取可能已切换的会话配置。质量模型代替轻量模型压缩成功，不会确认轻量模型的容量。匿名 SDK 回调只能在当前 Config 内恢复，不能持久化到其他模型。
6. 未报告最大容量时，成功 prompt 的 token 数作为可用预算的下界，修正过低估计；它是保守的可用预算，不是测得的物理最大容量。
7. 确认成功后落库并发 confirmed 事件。`previous_threshold_tokens` 保留本轮恢复开始前的阈值：500K → 300K → 200K 的恢复成功后，事件表达 500K → 200K。并发已学习结果只能收紧；每次手动设置生成版本，旧版本的迟到校准不得覆盖新设置或生成自动调整通知。新手动设置之后发生的真实超限仍可重新校准，以这次手动值作为新的恢复起点。
8. 持续失败、取消和其他模型成功不落库。后续会话只读取上次成功确认或用户手动保存的结果。

## 前端事件契约

`model_context` 包含 `provider/model/prompt_tokens/context_tokens/threshold_tokens/source/pending/too_small/minimum_threshold_tokens/disable_auto_adjust`。`change` 包含唯一 `id`、`reason`（overflow/confirmed/manual）及 `previous_threshold_tokens/threshold_tokens`。

- `overflow`：阈值线即时更新，模型行展示紧凑的“自动调整中”标记，可悬停或聚焦查看说明；连续重试不逐次弹通知。
- `confirmed`：成功后只通知一次，说明本轮恢复起点与最终阈值，任务继续运行；后续会话复用已保存阈值。未知上限的成功请求可修正阈值线。
- 状态定期重复时 `change.id` 不变；客户端按会话去重，历史同步 `IsSync` 只恢复状态、不弹通知。
- 模型行保留“自动调整”标记，悬停或聚焦解释原因。新会话没有变更历史时只解释当前阈值来自模型响应，不编造原值。不展示模型估计容量。面板关闭时仍能收到成功通知。
- 手动保存静默，立即预览新值并隐藏旧的自动调整标记；只有对应确认才结束待确认状态，旧状态不能让阈值跳回。失败或超时展示错误并恢复已确认值。新的自动调整事件到达时更新阈值与标记。

## 可执行验收

| 行为 | 回归测试 |
| --- | --- |
| 十进制 500K 默认值、100K 下限、1M 上限；旧手动/已学习记录不缩放、不取整 | `TestAIModelContextDecimalUnitsPreserveStoredTokens` |
| 关闭时首次超限即停止，仅一次模型调用、零压缩重试，阈值不变 | `TestReAct_DisabledAutoAdjustStopsOnFirstOverflow` |
| 手动上下限、开关独立持久化、非法组合编辑无副作用 | `TestAIModelContextManualBoundsAndAutoAdjustPreference` |
| 模式热更新使旧探测失效、阈值修改保留开关、重新开启恢复 | `TestModelContextContractDisabledAutoAdjustInvalidatesPendingAndPersists` |
| 无会话开关开/关、显式 false 的 gRPC 往返 | `TestAIModelContextAutoAdjustSwitchBeforeSession` |
| 无会话查询与保存、100K/1M 边界、非整 K 精确输入、模型隔离、100K 小容量告警边界 | `yakgrpc/TestAIModelContextBeforeSession` |
| 启动入口拒绝非法阈值 | `sessionruntime/TestModelContextContractRejectsInvalidSessionStart` |
| 100K 边界、600K 可保存、失败无状态副作用 | `TestModelContextContractManualFloorCannotBeBypassed` |
| 两个供应商同名模型分别 pending/确认 | `TestModelContextContractIndependentPendingModels` |
| 取消不学习 | `TestModelContextContractCanceledRecoveryDoesNotLearn` |
| 另一个会话手动更新胜过迟到探测 | `TestModelContextContractLateProbeCannotOverwriteManualEdit` |
| 多次收缩保留原始设置，成功后落库；新会话复用；手动重设可重新学习 | `TestModelContextContractRecoveryExplainsOriginalSettingAndPersistsSuccess` |
| 新手动设置使旧恢复失效，重新恢复从新的手动值计算 | `TestModelContextContractManualEditStartsANewRecoveryOrigin` |
| 新会话、0 控制值复用模型；另一模型仍为 500K | `TestModelContextContractZeroFollowsSavedModelAcrossSessions` |
| pending/confirmed 数值、低容量警告、稳定事件 ID | `TestModelContextContractEmitsPendingAndConfirmedChanges` |
| 第一轮超限，前缀压缩保留新历史，其他模型成功不确认 | `TestReAct_FirstOverflowShrinksPrefixAndLearnsAfterSuccess` |
| 历史成功边界恢复 | `TestReAct_OverflowCompressionUsesLastFittingBoundary` |
| 压缩成功但业务持续超限，仍按预算退出 | `TestReAct_CompressionSuccessCannotResetOverflowBudget` |
| 永久超限在预算内终止且不落库 | `TestReAct_PermanentOverflowIsBoundedAndNeverLearns` |
| 有/无上限证据、未知上限成功修正、无进展及 4K 边界 | `prompt_overflow_limit_test.go` |
| 供应商错误识别、普通错误不压缩 | `prompt_overflow_test.go`、`reactloopstests/prompt_overflow_retry_test.go` |

前端组件测试另约束：拖动松开提交、取消不提交、键盘下限、超过 750K 警告、后端更新移动阈值线、通知去重、历史回放静默、模型身份随热更新发送、旧会话默认设置入口迁移。`aiPerf.handlers.test.ts` 约束多次重试不弹通知、成功只提示一次、K/M 格式和小容量即时警告；`ModelContextControl.test.tsx` 约束自动调整标记与解释；`ModelContextPopover.test.tsx` 约束手动重设与新自动结果的交接。

这些测试使用可控供应商回调与实际 ReAct/Timeline/数据库路径，验证恢复机制与路由隔离；不能代替真实模型长任务的答案质量评测。
