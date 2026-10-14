# 25. 运行时上下文统一进入 Timeline：实施进度文档

状态：已完成（T1-T5 全部完成，19 个新增/扩展测试，全量回归通过）。
指导文档见 [24-timeline-runtime-context-unification-task-guide.md](./24-timeline-runtime-context-unification-task-guide.md)。

本文件只记录实际完成情况、验证证据和剩余项。未完成的内容不写入“已完成”。

## 当前阶段

T1-T5 已完成。全部改动通过 `aicommon` 与 `aireact` 全量回归。

## T1：盘点调用方并建立失败测试

### 已完成

指导文档已完整阅读并确认五条设计结论与实施顺序。

调用点盘点结果（生产代码，不含测试）：

Skill 正文直接注入 prompt 的入口：

- `common/ai/aid/aireact/reactloops/prompt.go`：`generateLoopPrompt` 调用 `RenderStable`、`RenderForcedSkills`、`RenderAutoLoadedSkills`，分别进 Semi1、Frozen、Semi2。
- `common/ai/aid/aireact/prompts.go`：`renderSkillsContextForLoop` 调用 `mgr.RenderStable()`，helper prompt 路径。

Skill 写入入口：

- `SkillsContextManager.LoadSkill`：普通加载，写入 `loadedSkills`。
- `SkillsContextManager.LoadAutoSkill`：AI 意图驱动加载，写入 `autoLoadedSkills`。
- `SkillsContextManager.LoadForcedSkill`：用户强制加载，写入 `forcedSkills` 与 `loadedSkills`。
- `SkillsContextManager.LoadSkillResource` / `LoadSkillResourceAsScript`：资源读取与分页，写入 `ViewWindows`。
- `SkillsContextManager.LoadSkillViewOffset`（翻页）与 `IsSkillLoaded` / `IsSkillLoadedAndUnfolded`（已可见判断）。

Skill 写入调用方（生产代码）：

- `common/ai/aid/aireact/reactloops/loopinfra/action_load_capability.go`：`handleLoadSkill` 调用 `mgr.LoadSkill`。
- `common/ai/aid/aireact/reactloops/loopinfra/action_load_skill_resources.go`：`LoadSkillResource` / `LoadSkillResourceAsScript`。
- `common/ai/aid/aireact/reactloops/loopinfra/action_change_skill_view_offset.go`：翻页。
- `common/ai/aid/aireact/re-act.go` 与 `re-act_mainloop.go`：持久会话恢复、强制加载。
- `common/ai/aid/aireact/reactloops/options.go`、`reactloop.go`：manager/loader 绑定与默认自动加载。

Evidence 相关入口：

- 业务写入/更新/删除：`common/ai/aid/aicommon/config_evidence.go`、`timeline_evidence.go`。
- 压缩快照精确保护：`timeline_compression_snapshot.go`，`isPromotableTimelineItem` 进入 `ExactItemIDs`。
- 半动态投影：`timeline_evidence.go` 的 `projectEvidenceLocked` 与 `renderSessionEvidencePromptBlock`。

压缩链路入口：

- 唯一时机：`common/ai/aid/aicommon/timeline_compression_before_prompt.go` 的 `CompressBeforePrompt`；主循环调用点 `common/ai/aid/aireact/reactloops/timeline_compression_before_prompt.go`。
- 快照候选：`captureCompressionSnapshotLocked`，特殊 item 与用户输入进 `ExactItemIDs` 并被 `continue` 跳过，不参与候选与体积统计。
- 提交事务：`commitCompressionSnapshot`，先 `rebuildPromotedStateLocked` 再退休未选 ID。

包依赖约束：

- `aicommon` 已引用 `aiskillloader`（如 `config.go`、`capability_inventory_skills.go`）。
- `aiskillloader` 不能反向 import `aicommon`，需要窄接口注入。

### 验证证据

已完成代码盘点, 见上文调用点清单。

## T2：接入最小类型和读写封装（已完成）

实现方式：复用现有 `PromotableTimelineItem` 机制（与 Evidence 同构），新增
`TimelinePromotedKindSkillContent`，不新建独立 value 类型。冻结、水位、提升、
序列化、分支隔离全部复用既有事务。

新增/修改：

- `aicommon/timeline_skill_content.go`：`PushSkillMetadata`、`PushSkillFileContent`、`VisibleSkillParts`、
  `renderPromotedSkillContent` 与 `SkillContentTimelineAdapter`（实现 aiskillloader 侧
  的 `SkillContentSink`；早期草稿里重复定义的 `SkillContentTimelineView` 已删除）。
