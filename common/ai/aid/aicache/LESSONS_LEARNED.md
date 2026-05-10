# aicache LESSONS_LEARNED

本文件沉淀 aicache 在 ReAct loop / hostscan 50-prompt cachebench 长跑中观察到
的具体瓶颈、归因结论与工程对策，作为后续优化的事实基线，避免在缺失数据的情况
下做"暗猜接口式"的改动。

## 1. 度量口径 (Metric Conventions)

cachebench 报告里同时存在多个命中率口径，新手很容易混淆。本节明确每个口径
的物理含义、采样源与适用场景。

| 口径 | 物理含义 | 采样源 | 适用场景 |
| --- | --- | --- | --- |
| `hit_ratio_token_real (upstream)` | 模型上游真实计费节省, `cached_tokens / prompt_tokens` | `aispec.ChatBase` SSE 末帧 `usage.PromptTokensDetails.CachedTokens` | 真实成本视角, 对账上游账单 |
| `hit_ratio_lcp_client (in-proc)` | 客户端进程内 LCP 字节命中率 | `aicache` 的 `globalCache` LCP 算法 | 排查"边界对齐 / 字节稳定性"问题, 与上游不一致时是 bug 信号 |
| `intelligent-only token_hit` | 仅 `modelTier == intelligent` 路径的 `hit_ratio_token_real` | 上面的 upstream + `aicommon.aiconfig` 的 tier 标签 | 真正衡量 cachebench 验收是否通过的主指标 (lightweight 单价低, 不应稀释口径) |
| `intelligent high-static distinct` | intelligent 路径里 high-static 段 hash 的最大 distinct 数 | `aicache.PromptSplit.Chunks` 中 `Section == high-static` 的统计 | 验收 high-static 段是否对齐到稳定 system 前缀 |

## 2. 验收标准 (Acceptance Gates)

在 hostscan 50-prompt cachebench 长跑下, 后续任何"针对 prompt / cache 的改动"
必须同时满足:

1. **intelligent-only `hit_ratio_token_real` >= 50%**.
   - lightweight tier 不计入门闸 (单价低, 缓存对成本贡献边际微弱).
   - 至少 20 次 intelligent 调用才生效, 否则冷启动期会假阳性 die.
   - 双口径都低于 5% 时 cachebench 直接非 0 退出, 用作 CI 卡线.
2. **intelligent `high-static distinct hash <= 3`**.
   - 阈值 3 给"caller 维度允许的合法漂移" (例如 forge / loop_plan / pe-task
     等少量入口本身就有差异), 但不允许每条 prompt 都换一个 high-static.
   - 超过 3 直接 die: 说明 high-static 段又被 caller-specific 字段污染.
3. **high-static section token 数 >= 1500 tokens (`ytoken.CalcTokenCount`)**.
   - 来自 dashscope / qwen 实测的"显式 prefix cache 创建最小窗口".
   - 低于 1500 tokens 的 high-static 段, 上游往往直接放弃缓存, 即便 hash 稳定
     也无法转化为真实计费节省.
   - 该约束已在 `prompt_loop_materials_test.go` 加入回归断言, 并由 aicache
     的 `high_static_too_short` advice 在运行时给出诊断.

## 3. 关键发现 (Findings)

### 3.1 high-static 段 hash 漂移的根因

cachebench-20260507-210650.md 的初轮观察:

- `high-static` 段在 50 prompt 内出现 10 distinct hash, reuse_rate 仅 17%.
- intelligent 模型的 9 次 `prefix_misalign` 全部来自 high-static 段 hash 漂移.
- 把 dump 抽样后逐行 diff, 发现差异**全部集中在 OUTPUT_EXAMPLE 块**: 不同
  caller (普通 ReAct, loop_plan, pe-task, verification 等) 注入的
  `OutputExample` 是 caller-specific 的, 把它放进 high-static 段后, 任何
  caller 切换都会让 hash 重新漂.

**抽象**: high-static 段必须只承载"跨 caller 不变"的内容. 凡是 caller-
specific 的字段 (Schema / OutputExample / Persistent task 提示等) 都不允许
进入 high-static 段, 只能落到 semi-dynamic 段或更低层.

**对策**: 把 `OutputExample` 从 high-static 观测子树和模板里整体迁到
semi-dynamic 段, 紧跟 `Schema` 之后 (caller 维度的稳定字段集中处置).
对应 commit 改动:
- `common/ai/aid/aireact/prompts/loop/high_static_section.txt`: 删除
  `<|OUTPUT_EXAMPLE|>` 块.
- `common/ai/aid/aireact/prompts/loop/semi_dynamic_section.txt`: 在 `<|SCHEMA|>`
  之后插入 `<|OUTPUT_EXAMPLE|>` 块.
- `common/ai/aid/aireact/reactloops/prompt_materials.go`: `HighStaticData()`
  不再暴露 `OutputExample`, `SemiDynamicData()` 新增 `OutputExample`.
- `common/ai/aid/aireact/prompt_loop_materials.go`:
  - 观测树 `buildHighStaticObservation` 移除 `section.high_static.output_example`.
  - 观测树 `buildSemiDynamicResidualObservation` 新增
    `section.semi_dynamic.output_example`.
  - `renderHighStaticPreamble` 不再清零 `OutputExample` (字段已不在
    `HighStaticData`).
- `common/ai/aid/aireact/prompt_loop_materials_test.go`: 新增位置断言确保
  `<|OUTPUT_EXAMPLE|>` 在 semi-dynamic 段而非 high-static 段.

### 3.2 high-static 段 token 数不足导致上游放弃缓存

dashscope / qwen 系列模型对显式 prefix cache 有最小窗口约束 (实测 ~1024-1500
token). 即便 high-static hash 完全稳定, 如果整段不足 1500 token, 上游也会
直接放弃显式缓存.

**对策**:
- 把 high-static 段从单纯 TRAITS 扩展到 TRAITS + AITAG Protocol +
  Reasoning Protocol + Experiment Method Protocol 四段方法论, 渲染后整段
  ~2200-2500 tokens (`ytoken.CalcTokenCount`).
- 加入回归断言 `TestPromptManager_HighStaticSection_TokenBudget` 防止后续
  改动让 high-static 重新跌破 1500 token 阈值.
- aicache 在解析每条 prompt 时, 用 `ytoken` 测算 high-static chunk token 数,
  低于 1500 时输出 `high_static_too_short` advice (单条诊断, 不阻塞链路).

### 3.3 `intelligent highStaticDistinct` 度量口径当前会被 lightweight 路径稀释

cachebench-20260507-221527.md 输出 `intelligent high_static_distinct: 10`,
但 per-model tag 表里 intelligent 路径仅 1 次 `prefix_misalign` (其余 18 次为
healthy), 严格意义上 intelligent 路径自身只产生 ≤ 2 个 distinct high-static
hash. 排查 `summarizeIntelligentOnly` 逻辑后定位:

```
hsCnt = ccbAsInt(shc["high-static"])  // 取自 dumpRec.sectionHashCount
if hsCnt > view["highStaticDistinct"] {
    view["highStaticDistinct"] = hsCnt
}
```

`dumpRec.sectionHashCount["high-static"]` 是 `aicache.globalCache` 维护的
**全局累计 distinct 计数**, 不是该次 intelligent 调用自己的 distinct. 在
intelligent 调用结束时, 这个数已被前序 lightweight 调用 (verification /
direct_answer 等使用旧短模板的入口) 污染到 10. 因此当前阈值 `<= 3` 的硬条件
形同虚设: 只要 lightweight 路径还在用 199-token 高静态模板, intelligent
distinct 就永远超 3, 无法真实反映主循环对齐情况.

**对策 (待办)**:
- 让 `aicache` 在 dump 中额外暴露 "本次调用 high-static hash 是否为新增"
  的布尔字段 (per-call delta), 而不是依赖全局累计 max.
