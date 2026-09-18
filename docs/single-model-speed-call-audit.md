# Speed 模型辅助任务迁移审计

> 分支：`feat/single-ai-model-mode`  
> 更新日期：2026-09-18  
> 定义边界：**凡执行路径明确选择 Speed 模型的任务，均属于辅助任务。**

## 一、边界约束

本次迁移只处理以下路径：

- `InvokeSpeedPriorityLiteForge`
- `CallSpeedPriorityAI`
- `WithLiteForge_SpeedPriority`
- `MustGetSpeedPriorityAIModelCallback`
- `Config.InvokeLiteForge` 中实际用于 Speed 辅助工作的调用
- Yak 的 `liteforge.speedPriority()`

以下 Intelligence/Quality 路径不属于辅助任务，本次不得迁移或降级：

- `InvokeQualityPriorityLiteForge`
- 默认走 Quality 的 `InvokeLiteForge`
- `CallQualityPriorityAI`
- 主 Intelligence callback 的普通 `CallAI`

注册表中曾经误包含以下 Quality 任务，现已移除：

- `evaluate-internet-research-next`
- `sub_react_agent_goal_elaboration`
- `plan_from_document`
- `plan_guidance_document`

它们的调用实现没有修改。

## 二、辅助任务执行入口

### 2.1 LiteForge Speed 任务

统一通过：

```go
Config.ScheduleAuxiliaryTask(...)
```

该入口负责延迟构建 Prompt、应用注册表决策、传递 Schema/流式回调，并调用严格的 Speed 执行桥。

调度器调用 `Config.invokeSpeedPriorityLiteForge`，其 callback 顺序为：

```text
Speed → Original fallback
```

它不会回退到 Quality/Intelligence callback。通用 `Config.InvokeLiteForge` 原有的 Speed → Quality → Original 行为仅保留给原有通用调用，不再被辅助任务调度器使用。

模型调用统一由所属 Config 决定；辅助任务不提供逐任务覆盖 AI caller 的选项。Timeline 即使保留旧 `m.ai`，压缩也只通过绑定 Config 调度。测试通过 Config 注入 Speed mock，并验证旧 caller 不会执行。

### 2.2 流式辅助任务

Timeline 两处压缩和 Interval Review 已统一通过 `ScheduleAuxiliaryTask → LiteForge` 执行，业务侧不再查询策略或自行调用 AI 事务。

- `WithAuxiliaryOutputSchema` 将任务标签与原输出 action 分开，保留 `timeline-reducer` 和 `interval-toolcall-review` 协议。
- `WithGeneralConfigStreamableFieldResponseCallback` 提供字段 reader、当前响应和响应绑定的 Emitter；既支持逐块转发，也支持完整读取字段后处理。
- `WithAuxiliaryEmitter` 保留并发工具的事件归属，不修改共享 Config。
- `WithAuxiliaryOnError` 在最终失败时处理回退；成功结果只进入 `onResult`。
- Timeline 的状态更新在成功回调中完成，两处压缩均禁用 LiteForge 自动追加 Timeline。
- Interval Review 保留独立 checkpoint，并对完整 LiteForge Prompt 检查 9000-token 上限。

仍未迁移的特殊执行路径可以查询 `ResolveAuxiliaryTask`，但不能据此视为完成调度迁移。

## 三、已迁移的 LiteForge Speed 任务

生产代码中已没有活跃的直接 `InvokeSpeedPriorityLiteForge(...)` 业务调用；剩余匹配仅为接口、实现和 mock。

### 3.1 Skip 类