- `aicommon/timeline_promoted.go`：新增 `TimelinePromotedKindSkillContent`。
- `aicommon/user_input_boundary.go`、`timeline_compression_freeze.go`：Open 尾段渲染
  与白名单接入。
- `aiskillloader/timeline_binding.go`：`BindTimelineSink`、一次性导入、
  `SkillContentSink` 窄接口（避免反向 import）。
- `aiskillloader/skills_context_manager.go`、`resource_loader.go`：普通/自动/强制
  加载、资源读取、翻页在操作完成后追加正文。

验证：

```sh
go test ./common/ai/aid/aicommon/ -run 'TestSkillContent' -count=1
```

结果：通过。覆盖「先 Open 不提前提升」「不被技能名错误去重」「按精确状态保留」。

## T3：迁移分类渲染及所有 prompt 入口（已完成）

- `skills_context_manager.go` 的 `RenderStable` 只读取 Timeline；旧来源分类渲染已删除。
- `BindTimelineSink` 只切换引用，不导入缓存、不修改 Timeline。构造器不再默认加载；
  `reactloops/reactloop.go` 绑定后显式调用 `LoadDefaultSkills`，已有历史的默认技能不重放。
- `GetCurrentSelectedSkills` 按目标 Timeline 的可见名单查看，loader/窗口缓存只补充元信息。
- 前缀目录等派生信息仍来自已冻结快照。

验证：

```sh
go test ./common/ai/aid/aicommon/ -run 'TestSkillContentLoadingKeepsCachedPrefixStable|TestForcedSkillContentWaitsForFreezeBoundary' -count=1
go test ./common/ai/aid/aireact/ -run 'Prompt|Skill|Evidence|Timeline' -count=1
go test ./common/ai/aid/aireact/reactloops/ -count=1
go test ./common/ai/aid/aireact/reactloops/loopinfra/ -count=1
go test ./common/ai/aid/aireact/reactloops/reactloopstests/ -run 'Skill|Resource|Prompt' -count=1
```

结果：全部通过。加载技能不改变 Frozen 段字节，只有 Open 变化。

## T4：skill 保持精确状态（已完成）

- `timeline_compression_snapshot.go`：skill 与其他 promoted item 一样
  只进入 `ExactItemIDs`，不进入压缩候选、摘要或 `ratain_timeline_item_range`。
- `timeline_compression_transaction.go`：保持「先退休未选中、后重建 promoted」
  的提交顺序，确保普通历史按统一选择结果退休。
- `compression_instruction.txt`：明确技能正文是请求之外的精确状态，不写入
  summary；未冻结的 pending 仍参与冻结阈值统计。

验证：

```sh
go test ./common/ai/aid/aicommon/ -run 'TestSkillContentStaysExactDuringCompression|TestSkillContentAbsentFromCompressionRequest|TestSkillContentSurvivesCompression|TestPromotedSkillContentStaysExactAcrossCompression|TestSkillContentPendingDeltaTriggersFreezeOnly' -count=1
```

结果：通过。技能正文在压缩前后、保存恢复后均保持不变。

## T5：恢复、兼容、回归与清理（已完成）

已完成：

- 被总结后允许重读：`IsAutoSkillLoadedAndUnfolded` 以 Timeline 可见性为准；
  `LoadSkill`/`LoadAutoSkill` 命中已退休正文时重新追加新 item。
- 翻页 `ChangeViewOffset` 改为锁内收集、锁外追加，避免写锁交叉。
- 内容保真：迁移后的正文按 `renderFull` 等价内容推送（技能说明、关联技能提示、
  文件树、阅读窗口的 Total Lines / Current Offset 元数据），迁移不丢既有信息。
- Open 段技能正文带 `SKILLS_CONTEXT_OPEN_*` 可识别标签；Open 不属于缓存前缀，
  冻结提交后才改由独立技能块渲染。
- 加载写入时检查「技能名 + 路径 + 正文」是否完全重复，仅仍存在的相同正文不追加；
  不同阅读窗口作为独立 item 进入 Open / 候选 / 统计，由统一选择和总结决定去留。
- 冻结技能进入 `promotedState`，按技能名累计正文；新来源 hash 与上一次相同则忽略，
  不同则拼接老正文与新正文。当前阶段 skill 与其他 promoted item 一样
  属于精确状态，不进入压缩候选。
- `RenderTimelineFrozenOpenWithOptions` 在同一读锁内产出 Open 和已提升技能块，
  经共享材料链路传入 SemiDynamic1；主循环与 helper 不再经过 manager 渲染旁路。