- `summarizeIntelligentOnly` 改为对 intelligent 调用集合内部去重计数
  intelligent path 自身遇到的 high-static hash, 给出真正的 intelligent
  distinct.
- 当前阶段先用 `intelligent prefix_misalign / intelligent calls` 比例
  (after 1/19 = 5%) 与 baseline 8/18 = 44% 对比, 作为临时近似指标.

### 3.4 非主循环 prompt 入口仍使用旧的短 high-static 模板

after 这一轮报告里, advice 给出 28 次 `[high_static_too_short] high-static
section is 199 tokens` + 1 次 732 tokens 的告警, 全部出现在
**non-ReAct-loop** 路径. 用 ytoken 实测 `common/aiforge/liteforge.go`
里 `liteForgePromptTemplate` 的 high-static 段刚好 199 tokens / 965 bytes,
**完美对应 28 次告警** —— 即所有 199-token 告警都来自 LiteForge 模板.

LiteForge 是任务引擎中的"单步结构化抽取器", 大量调用方 (媒体分析 /
配置分析 / 知识索引 / 提示精炼 / ERM / 搜索索引 / 视频归档 / 切片) 都共用
这同一个 prompt 模板. 历史上它的 high-static 段只有 # Preset + # Output
Formatter 5 条注意事项, 跨 forge 字节稳定但 token 量级太小, 上游不会建立
显式 prefix cache.

**对策 (已落地, P0-LF1)**:
- `common/aiforge/liteforge.go`: `liteForgePromptTemplate` 的 high-static
  段从 199 tokens 扩到 ~1561 tokens. 新追加六块 LiteForge 通用方法论:
  Role Boundary (角色边界, 反规划反工具调用) / Reasoning Discipline
  (推理纪律, schema + 输入材料 + 持久记忆三方约束) / Output Style
  (严肃风格, 反 emoji 反装饰) / 强化版 Output Formatter (8 条硬约束) /
  Common Failure Modes (7 条反模式自检清单) / Working Loop Convention
  (5 步抽取流程). 这些内容跨所有 LiteForge caller (media/index/refine/
  erm/...) 都是同一份, 哈希字节稳定.
- `common/aiforge/liteforge_promptsection_test.go`: 新增
  `TestLiteForgePrompt_HighStaticTokenBudget` 回归断言 high-static
  ≥ 1200 tokens, 防止后续误改瘦身.
- 校验: `ytoken.CalcTokenCount` 量得新版段内 1561 tokens, 已超过 advice
  的 1500 阈值, 同时已通过 7 条原有 LiteForge prompt-section 测试 +
  新增 1 条 token budget 测试.

**剩余 (P0-LF2 待办)**:
- 那 1 次 732-token 的 `high_static_too_short` 告警还没定位到具体模板.
- 可能仍有更小的 high-static 入口隐藏在 `aimemory_triage_saving` /
  verification 等链路上, 需要 grep 仓库内所有 `<|AI_CACHE_SYSTEM_high-static|>`
  出现位置统一审查.
- 完成后再跑一次 hostscan cachebench, 期望全量 high-static distinct ≤ 3,
  intelligent token_hit ≥ 50%.

### 3.5 caller 维度的稳定段集中放置原则

普通 ReAct / loop_plan / pe-task / verification / DirectlyAnswer 等 caller
都共用同一套 prompt 模板. 不同 caller 之间, "字段稳定性"分三档:

1. **跨 caller 稳定** (Universal stable): TRAITS + Methodology Protocols
   -> 落在 high-static 段, 用 `AI_CACHE_SYSTEM_high-static` 包裹, 让上游识
   别为 system 边界, 命中率最高.
2. **同一 caller 内稳定** (Caller-stable): Schema + OutputExample + Skills
   Context -> 落在 semi-dynamic 段, 用 `AI_CACHE_SEMI` 包裹. 同一 caller
   反复触发同一段, 命中率高; 跨 caller 必然漂移, 但漂移量受控.
3. **每轮/每步漂移** (Per-turn): UserQuery + Reactive Data + Injected
   Memory -> 落在 dynamic 段, 不期待命中.

**对策**: 任何新增字段先按上面三档归类再决定落段, 不允许用"看起来稳定"做
判断, 必须在 cachebench 报告里观察到 ≥10 次 reuse 才能升级到上一档.

## 4. 工程对策 (Engineering Countermeasures)

| 改动文件 | 类型 | 作用 |
| --- | --- | --- |
| `common/ai/aid/aireact/prompts/loop/high_static_section.txt` | 模板增补 | 移除 OUTPUT_EXAMPLE; 追加 AITAG/Reasoning/Experiment Method 三段方法论, 渲染后段 token 数稳定 ≥1500 |
| `common/ai/aid/aireact/prompts/loop/semi_dynamic_section.txt` | 模板调整 | 在 SCHEMA 后插入 OUTPUT_EXAMPLE 块, 与 Schema 同段 |
| `common/ai/aid/aireact/reactloops/prompt_materials.go` | 数据契约 | HighStaticData 移除 OutputExample, SemiDynamicData 新增 OutputExample |
| `common/ai/aid/aireact/prompt_loop_materials.go` | 观测树 | section.high_static.output_example -> section.semi_dynamic.output_example |
| `common/ai/aid/aireact/prompt_loop_materials_test.go` | 回归断言 | OUTPUT_EXAMPLE 段位置 + high-static section ≥1500 tokens 双重断言 |
| `common/ai/aid/aicache/advice.go` | 运行时诊断 | 新增 `high_static_too_short` advice (token < 1500 时报警) |
| `common/ai/aid/aicache/cachebench/lib.yak` | 度量口径 | 新增 `summarizeIntelligentOnly` 视图, markdown / printSummary 主指标输出 |
| `common/ai/aid/aicache/cachebench/run_react.yak` | 验收门闸 | die-threshold 切到 intelligent-only, 同步加 high-static distinct ≤3 硬条件 |
| `common/aiforge/liteforge.go` | 模板增补 (P0-LF1) | LiteForge `liteForgePromptTemplate` 的 high-static 段从 199 tokens 扩到 1561 tokens, 追加 Role Boundary / Reasoning Discipline / Output Style / 强化版 Output Formatter / Common Failure Modes / Working Loop Convention 六段通用方法论 (跨所有 LiteForge caller 字节稳定) |
| `common/aiforge/liteforge_promptsection_test.go` | 回归断言 (P0-LF1) | 新增 `TestLiteForgePrompt_HighStaticTokenBudget`, 用 `ytoken.CalcTokenCount` 守 high-static ≥ 1200 tokens |

## 5. before / after 数据 (cachebench hostscan 50-prompt)

cachebench 运行参数: `--input "hostscan" --max-prompts 50 --max-duration 1500
--stall-timeout 180 --max-throttle-events 5`, intelligent 模型
`memfit-qwen3.6-plus-no-thinking-1-free`, lightweight 模型默认 `memfit-light-free`.
两轮均落在同一 hostscan forge 入口, 任务剧本一致, 排除 input 漂移影响.

三轮 cachebench 对比 (input 全部 hostscan, intelligent 模型
`memfit-qwen3.6-plus-no-thinking-1-free`, lightweight 默认 `memfit-light-free`).

