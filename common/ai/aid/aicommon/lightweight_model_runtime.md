# 轻量模型运行机制与回归范围

普通分层模式分别配置质量、轻量和视觉模型。网关中的模型名称只是路由标识，不能仅凭名称推断其实际基础模型、上下文容量、质量或延迟。本文描述运行机制与可执行回归测试，不包含本机配置或真实供应商评测结果。

## 调用优先级

| 入口 | 路由与用途 |
| --- | --- |
| `Config.CallAI` | 优先使用已绑定的 Quality callback；不存在时才找 Speed，再找 Original。选中 callback 返回错误后不会自动切到下一档。 |
| `CallQualityPriorityAI` | Quality → Original；主任务推理与上下文压缩。 |
| `CallSpeedPriorityAI` | Speed → Original；不会因为 Speed 失败自动升级到 Quality。 |
| `ScheduleAuxiliaryTask` | 统一辅助任务调度，经 LiteForge 调用 Speed，继承上下文取消、响应校验/重试、事件与用量回调。 |
| `WithUseSpeedPriorityAICallback` | 某些 ReAct 子循环把普通回合交给 Speed，例如意图分析。压缩回合仍交给 Quality。 |
| `GetDefaultAIModelCallback` | 单独的策略入口：performance 选择 intelligent，其余策略选择 lightweight；不等于 `Config.CallAI` 的优先级规则。 |

主要代码：`aitier_callback.go`、`config.go`、`config_auxiliary_scheduler.go`、`aireact/reactloops/exec_call_ai_loop_transaction.go`。

普通分层模式下，LightweightModels 列表第一项被选中；callback 每次调用时读取该档最新模型配置。轻量模型使用自己的 reasoning 配置，并不会仅因为它是 lightweight 就强制关闭 thinking。因此“轻量”是任务路由角色，不保证某种固定延迟、价格或模型规模。

## 哪些功能会调用轻量模型

`auxiliary_labels.go` 与调用点涵盖会话命名、意图关键词和能力推荐、工具调用原因/间隔检查、记忆分类和去重、知识选择与重排、短文本抽取、mini task 的 prompt 优化/TODO 草稿/摘要、目标验收，以及无法确定的上下文超限错误分类。

需要区分 mini task 的 `timeline_summary` 和主 ReAct 的 `compress_context`：前者是轻量辅助任务，后者决定长期上下文保留，使用质量模型。

模糊超限错误分类只把供应商错误字段交给 Speed 判断，15 秒超时，成功判断缓存，阻止递归调用，失败不缓存。它不是在所有 API 错误发生时都盲目压缩。

## 单模型模式

全局或会话启用单模型时，Quality 和 Speed 角色仍存在，但都绑定 intelligent 档模型；Speed 的 LiteCall 将 reasoning 设为 `none`，不会改写 Quality 的配置。此处不再使用单独配置的 lightweight 档模型。

非关键辅助任务被跳过，例如标题、展示文案、部分评估、记忆分类和意图推荐。记忆、深意图、间隔检查和 RAG 增强另有入口开关。未注册为 skip 的必要辅助任务仍运行；超限分类明确保留。模式在会话构造时固定，后续全局模式变化不会偷偷改变既有会话的角色。

## 压缩恢复的路由约束

- 原生 tool-call 的 Speed 循环在压缩模式仍走 Speed，文本模式却走 Quality。已统一两种协议；`TestContextCompressionTransactionEnforcesAction` 现在在 Speed 循环下验证两种协议均只调用 Quality，并保留结构校验/重试。
- 原先使用会话可变模型身份确认容量，可能把 Quality 压缩成功误算到失败的轻量模型。现在使用实际成功响应身份；各 provider/model 独立 pending。集成测试明确验证其他模型成功压缩不确认原模型，直到原模型接受请求才持久化。
- 回退网关或轻量循环与初始配置模型不同时，明确超限会更新当前恢复目标，使重建 prompt 使用失败模型的预算。

## 检查结果与证据范围

以下可控回调测试通过：

- `TestWithTieredAICallbackSingleModelOption`：普通模式确实选 Speed；单模型模式选 Quality 模型但关闭 Speed thinking。
- `TestCustomSpeedCallbackKeepsOptionSemantics`、`TestPR5128_ModeSnapshot_*`：自定义回调、模式快照及子配置继承。
- `TestSpeedLoopAuxiliaryProtocol`：文本/tool-call、响应校验重试、错误、跳过路径。
- `TestAuxiliaryResponseHandlerTransaction`、`TestAuxiliaryCustomCallbackOptionsAreRequestLocal`：LiteForge 实际执行链与请求参数隔离。
- `TestPR5128_AuxiliaryLifecycle_*`、`TestPR5128_SpeedLoop_LifecycleAcrossRounds`：父任务存活期间的成功/失败/取消、清理和跨轮生命周期。
- `TestMiniAITasksUseConfigLiteCall`、`TestGoalAcceptanceUsesAuxiliaryConfig`、`TestPromptOverflowJudgeCachesAndRejectsReentry`：关键辅助入口与错误分类。

这些测试验证路由、隔离和恢复契约，不构成任何真实模型的答案质量或速度评分。首字节和总耗时属于不持久化的瞬时性能事件，历史项目数据库不能作为完整的延迟或成功率评测数据集。

ReAct Speed 循环能使用主循环的压缩恢复机制，但单次 LiteForge 辅助请求本身没有完整 Timeline 压缩状态机：它按事务重试预算及调用点的错误处理结束。不能据此宣称所有轻量辅助任务都能自动适应任意大输入。

后续真实评测应固定同一批抽取、重排、验收样例及输入长度，记录结构成功率、语义正确率、重试次数、首字节/总延迟和实际模型身份，分别比较分层 Speed 与单模型 LiteCall；长上下文恢复测试继续独立约束稳定性。
