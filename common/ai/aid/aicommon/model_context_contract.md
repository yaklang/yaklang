# 主模型上下文阈值行为契约

## 本 PR 的范围

本功能只控制质量优先（主模型）ReAct 的 timeline 压缩阈值：未设置时为 500,000 tokens，用户可调整，原有超限恢复得到的阈值在成功请求后保存。1K = 1,000，1M = 1,000,000。

核心压缩算法、触发判断、历史边界选择、错误分类、单轮重试预算、外层循环错误处理和压缩调用路由保持原行为。没有新增首次失败的前缀压缩、累计 9 次停止条件或基于成功 prompt 长度上调阈值。对于原来无法恢复的输入，本功能不承诺使其恢复。

轻量/速度优先与视觉模型不读取、确认或保存这份主模型设置。SDK 未绑定主模型配置时也走原路径；原 `DefaultTimelineContentSizeLimit = 300 * 1024`、小预算及旧启动/热更新语义不变。主模型默认值单独取 500,000，不全局修改共用常量。主模型与轻量模型即使使用相同物理服务，仍按调用角色区分。

## 保存身份

`provider type + model name` 不足以区分两个 OpenAI 兼容服务。新增模型表单允许手填凭据和地址，`ProviderId` 又可以为空，因此也不能单独用它作为身份。

每条已保存的提供商配置使用内部稳定 `ContextOwnerId`（UUID）作为归属，与可选的旧 API Key 记录 `ProviderId` 分离。新增时分配，编辑或切换模型时保留；两条完全同名、同地址、同账号的配置也各自拥有归属。

后端生成 `<ContextOwnerId>:<SHA-256>` 标识。摘要包括有效厂商类型、模型名、BaseURL/Domain、启用的 Endpoint、API 类型、账号凭据、租户/自定义请求头，以及脚本提供方配置。API Key 与请求头只参与摘要，不明文写入元数据 key 或事件。代理、思考强度、采样参数不参与身份。

- 同一提供商配置下的同一模型跨会话共享阈值和自动调整开关。
- 切换 A → B → A，保留 A、B 各自的设置；重排或移动配置不删除历史模型设置。
- 更换地址、API 类型或账号使用独立设置；之前的设置仍归原提供商配置所有。
- 删除整条提供商配置时，在同一数据库事务内删除该归属下所有模型和连接的缓存；其它配置不受影响。
- 删除后重新添加相同内容会分配新的归属，从默认 500K 开始。
- 迟到的手动保存、成功响应持久化必须检查归属仍存在，并与删除操作互斥；不能重新创建已删除的缓存。
- 数据保存在 profile 数据库的 `ai-model-context:<ContextOwnerId>:*` 元数据中。不增加数据库迁移或旧实验记录兼容逻辑，整数 token 数不缩放、不取整。

配置绑定字段只用于本地设置管理，不发送为上游 API 参数，也不改变主模型、辅助模型或视觉模型的路由。

无会话时 `GetAIModelContext` / `SetAIModelContext` 传完整 `Config`，后端返回 `ContextKey`。之后可传 `Provider/Model/ContextKey`；只传类型和名字会被拒绝。运行时事件返回同一标识，热更新传 `AIService/AIModelName/ModelContextKey`。配置已切换时拒绝旧标识，避免误改另一个同名模型。

## 手动设置

新设置入口下限为 100,000 tokens，默认滑条上限 1,000,000；服务端同时校验。恢复默认明确写入 500,000。模式编辑可传阈值 0 并显式携带 `DisableAutoAdjust`，保留当前阈值；单独保存阈值 0 不合法。自动恢复沿用原内部预算，可以低于手动下限。

新前端启动时传 `RuntimeContextSizeLimit=0`，设置按模型读取，不在每个会话复制 500K。旧客户端显式的启动预算保持兼容；有明确模型设置时主模型读取保存值。无 `ModelContextKey` 的旧热更新仍保留原 SDK 行为。

自动调整默认开启。用户显式关闭后，主模型的上下文超限会停止任务并保留手动阈值，这是明确选择的退出行为；轻量/视觉错误处理不受该开关影响。