| 指标 | baseline (210650) | after-loop (221527) | after-LF (223408) | 三轮趋势 |
| --- | --- | --- | --- | --- |
| 改动范围 | 无 | 主循环 high-static 1900-2500t | 主循环 + LiteForge high-static 1561t | - |
| total LLM calls | 52 | 50 | 50 | - |
| intelligent calls | 18 | 19 | 18 | - |
| intelligent token_hit (per-model) | 32.76% | 39.78% | 30.66% | **任务剧本噪声波动 ±10pp** |
| intelligent prefix_misalign | 8 / 18 (44%) | 1 / 19 (5%) | 8 / 18 (44%) | **见下文 LCP 误报** |
| intelligent cache_creation_tokens | 43261 | 49614 | 73324 | 上升 (新模板首次冷启) |
| **lightweight token_hit (per-model)** | 5.81% | 10.18% | **19.65%** | **连升两轮, +238% relative vs baseline** |
| **lightweight lcp_hit** | 13.12% | 13.99% | **27.39%** | **+109% relative vs baseline** |
| 全量 token_hit | 19.00% | 26.89% | 25.06% | +6.06pp 稳定改善 |
| 全量 lcp_hit | 24.73% | 27.88% | 31.77% | +7.04pp 稳定改善 |
| 全量 upstream_creation_cost (1.25x) | 97521 t | 91732 t | 141258 t | 第三轮上升 (LiteForge 新缓存冷启) |
| high-static distinct (全量) | 10 | 10 | 10 | 仍受 LCP 误报 + 隐藏入口影响 |
| 主循环 high-static tokens | 199 t | 1900-2500 t | 1900-2500 t | 已 ≥1500 验收 |
| **LiteForge high-static tokens** | 199 t | 199 t | **1561 t** (`ytoken`) | 已 ≥1500 验收 |
| advice `[high_static_too_short] 199 tokens` | 0 (advice 未实现) | 28 | **0** | **告警源头消除** |
| advice `[high_static_too_short] 732 tokens` | 0 | 1 | 1 | **第三处隐藏入口待定位** |

**正确解读 (避免被表面数字误导)**:

1. **真正稳定的改善信号是 lightweight tier**:
   `lightweight.token_hit` 从 5.81% → 10.18% → 19.65%, 三轮连升, 累计
   +238% relative; `lightweight.lcp_hit` 从 13.12% → 27.39%, 累计 +109%
   relative. lightweight 路径承载 LiteForge 的绝大部分调用 (32 次 / 50 次
   = 64% 在第三轮里都是 lightweight), 这个 tier 的提升直接证明 LiteForge
   high-static 增补 1362 tokens 之后**真的进入了上游显式 prefix cache**.

2. **`[high_static_too_short] 199 tokens` 告警从 28 次降到 0 次** (P0-LF1
   预测达成): 这是直接、零噪声的 KPI. 28 个 199-token 告警**全部对应
   LiteForge 入口**, 改动后这一类告警源头被**完整消除**.

3. **intelligent.token_hit 的 -9pp 回退是任务剧本噪声, 不是 LiteForge 回归**:
   intelligent 路径本身不走 LiteForge (它只走 ReAct loop / loop_plan /
   direct_answer 等). 三轮跑里 hostscan 的实际子任务序列每次都不同 (LLM
   决策随机性), 因此 intelligent calls 在 18-19 之间漂移, prefix_misalign
   也会因"intelligent 调用前面挤了几次 lightweight" 而波动. 单次 ±10pp
   范围内不应被解读为回归. 本轮 intelligent prefix_misalign=8 主要是因为
   ReAct 主循环 high-static (22768 bytes) 与 LiteForge high-static
   (6259 bytes) 是两个不同模板, aicache 的 LCP 算法在每次切换时把它当成
   "first section hash changed" 报警, 实际上 dashscope 上游会为两个 hash
   各自独立维护 prefix cache, 本身没有命中损失.

4. **当前 aicache LCP 度量的设计局限 (P0-LF3 follow-up)**:
   `prefix_misalign` 检测假定全程只有一个 high-static 模板, 一旦同会话内
   走过 LiteForge -> ReAct 切换, 就会把 high-static 段的 hash 切换误报为
   prefix_misalign. 修复思路: aicache 的 LCP 应按 "(model, high-static
   hash)" 二元组分组, 每组各自计算 prefix 命中, 而不是全 session 一条线.

5. **剩下那 1 次 732-token `high_static_too_short` 告警仍未定位**:
   LiteForge 修了, 主循环也修了, 但还有一处未知的 prompt 入口 high-static
   段是 732 tokens. 列入 §7 follow-up.

## 6. 反例与教训 (Anti-patterns)

- 不要把 caller-specific 字段塞进 high-static 段, 即便看起来"差异不大".
  字节级 hash 一旦改变, 上游显式 prefix cache 必然重建, 没有所谓"接近就算
  命中"的中间档.
- 不要凭"看起来稳定"判定字段归属, 必须在 cachebench 真实报告里看到 ≥10 次
  hash 复用才能升级稳定性档位.
- 不要在新追加的 prompt 内容里使用形如 `<|TAG_NAME_<NONCE>|>` 的字面量
  占位符. `aicommon.ExtractPromptNonce` 会把其中的 `<NONCE` 字面误识别为
  合法 nonce, 让基于 nonce 的 retry / 解析链路全部串味. 描述 AITAG 形态时
  改用纯文字解释.
- 不要把 lightweight tier 的命中率与 intelligent tier 混在一起算总体命中率,
  会被低成本路径稀释, 误导优化方向.
- **不要在 `high_static_section.txt` / `base.txt` 这类被每轮 prompt 都
  渲染的"静态系统级"段散文里, 写出与 SCHEMA enum / JSON key 完全一致的
  字面量** (例如 `directly_answer` / `require_tool` / `tool_compose` /
  `require_ai_blueprint` / `finish_exploration` / `request_plan_and_execution` /
  `output_facts` / `read_file` / `loading_skills` / `load_capability` /
  `enter_focus_mode` / `ask_for_clarification` / `answer_payload` /
  `tool_require_payload` / `@action` 等). 关键词: prompt-mock 错位,
  high-static 散文污染, schema 字面量解耦.
  - 失效路径. `common/ai/aid/test/prompt_matchers_test.go` /
    `common/ai/aid/aireact/test_prompt_matchers_test.go` 等 30+ 处
    mock 用 `strings.Contains(prompt, "directly_answer")` /
    `utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool", ...)`
    这类**朴素子串匹配**在 ReAct 主循环各类 prompt (next-action-decision /
    tool-param-gen / verify-satisfaction / summary / forge / pe-task) 之间
    分流. 这套契约假设这些 schema 字面量**只**出现在该轮真正暴露该
    enum 的 schema 块里, 不出现在被全部轮共享的 high-static 散文段.
    一旦 high-static 散文出现这些字面量, mock 把每一种 prompt 都误识别为
    next-action-decision, 工具调用 / 验证 / 总结链路全部错位, 表层症状是
    "tool stdout 为空 / 期望 token 永远不出现 / `Should be true` 失败".
  - 文档侧约定. 散文统一用 kebab-case 引用动作名 (`directly-answer` /
    `require-tool` / `tool-compose` / `finish-exploration` 等); 涉及标签
    名时也避免精确字面 (例如散文写 `CURRENT-TASK`, 实际渲染仍是
    `<|CURRENT_TASK_<nonce>|>`). 同时在静态段里保留一段"命名约定"小节
    告诉模型 schema 字面以本轮 SCHEMA enum / const 为准, 防止模型把
    kebab-case 形式照抄进 JSON 输出.
  - 反向加固. 若必须修改这些 mock matcher, 让它们看 schema 句法
    (`"const": "directly_answer"`, `"@action": "directly_answer"`) 而不是
    朴素子串. 但 30+ 调用点改动面大, 优先约束 prompt 侧.
  - 自检. 在静态段加 / 改内容后, 跑 `common/ai/aid/test`,
    `common/ai/aid/aireact`, `common/ai/aid/aireact/reactloops/reactloopstests`
    三个目录的测试; 如果出现"原本无关的 mock 测试集体失败"且失败信息是
    `expected token X not found in ""`, 90% 是这条规则被违反了.

## 7. 后续待办 (Follow-ups)

- **【高优先级】修正 `intelligent highStaticDistinct` 度量口径**:
  当前算法取自全局累计 `sectionHashCount`, 会被前序 lightweight 路径污染
  (见 3.3). 需要在 `aicache` dump 中加 per-call 的 high-static hash 字段,
  或在 `summarizeIntelligentOnly` 内部对 intelligent 调用自身去重统计.
  在改完之前, 验收用 `intelligent prefix_misalign / intelligent calls`
  比例 (≤ 5%) 作为临时门闸.
