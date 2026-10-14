# 26. promoted 状态参与压缩：任务指导文档

状态：待实施。本文件对照 `rookie/enhance/skill-merge-to-timeline` 分支的当前实现修订（Skill 已落地 metadata/content 分离与行范围模型）；文档中的后续压缩设计尚未落地。

前置文档：[24-timeline-runtime-context-unification-task-guide.md](./24-timeline-runtime-context-unification-task-guide.md)、[25-timeline-runtime-context-unification-progress.md](./25-timeline-runtime-context-unification-progress.md)。它们解决内容归属与冻结转移，不代表所有 promoted 内容已经可以压缩。若其中仍有早期方案措辞，现状以本文件第 2 节及对应代码、测试为准。

本文件先明确当前边界，再给出最小的后续实施范围：先开放 recent-tool-cache 的聚合候选；skill、evidence 和 user-input 暂时保持精确。Skill/Evidence 的后续开放条件见第 4 节，不作为本期已确定交付。实施时可另立 `27-timeline-promoted-state-compression-progress.md`，只记录实际完成项与验证结果。

## 1. 目标与确定的约束

目标：让允许淘汰的 promoted 正文复用现有“挑选 ID + 文本总结”机制，不因已经进入 Semi1 就永远避开压缩。统一的是压缩事务，不是把所有 kind 无差别改成可删除历史。

确定的约束：

1. 复用主循环 `compressTimelineBeforePrompt` → `Timeline.CompressBeforePrompt` → 快照 → 至多一次总结流程 → `commitCompressionSnapshot`，不新增第二套压缩器、后台任务或按类型摘要 head。只有精确 delta、没有待总结历史的事务仍可只冻结、不调用 AI。
2. 系统决定哪些 kind 可以进入候选，AI 只决定候选的保留结果。未知 kind 默认精确保留，调用方不能通过 payload 标注自行解除保护。
3. journal 是权威来源，`promotedState` 是已冻结内容的物化投影。压缩请求使用脱离锁的快照；prompt 不从 manager 重建正文，也不直接使用带 Open 叠加的有效视图填充 Semi1。
4. 原文退休、冻结成员、promoted 重建与摘要在同一 Timeline 写锁内发布。先完成所有校验与退休计划，再做不会失败的本地提交；退休必须早于 promoted 重建。
5. 不改变六段 prompt 的位置和过滤规则。新内容先 Open，只有成功提交冻结后才进入 Semi1；请求标注不进入决策 prompt。合法冻结或退休可以改变前缀，不能承诺“压缩后缓存永不失效”。
6. 不把文本摘要当可执行 Schema，不把压缩选择默认为 evidence 业务删除，不恢复已淘汰正文来抵消压缩结果。

## 2. 当前分支基线（已实现，不是后续目标）

### 2.1 压缩集合、参考材料与触发统计

以下行为分别由 [timeline_compression_snapshot.go](../../../aicommon/timeline_compression_snapshot.go) 的 `captureCompressionSnapshotLocked`、[timeline_compression_summary.go](../../../aicommon/timeline_compression_summary.go) 的 `renderCompressionSummaryPrompt` 和 [timeline_compression_before_prompt.go](../../../aicommon/timeline_compression_before_prompt.go) 的 `CompressBeforePrompt` 决定。

| 内容 | 快照与压缩请求 | 当前可以因未选中而退休吗 |
| --- | --- | --- |
| 普通任务历史 | 进入 `snapshot.Items`，按 ID 展示，接受 `ratain_timeline_item_range` 选择 | 可以 |
| `skill`，包含技能目录 | 只进 `ExactItemIDs`；类型化正文不进入总结请求，也不写入 summary | 不可以 |
| `recent-tool-cache` | 只进 `ExactItemIDs`；类型化 Schema 不进入总结请求 | 不可以 |
| `session-evidence` | 只进 `ExactItemIDs`；当前有效值另以 `SESSION_EVIDENCE` 供核对，不是候选 | 不可以；显式业务删除与自身预算淘汰是其他路径 |
| `user-input` | 只进 `ExactItemIDs`；以独立用户上下文供参考，不是候选 | 不可以 |

“出现在总结请求中”“进入原文选择集合”“参与触发统计”是三个不同概念。独立参考材料不是可退休候选；普通历史自己引用的技能名或工具结果也不会因此变成 promoted 候选。

