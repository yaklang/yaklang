# 24. 运行时上下文统一进入 Timeline：任务指导文档

状态：已完成（实现与回归见 25 号进度文档）。本文件保留为已收敛设计的权威说明；第 4/5 节已按最终实现（单一技能块）校正。

## 1. 任务目标与已确定的设计

把运行过程中累积、需要交给模型阅读的 Skill、Evidence 等内容统一交给 Timeline 管理。不同内容作为不同类型的 item 存入同一条 Timeline：新内容先在 Open 尾段展示，只有已提交冻结且仍保留原文的内容，才转入前面的独立类型块。

Manager 保留业务加载和查询能力，但不再作为独立正文来源参与 prompt 构建。压缩复用现有主循环的 `compressTimelineBeforePrompt`，仍然是模型挑选保留 ID，加一份文本总结。

本任务的六条确定结论：

1. 复用现有 Timeline 容器、ID、时间、持久化、快照和压缩事务，不重新设计一套容器。
2. 新增的内容 item 以“追加、读取”为核心，不提前设计通用更新、删除、版本和对象状态机。
3. 不新增 Skill 压缩器、Evidence 压缩器、按类型独立的摘要 head，也不更换压缩算法。
4. Prompt 原文是否继续展示，以 Timeline 压缩后的保留结果为准，manager 不能把被总结掉的正文重新注入。
5. 写入 Timeline 不等于立即进入 Semi/Frozen。已提交的冻结边界决定何时允许转移，不能因加载技能提前改写可缓存前缀。
6. **加载来源不再是一个渲染分类。** 用户指定、AI 自动、普通加载和持久恢复都是同一种 skill-content item，只在 payload 里带一个来源标记；冻结后统一进入 SemiDynamic1 的**单一技能块**（与 `prompted_recent_tools` 同段）。原先按来源拆分的三个容器、三个渲染入口和按来源过滤的投影都是过度设计，已删除。

这里统一的是“运行时上下文内容”，不是所有运行对象。工具实现、文件系统、数据库连接、回调、取消句柄和 coordinator 的执行控制逻辑仍由原组件负责。

## 2. 实现范围

### 2.1 本次必须完成

- Skill 正文及已读取资源进入 Timeline，覆盖正常加载、自动加载、用户指定加载和持久会话恢复入口。
- SkillManager 的上下文追加和读取改为通过 Timeline；不再维护一份独立、持续拼进 prompt 的正文集合。
- Skill 翻页、二级资源、重复加载检查适配 Timeline 的当前可见内容。
- Evidence 复用现有 Timeline 写入路径，使适合总结的 Evidence 正文能进入统一的原文筛选范围。
- Timeline 按内容类型生成展示材料，主循环与相关 helper prompt 读取同一来源。
- 新 Skill 正文、资源页和目录变化先进入 Open，冻结提交后再提升到对应独立类型块；不能提前重建前面的半动态段。
- 统一压缩的候选内容、体积统计、保留结果和后续渲染覆盖这些运行时材料。
- 序列化、恢复、分支隔离、上下文成分观测及相关测试跟随改造。

### 2.2 现有类型的边界

| 内容 | 本次处理方式 |
| --- | --- |
| 普通运行历史、工具观察 | 原路径保留，与新增运行时内容共用压缩事务 |
| 自动/普通 Skill 正文、资源阅读内容 | 接入 Timeline，并进入统一筛选候选 |
| 用户强制加载 Skill | 同样由 Timeline 承载；加载来源及已有原文保护语义不得在迁移中无声丢失 |
| Evidence | 复用已有 journal；修改正文压缩与投影，不重做现有业务更新/删除接口 |
| 最近工具及完整参数 Schema | 已由 Timeline 承载；保留完整 Schema 的既有保护，不拿文本摘要当可执行 Schema |
| 用户输入 | 已由 Timeline 承载；本次不顺带改变当前用户要求的原文保护规则 |
| 运行时技能推荐目录 | 如当前会话展示它，改从 Timeline 的类型化目录内容读取；不复制整个静态技能库 |
| ExtraCapabilities、检索记忆等其他累计材料 | 逐项盘点读写位置，可沿同一路径接入；不能仅为追求字段清空而改变原有语义 |
| TODO、PlanStatus、PlanDocument、CurrentReport | 本次不重做其业务状态或编辑模型；保持现有行为，列出仍独立提供材料的入口 |
| 静态指令、输出示例、每轮 Action Schema、时钟和固定环境 | 仍属于静态/请求级材料，不为统一而每轮追加进 Timeline |