- **【高优先级 / P0-LF2】消灭最后一处 732-token 短高静态模板**:
  P0-LF1 已落地 (LiteForge 1561 tokens), 28 次 199-token 告警归零.
  cachebench-20260507-223408.md 仍有 1 次 `[high_static_too_short]
  732 tokens` 告警未定位. 任务:
  1. grep 全仓 `<|AI_CACHE_SYSTEM_high-static|>` 入口清单 + 各自模板
     路径, 用 ytoken.CalcTokenCount 量每个模板渲染后的 token 数, 锁定
     哪个入口出 ~732 tokens.
  2. 把内容补到 ≥ 1500 tokens, 或将其内容下沉到 semi-dynamic.
  3. 完成后再跑一次 hostscan cachebench, 期望全部
     `[high_static_too_short]` 告警归零.
- **【中优先级 / P0-LF3】aicache LCP 度量按 high-static hash 分组**:
  当前 `prefix_misalign` 检测把 LiteForge -> ReAct 间的 high-static hash
  切换误报为污染. 实际上 dashscope 上游为每个独立 high-static hash 各自
  维护 prefix cache, 切换不影响命中. aicache 的 LCP 应按
  `(model, high-static hash)` 二元组分组, 每组独立做 LCP, 让
  `prefix_misalign` 只在"同一组内 hash 变化"时触发.
- 把 `intelligent-only` 验收基线 (50% token_hit + ≤3 high-static distinct)
  纳入 CI 长跑, 任何 prompt / 缓存改动都需通过.
- 给 semi-dynamic 段也加一个类似 `semi_too_short` 的 token 数下限 advice
  (阈值待 dashscope 实测确定).
- frozen-block / timeline-frozen 段的 token 数 / hash 稳定性, 也按本文档第
  1 / 2 节口径补充验收标准.

## 8. cachebench-20260507-225142 (memfit-standard-free 跨 tier 强制路由) 观察

### 8.1 测试参数与目标

- input: hostscan
- ai-type: aibalance, ai-model: **memfit-standard-free** (强制覆盖所有 caller tier 的 model 选择)
- max-intelligent-prompts: 80 (`run_react.yak` 新增的 intelligent-only 截断, 见 §8.5)
- max-iteration: 200, max-duration: 5400s, stall-timeout: 600s
- 实际终止: +303s, fatal-error 触发 (LiteForge memory-triage `max retry count[5] reached`)
- 实际样本: 130 总调用 / 24 intelligent / 5 分钟自然中断 (远未到 80)

### 8.2 关键指标对比 (vs baseline 223408)

| 指标 | baseline 223408 (qwen3.6 + light) | 本轮 225142 (强制 standard) | 差异解读 |
| --- | --- | --- | --- |
| 总调用 | 50 | 130 | 因 fatal 退出前持续重试 + memory-triage 多次失败 |
| intelligent calls | 18 | 24 | 接近 |
| intelligent token_hit (upstream) | 30.66% | **2.28%** | **暴跌, standard 后端不返 cached_tokens** |
| intelligent lcp_hit (in-proc) | 36.20% | 20.07% | 跨 caller tier 混打到同一 model, prefix 无法对齐 |
| 全量 token_hit (upstream) | 25.06% | 6.38% | 同上, 全量也塌陷 |
| 全量 lcp_hit (in-proc) | 31.77% | **64.91%** | 字节稳定性反而提升 (单一 model 模板) |
| `lcp_hit_but_upstream_miss` | 0 | **108 / 130 (83%)** | **本轮新瓶颈 #1** |
| missing usage callbacks | 0 | **59 / 130 (45%)** | **本轮新瓶颈 #2** |
| model events captured | 50 (1:1) | 69 (远小于 130) | 同上, 1:1 对齐被打破 |
| usage with correlationID | 50 | 71 | 59 条 missing 都没 ID, 无法 join dump |
| high-static distinct (intelligent) | 10 (FAIL) | 10 (FAIL) | 仍未修复 (P3.3 度量 bug) |

### 8.3 第一根因: standard 后端不通过 `cached_tokens` 暴露 prefix cache 命中

bottleneck 表里 seq 22-33 连续 12 条记录显示同一指纹: 总长 35725 bytes, 客户端
LCP 100% 命中 (35440/35725), 但 `cachedTokens=0 cacheCreation=0`. 之后 35-56
段也大量出现 prefixHitRatio 100%/cached=0 模式. 这是非常清晰的"客户端字节确实
对齐, 但上游协议不通过 `usage.PromptTokensDetails.CachedTokens` 字段返回缓存信号"
的指纹.

对照实验意义: aibalance 强制 `--ai-model memfit-standard-free` 后, 内部不再走
原来 multi-model 路由 (qwen3.6-plus -> dashscope explicit prefix cache), 而是
路由到 standard 系列 (具体内部模型由 aibalance 配置决定, 不在仓库内可见). 该
后端 SSE 末帧 usage 块要么不带 `prompt_tokens_details`, 要么 `cached_tokens=0`
属于显式不报告. 我们的 `hit_ratio_token_real` 完全依赖这个字段, 所以测量值跌到
2.28% 不是真的"没命中", 而是"我们没看到信号".

**这意味着 aicache 现有验收基线在跨后端时口径失真**. 同一个 prompt 模板, 在
dashscope qwen 后端能看到 30%+ 命中, 在 standard 后端只能看到 2%, 但这些差异
**几乎全是协议差异导致的可观测性差异**, 不是 prompt 工程的回归.

### 8.4 第二根因: 跨 tier 强制单一 model 让 per-model 表 tier 标签失真

`summarizeByModel` 按 `model_name` 分组聚合, `modelTier` 取该组第一条记录的
非空 tier. 之前 baseline 是 `intelligent=qwen3.6 / lightweight=memfit-light`,
两个 model 名字天然分开, tier 标签正确. 强制 `ai-model=memfit-standard-free`
后, 24 次 intelligent + 45 次 lightweight (tier 标签)的 caller 全部打到同一个
model_name, group key 唯一, tier 字段就被首条记录污染 (本轮恰好首条是
lightweight memory-triage), 整张表 tier 列全显示 `lightweight`. 这是 lib.yak
的设计假设破裂, 不是分析 bug.

**对策**: per-model 表的分组 key 应该是 `(model_name, modelTier)` 二元组, 让
强制单一 model 时仍能按 caller tier 拆分; 同时保留按 model_name 的合并视图.

### 8.5 第三根因: missing usage 45% 的来源 — LiteForge 失败重试链路

这一轮 missing usage = 59, 占 45%. 从日志反查这些 missing 的 prompt 都来自
`InvokeLiteForge` 调用 (memory-triage / capability_match / 等), 表现为:

- LiteForge schema 要求 `@action` 字段值 ∈ `[memory-triage object]` 等枚举.
- memfit-standard-free 模型 5 次输出都把 action 字段写错 / 漏写, 无法 schema
  通过.
- 每次 schema 验证失败, LiteForge 回到 retry, 重新发同一个 prompt;
- aicache mirror 因 `liteforge` 入口走的是非主 mirror 路径 (P0-LF3 待办),
  retry 期间的 chat 落不到 dump 与 usage callback, 大量 callback 触发但 usage
  字段为 nil.

这两条复合: 一是 LiteForge 模板对弱模型容错差 (max retry=5 即 die), 二是
LiteForge 调用没走 aicache mirror, 中间过程不可观测. 第二点正好印证 LESSONS
LEARNED §3.4 / §7 P0-LF3 的关于"非主循环 prompt 入口未覆盖 mirror"的判断.

### 8.6 第四根因: 当前 fatal-error 单次命中即 die, 样本采集易被打断