| CallerLabel | 原用途 | 当前入口 |
|---|---|---|
| `session-init-generator` | Session 目录名/标题初始化 | `Config.ScheduleAuxiliaryTask` |
| `session-title-generator` | Session 标题 | `Config.ScheduleAuxiliaryTask` |
| `intent-keyword-gen` | 意图关键词生成 | `Config.ScheduleAuxiliaryTask` |
| `intent-capability-recommend` | 意图能力推荐 | `Config.ScheduleAuxiliaryTask` |
| `tool-call-reason` | 工具调用展示理由 | `Config.ScheduleAuxiliaryTask` |
| `i18n-translation` | Stream Node ID 翻译 | `Config.ScheduleAuxiliaryTask` |
| `task-short-id` | 任务短标识符 | `Config.ScheduleAuxiliaryTask` |
| `smart-evaluation` | Internet Research SMART 评价 | `Config.ScheduleAuxiliaryTask` |
| `insufficient-reason-analysis` | 搜索结果不足原因 | `Config.ScheduleAuxiliaryTask` |
| `memory-triage` | 记忆提取 | `Config.ScheduleAuxiliaryTask` |
| `tag-selection` | 记忆标签选择 | `Config.ScheduleAuxiliaryTask` |
| `batch-memory-deduplication` | 记忆批量去重 | `Config.ScheduleAuxiliaryTask` |
| `perception` | 主动感知 | `Config.ScheduleAuxiliaryTask` |

这些任务原有的子系统入口 gate 仍保留。入口 gate 用于整组关闭，调度器用于统一执行和防御性决策，两者不冲突。

### 3.2 LiteCall 类

| CallerLabel | 调用位置/用途 | 当前入口 |
|---|---|---|
| `extract-explore-target-path` | Dir Explore InitTask | `Config.ScheduleAuxiliaryTask` |
| `extract-http-request-from-user-input` | HTTP Fuzz InitTask | `Config.ScheduleAuxiliaryTask` |
| `analyze-report-intent` | Report InitTask | `Config.ScheduleAuxiliaryTask` |
| `capability-catalog-match` | 能力目录语义匹配 | `Config.ScheduleAuxiliaryTask` |
| `knowledge-compress` | 长文本知识压缩 | `Config.ScheduleAuxiliaryTask` |
| `knowledge-compress-bench` | Knowledge Bench 压缩 | `Config.ScheduleAuxiliaryTask` |
| `select_knowledge_base` | 知识库选择 | `Config.ScheduleAuxiliaryTask` |
| `evaluate-next-search` | 知识增强搜索评估 | `Config.ScheduleAuxiliaryTask` |
| `plan_facts_hook` | Plan Facts 增量维护 | `Config.ScheduleAuxiliaryTask` |
| `plan_direct` | Speed 模型直接计划生成 | `Config.ScheduleAuxiliaryTask` |
| `analyze-requirement-and-search` | SyntaxFlow/Yaklang InitTask | `Config.ScheduleAuxiliaryTask` |
| `extract-ranked-lines` | Yaklang 示例片段抽取 | `Config.ScheduleAuxiliaryTask` |
| `http_fuzztest_init_booststrap` | HTTP Fuzz 灵感提示 | `Config.ScheduleAuxiliaryTask` |
| `scan_plan` | Code Audit 类别选择 | `Config.ScheduleAuxiliaryTask` |
| `llm-rerank` | Knowledge Bench 重排 | `Config.ScheduleAuxiliaryTask` |

`analyze-requirement-and-search` 有两个生产调用位置，但共用一个 CallerLabel 和决策。

### 3.3 PassThrough 类

| CallerLabel | 用途 | 当前入口 |
|---|---|---|
| `http_flow_analyze_finalize_summary` | HTTP Flow 最终摘要 | `Config.ScheduleAuxiliaryTask` |
| `skill-conflict-resolver` | Skill 加载冲突裁决 | `Config.ScheduleAuxiliaryTask` |

虽然这两个任务当前保持原 Speed 行为，它们仍通过统一入口，便于审计和后续调整。

## 四、原直接 Speed 调用的迁移状态

| CallerLabel/路径 | 原执行方式 | 当前接入方式 |
|---|---|---|
| `timeline-batch-compress` | `CallSpeedPriorityAI` | `ScheduleAuxiliaryTask` + LiteForge 字段流及成功回调 |
| `timeline-head-refine` | `CallSpeedPriorityAI` | `ScheduleAuxiliaryTask` + LiteForge；失败规则截断 |
| `toolcall-interval-review` | `CallSpeedPriorityAI` | `ScheduleAuxiliaryTask` + LiteForge；保留工具 Emitter、错误和独立 checkpoint |
| `ai_value_feedback` | 强制 lightweight LiteForge | `ResolveAuxiliaryTask` + 原强制 callback |
| Speed ReAct Loop | 条件选择 `CallSpeedPriorityAI` | 仅在 `useSpeedPriorityAI=true` 时读取辅助决策参数；普通 Intelligence Loop 不受影响 |