Skill/Evidence 的完整接入是本次验收主体。其他运行时累计材料未迁移时，交付必须列出具体剩余项，不得声称已经承载“所有运行时数据”。

### 2.3 明确不做

- 不设计通用的 `Scope + Key + Revision + Operation + SourceRef` 事件模型。
- 不新增通用删除/版本状态机。仅提供“移除某技能当前可见正文”的最小操作（退休 Timeline item，不回滚已提交的冻结边界），供显式 unload 使用；不扩展成按类型通用的生命周期模型。
- 不增加独立后台压缩、写入即压缩、读取即压缩或保存时压缩。
- 不增加固定压缩比例、按类型硬裁剪、先截断再总结或多轮摘要合并算法。
- 不把 Plan/Report 改成基线加 patch 链；当前 prompt 已按同 ID 取最新快照。
- 不改变六段 prompt 的外层缓存协议，不重做子 Agent 上下文继承和权限规则。
- 不要求摘要支持精确恢复任意历史状态；保留现有摘要覆盖范围内不能任意回滚的边界。

## 3. 现状与代码阅读入口

以下是定位入口，不是要求全部重写的文件清单。以实现时的函数和测试为准，避免依赖易变化的行号。

| 关注点 | 代码入口 | 当前行为及改造关注点 |
| --- | --- | --- |
| 唯一主循环压缩入口 | [timeline_compression_before_prompt.go](../timeline_compression_before_prompt.go)，`compressTimelineBeforePrompt` | 已具备调用时机、上下文优化状态、取消和失败保留逻辑，继续复用 |
| 主循环材料采集 | [prompt.go](../prompt.go)，`generateLoopPrompt` | 直接调用 SkillManager 的三个 `Render*`，需要退出这条正文注入路径 |
| Item 外壳与序列化 | [timeline_item.go](../../../aicommon/timeline_item.go) | 已有 `TimelineItemValue`、ID、时间和类型序列化，新增内容沿用它 |
| 写入、读取和退休 | [timeline.go](../../../aicommon/timeline.go) | 复用已有锁、ID 和追加协议，不直接修改返回的底层有序 map |
| 特殊类型与提升 | [timeline_promoted.go](../../../aicommon/timeline_promoted.go) | 已有 Evidence/tool cache 特殊 item；不能仅把新正文写入这里就认为完成统一压缩 |
| 压缩快照 | [timeline_compression_snapshot.go](../../../aicommon/timeline_compression_snapshot.go) | `isPromotableTimelineItem`/用户输入目前进入 `ExactItemIDs`，整体不作为原文筛选候选 |
| 压缩触发统计 | [timeline_compression_before_prompt.go](../../../aicommon/timeline_compression_before_prompt.go) | 已提升的精确状态不在当前可压缩统计内，要覆盖新接入的可见正文 |
| 选择协议和解析 | [timeline_compression_output.go](../../../aicommon/timeline_compression_output.go) | 复用完整 item ID 选择，保留现有字段及协议拼写 |
| 压缩提交 | [timeline_compression_transaction.go](../../../aicommon/timeline_compression_transaction.go) | 复用快照校验、冻结、摘要、退休的原子事务，检查特殊类型是否会被重新物化输出 |
| 类型化投影 | [prompt_materials.go](../../../aicommon/prompt_materials.go)、[timeline_compression_freeze.go](../../../aicommon/timeline_compression_freeze.go) | 现在已区分普通历史和 Semi 特殊材料，扩展并统一实际保留集合 |
| 最终 prompt 和观测 | [prompt_loop_materials.go](../../prompt_loop_materials.go) | 保持实际正文与新增/迁移观测子块的数据来源一致 |
| Helper 的技能读取 | [prompts.go](../../prompts.go)，`renderSkillsContextForLoop` | 仍直接调用 `mgr.RenderStable()`，不能只改主循环而漏掉此处 |
| Skill 状态与读取 | [skills_context_manager.go](../../../aicommon/aiskillloader/skills_context_manager.go) | 三种加载容器、可见性查询、LRU 折叠和直接渲染需要明确哪些退出正文权威来源 |
| Skill 资源与分页 | [resource_loader.go](../../../aicommon/aiskillloader/resource_loader.go)、[view_window.go](../../../aicommon/aiskillloader/view_window.go) | 复用资源解析和阅读窗口能力，不让窗口缓存重新注入已移出的正文 |
| Skill Action | [action_loading_skills.go](../loopinfra/action_loading_skills.go)、[action_load_skill_resources.go](../loopinfra/action_load_skill_resources.go)、[action_change_skill_view_offset.go](../loopinfra/action_change_skill_view_offset.go) | 加载、读取、翻页和“已可见”短路必须统一 |
| Skill 初始化 | [options.go](../options.go)、[reactloop.go](../reactloop.go) | 显式 manager、loader、默认自动加载和恢复入口都要接入 |
| Evidence 精确写入 | [timeline_evidence.go](../../../aicommon/timeline_evidence.go)、[config_evidence.go](../../../aicommon/config_evidence.go) | 复用现有保存、同步、业务更新/删除语义，不凭摘要重建业务事实 |
| 原文归档与恢复 | [timeline_history.go](../../../aicommon/timeline_history.go)、[timeline_marshal.go](../../../aicommon/timeline_marshal.go) | 新类型必须正确保存/恢复；退休后不应通过旧独立字段复活 |