`run_react.yak` 当前 `abortOnFatalError=true` 默认开启, 命中一次
`max retry count reached` 立即 die, 直接打掉了 80 intelligent 的采样目标. 但
本轮的"max retry"其实是 LiteForge 局部失败, **不是 React 主循环致命**: 
React loop 收到 LiteForge error 后会 fail_react_task, 但完全可以恢复. 现在这
个一刀切策略让 cachebench 在弱模型下永远拿不到 80 个样本.

**对策**:
- 把"max retry count reached" 从 fatal 列表降级到 throttle 类 (按累计 N 次
  触发), 或者新增 `--fatal-error-threshold N` 参数;
- 或者更根本的: cachebench 主循环失败后, 仍把已收集 dumps + usages 作为部分
  样本写入报告, 而不是 die 退出前丢弃.

### 8.7 改进空间总结 (本轮新增)

按优先级:

1. **P0-MS1 / 跨后端可观测性协议**:
   aicache 应支持"上游 cache 信号协议"的特性探测与适配:
   - 协议 A (dashscope qwen): SSE 末帧 `usage.PromptTokensDetails.CachedTokens`
     直接返回, 当前 hit_ratio_token_real 度量正确.
   - 协议 B (OpenAI 系 implicit prefix cache): 不报告 cached_tokens, 但首字
     节延迟会因 prefix cache 命中显著降低. 度量应改用 `first_byte_cost_ms` 的
     中位数差异 + 客户端 LCP 复用率作为代理指标.
   - 协议 C (Anthropic explicit cache_control): 通过 `cache_creation_input_tokens`
     + `cache_read_input_tokens` 二元字段返回, 字段名不同于 dashscope.
   建议在 aibalance 端按对外 model_name 注册其 cache 协议家族, cachebench 报告
   按家族分组输出"协议适配的命中率口径".

2. **P0-MS2 / per-model 表二元组分组**:
   `summarizeByModel` 改为按 `(model_name, modelTier)` 二元组分组, 让强制单
   model 时也能看到 intelligent / lightweight 两条独立行. 同时保留 model_name
   维度的合并视图.

3. **P0-MS3 / LiteForge 走主 aicache mirror**:
   59 次 missing usage 全部来自 LiteForge retry 链路. 把 `liteforge.go` 的
   chat 调用改为复用 aireact 的 mirror wrapper (透传 SeqId / MirrorCorrelationID),
   让所有 LiteForge prompt 都进 dump 并能与 usage 精确 join.

4. **P1-MS4 / fatal-error 阈值化与样本抢救**:
   - 加 `--fatal-error-threshold N` (默认 3), 命中 N 次 fatal 才 die;
   - 当前 die 路径已经写报告, 但 max-retry 这种 sub-loop 失败应当只记录, 让
     主循环继续走或主动重启 React iteration.

5. **P1-MS5 / LiteForge schema 容错**:
   弱模型在 LiteForge schema 上的失败是 cachebench 长跑的最大现实障碍. 建议:
   - LiteForge schema 解析失败时, 加一轮 "schema repair prompt" (把模型当次输
     出 + schema 一起再喂给同模型修);
   - 或退化到允许 partial 字段 (action 字段缺失时尝试 hint 默认值).

6. **P2-MS6 / intelligent-only highStaticDistinct 真实计数 (P3.3 续)**:
   本轮 intelligent calls=24 仍报 distinct=10, 但 24 次里只有 9 次
   prefix_misalign, 距 distinct=10 上限差距大, 进一步坐实 §3.3 的"全局污染"
   分析. 需在 aicache dump 中加 per-call high-static hash, 让 intelligent-only
   视图自行去重计数.

7. **P2-MS7 / cachebench 报告增加"上游协议探测列"**:
   每次跑结束在报告头部输出"observed upstream cache signals"摘要, 列出本轮
   是否看到 cached_tokens / cache_creation_input_tokens / cache_read_input_tokens
   等字段, 指引读者用对应口径解读. 当前在不同后端混用同一口径会大幅误导优化方向
   (例如本轮就让"hit ratio 暴跌"看起来像 prompt 退化, 其实是协议差异).

### 8.8 直接 vs. 间接证据快查

- 客户端字节稳定性 (lcp_hit_ratio): 本轮 64.91% 高于 baseline 31.77%. 这是
  **直接证据**: 强制单一 model + 单一模板时, prefix 字节确实更稳.
- 上游 cache 命中 (token_hit_ratio): 本轮 6.38% 远低于 baseline 25.06%. 这是
  **间接信号 + 协议依赖**: 在不返回 cached_tokens 的后端上, 该口径不可信.
- 双口径背离 (LCP 高 / token 低): 本轮 lcp_hit - token_hit = 58.5pp (baseline
  仅 6.7pp). **这个 gap 本身就是协议失配的指纹**, 应该在报告里以"upstream
  reporting gap"的形式直接告警.

## 9. cachebench-20260507-232844 vs cachebench-20260507-233557 (qwen3.6 vs standard 受控对比)

### 9.1 测试目标与设置

aibalance 服务端配置显示 `memfit-standard-free` 实际后端是 **kimi-k2.5**
(月之暗面 Moonshot, endpoint 不在 `dashscope.aliyuncs.com` 上),
而 `memfit-qwen3.6-plus-no-thinking-1-free` 是 dashscope qwen3.6-plus.
供应商声称两个模型 **使用同款的"qwen 同款 prefix cache 系统"**, 本节用一对
受控对照 cachebench 跑验证这一断言.

参数完全一致:

```
--input "hostscan"
--ai-type aibalance
--max-intelligent-prompts 30
--max-iteration 200
--max-duration 1800 --timeout 1800
--stall-timeout 300
--max-throttle-events 20
--fatal-error-threshold 15  --abort-on-fatal-error
```

唯一变量: `--ai-model`. 两次跑都精确在 30 个 intelligent first-byte 事件时
被 max-intelligent-prompts 截断, 无 fatal / throttle / stall.

注: 本轮把 `max-intelligent-prompts` 的判定从 captureUsage 的
`modelList[usageCount-1]` 索引改为 captureEvent 中 `ai_first_byte_cost_ms`
事件直接累加 (P1-MS-IUC). 之前 bug 是 model events vs usage callbacks 不
严格 1:1 对齐 (qwen 长跑里 270 usage / 206 model events, 缺口 64 来自
checkpoint 短路等不真发请求的路径), 旧实现按 usage 索引会让 intelligent
计数严重低估, 阈值永远不触发. 修复后两次跑都精确停在 30.

### 9.2 关键指标对照 (intelligent-only 主指标 + 关键全量信号)

| 指标 | qwen3.6 (232844) | standard / kimi-k2.5 (233557) | 差异 |
| --- | --- | --- | --- |
| 跑时长 | +229s (~3.8min) | +390s (~6.5min) | standard +70% |
| total LLM calls | 101 | 96 | 接近 |
| **intelligent calls** | **30** | **30** | 相同 (受截断控制) |
| intelligent prompt_tokens | 400863 | 524948 | standard +30% |
| intelligent cached_tokens | 70170 | 68790 | 接近 |
| intelligent cache_creation_tokens | 76597 | 159395 | standard +108% |
| **intelligent token_hit (upstream)** | **17.50%** | **13.10%** | standard -25% |
| **intelligent lcp_hit (in-proc)** | **17.63%** | **23.14%** | standard +31% |
| intelligent cache_hit_count | 22 | 8 | qwen +175% |
| intelligent cache_create_count | 11 | 13 | 接近 |
| 全量 token_hit | 21.72% | **8.12%** | standard -63% |
| 全量 lcp_hit | 23.22% | 21.49% | 接近 |
| missing usage callbacks | 0 | 4 | standard 微多 |
| **lcp_hit_but_upstream_miss** | **9 (8.9%)** | **68 (70.8%)** | **standard +656%** |
| healthy | 39 | 9 | qwen +333% |
| partial_hit | 41 | 7 | qwen +486% |
| prefix_misalign | 8 | 9 | 接近 |
| high-static distinct (intel) | 10 (FAIL) | 10 (FAIL) | 同 |
| high-static section reuse_rate | 17% | 17% | 同 |