当前触发统计为 `MeasureTokens(snapshot.InputText + pending)`：

- `InputText` 是旧摘要加普通候选正文。
- `pending` 遍历 `ExactItemIDs` 中 `id > FrozenThroughID` 的全部条目，渲染其 Open 文本；包括 Skill、Evidence、工具缓存与用户输入，不是只统计 Skill/Evidence。
- 已冻结的精确状态不计入这项触发统计；这也意味着当前阈值不是整个模型 prompt 的总预算。
- 只有待冻结的精确内容、没有普通历史或旧摘要时，可以提交 freeze/promote 而不调用 AI；已有旧摘要时不能一概声称只冻结。

默认 `TimelineContentSizeLimit` 已是 `300 * 1024`，单位是 `MeasureTokens` 的 token 计数，不是字节（[config.go](../../../aicommon/config.go)）。冻结分桶使用的字节预算是另一项指标，不得混淆。

### 2.2 Skill 写入、聚合与渲染

入口：[timeline_skill_content.go](../../../aicommon/timeline_skill_content.go)、[timeline_promoted.go](../../../aicommon/timeline_promoted.go)、[prompt_materials.go](../../../aicommon/prompt_materials.go)。

固定 Skill Inventory 与下述运行时 skill item 不同：它在 manager 初始化时生成，只有有限的名称与简短描述，在 FrozenBlock 紧跟 Tool Inventory 渲染；不包含加载状态、不随 loader 变更刷新、不进入 Timeline 或压缩快照。下面所说的“技能目录”指运行时意图匹配的推荐目录，不是这份固定 inventory。

- 所有加载方式共用 `skill` kind，但技能被拆成两类 item：`metadata`（说明、关联技能、文件树）与 `content`（一个文件的一段行范围）。资源读取、翻页追加新的 content item，不替换旧页。`[source: user-forced]` 只出现在 metadata，不参与正文身份。
- content 携带 `range: {start_line, end_line, total, reached_end}`；`end_line == total` 时渲染 `END`，明确告诉模型该文件已经翻完。
- 写入去重比较当前仍存活的 part 身份与正文：metadata 按技能名，content 按“技能名 + 路径 + 起止行”。同一范围内容相同则丢弃，内容不同则拼接；不同范围即使文本相同也分别保留。去重不在压缩或渲染时补做。
- 未冻结时由 `timelineSkillContentOpenPrompt` 渲染到 Open：metadata 与各文件范围各自一条，使用 `[skill <name> | metadata]` 与 `[skill <name> | <path>:<start>-<end>/<total> END?]` 标签，不重复技能标题和文件树。
- 冻结后，`rebuildPromotedStateLocked` 通过 `appendPromotedSkillContent` 按 part 身份聚合：同一 part 的 hash 相同则忽略，不同则拼接。`renderPromotedSkillContent` 再按技能名分组展示，组内 metadata 在前，其余按路径和范围排序，输出 Semi1 单一技能块。
- `PromotedTimelineEntry.Payload` 是单个 part 的展示文本，`PayloadHash` 是该 part 最后一次追加的来源 hash。journal key 与 promoted part key 都是 part 身份；按技能名分组只发生在渲染阶段，不改变聚合粒度。
- `renderPromotedSkillContent` 只读取已冻结的 `promotedState`，按技能名分组后输出。主循环与 helper 经 `RenderTimelineFrozenOpenWithOptions` → `PromptFrozenOpenMaterials` → `PromptMaterials` 共用这一材料链路；Open 与技能块在同一读锁内采集。
- [aiskillloader/timeline_binding.go](../../../aicommon/aiskillloader/timeline_binding.go) 的 `BindTimelineSink` 只切换引用，不加载、不补写 Timeline。Manager 是 loader/viewer，不是 prompt 正文来源；本任务不增加第二个读写接口。
- 当前只有显式 `DeleteSkillContent` / unload 等移除操作才会使 Skill 原文退出上下文，之后可重新加载。压缩不会淘汰 Skill，不能写成“被总结后重读”。

当前 [compression_instruction.txt](../../../aicommon/promptloader/prompts/ai/aid/aicommon/prompts/timeline/compression_instruction.txt) 已明确技能正文是请求外的精确状态，没有 Skill 可选候选的规则。进入 `promotedState` 不等于进入压缩候选。

### 2.3 事务、退休与恢复边界