压缩现状文档：[timeline_compression.md](../../../aicommon/timeline_compression.md)。其中“精确材料不参与总结”等约定是本任务需要有选择地调整的内容，不能直接复制为新验收标准。

## 4. 最小数据模型和读写约定

### 4.1 Item 外壳不变

复用 `TimelineItem`。新增一个 Skill 内容 value 的最小示意为：

```go
type SkillTimelineItem struct {
    ID      int64
    Name    string
    Content string
}
```

该结构是语义示意，不要求必须使用这个名字。可采用具体 value 类型或现有 value 的明确类型字段，二者择一，不同时堆叠 `Type/Kind/Category`。

- `ID` 沿用现有分配器，不建立第二套内容 ID。
- `Name` 用于技能识别和读取；资源读取可带必要的资源路径、窗口定位信息。
- `Content` 保存实际读取到的正文，不只记录“加载成功”一句话。
- 来源区分只为保留实际的普通/自动/用户指定加载语义，不扩展为通用权限系统。
- 不新增“用于未来可能需要”的版本、操作、分支和状态字段；分支继续由现有 Timeline 容器隔离。

### 4.2 追加发生在操作完成时

成功加载、读取资源或切换阅读窗口后，追加内容 item。加载失败仍沿用普通错误观察，不追加伪成功正文。

不要在每次 prompt 构建时把 manager 的全部内容重新追加一遍。也不要把“默认自动加载”每轮重复执行。

重复请求时，如果所需原文已经在当前可见集合中，可以复用；如果已经只剩摘要，就允许重新读取并追加。相同技能的不同资源或阅读窗口不能仅按技能名去重，否则会吞掉新的阅读内容。

### 4.3 Manager 不再是第二份正文存储

Manager 仍可以负责：

- SkillLoader、搜索和资源路径校验。
- 文件读取缓存、阅读窗口计算和加载结果反馈。
- 对 Timeline 的追加、查找和枚举封装。

Manager 不再负责：

- 维护 `loadedSkills / forcedSkills / autoLoadedSkills` 三套正文容器。
- 在 `RenderStable/RenderAutoLoadedSkills/RenderForcedSkills` 中从私有容器持续重建模型正文。
- 按加载来源拆分渲染块，或让来源影响正文是否可被压缩。
- 独立决定某份正文永远留在 prompt 中。
- 在 Timeline 已退休内容之后，仅凭内存缓存或恢复名单把正文重新塞回去。

实现后的 manager 只保留一份轻量的“查看状态”（文件树、阅读窗口、翻页 offset），它不参与 prompt 拼装。旧 `RenderForcedSkills/RenderAutoLoadedSkills` 保留为空字符串的兼容入口，`RenderStable` 直接转发 Timeline 的单一技能块。

### 4.4 避免 Go 包循环依赖

`aicommon` 当前已经引用 `aiskillloader`。不能让 `aiskillloader` 反向 import `aicommon` 来持有具体 `Timeline`。

用一个窄的“追加、查找、枚举”接口注入能力，或把最小接口/数据类型放到无反向依赖的子包。无需为此拆分完整 Timeline 包，也不需要通用插件注册框架。

绑定必须覆盖 `WithSkillLoader`、`WithSkillsContextManager`、config 自动构建和持久恢复。`BindTimelineSink` 只接入目标 Timeline，不加载、补写或修改内容；manager 的可见性、已加载名单与渲染读取目标 Timeline，不用本地缓存覆盖它。默认自动加载在新会话绑定后的初始化入口显式调用 `LoadDefaultSkills`，持久恢复不重放默认加载。已有 skill-content 历史（包括已退休正文）的技能不得重新展开。未绑定时的本地查看状态不能进入 prompt，也不能在绑定时自动导入；只有显式加载操作可以写入目标 Timeline。