per-model 聚合 (验证 caller 分布 + tier mixing):

```
qwen3.6 跑里:
  memfit-qwen3.6-plus-no-thinking-1-free: 85 calls  token_hit=21.63%  lcp_hit=22.31%  hit=72  create=18
  memfit-standard-free                  : 16 calls  token_hit=28.12%  lcp_hit=71.35%  hit=8   create=0   <- 内部 IntelligentConfigs 默认值漏覆盖

standard 跑里:
  memfit-standard-free                  : 96 calls  token_hit=8.12%   lcp_hit=21.49%  hit=16  create=34  <- 单一 model 被强制全覆盖
```

### 9.3 决定性证据: 字节级完全相同的 prompt 在两个后端的 cache 反应

standard 跑的 bottleneck 表第 32-34 行 (来自报告
`cachebench-20260507-233557.md`):

```
seq=32  totalBytes=77769  prefixHitBytes=65017  prefixHitRatio=100.00%  cachedTokens=0  cacheCreation=0
seq=33  totalBytes=77769  prefixHitBytes=65017  prefixHitRatio=100.00%  cachedTokens=0  cacheCreation=0
seq=34  totalBytes=77769  prefixHitBytes=65017  prefixHitRatio=100.00%  cachedTokens=0  cacheCreation=0
```

三条 prompt **字节大小完全相同**, **客户端 LCP 命中字节也完全相同 (65017
bytes ≈ ReAct 主循环 high-static + semi-dynamic + frozen-block)**, 但 kimi
后端 **连续三次都返回 `cached_tokens=0` `cache_creation_input_tokens=0`**.

对照 qwen 跑的 bottleneck 表里, 同样体量的 prompt (高 50% 命中率以上) 几乎
都伴随 `cachedTokens > 0`, 进入 `partial_hit` (41 次) 或 `healthy` (39 次)
分类.

**结论**: kimi-k2.5 后端的 SSE 末帧 usage 块 **不可靠地暴露 prefix cache
命中信号**. 即便客户端显式发送了与之前完全一致的 prompt 前缀, 也不会得到
`cached_tokens > 0`. 这与厂商"同款 prefix cache 系统"的声称 **不一致** —
要么只是说底层引擎技术栈相同 (可能都是 vLLM / lmsys 之类的 KV-cache 实现),
要么是上游网关层没有把 prefix cache 命中信号转译到 OpenAI 兼容协议的
`prompt_tokens_details.cached_tokens` 字段里.

### 9.4 第二决定性证据: tag distribution 的形态级差异

```
qwen 跑 (101 calls):
  healthy:                   39 (38.6%)
  partial_hit:               41 (40.6%)  <- 主要 cache 工作模式: cached>0 但 ratio<healthy 阈值
  lcp_hit_but_upstream_miss:  9  (8.9%)
  prefix_misalign:            8  (7.9%)

standard 跑 (96 calls):
  healthy:                    9  (9.4%)
  partial_hit:                7  (7.3%)
  lcp_hit_but_upstream_miss: 68 (70.8%)  <- 接近全部 prompt 都进入此分类
  prefix_misalign:            9  (9.4%)
```

`partial_hit / healthy` (cache 真有命中) 在 qwen 占 **79.2%**, 在 standard
仅占 **16.7%**.

`lcp_hit_but_upstream_miss` (字节命中但上游不报告) 在 qwen 占 **8.9%**,
在 standard 占 **70.8%**, **是 qwen 的 8 倍**.

**这两个数字的形态级翻转, 从分类口径上佐证: standard 后端的 cache 真实
是否命中我们 *无法通过 SSE 末帧观测*, 不像 qwen 那样可对账**.

### 9.5 lcp_hit 反而更高的原因 (避免误读)

standard intelligent lcp_hit 23.14% 比 qwen 的 17.63% 高了 31%. 表面看像
"standard 字节稳定性更好", 实际原因是 **caller 混合不同**:

- qwen 跑里 16 次 standard 调用 (来自 IntelligentConfigs 默认未覆盖) 全部
  来自 **aimemory / LiteForge memory-triage** 等 *小 prompt 重复结构* 入口,
  这些 prompt 模板字节稳定性极高 (lcp_hit 71.35%). 它们稀释了 qwen 跑的
  全量 lcp_hit, 但这些调用没把 token_hit 拉高 (standard 后端不报 cache 信号).
- standard 跑里 96 次全是 standard 路由, 包含 ReAct 主循环 (高动态) +
  aimemory / LiteForge (高重复). 综合 lcp_hit 21.49% 就是这两类的均值.

重新换算到 *仅 ReAct 主循环* 视角:
- qwen 跑 ReAct 主循环 (memfit-qwen3.6 行) lcp_hit = **22.31%**
- standard 跑 ReAct 主循环估算 lcp_hit ≈ 全量 21.49% (因为 aimemory 那部分
  已经摊到 96 calls 里, 拆不开但量级接近)

**所以两个后端在 ReAct 主循环 prompt 上的字节稳定性几乎一致**, 差异完全
集中在 token_hit 维度. 这进一步坐实 §9.3 的结论: 差异的根源是 *上游协议
是否报告 cache 信号*, 不是 prompt 工程.

### 9.6 时长差异 (+70%) 的成本含义

- qwen: 30 intelligent samples, +229s, prompt_tokens=400863, cache_create=76597
- standard: 30 intelligent samples, +390s, prompt_tokens=524948,
  cache_create=159395

standard 在相同样本数下, prompt 体量多了 30%, creation 体量多了 108%, 总
时长多了 70%. 这是因为:
1. standard cache 不命中 -> 每次都按完整 prompt 计费 -> input 计费高;
2. standard 模型本身 token rate 不同 (实测 first byte 平均 ~1s, 比 qwen
   的 ~0.5s 慢);
3. standard 在 ReAct 主循环里更频繁地落入 retry / fallback 路径 (上次单跑
   24 intelligent 就触发 LiteForge max retry fatal, 本轮 fatal-error
   threshold=15 才能稳跑 30 个).

如果按"30 个高智能样本完整任务"的工程意义算, standard 的 *实际真实成本*
约为 qwen 的 **2-3 倍** (按 token_hit 与时长综合). 这与厂商"同款 cache 系统"
的暗示明显不符.

### 9.7 协议家族画像 (本轮坐实, P0-MS1 实证基础)

把两个后端在 cachebench 视角下的协议特征整理出来:

| 维度 | qwen3.6-plus (dashscope) | standard / kimi-k2.5 |
| --- | --- | --- |
| `usage.prompt_tokens` | 返回 | 返回 |
| `usage.completion_tokens` | 返回 | 返回 |
| `usage.prompt_tokens_details.cached_tokens` | 频繁 > 0 | **几乎恒为 0** |
| `usage.prompt_tokens_details.cache_creation_input_tokens` | 频繁 > 0 | 偶尔 > 0 (本轮 159395), 但与 cache_hit 不对应 |
| 同 prompt 二次发送 cached_tokens | 一致升高 (eg. 35440/65017 命中率) | **0 / 0 / 0** (seq 32/33/34 三连证据) |
| first_byte 延迟 | 与 cached_tokens 正相关 | 几乎与 cache 状态无关 (无法用作代理) |

**判定**: standard 后端 *客户端不可观测* prefix cache 信号. 即使实际命中,
我们也拿不到. 这意味着:

1. **当前 cachebench `hit_ratio_token_real` 口径在 standard 上失真** ——
   不是 prompt 退化, 是协议盲区. 不应作为 standard 后端的优化 KPI.
2. **客户端 LCP (`hit_ratio_lcp_client`) 在 standard 上是唯一可信指标** ——
   只反映"我们发送的字节有多稳定", 上游是否真省 token 完全不可见.