- [timeline_compression_transaction.go](../../../aicommon/timeline_compression_transaction.go) 当前提交顺序是：验证快照 → 计算结果与批次 → 发布 freezeState → 退休未选中的普通 item → 重建 promotedState → 更新摘要。这些本地变更都在同一写锁内；“先退休、后重建”的顺序可复用，但不是 Skill 压缩已经完成的证据。
- [timeline.go](../../../aicommon/timeline.go) 的 `retireTimelineItemLocked` 在内存中标记 `deleted`，不修改原始 payload、不复用 ID。[timeline_marshal.go](../../../aicommon/timeline_marshal.go) 保存时省略已退休条目，恢复时按 journal 与冻结水位重建 promotedState。
- 因此，退休不是持久化的删除事件，也不保证任意原文可恢复。`HasSkillContentHistory` 能看到当前容器内的已退休记录，但不能仅凭它保证保存/恢复后仍知道“该技能曾被 unload”；旧名单导入边界需要专项验证。
- 显式删除已冻结 Skill 虽不回滚 freezeState，仍会重建并改变技能块。冻结元数据不变不等于前缀字节不变。

## 3. 本期设计：recent-tool-cache 聚合候选

### 3.1 最小策略与范围

| kind | 当前行为 | 本期目标 |
| --- | --- | --- |
| `recent-tool-cache` | exact | 按 key 的 retirable 候选 |
| `skill` | exact | 不变，所有来源及技能目录继续精确保留 |
| `session-evidence` | exact，作为核对材料 | 不变，不授予总结器业务删除权 |
| `user-input` 与未知 kind | exact | 不变，不进入选择集合 |

这里只需要系统侧的候选判定与请求中的类型/重要性提示。可以使用小函数集中判断，不强制新增通用 `PromotedRetentionTier`、持久化 policy 字段或 pinned/protected/retirable 状态体系。

本期淘汰的是 Timeline 中的工具 prompt 缓存，不是卸载工具实现，也不自动驱逐 ToolManager 的运行时 LRU。后者与 prompt journal 是不同用途的状态，不能把“从 Semi1 移除”写成“工具不可调用”。

### 3.2 快照：一个有效 key，一个候选

在快照水位 `ThroughID` 内重放工具 journal，取得包含待冻结 delta 的有效状态。只为仍有效的 key 创建候选，最终为 delete 的 key 不作为可选择 Schema。

每个候选至少需要：

- `representativeID`：该 key 在本水位内最新存活的 journal item ID；复用真实 ID，不分配合成 ID。
- `PromptText`：该 key 的完整有效工具描述与参数 Schema，加请求专用标注；不是单条 REUSE 文本或逐条历史版本。
- 快照内的映射：代表 ID → key 与该 key 在水位内的全部存活 journal ID，包含 upsert/reuse/delete。它只用于保留展开与退休计划，不持久化为第二套状态。

代表条目进入 `snapshot.Items` 并按 ID 排序；同 key 的其余 journal 成员不作为独立选择对象。候选链从 `ExactItemIDs` 移出，其他精确类型与未开放的控制记录保留原行为；不能让同一链同时被当成“永不删除的 exact”和“未选中可删除的候选”。

冻结批次构造随之调整：精确 ID、保留的普通 ID、保留或取消退休的候选整链 ID，以及 `ThroughID` 水位哨兵。不能照抄现有 `ExactItemIDs ∪ RetainedIDs ∪ {ThroughID}` 而丢掉只由代表 ID 选中的链成员。

注意：

- `effectivePromotedEntries` 自己获取读锁；快照采集既有读锁也有写锁调用路径，不得在其中直接嵌套调用它。可拆出锁内版本，或在锁内做限定水位的重放，并让公开入口复用，避免复制两套聚合算法。
- 候选 Schema 使用与 `renderPromotedRecentTools` 相同的有效 payload，冻结后的渲染规则不变。聚合视图只用于快照，不能在采集时 promote 或混入水位后的写入。
- 已失效 key 的旧操作不以有效 Schema 复活，本期也不额外引入清理这些历史链的策略。

请求标注示意：

```text
# item=310 [promoted][recent-tool-cache][retirable] key=alpha last_used=item-310
<完整工具描述与参数 Schema>
```

### 3.3 选择协议与兼容保护

继续使用 [timeline_compression_output.go](../../../aicommon/timeline_compression_output.go) 的完整 ID 选择和现有 `ratain_timeline_item_range` 字段，不新增第二份退休列表。