- 技能目录（意图识别 top-N）同样接入 Timeline，写入时去重，遵守同一冻结边界。
- 既有回归：`aicommon`、`aiskillloader`、`reactloops`、`loopinfra`、
  `reactloopstests`、`aireact` 相关测试全部通过。

待办：

- [x] 保存/恢复路径下新类型的专项断言：`TestSkillContentSaveRestoreKeepsFreezePosition`。
- [x] fork/clean/子 Agent 私有追加不泄漏的专项断言：`TestSkillContentBranchIsolation`。
- [x] 上下文成分面板统计与实际迁移正文同源的断言：
  `TestSkillContentMaterialsMatchModelView`（提升前只在 Open；提升后模型视图
  中正文恰好出现一次）。
- [x] 其余累计材料是否接入的最终清单（见下方「未迁移项清单」）。
- [x] Prompt 渲染不再调用 manager：主循环和 helper 统一使用
  `RenderTimelineFrozenOpenWithOptions` → `PromptFrozenOpenMaterials` → `PromptMaterials`。
  已删除 `LoopPromptAssemblyInput.SkillsContext` 及对应旁路赋值，避免覆盖统一冻结快照。
  manager 的 `RenderStable` 仅作为查看兼容入口。
- [x] 恢复边界修正：显式初始化和旧会话导入只迁移 Timeline 中没有任何
  skill 记录的技能；已有历史（含已被总结退休）的正文不会被旧名单
  重新展开。`HasSkillContentHistory` + 两个专项测试。
- [x] 可见性短路修正：`IsSkillLoadedAndUnfolded` 与 `IsAutoSkillLoadedAndUnfolded`
  统一以 Timeline 可见性为准；`TestSkillReloadAfterSummarized`、
  `TestSummarizedSkillIsNotVisible` 覆盖。

### 未迁移项清单（明确不在本次范围）

以下内容仍由各自独立入口提供，本次未接入 Timeline，交付不得声称已承载
「所有运行时数据」：

- `ExtraCapabilities`（深意图识别的推荐能力），`prompt.go` 的 `extraCapabilities.Render(nonce)`。
- 注入记忆 `InjectedMemory`，`exec.go` 传入 `r.GetCurrentMemoriesContent()`。
- 感知上下文 `perception_awareness` / `perception_knowledge`，经 ContextProvider 注入 dynamic 段。
- 内联附件与 `@` 文件上下文，经 `ContextProviderManager` 注入 dynamic 段。
- TODO、PlanStatus、PlanDocument/PlanDefinition/CurrentReport：保持既有独立来源。
- 静态指令、输出示例、每轮 Action Schema、时钟与固定环境：属请求级材料，不追加 Timeline。

### 本轮回归证据

```sh
go test ./common/ai/aid/aicommon/... -count=1
go test ./common/ai/aid/aireact/... -count=1
```

结果：全部通过（`aicommon` 六个子包 + `aireact` 全量含 `reactloopstests`）。

最终全量（本轮最后一次执行，含全部新增测试）：

```sh
go test ./common/ai/aid/aicommon/... ./common/ai/aid/aireact/... -count=1
```

结果：无失败。`aicommon` + 5 个子包、`aireact` 全量 21 个包均为 `ok`。

### 验收标准逐条对照

| 指导文档第 10 节要求 | 证据 |
| --- | --- |
| 新增类型字段最小，无通用状态模型 | 复用 `PromotableTimelineItem`；载荷只含 name/path/content/source |
| Skill 正文进入实际 Timeline，资源/分页无旁路 | `PushSkillMetadata` / `PushSkillFileContent` 接入 5 条路径；`TestSkillContentStaysInOpenUntilFrozen` |
| 主循环/helper 不再从私有容器重建全文 | 4 处 `Render*` 调用全部转发 Timeline 投影 |
| 正文与目录先 Open，冻结保留后才进类型块 | `TestSkillContentLoadingKeepsCachedPrefixStable`、`TestForcedSkillContentWaitsForFreezeBoundary` |
| 前缀字节/hash 一致性测试 | 同上，直接断言 `before.Frozen == after.Frozen` |
| skill 保持精确状态；pending 只触发冻结 | `TestSkillContentPendingDeltaTriggersFreezeOnly`、`TestSkillContentAbsentFromCompressionRequest` |
| 压缩前后正文不丢失，摘要不替代原文 | `TestSkillContentSurvivesCompression`、`TestPromotedSkillContentStaysExactAcrossCompression` |
| 显式移除后可重读；来源标记保留 | `TestSkillReloadAfterSummarized`、`TestForcedSkillOnlyMarksSource` |
| Evidence 既有语义保持 | 既有 Evidence 压缩/事务测试全绿 |
| 仍只有 prompt 前压缩入口 | 未新增压缩器；仅扩展候选与指令 |
| 保存/恢复、初始化、分支、并发、失败路径 | `TestSkillContentSaveRestoreKeepsFreezePosition`、`TestSkillContentBranchIsolation`、`TestImportSkipsSkillsWithTimelineHistory`；失败/取消路径沿用既有事务测试 |
| 六段协议保持，观测与正文同源 | `TestSkillContentMaterialsMatchModelView`；`reactloopstests` 全量通过 |
| 剩余累计入口已明确 | 见「未迁移项清单」 |
| 测试与文档准确，不夸大 | 本文件状态与清单逐项标注 |