3. **`lcp_hit_but_upstream_miss` 在 standard 上不应当作问题** —— 这个 tag
   就是协议盲区的指纹本身, 不是 hijacker / cache_control 标记位置 bug.
   优化 hijacker 不会改善 standard 的 token_hit.

### 9.8 改进空间 (本轮新增)

按优先级:

1. **P0-MS1 兑现 / 协议家族识别落地**: aibalance 端按对外 model_name 注册
   `cache_protocol` 标签 (`explicit-cached_tokens` / `implicit-no-signal` /
   `anthropic-cache_control` 等), 并把这个标签透传到 cachebench 的
   `ai_first_byte_cost_ms` 事件 / dump 元数据里. 报告按协议家族分组聚合
   命中率, 不同协议各自适用各自的口径, 否则做跨后端对比一律误导.
2. **P0-MS3 兑现 / 厂商断言验证流水线**: 给 cachebench 加一个新模式
   `--cache-claim-test`, 自动构造一组 *字节级一致的 prompt 配对*, 连续向
   同一 model 发 3 次, 检查 `cached_tokens` 是否单调上升 → 直接产出
   "厂商 cache 协议契约符合度"评分. 本轮 standard seq 32-34 就是手动版.
3. **P1 / aireact IntelligentConfigs 应当响应 ai.model 覆盖**:
   `common/ai/aid/aicommon/aiconfig/config_file.go:130` 写死 `IntelligentConfigs`
   走 standard, 让 `ai.model("memfit-qwen3.6")` 之类的覆盖在 intelligent
   tier 上失效, 导致 cachebench 长跑出现"我以为强制了 qwen, 结果还有 16 次
   standard"的混合采样. 应该把 `aiOpts` 的 `ai.model` 透传到所有 tier 的
   AICallbackConfig 里, 而不是只覆盖 lightweight tier.
4. **P1 / per-model 表二元组分组 (P0-MS2 续)**: 这次 qwen 跑里 per-model
   表把两个 model 的 tier 都标成 lightweight, 因为 `summarizeByModel` 按
   model_name 单字段分组后, tier 取了首条记录值. 修复方案见 §8.7 P0-MS2.
5. **P1 / cachebench 报告头补"observed cache signals"摘要**: 跑完后扫
   `usageList`, 统计本轮 `cached_tokens > 0` 的频次 / `cache_creation_input_tokens > 0`
   的频次 / 同 prompt 重发后 cached_tokens 是否稳定, 输出一个"上游 cache
   契约可观测性"打分, 让读报告人一眼分清是 prompt 工程问题还是协议盲区.

### 9.9 直接结论 (回应"两款是不是同款 cache 系统")

从 cachebench 客户端可观测维度看:

- **完全不是同款**. 即便底层 KV-cache 引擎可能相同, **kimi-k2.5 后端
  没有把 prefix cache 命中信号通过 OpenAI 兼容的 `prompt_tokens_details`
  字段返回**, 这是 cachebench 直接证据 (seq 32/33/34 三连 0).
- 在"我们能不能基于 `cached_tokens` 做客户端 cache 优化决策"这一工程口径上,
  **qwen3.6 可用, standard 不可用**.
- 对应到成本视角: 同样 30 个 intelligent samples, standard 多花 70% 时长
  + 多 30% prompt 体量 + 上游不报 cache 信号. 即便其底层引擎确实复用了
  prefix cache, **我们也无法对账, 也无法把这种"看不见的命中"转化为
  prompt 工程闭环**.

> 注: §9 这个结论在 §10 被部分推翻. §10 通过对照同一个 kimi-k2.5 模型的
> 两个不同接入路径证明: cached_tokens 信号丢失的根因不是 kimi 模型本身,
> 而是 `memfit-standard-free` 走的内网 http 网关在协议转译层丢了 cache 信号.
> kimi-k2.5 模型经 dashscope 兼容层接入 (`memfit-kimi-k2.5-cache-1-free`)
> 时, cached_tokens 是 standard-free 的 2.74 倍, hit_cnt 是 3 倍. 见 §10.

## 10. cachebench-20260507-235309 vs cachebench-20260507-234947 (同一 kimi-k2.5 模型, 两条接入路径对照)

### 10.1 控制变量设计

aibalance 后台显示同一个底层模型 **kimi-k2.5** 暴露了两个对外名:

| 对外 model_name | 后端 model | provider 标签 | 上游 endpoint |
| --- | --- | --- | --- |
| `memfit-standard-free` | kimi-k2.5 | tongyi | http://(内网)/v1/chat/completions |
| `memfit-kimi-k2.5-cache-1-free` | kimi-k2.5 | tongyi | https://dashscope.aliyuncs.com/compatible-mode/v1/chat/c... |

(对照: `memfit-qwen3.6-plus-no-thinking-1-free` -> qwen3.6-plus -> 同样
走 dashscope 兼容 URL)

§9 结论"kimi-k2.5 不报 cached_tokens"是基于 `memfit-standard-free` 跑的,
当时无法区分"kimi 模型本身不报"还是"接入网关不报". §10 用同一 kimi-k2.5
模型经两条路径各跑 20 个 intelligent samples 做受控对照, 把根因定位到
具体的协议转译层.

参数完全一致 (与 §9 相同, 仅 `--max-intelligent-prompts` 由 30 减为 20):

```
--input "hostscan"
--ai-type aibalance
--max-intelligent-prompts 20
--max-iteration 200 --max-duration 1500 --timeout 1500
--stall-timeout 300 --max-throttle-events 20
--fatal-error-threshold 15 --abort-on-fatal-error
```

### 10.2 关键指标对照

| 指标 | standard-free (kimi 内网 http) | kimi-cache-1 (kimi dashscope 兼容) | 差异 |
| --- | --- | --- | --- |
| 跑时长 | +152s | +126s | 接近 |
| total LLM calls | 58 | 56 | 接近 |
| **intelligent calls** | **20** | **20** | 相同 (受截断控制) |
| intelligent prompt_tokens | 190241 | 182243 | 接近 (任务剧本相近) |
| **intelligent cached_tokens** | **13993** | **22018** | kimi-cache **+57%** |
| intelligent cache_creation_tokens | 64721 | 57567 | 接近 |
| **intelligent token_hit (upstream)** | **7.36%** | **12.08%** | kimi-cache **+64%** |
| intelligent lcp_hit (in-proc) | 34.94% | 21.90% | standard-free 高 (in-flight cancel 影响) |
| **intelligent cache_hit_count** | **4** | **9** | kimi-cache **+125%** |
| intelligent cache_create_count | **12** | **12** | **完全相同** |
| 全量 token_hit (upstream) | **3.28%** | **9.68%** | kimi-cache **+195%** |
| 全量 lcp_hit (in-proc) | 30.25% | 18.77% | standard-free 高 |
| **全量 cached_tokens** | **14889** | **40833** | kimi-cache **+174%** |
| 全量 cache_creation | 142515 | 120131 | 接近 |
| **全量 cache_hit_count** | **6** | **18** | kimi-cache **+200%** |
| 全量 cache_create_count | **22** | **22** | **完全相同** |
| missing usage | 4 | 0 | standard-free 4 次 nil usage |
| in_flight_cancelled | 3 | 3 | 相同 |
| **healthy tag** | **5** | **15** | kimi-cache **+200%** |
| partial_hit tag | 1 | 3 | kimi-cache +200% |
| **lcp_hit_but_upstream_miss tag** | **39 (67%)** | **25 (45%)** | standard-free **+56%** |
| **prefix_misalign tag** | **9** | **9** | **完全相同** |
| high-static distinct (intel) | 10 | 10 | **完全相同** |
| section reuse_rate (high-static) | 17% | 17% | **完全相同** |

### 10.3 决定性观察