- 选中代表 ID：保留该 key 的整条 journal 链，冻结后仍由 promotedState 渲染完整 Schema。
- 有效选择中未选中代表 ID：计划退休该 key 在快照内的整链，不把摘要变成新的 Schema。
- ID 范围只按候选集合解释，不能把范围覆盖到的任意 journal ID 都作为独立选择结果。
- `RetainedIDs` / `RetainedRange` 继续表达 AI 选择的普通 ID 与代表 ID；freeze 成员另外展开。`RetiredIDs` 记录实际退休的完整 journal ID，不只记录代表 ID。

现有解析器会把缺失、非字符串或非法 range 降为无选择。普通历史的既有兼容语义不在这里重做，但**不能让这种降级顺带删除全部工具缓存**：

- 解析时区分“有效空字符串”和“缺失/非法选择”，必要时在 detached output 中增加一个内部有效性标记；流式输出、完成回退与结果复制路径都要保留这个区别。
- 有效空字符串表示 AI 明确认为候选都可概括；缺失或非法字段则对 promoted 候选全部保留，仍可提交可用的普通历史摘要。
- 摘要/选择校验失败、取消、提交前超限或快照失效时，沿用现有整笔不提交的规则。记忆尾流仍遵循既有语义：非 `FinalizeMemory` 路径允许先提交可用摘要，后到的记忆错误不能倒推撤销已提交的 Timeline。

### 3.4 并发与原子提交

提交时在同一写锁内依次执行：

1. 调用 `validateCompressionSourceLocked`，验证旧 head、freeze version 与水位内原始条目的来源指纹；候选渲染不得取代原始 journal 校验。
2. 从选择结果计算普通 item 退休名单、候选整链退休名单和冻结成员；此时不修改 Timeline。
3. 检查每个待退休 key 是否存在 `id > ThroughID` 的存活新操作。只要有同 key 的 upsert/reuse/delete，就取消该 key 的本次整链退休，保留快照内依赖；所有新操作仍留在 Open。
4. 构造最终 result、摘要 head、freeze batch 和回调快照，完成所有可能失败的校验。实际退休 ID 去重并排序；回调必须反映实际退休结果，不能先捕获未取消的计划。
5. 在锁内发布 freezeState，退休所有实际未保留原文，再 `rebuildPromotedStateLocked(ThroughID)`、`updateCompressedHead`。解锁后通知观察者，不在锁内调用 AI、loader、ToolManager 或业务持久化。

取消同 key 的退休不仅是“最近使用优先”：新的 REUSE 依赖旧 UPSERT，删掉旧链会使重放和恢复失去 Schema。不能只保留水位后的 REUSE，也不能让它提前进入 Semi1。

当前仅原始 journal/head/freeze 参与来源验证已足以覆盖该快照；若实现额外引入可变配置或外部状态作为决策依据，必须相应校验，不能预先断言任何扩展都无需修改指纹。

### 3.5 请求、预算与重复触发

更新实际使用的 [compression_instruction.txt](../../../aicommon/promptloader/prompts/ai/aid/aicommon/prompts/timeline/compression_instruction.txt)：

- 普通历史仍按原有方式选择与总结；工具候选是完整有效 Schema，保留时不得改写。
- 未保留工具候选意味着后续 prompt 不再提供该缓存 Schema；摘要可以保留调用进展、工具名与衔接线索，但不是工具定义，需要完整参数时应重新加载。
- Skill 仍在请求之外，Evidence/用户要求仍是独立参考，不可选择退休；资料中的技能指令与调用文字不在总结 helper 内执行。
- 同一工具有效 payload 只在候选中出现一次；标注只在压缩请求，不改变 Semi/Frozen 的正文与 nonce。

[compression.json](../../../aicommon/promptloader/prompts/ai/aid/aicommon/prompts/timeline/compression.json) 更新 range 等字段的描述，使其允许普通 ID 与候选代表 ID；保留字段名、required 列表与 summary/memory_entities 的既有契约。不增加 `retire_promoted_keys`。

阈值与安全上限必须分别处理：