本轮修复的三个集成回归（此前因迁移丢失既有渲染信息而失败）：

- `TestReActLoop_Skills_LoadedContentInPrompt`
- `TestReActLoop_Skills_ChangeViewOffset`
- `TestReActLoop_CrossSkillHints`
- `TestReActLoop_SkillsContext_VisibleInAllSubsequentCallbacks`

## 附：加载来源分类渲染的简化（本轮追加）

### 结论

原先的“三态分离”（`loadedSkills` / `forcedSkills` / `autoLoadedSkills` 三套正文容器 +
`RenderStable` / `RenderForcedSkills` / `RenderAutoLoadedSkills` 三个渲染入口 + 按 source
过滤的 Timeline 投影）属于过度设计：三条加载路径推送的内容本来就是同构的正文，
差异只有 payload 里的一个来源标记。分类托管既不产生不同的压缩语义，也不产生不同
的展示语义，只增加同步与缓存一致性的负担。

### 现在的设计

1. 不论哪种加载方式，正文都只是 Timeline 里一条 skill item；manager 不再托管正文，
   只保留“知道有哪些 skill 可加载 + 把 skill 写进 Timeline + 查看/翻页/移除已加载正文”三项职责。
2. 冻结后只由 Timeline 的一个投影函数渲染**单一技能块**（SemiDynamic1，与 `prompted_recent_tools` 同段）。
   渲染不再依赖 manager，也不再按来源分块。
3. 技能正文进入 Timeline 时仍遵循视窗原则：先写 Open 尾段，只有既有冻结/压缩事务提交后才提升进
   可缓存前缀；翻页产生的阅读窗口是一条**新增** item，不替换旧 item。
4. 用户指定加载不再提供“不可总结”的强制保护，只把来源写进候选文本（`[source: user-forced]`），
   由压缩模型结合提示判断是否保留原文；被总结后允许重新加载。
5. `unload` 退化为“退休该技能当前可见的 Timeline item”，不改写已提交的冻结边界，也不回收 ID；
   之后重新加载会追加一份新正文。

### 删除清单

- `ForcedSkillRegistry` 整文件。
- 三个正文容器与三套 LRU 折叠（`ensureContextFits` / `ensureAutoSkillsFits`）。
- `RenderForcedSkills` / `RenderAutoLoadedSkills`（保留为空串兼容入口）、按 source 分支的
  `RenderSkillContentBySourceForPrompt` / `RenderSkillContentForPromptSources`。
- `PromptMaterials.ForcedSkills` / `AutoLoadedSkills` 字段与 `frozen_block_section.txt`、
  `semi_dynamic_2_section.txt` 中对应片段；观测面板的 `user_forced_skill` / `auto_loaded_skills` 子节点。

顺带修掉两个缺陷：`LoadForcedSkill` 在技能已被别的路径加载时会丢来源标记；manager 的 LRU
折叠置位不会被 Timeline 投影读取，导致 32K 预算对实际 prompt 失效。

### 验证

```sh
go test ./common/ai/aid/aicommon/... ./common/ai/aid/aireact/... -count=1
```

结果：全部通过。

新增/重写的关键测试：

- `timeline_skill_content_test.go`：`TestForcedSkillContentStaysExact`、
  `TestDeleteSkillContentRetiresWithoutRewritingFreeze`。
- `aiskillloader/timeline_reload_test.go`：`TestForcedSkillOnlyMarksSource`、
  `TestAutoAndManualSkillShareTimelineSemantics`。
- `aiskillloader/forced_skills_test.go`：来源标记、单一渲染、移除与重载。
- `aiskillloader/forced_dup_render_test.go`：单一技能块不重复渲染。
- 集成测试改为断言“先 Open、冻结后进入单一技能块”。