## 5. 按类型渲染，不按类型维护第二套内容

### 5.1 同一份 Timeline 内容，两种呈现用途

- 压缩用途：按现有 ID 顺序展示候选正文，并标明内容类型，让模型统一挑选和总结。
- 决策 prompt 用途：未冻结正文留在 Open；已冻结且保留的原文按类型划成 Skills、Evidence 等独立块；其余普通历史保留原布局。

“单独渲染”只是呈现方式，不得成为“单独存储、单独压缩、永远全文展示”的理由。

### 5.2 保留外层六段协议

继续使用 HighStatic、Frozen、Semi1、Semi2、TimelineOpen、Dynamic 的外层标签和拼接顺序。第一版不同时重排缓存布局。

技能加载来源不产生展示区分：普通技能、自动技能、用户指定技能在冻结后进入同一个技能块，来源只作为 payload 标记保留给压缩提示与观测。渲染位置由代码决定，不让正文或模型指定任意缓存标签。

只要组装下一轮 prompt 时该 item 尚未被合法冻结提交覆盖，新加载正文就应通过现有 Timeline Open 尾段呈现。不得在写入后就直接放进 Semi1、Semi2 或 Frozen 的独立技能块，也不得用所谓“未冻结类型视图”把它提前拼入这些前缀。

如果下一轮 prompt 前的既有压缩事务已经成功冻结了该 item，则按提交后的保留结果渲染，不需要人为多留一轮 Open；限制的是“不得先于冻结提升”，不是“必须先让模型看过一轮才能冻结”。

不要为了让技能尽快进入前缀而在加载、读取、分页或渲染时调用 `FreezeAll`；沿用既有合法冻结/压缩事务的提交时机。未达到触发条件时，正文继续留在 Open，不影响使用。

### 5.3 冻结转移和缓存不变量

明确执行下面的状态转换：

```text
读取 Skill/资源 -> 追加 item -> Open 尾段立即可见
  -> 既有冻结/压缩事务成功提交
     -> 原文被保留：从 Open 移出，进入对应独立类型块
     -> 原文未保留：由统一摘要承接，不进入独立类型块
```

用户指定加载不再提供“不可总结”的强制保护：正文同样进入统一候选集，只在候选文本里带 `[source: user-forced]` 提示，由压缩模型判断是否保留原文。所有正文都必须先经过冻结再进入独立块。

独立类型块只能读取“已提交冻结成员中的原文保留集合”。不要仅按 item 类型、manager 的当前加载列表或“ID 小于当前最大 ID”判断它应该进入前缀；这些条件不能代替冻结成员关系和保留结果。

在没有新的合法冻结提交、且其他指令/Schema/前缀材料不变时：

- 新增 Skill、读取新资源、切换页面、重新加载已总结内容，都只改变 Open 尾段。
- 前面的 Skill 独立块保持字节一致，不能把 Open 新内容混入已冻结快照。
- 技能目录、加载数量、文件树、折叠状态和排序等派生信息，也不能从实时 manager 读取后提前改写前缀；需要展示的新变化同样留在 Open。
- 半动态/冻结块的标签、nonce 和排序继续使用稳定规则，不能引入逐轮随机值或因 Open 访问顺序变化而重排旧内容。

冻结成员、保留原文集合、类型提升和摘要必须从同一提交快照读取，不能先发布新的技能类型块，再发布冻结水位。压缩失败、取消或拒绝过时提交时，不得提前迁移；快照水位之后并发新增的内容仍留在 Open。

冻结成功后，前缀可以因保留内容的提升发生预期变化，对应缓存边界也可能失效。这是提交导致的明确变化，不保证“永不失效”；要避免的是未冻结内容在普通加载/渲染时提前污染缓存前缀。

### 5.4 一份正文只有一个原文展示位置

同一 item 不能同时以普通历史全文、Skills/Evidence 全文和 manager 全文三个入口重复出现。

同一原文在冻结前只出现在 Open，冻结并提升后只出现在对应独立块；转移不能造成重复展示或短暂丢失。不同读取操作产生的不同 item 仍按各自冻结状态处理。

加载成功事件可以保留短观察；真正的正文作为类型化内容只输出一次。读取操作本身的成功/失败不应因为正文被单独渲染而从审计历史中消失。