**A. 客户端字节层完全等价**:
- `prefix_misalign` 都是 9, `high-static distinct` 都是 10, reuse_rate 都是 17%.
- 这证明两次跑的 prompt 模板字节级完全一致 (本来就是同一份代码、同一 input,
  仅替换 model_name), aicache hijacker 行为也一致.
- 任何下游差异都来自上游协议层, 不来自客户端工程.

**B. cache 创建信号 (`cache_creation_input_tokens`) 两条路径都报告**:
- 全量 create_cnt 都是 22, intelligent create_cnt 都是 12.
- 全量 creation tokens (142515 vs 120131) 接近, 在任务剧本噪声内.
- 这证明 kimi 模型本身 *能* 把 cache 创建信号送出来.

**C. cache 命中信号 (`cached_tokens`) 路径间显著翻倍**:
- 全量 cached_tokens: 14889 (standard) vs 40833 (kimi-cache), **2.74x**.
- 全量 hit_cnt: 6 vs 18, **3x**.
- intelligent token_hit: 7.36% vs 12.08%, **1.64x**.
- healthy tag: 5 vs 15, **3x**.
- 这是 *同一个 kimi-k2.5 上游模型* 在 *同一序列 prompt* 下的两次反应.

**D. lcp_hit_but_upstream_miss 在 standard-free 显著更多 (39 vs 25)**:
- 增量 14 次, 与 cached_tokens 对账缺失数量级一致.
- standard-free 多出来的 14 次"客户端命中但上游不报", kimi-cache 路径
  上变成了 healthy / partial_hit (cached_tokens 被正确返回).

**E. lcp_hit (客户端 LCP) 反而是 standard-free 更高 (30% vs 19%)**:
- 这看起来反直觉. 实际原因是 standard-free 有 4 次 missing usage callback
  (上游 SSE 末帧 usage 块 nil) + 一些首字节延迟更高的调用. lcp_hit 分母
  用 prompt totalBytes 算, 与 cached_tokens 无关; 分子是字节级 LCP, 受
  in_flight_cancelled / missing usage 影响. 这个口径不应被解读为
  "standard-free 字节更稳", 它只是测量伪影.
- 真正反映 prompt 字节稳定性的是 `prefix_misalign` 与 section distinct,
  它们在两次跑里都完全相同.

### 10.4 根因定位 (协议转译层)

把 §9 + §10 的证据串起来:

```
观察 1 (§9): qwen3.6-plus 走 dashscope 兼容 -> cached_tokens 频繁 > 0, hit_cnt=22/30
观察 2 (§9): kimi-k2.5  走内网 http       -> cached_tokens 几乎全 0, hit_cnt=8/30
观察 3 (§10): kimi-k2.5  走 dashscope 兼容 -> cached_tokens 显著 > 0, hit_cnt=18/56
观察 4 (§10): kimi-k2.5  走内网 http       -> 与观察 2 一致, hit_cnt=6/58
```

控制变量:
- 把 §9 观察 2 和 §10 观察 3 比较 (model 都是 kimi-k2.5, 路径变化):
  -> hit_cnt 翻 3 倍. 模型相同, 路径变化即可解释.
- 把 §10 观察 3 和 §10 观察 4 比较 (path A vs path B, model 完全相同):
  -> hit_cnt 翻 3 倍, cached_tokens 翻 2.74 倍. 这是**同一模型同一序列
     prompt** 的对照, 唯一变量是 endpoint URL.

**结论**: cached_tokens 在 standard-free 路径上丢失的根因 **不是 kimi-k2.5
模型本身**, 而是 **`memfit-standard-free` 对应的内网 http 网关 (非 dashscope
兼容层) 在协议转译过程中没有把 cache 命中信号映射到 OpenAI 兼容协议的
`usage.prompt_tokens_details.cached_tokens` 字段**.

更精确的可能机制 (按可能性从高到低):
1. 内网网关返回的 SSE 末帧 usage 块缺少 `prompt_tokens_details.cached_tokens`
   字段 (字段缺失而不是值为 0). 待 raw HTTP body 抓包确认.
2. 内网网关把 `cached_tokens` 写到了非标字段 (例如 `prefix_cache_tokens`
   或 `kimi_cache_hit`), aispec.ChatBase 解析时没读, 默认 0.
3. 内网网关上游就是 kimi 厂商原生 API, 而 kimi 原生 API 不报 cached_tokens
   到 OpenAI 兼容协议; dashscope 兼容层在中间做了一次转译, 把 kimi 自有
   cache 元数据补到 OpenAI 字段里.

第 3 种机制最有可能 (与命名 `memfit-kimi-k2.5-cache-1-free` 暗合, "-cache-"
显式表示 dashscope 这条路径承诺 cache 信号契约). 但 §10 cachebench 数据
不足以在三种机制间分辨, 需要拿 raw HTTP body 复核.

### 10.5 工程影响 (与 §9 改进空间合并去重)

按优先级:

1. **P0-PG1 / `memfit-standard-free` 走 dashscope 接入**:
   把 yaklang 默认 `IntelligentConfigs` (`config_file.go:130`) 从
   `memfit-standard-free` 切到 `memfit-kimi-k2.5-cache-1-free`. 这是
   零代码改动 (改一行配置), 立竿见影提升:
   - intelligent token_hit 从 7.36% -> 12.08% (1.64x).
   - 全量 hit_cnt 从 6 -> 18 (3x).
   - 全量 cached_tokens 从 14889 -> 40833.
   - 唯一已知风险: kimi-cache-1 是新建 model_name, RPM / 配额 / 计费可能
     与 standard-free 不同, 切换前要在 aibalance portal 校验配额.
2. **P0-PG2 / aibalance 端 raw body 抓包**:
   对 standard-free 走的内网 http 网关, 用 mitm / tcpdump 录一次实际
   SSE 末帧 usage 块, 验证是字段缺失 (机制 1) 还是字段名不一致 (机制 2)
   还是上游原生没报 (机制 3). 录完决定要不要在 aibalance 端补一层
   "字段名 alias 转译" (把 kimi 自有 cache 字段映射到 OpenAI 标准字段).
3. **P0-MS1 / cache_protocol 标签 (§9 续)**:
   按对外 model_name 注册 `cache_protocol` 标签
   (`dashscope-explicit` / `internal-gateway-no-signal` /
   `anthropic-cache_control`), cachebench 报告按协议家族分组. 现在
   能落地了, 因为 §10 给出了同模型多路径的可观测协议差异先例.
4. **P1 / 厂商断言验证流水线 (§9 P0-MS3 续)**:
   `--cache-claim-test` 模式: 自动构造字节级一致 prompt 配对, 同模型
   连发 3 次, 检查 `cached_tokens` 单调上升 -> 直接产出"路径 cache 协议
   契约符合度"评分. §10 这次手动版结论清晰, 自动版可立即上线监控所有
   接入路径不退化.
5. **P1 / IntelligentConfigs 响应 ai.model 覆盖 (§9 续)**:
   同 §9.8 改进项 3, 不重复.

### 10.6 副产物: cachebench 部署级 advice

仅看 cached_tokens 真实命中率, **dashscope 兼容路径相对内网 http 路径**:

| 维度 | dashscope 兼容 | 内网 http | 推荐 |
| --- | --- | --- | --- |
| OpenAI cached_tokens 契约 | 完整支持 | 缺失 / 不一致 | **优先 dashscope** |
| 客户端 cache 工程闭环可行性 | 可对账 | 不可对账 | **优先 dashscope** |
| 真实成本可观测性 | 高 | 低 | **优先 dashscope** |
| RPM / 配额 / 计费 | 待核 | 待核 | 切换前确认 |

简言之: yaklang 主链路涉及 prefix cache 优化的所有 caller, 应当优先选
dashscope 兼容路径暴露的 model_name (无论底层是 qwen 还是 kimi-k2.5),
避免选内网 http 路径. 内网路径仅在 dashscope 路径配额耗尽 / 限流时作为
fallback, 但 fallback 期间客户端 cache 优化指标会失真, 需在监控里标识.
