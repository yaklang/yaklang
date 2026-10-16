# 26. Timeline 全量上下文压缩

实现进度与实际验证见 [27-timeline-promoted-state-compression-progress.md](./27-timeline-promoted-state-compression-progress.md)。

## 1. 判断依据与完成目标

Timeline 的占用是它实际提供给主模型 prompt 的材料，不是原始 journal 大小，也不是辅助压缩请求的大小。

`RenderTimelineFrozenOpenWithOptions` 是共用材料入口：

- 普通 Frozen Timeline（包含旧摘要）；
- 普通及 promoted Open Timeline；
- 已冻结的技能正文、工具 Schema、Evidence、用户输入历史。

模板消费这些字段；压缩检查使用同一返回值的 `PromptText()` 统计 token。主循环传入与组装 prompt 一致的 `IncludeLatestModelReplay`、`PromoteUserInput`、`UserInputOnly` 选项。统计不额外重放有效状态，也不把已经渲染过的正文重复加一遍。

上限与压缩目标不同：

```text
触发：实际占用 >= TimelineContentSizeLimit
完成：实际占用 <= max(1, TimelineContentSizeLimit / 6)
```

历史实现 `a7e888c2b^` 的 `timeline_compression.go` 使用 `currentSize / 6`。本次改为上限的二分之一，并按配置上限计算目标，使突发输入不会抬高压缩后的水位。默认上限仍为 `300 * 1024` tokens，目标为 `150 * 1024` tokens。

非正上限关闭自动检查。未触发时，即使材料大于目标，也不整理、不调用 AI、不改变冻结块。

## 2. 主流程

`CompressBeforePrompt` 负责完整流程：

1. 渲染主模型使用的 Timeline 材料并测量；
2. 未到上限则结束；达到上限后进入本次优化；
3. 调用一次 `foldTimelineWithoutAI`，独立提交不需要 AI 的确定性折叠；
4. 重新渲染，满足低水位目标即可结束，否则构建原始 item 压缩快照；
5. 折叠后仍高于目标时，最多一次模型调用完成本轮的普通历史筛选、摘要及 promoted 黑名单；
6. 原子提交后返回。一次检查不再继续调用模型；仍高于上限时，下一次检查再处理。

触发后由 `foldThenCompressTimelineToTarget` 按“无 AI 折叠一次 → 复测 → 必要时一次 AI 压缩”的顺序推进。整个检查有独立等待通道，多个检查者共享同一进程中的优化进展；AI 事务走 `compressTimelineOnceWithAI`。写入和渲染不会自行启动压缩。

取消和失败退避保持。折叠已经达到低水位就不再请求模型。一次 AI 后仍高于目标不是失败，也不在同一次检查里重试。用户输入等不可删材料本身过大时保留原文，不强删或截断。

## 3. 无 AI 折叠

在已经触发优化之后，`foldTimelineWithoutAI` 只在 AI 循环前执行一次，折叠 journal 中的冗余操作，并将捕获水位内仍存活的内容冻结、重建 `promotedState`。当前有效状态无损不代表所有历史版本仍能逐条回滚。

工具缓存：

- 同 key 的旧 UPSERT 已被覆盖，保留最后一次 UPSERT；
- 保留其后最后一次 REUSE，维持工具使用顺序；
- 最终操作为 DELETE 的 key，可以退休整条失效链。

Evidence：

- 同证据 ID 保留最终 UPSERT；
- 最终 DELETE 的失效链可全部退休；
- malformed/未知操作保守保留。

Skill 是追加阅读语义，不在这里折叠不同正文或不同范围。写入入口已有去重，不再凭“同名”清理技能。

整理和 AI 分别提交：整理后复测已达到目标就不请求模型；模型失败也不撤销已成功完成的确定性整理。退休前通过已有持久化入口保存源，锁外保存后重新校验来源；保存失败不退休。退休、冻结成员、state 重建在同一 Timeline 写锁中发布。

## 4. 压缩候选直接来自 item

`captureCompressionSnapshotLocked` 已经按 ID 遍历原始 Timeline。在已有 promotable 分支中直接分类：

| 类型 | 快照位置 | 选择方式 |
| --- | --- | --- |
| 普通历史 | `Items` | 普通原文保留白名单 |
| tool-cache / skill / evidence | `PromotedItems` | 原始 item 删除黑名单 |
| user-input | `ExactItemIDs` + `UserContexts` | 不可删除，参与实际占用统计 |
| 未知 kind / 未开放操作 | 精确保留 | 不可删除 |