## 原有恢复与新增保存

1. 原有错误分类与恢复分支决定是否重建或压缩；普通鉴权、限流等错误不进入阈值学习。
2. 原有数值算法保持：有最大容量证据取 80%，否则取失败 prompt 估算 token 的 60%，保留原内部 `4 * 1024` 下限，不扩大当前阈值。
3. 仅在原分支实际收缩阈值时，按本次质量模型身份保留 pending 状态；失败时不落库。
4. 原分支优先用最近成功历史边界；没有增加截短前缀、跨轮失败计数或停止条件。文本压缩和原生 Speed 压缩原来走哪档，仍走哪档。
5. 只有同一具体配置的成功质量响应才能确认保存。其它配置或辅助调用成功、取消、持续失败都不确认。响应身份贯穿流式 response/tee，而非仅凭会话的可变模型名。
6. 手动修改生成版本号；旧版本迟到的成功不能覆盖新设置。并发成功保存只收紧已有阈值。下次主模型会话直接复用保存结果。

## 前端

`model_context` 提供 `provider/model/context_key/prompt_tokens/threshold_tokens/source/pending` 等状态。自动收缩、成功确认和历史回放均静默更新阈值，不弹通知；只有用户明确设置失败才提示。面板内可以查看自动调整状态，不保存或展示估计容量，也不因自动收缩到手动下限以下新增告警。手动保存静默，拖动释放后提交，失败恢复确认值。

超过 750K 时，面板固定提示区域显示“过大的上下文可能降低模型表现，并增加延迟和费用。”，不写具体警戒数值，不改变面板尺寸。下限、K/M 换算、Drag Handle、恢复默认和默认开启的说明开关保留。

## 回归证据

| 约束 | 测试 |
| --- | --- |
| 同名不同地址/账号/API/租户隔离，独立配置归属隔离，思考强度不影响身份 | `aispec/TestModelContextIdentitySeparatesConfiguredServices` |
| 无会话 Config/ContextKey 往返，已删除配置拒绝读写，手动上下限 | `yakgrpc/TestAIModelContextConfiguredIdentityAndBounds` |
| 切换保留、删除清理、重新添加隔离、并发迟到写入不复活 | `yakit/TestAIModelContextProviderLifecycle`、`TestAIModelContextDeleteAndLateWriteAreAtomic` |
| 运行时边界与过期配置拒绝 | `TestModelContextContractManualBoundsAndIdentity` |
| 失败不落库、匹配成功保存、跨会话读取 | `TestModelContextContractSuccessPersistsOnlyMatchingConfiguration` |
| 取消、手动编辑、关闭开关或删除配置后的迟到成功不覆盖 | `TestModelContextContractLateSuccessAndCancellationDoNotOverwrite` |
| 与原运行时算法逐例比较、原 SDK 默认值 | `TestModelContextContractOriginalReductionFormula`、`TestModelContextContractEventsAndLegacyDefaults` |
| 主模型设置不修改 Speed 默认值/存储，关闭开关只作用主模型 | `reactloops/TestModelContextSettingOnlyControlsQualityLoop`、`TestModelContextDisabledOnlyStopsMatchingQualityOverflow` |
| 文本/原生协议压缩路由保持原有差异 | `reactloops/TestModelContextKeepsOriginalCompressionRouting` |
| 完整文本/原生 ReAct 超限恢复成功后保存，显式关闭首次拒绝即停止 | `aireact/TestReAct_ModelContextPersistsOriginalRecoveryOnlyAfterSuccess` |
| 原成功历史边界、重试行为 | 原 `invoke_overflow_fitting_boundary_test.go`、`prompt_overflow_retry_test.go` |
| 前端自动调整静默、同名配置不串用量/查询、热更新携带标识 | `aiPerf.handlers.test.ts`、`useSavedModelContext.test.ts`、`ModelContextPopover.test.tsx` |

测试使用可控回调和实际 ReAct/Timeline/数据库路径，约束兼容行为，不代表真实模型的答案质量评测。
