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
**non-ReAct-loop** 路径 (从 dump 上下文判断: `aimemory_triage_saving` /
verification 类 / direct answer 类等). 这些路径自有一套或几套 prompt 模板,
没有走 `prompt_loop_materials.go` 装配, 因此本轮对 `high_static_section.txt`
的增补对它们无效, 它们仍输出短 high-static, 上游也就直接放弃显式 prefix
cache, 在统计上把 `high-static distinct` 顶到 10.

**对策 (待办)**:
- grep 仓库内所有 `<|AI_CACHE_SYSTEM_high-static|>` 出现位置, 列出非主循环
  入口清单.
- 对每一处入口检查其 high-static 渲染来源, 若其也具备"跨 caller 稳定"语义,
  迁移到与主循环共用的 `high_static_section.txt`; 若是 caller-specific,
  则降档到 semi-dynamic 或完全不在 high-static 段输出.
- 完成后再跑一次 hostscan cachebench, 期望 distinct 全量降到 ≤ 3,
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

## 5. before / after 数据 (cachebench hostscan 50-prompt)

cachebench 运行参数: `--input "hostscan" --max-prompts 50 --max-duration 1500
--stall-timeout 180 --max-throttle-events 5`, intelligent 模型
`memfit-qwen3.6-plus-no-thinking-1-free`, lightweight 模型默认 `memfit-light-free`.
两轮均落在同一 hostscan forge 入口, 任务剧本一致, 排除 input 漂移影响.

| 指标 | before (cachebench-20260507-210650.md) | after (cachebench-20260507-221527.md) | 趋势 / 验收 |
| --- | --- | --- | --- |
| total LLM calls | 52 | 50 | 持平 |
| intelligent calls | 18 | 19 | 接近门闸 ≥20, 因主循环外仍有 light 路径分流 |
| intelligent token_hit_ratio (per-model) | 32.76% | 39.78% | +7.02pp / +21% relative; 仍未达 ≥50% 验收线 |
| intelligent lcp_hit_ratio (per-model) | 34.68% | 38.04% | +3.36pp |
| intelligent prefix_misalign 计数 | 8 / 18 (44%) | 1 / 19 (5%) | **-89% relative**, high-static 污染问题已解决 |
| intelligent cached_tokens | 61993 | 106897 | +44904 tokens 真实节省 |
| intelligent cache_creation_tokens | 43261 | 49614 | +6353 (任务路径变化, 与 prompt 总量同比) |
| 全量 token_hit_ratio (含 light) | 19.00% | 26.89% | +7.89pp |
| 全量 lcp_hit_ratio | 24.73% | 27.88% | +3.15pp |
| 全量 prefix_misalign 计数 | 9 | 1 | **-89%** |
| 全量 upstream_creation_cost (1.25x) | 97521 tokens | 91732 tokens | -5789 tokens (-5.9%) |
| high-static distinct (全量, 50/52 调用) | 10 | 10 | 持平 (仍由非主循环路径贡献) |
| high-static reuse_rate_min | 17% | 17% | 持平 (含非主循环干扰) |
| high-static section tokens (主循环) | ~199 tokens (旧模板) | 1900-2500 tokens (`ytoken.CalcTokenCount`) | ≥ 1500 验收已通过 |
| advice `[high_static_too_short]` 计数 | 0 (旧 advice 未实现) | 29 (28 次 199-token + 1 次 732-token) | **新发现**: 非主循环路径仍用旧短模板 |

**结论**: 主 ReAct loop 的 high-static 段污染问题已经根治 (intelligent 路径
prefix_misalign 从 8 次降到 1 次, intelligent token_hit 从 32.76% 提升到 39.78%).
但 **`[high_static_too_short]` 在新版本运行时持续报警 29 次**, 揭露了之前不可见
的入口分散问题: aicache 之外还有 ≥4 套 prompt 模板 (memory_triage / verification
/ direct_answer / perception 等) 仍输出 199 token 的精简 high-static, 它们才是
当前剩余 distinct=10 / token_hit 未到 50% 的主要瓶颈, 已列入第 7 节 Follow-ups.

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

## 7. 后续待办 (Follow-ups)

- **【高优先级】修正 `intelligent highStaticDistinct` 度量口径**:
  当前算法取自全局累计 `sectionHashCount`, 会被前序 lightweight 路径污染
  (见 3.3). 需要在 `aicache` dump 中加 per-call 的 high-static hash 字段,
  或在 `summarizeIntelligentOnly` 内部对 intelligent 调用自身去重统计.
  在改完之前, 验收用 `intelligent prefix_misalign / intelligent calls`
  比例 (≤ 5%) 作为临时门闸.
- **【高优先级】消灭非主循环路径的 199-token 短高静态模板**:
  `[high_static_too_short]` advice 在 hostscan 50-prompt 跑里告警 29 次,
  说明系统中至少存在 ≥4 套与主循环并行的 high-static 入口 (memory_triage /
  verification / direct_answer / perception 等). 任务:
  1. grep 全仓 `<|AI_CACHE_SYSTEM_high-static|>` 入口清单 + 各自模板路径.
  2. 跨 caller 稳定的内容统一接到 `high_static_section.txt`; caller-specific
     的内容降档到 semi-dynamic.
  3. 完成后再跑一次 hostscan cachebench, 期望全量 high-static distinct
     ≤ 3, intelligent token_hit ≥ 50%.
- 把 `intelligent-only` 验收基线 (50% token_hit + ≤3 high-static distinct)
  纳入 CI 长跑, 任何 prompt / 缓存改动都需通过.
- 给 semi-dynamic 段也加一个类似 `semi_too_short` 的 token 数下限 advice
  (阈值待 dashscope 实测确定).
- frozen-block / timeline-frozen 段的 token 数 / hash 稳定性, 也按本文档第
  1 / 2 节口径补充验收标准.