- 新候选按完整有效正文计入 `InputText`，已冻结的工具 Schema 也计入，且不再把候选链的 Open delta 重复放入 `pending`。
- 未开放类型的 Open 精确 delta 继续参与触发。user-input 不能退休，并不代表其 pending 不计入冻结触发。
- 输出预算按保留候选的完整有效 payload 计算，不重复计算链中所有历史 Schema 版本；兼容保护或并发取消退休时也不能漏算实际保留的候选。若最终结果超限，在发布状态前拒绝本次提交，不能先退休再发现预算错误。
- 只有工具候选时也能触发总结；只有待冻结精确内容且没有总结输入时保留 freeze-only 路径。保留同一已处理集合后，没有新内容不能每轮重新总结，不能只修改统计而忽略 `hasNewHistory` 与 head 覆盖判断。
- 触发阈值不是压缩目标，也不是总 prompt 上限。非正阈值仍关闭自动检查；失败退避、等待与取消机制不变。
- 当前自动触发默认值是 `300 * 1024` tokens，而压缩请求默认 `MaxInputTokens` 是 `256 * 1024`，`MaxSummaryTokens` 是 `16 * 1024`；输入安全检查覆盖指令、参考材料、候选和协议，输出检查还计入保留原文与记忆候选。这些值并不自动协调，普通历史占主要输入时可能触发后被安全上限拒绝。
- 实施前需明确阈值与辅助模型容量的配套策略并测试边界；不在本任务里悄悄回退 300K，也不截断输入绕过安全检查。新增候选不等于可以忽略完整请求的 token 预算。

### 3.6 恢复、自愈与分支

复用 [timeline_toolcache_runtime.go](../../../aicommon/timeline_toolcache_runtime.go)：

- 退休整链后，保存/恢复不得从旧 promotedState 把该 key 复活；未退休 key 的 payload 与相对排序应保持，Open 的新 delta 仍在 Open。
- ToolManager 的 LRU 即使仍有该工具，下一次记录使用或显式预加载也必须与目标 Timeline 比较。缺少对应 journal 时写完整 UPSERT；`hasToolCacheBeforeLocked` 校验 REUSE 失败时已有全量 UPSERT 回退。
- 这证明的是重新记录缓存可自愈，不保证模型仅凭摘要就能构造参数。测试应覆盖“退休 → 再次记录/加载 → 完整 Schema 在 Open → 冻结后回到 Semi1”。
- fork 内退休不能直接修改父 Timeline；[timeline_fork.go](../../../aicommon/timeline_fork.go) 的 MergeBack 不传播继承条目的 `deleted`。非 sub-agent 阶段合并新增链仍可能改变父级同 key 状态，不能概括为“合并永不影响父缓存”。
- 子 Agent 不调用 MergeBack，继续只通过既有结果通道返回结果。Evidence 的 session 同步不在这里改变。
- `TruncateAfter` 保持摘要覆盖范围内无法精确回滚的规则；退休工具链不会凭文本摘要恢复历史 Schema。

## 4. Skill / Evidence 后续开放条件（本期不实施）

### 4.1 Skill：不能把当前精确保留写成逐条压缩已完成

后续若明确开放 Skill 压缩，才把原始阅读 item 按真实 ID 放入候选，并通过退休原始 item 后重建累计技能块，使选择结果决定展示内容。不能把同名技能的累计文本当一个原始 item，也不能因未选中某一资源页而删除整个技能名。

需要另行明确并验证：

- 技能正文、资源页、目录共用 kind，候选粒度及目录处理不能只凭 kind 混为一谈。
- 用户指定来源只作重要性提示，不引入强制不可压缩特例；此时才把 `[source: user-forced]` 加入候选说明。
- 去重仍在加载时完成，不在压缩时补做版本清理；保留项冻结后仍按 part 身份（技能名、路径、行范围）聚合、渲染时按技能名分组，不建立 key+hash 的第二套物化状态。
- 退休后可见性判断、显式重读与旧名单导入都服从 Timeline；需要保存/恢复专项测试，尤其是删除记录被序列化省略后的历史识别。
- 不新增 SkillManager 正文容器、独立压缩器或渲染旁路。

这些是开放前的条件，不是授权本期改变 `ExactItemIDs` 中的 Skill 语义。

### 4.2 Evidence：正文退出 prompt 不等于业务事实被删除

当前 [timeline_evidence.go](../../../aicommon/timeline_evidence.go) 的 `evidenceStoreLocked` 从 journal 重放业务状态，[config_evidence.go](../../../aicommon/config_evidence.go) 同步会话与 DB 镜像。直接退休 upsert 会影响账本；追加 `EvidenceOperation{Op:"delete"}` 同样会真的删除业务证据，不只是减少 prompt。

