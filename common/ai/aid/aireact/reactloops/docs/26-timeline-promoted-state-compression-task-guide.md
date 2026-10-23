# 26. Timeline 全量上下文压缩

实现进度与实际验证见 [27-timeline-promoted-state-compression-progress.md](./27-timeline-promoted-state-compression-progress.md)。

## 1. 判断依据与完成目标

触发依据是完整主循环 prompt 的 token 数，不是原始 journal 大小，也不单独统计 Timeline。

`RenderTimelineFrozenOpenWithOptions` 是共用材料入口：

- 普通 Frozen Timeline（包含旧摘要）；
- 普通及 promoted Open Timeline；
- 已冻结的技能正文、工具 Schema、Evidence、用户输入历史。

模板消费这些字段，再拼入静态指令、任务、TODO 和动态状态。构建完成后统一测量最终 prompt，不重复计算已渲染的正文。

上限与压缩目标不同：

```text
触发：完整 prompt > TimelineContentSizeLimit
完成：实际占用 <= max(1, TimelineContentSizeLimit / 6)
```

当前按配置上限的六分之一计算目标，而不是按当前体积计算，使突发输入不会抬高压缩后的水位。默认上限为 `300 * 1024` tokens，目标为 `50 * 1024` tokens。正常组装和 prompt 模式选择集中在 `generateLoopPromptFromInput`；主循环统一解析并执行 `compress_context`。

非正上限回退到默认值。未触发时，即使材料大于目标，也不整理、不调用 AI、不改变冻结块。

## 2. 主流程

`generateLoopPromptFromInput` 负责构建和模式选择，返回 prompt、成分快照和模式：

1. 正常组装完整 prompt 并测量；
2. 未超过上限则返回正常模式；
3. 超限时直接通过 `RenderCompressionGuide` 读取当前 timeline，只把动态段换成压缩指引和当前任务只读状态，返回压缩模式；不保留快照，也不再单独做无 AI 折叠。

主循环在原有 AI transaction 中解析 action。`compress_context` 没有单独开关，普通轮也可以主动调用。verifier 只检查 action 类型、字段类型和禁止的 TODO 修改；handler 读取当前 timeline 并把 action 展开成 `TimelineCompressionSelection`，Timeline 按应用当时的内容原子提交。

action 只原子提交结果，不再请求 AI。压缩回合不重复写入模型响应；完成后继续循环，从正常组装重新开始。连续压缩仍无有效缩减时明确退出。用户输入等不可删材料保留原文，不强删或截断。

写入、渲染、保存和恢复不会自行启动 AI 压缩。模型报告超长时只降低运行时上限，并复用上述构建流程；旧自动调度器与失败退避状态已删除。

## 3. 压缩候选直接来自 item

`captureCompressionSnapshotLocked` 已经按 ID 遍历原始 Timeline。在已有 promotable 分支中直接分类：

| 类型 | 快照位置 | 选择方式 |
| --- | --- | --- |
| 普通历史 | `Items` | 普通原文保留白名单 |
| tool-cache / skill / evidence | `PromotedItems` | 原始 item 删除黑名单 |
| user-input | `ExactItemIDs` + `UserContexts` | 不可删除，参与实际占用统计 |
| 未知 kind / 未开放操作 | 精确保留 | 不可删除 |

`timelineCompressionPromotedItem` 复用普通快照项的 ID、时间戳、Frozen 标记、完整 `PromptText`，只补充 Kind、Key、Operation 用于提交依赖检查。

一个原始 item 对应一条快照项。没有聚合句柄、没有成员链、没有第二次全集收集，也不从 `promotedState` 反推候选。已经整理过的冗余 item 不会进入下一次 AI 快照。

正文沿用整个主循环上下文，不复制进另一份压缩请求。动态段的普通历史索引使用 ID 和定位片段；技能、证据分别列出可选的技能名、证据 id。定位片段不代替上文正文，也不要求模型复制正文来标识保留条目。工具 Schema 不能淘汰。

固定 Skill Inventory 是初始化目录，不在 Timeline 中，不参与此处的占用或删除；运行时推荐目录 item 则属于 skill。

## 5. 选择和提交

- `retain_timeline` 支持散落 ID 与多个闭区间混合，例如 `"3-8,12,20-26,37,41-45"`；只筛选本轮普通候选，未选中的原文由摘要承接。空字符串表示全部概括。
- `retire_skill_names` 按完整技能名整份淘汰，包含该技能的分页与 metadata。
- `retire_evidence_ids` 按当前观测 id 淘汰证据。
- 未点名的技能和证据默认保留。非法 ID、范围或名字直接报错，由 AI transaction 引导纠正。
- 用户输入、工具 Schema、TODO、任务指令和固定目录不可删除。淘汰技能或证据时，summary 要说明淘汰了什么，便于后续理解和重新加载。

解析阶段已将删除名单限制在本次快照列出的 promoted item ID 中。提交直接使用该名单，不扩大或取消删除，不按同 key 的 REUSE、DELETE 或水位后新操作调整选择。未点名的原始 item 和水位后的新 item 不退休；删除后按剩余 journal 正常重放。

继续验证原始 journal、旧 head、freeze version，不能用渲染文本替代来源校验。来源在捕获范围内变化时整轮作废，而不是修改单个删除决定。

在同一写锁内，完成校验、结果和 freeze 元数据的构造之后：发布 freeze → 退休指定 item → `rebuildPromotedStateLocked(ThroughID)` → 更新摘要。锁外通知观察者。`RetiredIDs` 包含实际退休的原始 ID。

## 6. 流式响应、预算与失败

transaction 完成响应解析后，action handler 才转换并提交摘要、普通选择与 promoted 淘汰名单；只会被校验的 action 输入不会直接改变 Timeline。退出和失败时释放快照占用。

存在普通历史或实际淘汰 promoted 时，摘要不能为空。动态指引给出完整 prompt 的占用、目标和缺口；目标覆盖整份主循环 prompt，而不是只约束 summary。

预算分别处理：

- 主上下文触发上限默认 300K；
- 目标为上限的六分之一；
- 压缩模式完整输入安全上限为 512K，模型/provider 仍可能有更小容量；
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