渲染必须是只读操作：不追加 item、不调用 loader、不更改冻结状态、不发起压缩。无新增操作的多次渲染应保持内容和稳定标签一致。

### 5.5 主循环、helper、UI 同源

主循环、工具参数生成及其他读取 Skill/Evidence 的 helper 都从同一 Timeline 内容视图获取材料，保留各 helper 已有过滤规则和相同的冻结转移边界。helper 也不能通过 manager 的实时集合提前填充前面的技能块。不得借迁移向 helper 注入主循环专属的 native action replay。

上下文成分面板继续区分 Skill/Evidence 子块，但必须统计实际送给模型的内容。不能只改 UI 的 `can_compress` 标记，就宣称已经支持统一压缩。

## 6. 复用现有“挑选 ID + 文本总结”

### 6.1 唯一时机和协议保持不变

继续在上一轮 Action 已完成、新一轮 `AssembleLoopPrompt` 之前调用：

```text
compressTimelineBeforePrompt
  -> Timeline.CompressBeforePrompt
  -> 现有快照、模型选择和总结
  -> 现有原子提交
  -> 从提交后的 Timeline 渲染下一轮 prompt
```

保留现有阈值配置、输入/输出安全上限、失败退避、并发等待和取消机制。安全上限不是目标输出长度，本次不得改成固定比例压缩。

复用当前输出的 `summary`、`ratain_timeline_item_range` 和现有记忆候选处理；本次不要顺带修正协议字段拼写或引入新的模型输出 Schema。

### 6.2 扩展候选，不重写总结器

当前特殊 item 会整体进入 `ExactItemIDs`。改造时要使允许总结的运行时正文进入现有带 ID 的候选集合，而不是只作为辅助参考出现在请求中。

不要简单移除所有特殊类型的保护：完整工具参数 Schema、当前用户要求等仍沿用现有准确性边界。用户指定 Skill 的保护也不得仅因它进入 Timeline 而自动消失。

候选范围包含旧摘要、仍保留的普通原文，以及本次允许压缩的 Skill/Evidence 正文。业务 journal 的全部历史操作不能被机械展开成候选全文；Evidence 同 ID 的旧版本需要按现有有效状态处理，不能把已删除或已纠正内容再次当有效证据。

### 6.3 体积统计覆盖实际可压缩正文

已单独渲染到 Semi 的可压缩正文也应计入触发范围，不能因为它“已经 promote”就永远不计。

需要满足：

- 没有普通历史、只有可压缩 Skill/Evidence 正文时，也能触发一次有效压缩。
- 同一正文在特殊块和候选视图中只能统计一次。
- 未保留正文不在下一轮触发统计中复活。
- 没有新内容时，不反复压缩同一摘要和相同保留集合。
- 只有受保护的精确材料过大时，不能循环触发一次实际无法减少正文的空压缩。

### 6.4 选择结果必须支配类型块

本次事务内被保留且已提交冻结的 item：原文保持完整，转入或继续留在对应独立类型块。未被冻结的新 item 仍在 Open，不能因为渲染器已知道其类型就提前转移。

未保留的 item：退出当前 prompt 的原文集合，由统一摘要承接；不能通过 manager、`promotedState`、旧 session 字段或 helper 再次全文输出。

摘要仍使用现有统一 head，历史摘要档案不叠加进当前 prompt。无需把模型总结再次按类型解析为多个独立摘要。

注意当前提交代码先重建 `promotedState` 再退休候选：如果特殊正文加入候选而重建逻辑未适配，可能把被淘汰正文物化回类型块。必须用提交后的完整 prompt 测试验证，不只检查 `RetiredIDs`。

### 6.5 Evidence 的精确操作不靠摘要代替

本次调整的是模型是否需要一直看到 Evidence 全文，不是要求用文本摘要解析并恢复 Evidence 的 ID、更新和删除状态。

沿用 Evidence 已有业务操作和准确读取来源。若一个 item 同时承担精确 journal 和正文展示，应区分“业务操作仍有效”和“正文被保留”两个用途，不能把压缩未选中直接当业务删除。

优先复用 Timeline 现有的原文保存、选择结果及投影能力。需要保留准确业务数据时，让它留在已有准确存储/索引中，而不是把大段正文当成保护内容继续无条件输出。不得凭空建立通用对象状态机。

如果某条原文仍被程序逻辑依赖，必须先验证现有保存/恢复路径可读取它，再从活跃展示集合移出；没有可靠保存能力时不得以“摘要里还有”证明原文可恢复。