后续必须先选择并明确一种语义：

- 只减少模型可见的 Evidence 全文：业务账本继续保留，需要明确展示保留状态如何存储、恢复、响应后续修订与 session 同步；当前实现尚无这一分离机制。
- 真正删除业务 Evidence：必须由明确业务策略授权，并走既有业务删除、同步和持久化路径，不能由一般压缩未选中隐式授权。

不能用“summary 包含 evidence ID”或 AI 给出 reason 证明信息等价。字符串命中不证明定位、精确值、依据和业务可读性都得到保留；也不能把 DB 镜像当成删除后必然可重建的独立档案。

若后续确需扩展输出协议，再设计可选字段、白名单与保守校验；本期不预埋 `retire_promoted_keys` 或空实现。业务路径还涉及 session 锁 → Timeline 锁顺序、可能产生 `id > ThroughID` 的新 delete op，以及数据库失败；不能在当前提交锁里调用自行取锁的 `applyEvidenceOperations`，也不能仅凭一个 Timeline 写锁声称业务与 DB 已原子提交。

## 5. 实施顺序与验收

### T1：固定基线与候选边界

- 保留 Skill/Evidence/user-input 的精确断言与 freeze-only 用例。
- 为工具聚合候选写失败测试：同 key 多次 upsert/reuse 只显示一个有效 Schema，已 delete 的 key 不成为候选；未知 kind 保持 exact。
- 确认 300K 触发与完整压缩请求安全上限的配套策略，记录默认与调用方覆盖行为。

### T2：聚合快照与选择解析

- 增加快照内代表 ID 到 journal 链的最小映射，候选与 exact 分类互斥。
- 复用锁内重放，固定完整 payload、ID 顺序和来源水位。
- 区分有效空选择与无效/缺失选择；补全 range、输入统计、输出安全预算测试。

### T3：提交退休与冻结转移

- 展开保留/退休整链，补同 key 水位后新写入的取消规则。
- 全部校验后，在同一写锁内退休、冻结、重建 promoted、发布摘要；回调反映实际结果。
- 测试真正的最终 prompt，不只检查 result：淘汰 Schema 消失，保留 Schema 完整，新 delta 留在 Open，Skill/Evidence 不变。

### T4：指令、兼容与恢复回归

- 更新 instruction 与 schema 描述，保留协议字段与普通历史的兼容行为。
- 固化保存/恢复、REUSE 依赖与 UPSERT 自愈、fork 隔离、回滚、`ExcludeToolCache` / `UserInputOnly` 过滤。
- 调整现有 `TestTimelineToolCacheSurvivesHistoryCompression`：选中或无有效选择时仍保留；另增合法未选中时退休的对照，不删除 Schema 保真断言。
- 保留失败、取消、超限和过时快照的无部分提交断言；不变集合不重复自动总结。

### T5：文档与回归交付

- 本期使用已有 `RetiredIDs` 与快照映射说明结果，不强制增加公开 `RetiredPromotedKeys`、Reason 字段或 UI 协议。确有消费者需求时再单独扩展。
- 更新实施进度与被本期改变的现状说明；明确 Skill/Evidence 压缩仍未实施。
- 报告实际执行的测试，不把本文件中的计划用例写成已通过。

## 6. 测试矩阵与执行入口

以下表格中的“新增”是后续要求，不代表当前分支已有支持。