`timelineCompressionPromotedItem` 复用普通快照项的 ID、时间戳、Frozen 标记、完整 `PromptText`，只补充 Kind、Key、Operation 用于提交依赖检查。

一个原始 item 对应一条快照项。没有聚合句柄、没有成员链、没有第二次全集收集，也不从 `promotedState` 反推候选。已经整理过的冗余 item 不会进入下一次 AI 快照。

请求示例：

```text
========== RETIRE_ID 310 ==========

========== RETIRE_ID 310 ==========
kind=recent-tool-cache name=read_file operation=upsert
[UPSERT] read_file
<完整工具描述与参数 Schema>
========== END RETIRE_ID 310 ==========
```

正文全量，不做预览或条数截断。内部技能身份中的 NUL 分隔符不会直接渲染。Evidence 全文已在 promoted 项中出现时，不重复添加 `SESSION_EVIDENCE` 参考块；该字段仍用于旧记忆快照兼容和现有记忆指纹。

固定 Skill Inventory 是初始化目录，不在 Timeline 中，不参与此处的占用或删除；运行时推荐目录 item 则属于 skill。

## 5. 选择和提交

- `ratain_timeline_item_range` 保留既有拼写，只解释普通 item ID：未选中的普通原文可由摘要替代。
- `retire_promoted_entries` 是可选字符串，只接受本次列出的原始 promoted item ID，逗号分隔，不支持区间。缺失、空字符串、非法类型均不删；未知 ID 忽略。
- 未点名的 promoted item 默认原样保留。不能从一个 ID 扩大到整个同名工具、技能或证据。
- Skill 不同资源页独立选择；删除后按剩余原始 item 重建累计技能块。
- Evidence 与其它 item 一样退休，不增加 Config/session 的第二条 post-commit 删除通道。它会改变当前 Timeline 重放出的证据账本。提示词要求按当前进度主动淘汰不再影响下一步的证据、工具和技能；用户输入不可删除。

解析阶段已将删除名单限制在本次快照列出的 promoted item ID 中。提交直接使用该名单，不扩大或取消删除，不按同 key 的 REUSE、DELETE 或水位后新操作调整选择。未点名的原始 item 和水位后的新 item 不退休；删除后按剩余 journal 正常重放。

继续验证原始 journal、旧 head、freeze version，不能用渲染文本替代来源校验。来源在捕获范围内变化时整轮作废，而不是修改单个删除决定。

在同一写锁内，完成校验、结果和 freeze 元数据的构造之后：发布 freeze → 退休指定 item → `rebuildPromotedStateLocked(ThroughID)` → 更新摘要。锁外通知观察者。`RetiredIDs` 包含实际退休的原始 ID。

## 6. 流式响应、预算与失败

有 promoted 候选时，提前发布必须等待删除名单字段；旧模型响应缺少字段时，完成解析后按“不删”回退。摘要、普通选择、黑名单一起成为已发布的选择。发布后尾流变更选择会使记忆尾流失败，不会倒推撤销已经提交的 Timeline。`FinalizeMemory` 保持等待完整结果的既有语义。

仅有 promoted 候选时允许空摘要；没有可决策材料时保留 freeze-only 能力。压缩请求直接给出当前占用、目标、缺口，以及普通历史和 promoted 内容两个可优化部分的大小。目标覆盖整个 Timeline，而不是只约束 summary。

预算分别处理：

- 主上下文触发上限默认 300K；
- 目标为上限的二分之一；
- 辅助请求完整输入默认安全上限为 512K，避免原来的 256K 本地上限先于 300K 触发拒绝；调用方可覆盖，模型/provider 仍可能有更小容量；
- 摘要、保留普通原文和记忆候选保持现有 16K 输出安全检查；
- 不截断输入绕过检查，不把未达目标当成功。

一轮校验、取消、请求/解析或预算失败，不部分发布该轮 AI 结果。此前独立成功的整理或 AI 轮次继续有效。

## 7. 边界与回归

- 保存时省略退休项，恢复按 journal 与冻结成员重建 state；水位哨兵保留，ID 不复用。
- 工具运行时 LRU 不等于 prompt 缓存，退休不卸载工具。再次记录使用时，无对应 journal 会写完整 UPSERT，先 Open 后冻结。
- fork 的退休不直接修改父 Timeline。
- 分支 Evidence 的后续 session 导入、以及全部技能原文省略后的旧技能名单重新导入，仍需独立处理；不能声称本次重构解决了跨组件恢复语义。
- 不重排六段 prompt，不新增后台压缩器或按类型摘要。

验证入口见 27。新增测试应匹配实际正则，不把旧版本测试通过记录当作本次验证。