## 五、已清理的迁移遗留

已删除以下没有生产调用者的旧 Speed helper，避免保留两套 Prompt/解析实现：

- `generateReasonByLiteForge`
- `extractTargetPath`
- `tryBootstrapFuzzRequestFromUserInput`
- `analyzeUserIntent`
- `evaluateSMART`
- `evaluateInsufficientReason`
- `matchCapabilityCatalogChunk`

## 六、仍未接入 Config 调度器的 Speed 边界

### 6.1 Crawler

`common/crawler/js_ai.go` 的 `crawler-js-path-extract` 直接创建 `WithLiteForge_SpeedPriority`。

它已登记为 LiteCall，但 `AIJSExtractConfig` 当前没有 AID Config/辅助策略，因此注册表无法生效。迁移需要新增显式的策略注入，不能在 Crawler 内反向依赖某个全局 Agent Config。

### 6.2 RAG 与独立能力

以下包级调用显式获取 Speed callback，但没有会话 Config：

- `rag/enhancesearch` 的关键词、HyDE、拆分和泛化；
- `BuildIndexQuestions`；
- VectorStore Smart Select；
- Index Content Processor；
- Knowledge Base answer synthesis；
- CVE/CWE 翻译；
- YakScript 元数据生成。

其中 Agent 内的 RAG Enhance 当前通过 `EnhanceKnowledgeManager` 入口 gate 关闭；独立产品能力不应强行绑定 AID Config。是否需要统一策略注入，应在后续配置设计阶段单独决定。

### 6.3 Yak 脚本

生产 Yak 中仍有以下 `liteforge.speedPriority()`：

- `coreplugin/base-yak-plugin/YakScript AI元数据生成.yak` 两处；
- `aitool/.../http/web_search.yak` 的搜索结果压缩；
- `aitool/.../ssa/syntaxflow_rule_completion.yak`；
- `loop_infosec_recon/embedded/js-static-extract-ai.yak`。

Yak 调用无法读取当前 `aicommon.Config`。需要先设计上下文/策略传播接口，再迁移；不能通过修改 Quality 调用或全局替换模型规避。

## 七、验证规则

后续可以增加静态检查，保证边界不回退：

1. `common/ai/aid` 生产代码不得新增直接 `InvokeSpeedPriorityLiteForge(...)` 调用；
2. 新增 Speed LiteForge 任务必须声明 CallerLabel 并走 `Config.ScheduleAuxiliaryTask`；
3. 新增直接 `CallSpeedPriorityAI` 必须调用 `ResolveAuxiliaryTask`；
4. Quality/Intelligence CallerLabel 不得注册到辅助任务注册表；
5. 辅助调度器不得回退到 Quality callback。

## 八、当前状态

在 AID Go 代码范围内：

- 活跃 Speed LiteForge 调用已经完成迁移；
- Timeline/Interval Review 已完成 LiteForge 调度迁移；AIVE 和 Speed Loop 仍仅接入决策；
- Quality/Intelligence 调用没有修改；
- 剩余边界集中在 Crawler、独立 RAG/产品能力和 Yak 脚本的配置传播问题。

这些剩余问题需要配置方案支持，适合在“辅助任务迁移完成”之后再进入单模型模式设计阶段。

## 九、流式迁移回归结果（2026-09-18）

- 删除逐任务 caller 覆盖后，`aicommon` 整包测试再次通过（90.064s）。
- LiteForge 及新增辅助任务流式测试通过：字段未结束时不提前交付结果、旧回调兼容、响应任务标识、错误/取消/Skip、Prompt 上限。
- Interval Review 定向回归通过：并发工具事件归属、取消隔离、独立 checkpoint、调用期望、额外指令、上下文取消。
- Timeline 状态测试通过：压缩后的序列化与恢复、Head 滚动与精简、失败规则回退、失败不删除原条目。
- `common/ai/aid/...` 和 `common/aiforge` 全部通过只编译检查；未运行整个 AID 的全部行为测试。

Timeline 内部状态单测按用例注册 reducer 测试桥并在结束时恢复；真实 LiteForge 流式链路在 aiforge 集成测试中验证，避免全局注册改变其他单元测试的环境。