| 范围 | 必须断言 | 优先扩展的现有文件 |
| --- | --- | --- |
| 当前 Skill 边界 | 不进候选/请求；metadata/content 分离；范围与 END 正确；同一路径不同范围分别保留；pending 可只冻结；压缩前后聚合正文不变 | `timeline_skill_content_test.go`、`timeline_skill_content_append_test.go`、`timeline_skill_promoted_test.go`、`timeline_skill_range_test.go` |
| Skill 统一材料与缓存 | 冻结前只改 Open；同锁快照无转移缺口；manager 绑定不补写；helper 同源 | `timeline_skill_content_prefix_test.go`、`timeline_skill_binding_test.go`、`aireact/prompt_skill_timeline_test.go` |
| 工具候选（新增） | 一个有效 key 一个真实代表 ID；完整 Schema；整链映射；exact 与候选不重叠 | `timeline_compression_snapshot_test.go`、`timeline_compression_toolcache_test.go` |
| 选择与预算（新增） | 选中保留整链；合法未选中退休；缺失/非法字段保留工具，流式与完成回退一致；输出预算包含实际保留的完整正文 | `timeline_compression_output_test.go`、`timeline_compression_summary_test.go`、`timeline_compression_stream_test.go` |
| 并发与提交（新增） | 同 key 新 REUSE/upsert/delete 取消退休；不同 key 不干扰；水位内修改拒绝；结果/回调/实际渲染一致 | `timeline_compression_transaction_test.go`、`timeline_callbacks_test.go` |
| 触发与失败（扩展） | 仅工具候选也可触发；精确-only 路径不退化；无新内容不重复；完整请求超限保留源 | `timeline_compression_before_prompt_test.go`、`timeline_compression_transaction_test.go` |
| 自愈与恢复（新增） | 淘汰 key 不复活；其他 key payload 与顺序保持；重新记录补 UPSERT；新写入先 Open 后 Semi1 | `timeline_toolcache_runtime_test.go`、`timeline_compression_restore_test.go` |
| Evidence 与用户保护 | 有效 store、Semi 与镜像语义不变；参考材料不被 range 退休；pending 统计不漏 | `timeline_compression_evidence_test.go`、`evidence_timeline_test.go`、`timeline_user_input_test.go` |
| 分支、回滚与过滤 | fork 退休不直接污染父；覆盖范围内回滚拒绝；helper 过滤和 replay 规则不变 | `timeline_fork_test.go`、`timeline_compression_fork_test.go`、`timeline_toolcache_test.go`、`timeline_compression_persistence_test.go` |

从仓库根目录执行的基线与后续验收命令：

```sh
go test ./common/ai/aid/aicommon -run 'Test(SkillContent|SkillBinding|PromotedSkill|ForcedSkill|ExactSkill|SkillPrompt|TimelineCompression|TimelineToolCache|TimelineEvidence|EvidenceSurvives)' -count=1
go test ./common/ai/aid/aicommon/aiskillloader -count=1
go test ./common/ai/aid/aireact -run 'Prompt|Skill|Evidence|Timeline' -count=1
go test ./common/ai/aid/aireact/reactloops/... -count=1
go test -race ./common/ai/aid/aicommon -run 'Test(Skill|PromotedSkill|TimelineCompressionTransaction|TimelineToolCache)' -count=1
go build ./common/ai/aid/...
```

新增用例需要核对筛选正则确实匹配；完整交付再运行 `go test ./common/ai/aid/aicommon/... ./common/ai/aid/aireact/... -count=1`。使用 mock 与必要的本地 SQLite，不依赖真实模型。`YAK_TIMELINE_COMPRESSION_EXAMPLES_DIR` 可导出请求样例供人工检查，但不是正确性的替代证据。

## 7. 不变量与不做项

不变量：

- journal payload 不改写，ID 不复用；退休以 `deleted` 标记并在保存时省略，不声称原文仍可持久恢复。
- 工具候选只在快照水位内整链退休；水位后的引用不能失去依赖，不能提前冻结。
- 真正保留精确的 kind 不因字段缺失、范围解析降级或通用 kind 分支被误删。
- 失败不部分发布 Timeline 状态；内存事务原子性不等于数据库与外部 manager 的跨组件原子性。
- 冻结前的新内容与派生信息不进入缓存前缀；合法提交后前缀允许预期变化。
- 保留的 Schema 是完整原文，摘要不承担执行定义；Evidence 业务状态不因压缩而改变。

不做项：

- 不新增第二套压缩器、按类型摘要 head、固定压缩比、后台或写入/渲染时压缩。
- 不引入通用对象生命周期、可配置评分体系、冗余接口或没有消费者的观测协议。
- 不在本期开放 Skill/Evidence/user-input 压缩退休，不改变 manager 与 Timeline 的正文归属。
- 不重排六段 prompt，不修正 `ratain_timeline_item_range` 的既有拼写。
- 不以恢复、重新加载或直接改 promotedState 的方式偷偷抵消已经提交的退休结果。

本期完成标准：普通历史继续使用原机制；工具有效 Schema 成为可选择的聚合候选，选择结果真实支配冻结后的工具块；并发、失败、恢复和再次加载不破坏这一结果。Skill/Evidence 精确状态与现有冻结转移全部保持，不能将“统一容器已完成”夸大为“所有运行时内容均已可压缩”。