### 6.6 更新压缩指令，但不赋予摘要执行权

修改 `promptloader/prompts/ai/aid/aicommon/prompts/timeline/compression_instruction.txt` 等实际使用的压缩资源，使 Skill/Evidence 正文明确成为可选择 item，而不是全部作为“独立保存、不可替代”的参考材料。

说明保留原文的理由可以是：后续仍需照原文执行的技能步骤、准确 payload/定位内容或关键证据。判断继续由统一选择机制完成，不另写一套 Skill 专用评分器。

压缩请求中的 Skill 指令仍是被整理的资料，不在压缩 helper 内执行。全文、摘要、证据和工具定义的来源属性不得被混淆；不把工具观察升级为用户指令，不把摘要当参数 Schema。

## 7. 加载、重读和恢复的具体语义

### 7.1 “已加载”的上下文判断

对 `loading_skills` 的短路判断，“已加载且可见”指当前 Timeline 仍有本次需要的 Skill 原文，不是 loader 缓存里曾经下载过它。Open 中已经可见的正文也算可见，不能因为尚未提升到独立块就重复加载。

正文被总结掉后：

1. 再次读取同技能不得被“已在上下文中”拦住。
2. 允许从 loader 或资源缓存重新读出正文，追加新的内容 item。
3. 新 item 未冻结时在 Open 显示新原文，并保持原先摘要作为历史背景；只有覆盖该 item 的冻结事务提交后，才按保留结果转入前面的独立块。

用户指定的受保护技能仍按既有保护语义处理；不要因为通用自动 Skill 能退休就一并解除其保护。

### 7.2 翻页与资源读取

复用当前窗口大小、路径解析和读取能力。通过追加新的阅读内容表示这次读取，不为翻页设计通用业务更新 API。

必要的技能名、资源路径和偏移可作为本类型的数据。相同路径的新页不能被错误去重；已经移出的页重新读取也应有效。是否保留旧页继续由现有选择机制决定。

加载写入时检查当前仍存在的「技能名 + 路径 + 正文」是否完全相同，来源标记不作为正文身份。不要按路径判定旧版本失效，也不要在 Open 渲染、压缩候选或提交时补做版本清理。冻结后的保留内容进入 `promotedState`：同一技能名只有一条累计正文，新来源 hash 与上一次相同则忽略，不同则拼接新旧正文。该提升态在统一 Timeline 渲染入口与 Open 同时读取，再经共享材料链路进入 SemiDynamic1，不能在外面依赖 manager 单独补渲染。

阅读窗口计算可以留在 manager，但窗口缓存不能独立参与 prompt 全文构建。Action 的成功反馈应准确描述新内容在哪里可见，不再指向已退出的 manager 渲染入口。

### 7.3 恢复不等于无条件重载全部历史技能

新版本 Timeline 已保存技能原文和保留集合时，从它恢复；旧的已加载技能名单不能把被压缩正文全部重新展开。

旧 session 确实没有对应类型 item 时，可在初始化一次性读取并导入。对当前用户显式启用的技能仍执行配置入口，不能将这种新请求误判为旧历史重放。

保证“保存 -> 恢复 -> 渲染”的内容及冻结位置一致。恢复的 Open item 仍在 Open，只有恢复后确实属于已提交冻结成员且原文被保留的内容才进独立块；不要在读取 prompt 时迁移或冻结数据。

### 7.4 分支沿用已有容器语义

SkillManager 必须绑定实际执行分支的 Timeline，不要把父 Timeline 捕获在一个跨分支共享的可写封装里。

fork/clean 按已有配置继承规则工作；子 Agent 的私有内容不能因为统一存储而全部写回父 Timeline。Evidence 的现有会话共享/同步也不在本次改成另一种 scope 规则。

## 8. 实施顺序与阶段交付

每个阶段都需要对应测试，不能先把大量写入口迁移完、最后才发现压缩和渲染没有使用同一集合。

### T1：盘点调用方并建立失败测试

- 搜索 Skill 的 `Render*`、加载、资源读取、恢复和已可见判断入口。
- 搜索 Evidence 的 journal、有效状态、精确保护和半动态渲染入口。
- 列出其他仍由独立 manager 提供的累计正文，明确本次迁移项和剩余项。
- 先写“新 Skill 只进入 Open、冻结前不改前缀”“压缩后不再注入”“被总结的 Skill 可以重读”“只有特殊内容也触发压缩”的测试。

交付：调用点清单，以及能暴露现有缺口的针对性测试。

### T2：接入最小类型和读写封装

- 添加必要的内容 value，复用 ID、追加锁和序列化。
- 接入 manager 的窄接口，处理默认加载和绑定顺序。
- 加载、资源读取、翻页只在操作完成时追加到 Open，不触发冻结或提前提升。
- 提供从当前 Timeline 查询可见内容的读取路径。

交付：直接写入、查询、序列化恢复和分支隔离测试通过。

### T3：迁移分类渲染及所有 prompt 入口

- 扩展 Timeline 材料投影：Open 保留未冻结正文，独立类型块只读取已冻结且保留的原文。
- 删除主循环对私有 manager 正文的直接采集。
- 同步 helper 的读取入口，保持 helper 自身过滤规则。
- 避免普通历史与类型块重复全文；保持六段标签和稳定渲染。
- 前缀中的目录、数量、窗口和排序也只使用已冻结快照，不能通过实时派生信息绕过正文的转移限制。
- 迁移对应的 PromptObservation 子块来源。

交付：尚未冻结时，新材料在 Open 可用且前缀稳定；冻结提交后才进入正确的独立类型块，来源已统一。

### T4：扩展现有统一压缩

- 调整候选排除和实际 token 统计。
- 修改压缩指令中与新候选冲突的旧规则。
- 使保留 ID 的结果影响所有迁移类型块。
- 适配 promoted 投影，验证业务准确状态不被正文退休破坏。
- 在同一事务/快照中发布冻结成员、保留结果和类型提升；失败不转移，水位之后的新内容继续留在 Open。
- 保留单入口、单事务、现有取消/失败/并发行为。

交付：混合类型的选择、统一总结、准确状态和最终 prompt 断言通过。

### T5：恢复、兼容、回归与清理

- 旧数据初始化迁移一次，新数据直接从 Timeline 恢复。
- 修正重复加载短路和反馈措辞。
- 清理无调用方的正文渲染和重复存储路径；保留仍有真实调用方的兼容封装。
- 更新被本任务改变的文档和测试，不删除仍有效的故障、Schema 准确性及隔离测试。
- 报告仍未接入的其他累计材料，不把阶段成果写成全系统完成。

交付：完整回归结果、未迁移项清单、代码改动说明和最终验收勾选。

## 9. 必须覆盖的测试

测试使用 mock 压缩回调；需要持久化时使用本地 SQLite。不要为了验收调用真实模型或依赖外部服务。

| 测试场景 | 必须断言 |
| --- | --- |
| 正常加载 Skill（未触发冻结） | 新 ID 和正文进入 Open；下一轮可见，但尚未进入 Semi/Frozen 独立块 |
| 下一轮 prompt 前已完成冻结 | 直接按已提交的保留结果渲染，不额外强制多等一轮，也不绕过冻结边界 |
| 冻结前前缀一致 | 固定其他指令/Schema，加载 Skill 或读取新页后，HighStatic/Frozen/Semi1/Semi2 的字节与 hash 不变，只有 Open 变化 |
| 派生信息不绕过边界 | 加载数量、目录、文件树、折叠状态和排序不能因 Open 新内容提前改变前缀 |
| 冻结后的提升 | 成功提交后，保留 item 从 Open 转入对应独立块；正文恰好展示一次，不重复、不丢失 |
| 未选中原文不提升 | 冻结提交后只剩统一摘要，不把未保留的 Skill 正文搬入独立块 |
| 仅有冻结之前的查询 | manager 能读到 Open 原文，“已可见”判断不会因尚未 promote 而重复加载 |
| 重复构建 prompt | 不重新加载、不增加 item、不改变冻结版本、正文不重复 |
| 同技能多个资源/页 | 新读取内容不被按技能名错误去重 |
| 自动、普通、用户指定加载 | 来源和已有保护语义保持；所有来源都遵守先 Open 后冻结提升，正文不在多个块重复出现 |
| 只有大量 Skill/Evidence | 不需要普通历史也能触发现有压缩 |
| 混合类型保留选择 | 模型能看到各类型 ID；选中的完整原文仍在正确类型块 |
| 未选中正文 | 实际最终 prompt 不再包含该全文，统一摘要可见 |
| 再次构建及 helper | manager、promoted 投影和 helper 不让淘汰全文复活 |
| Skill 重读 | 原文被总结后允许再次加载，新 item 可见 |
| Evidence 修订/删除 | 既有准确业务操作仍成立，旧内容不以有效证据形式复活 |
| 工具 Schema 回归 | 未被无声摘要化；可执行 Schema 仍完整准确 |
| 不变内容重复检查 | 不重复总结同一 head/保留集合，不发无效优化状态 |
| 超限、解析失败、取消 | 原文、类型材料和冻结状态按现有失败语义保留；不提前提升正文，不改写已冻结前缀 |
| 压缩期间追加 | 更大 ID 的新 Skill/资源仍留在 Open，不被旧快照退休或提前提升 |
| 快照范围被修改 | 拒绝过时提交，不部分发布摘要或类型材料 |
| 保存与恢复 | 类型、正文、ID、保留结果和冻结位置一致；未冻结正文不会在恢复时提前进入独立块 |
| 旧技能名单恢复 | 一次迁移，不每轮重载，不展开已退休正文 |
| fork/clean/子 Agent | 继承规则未改变，私有追加不泄漏回父 |
| text/function-call/lightweight/helper | 各路径按原角色规则读取同源材料，不注入不属于它的 replay |
| 上下文成分面板 | Skill/Evidence 统计与实际送入模型的迁移正文一致 |

优先扩展现有测试入口：

- `aicommon/timeline_compression_{snapshot,output,transaction,before_prompt,evidence,toolcache}_test.go`。
- `aicommon/timeline_marshal_test.go`、`timeline_fork_test.go`、`timeline_memory_lifecycle_test.go`；新类型原文归档的专项断言可另加测试文件。
- `aiskillloader/forced_dup_render_test.go`、`skills_persistence_test.go`、`resource_loader_test.go`。
- `reactloops/timeline_compression_before_prompt_test.go`、`prompt_observation_test.go`。
- `reactloops/loopinfra/action_load_skill_resources_test.go`。
- `reactloops/reactloopstests/skill_integration_test.go`、`skill_resource_integration_test.go`。
- `aireact/prompt_loop_materials_test.go` 以及已有 helper/工具参数生成集成测试。

示例执行命令，均从仓库根目录运行：

```sh
go test ./common/ai/aid/aicommon -run 'Test(Timeline|Evidence)' -count=1
go test ./common/ai/aid/aicommon/aiskillloader -count=1
go test ./common/ai/aid/aireact/reactloops -count=1
go test ./common/ai/aid/aireact/reactloops/loopinfra -count=1
go test ./common/ai/aid/aireact/reactloops/reactloopstests -run 'Skill|Resource|Prompt' -count=1
go test ./common/ai/aid/aireact -run 'Prompt|Skill|Evidence|Timeline' -count=1
```

按新增测试名补充筛选条件，不能因为正则未匹配测试就视为已验证。环境依赖或既有失败需要单独报告；新增回归失败不能用扩大 skip 范围掩盖。

## 10. 完成标准与交付要求

- [x] 新增内容类型保持最小字段，没有为未来通用操作引入复杂状态模型。
- [x] Skill 读取正文进入实际执行 Timeline；资源和分页也没有独立正文旁路。
- [x] 主循环和相关 helper 不再从 manager 私有容器重建全文。
- [x] 新正文及其派生目录信息先留在 Open；只有已提交冻结且保留原文的 item 才进入前面的独立类型块。
- [x] 未冻结的技能加载/资源读取不改写可缓存前缀；冻结转移只发生在合法提交边界，有字节/hash 一致性测试。
- [x] Skill/Evidence 正文进入现有选 ID 和总结机制，并计入正确的触发范围。
- [x] 被选择原文按类型完整展示；未选择原文不会被任何迁移入口复活。
- [x] Skill 被总结后能重读；工具 Schema 准确性未被破坏。
- [x] 三种加载来源收敛为同一 item 类型与同一渲染块，来源只影响压缩提示文本。
- [x] Evidence 既有业务保存、更新、删除和共享语义仍正确。
- [x] 仍只有 prompt 前的压缩入口，没有引入新的后台或按类型压缩器。
- [x] 保存/恢复、初始化、分支、并发和失败路径有测试证据。
- [x] 六段协议保持，观测与实际迁移正文同源。
- [x] 已明确其他累计上下文的迁移结果及剩余入口。
- [x] 更新的测试和文档准确描述新行为，不把待实现内容写成已完成。

最终交付说明应包含：迁移的内容类型、退出 prompt 主链的 manager 渲染入口、现有压缩链路的具体改动、执行过的测试及结果、仍保留独立来源的内容。

本次改造的核心验收问题不是“是不是新增了 Skill item”，而是：同一份运行时正文是否只由 Timeline 管理，并真正服从同一套选 ID 和文本总结的结果。
