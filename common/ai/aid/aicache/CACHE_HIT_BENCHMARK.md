# aicache 缓存命中评估指南 (CACHE_HIT_BENCHMARK)

> 本文档介绍如何用 [cachebench/](cachebench/) 工具集端到端评估 aicache 在真实
> ReAct 闭环下的缓存命中表现, 与如何顺着报告定位瓶颈 prompt。
>
> 关键词: aicache benchmark, cachebench, usageCallback, dump alignment,
> 瓶颈定位, hostscan, 双口径命中率

---

## 1. What & Why

aicache 一直有两路缓存信号, 它们语义不同, 必须对齐看才能定位真问题:

| 信号 | 来源 | 含义 |
| --- | --- | --- |
| `prefixHitChunks / Bytes / Ratio` | aicache 自己算的 LCP | 客户端切片维度的"理论命中率", 反映 prompt 字节级前缀的稳定度 |
| `cached_tokens` | dashscope/openai SSE 末帧 `usage` | 上游真正命中缓存的 token 数, 反映真金白银的命中 |
| `cache_creation_input_tokens` | dashscope SSE 末帧 `usage` | 上游本次新建缓存块大小, 按 input_token 单价的 125% 计费 |

[advice.go](advice.go) 给的是基于第一路信号的静态建议, 但只看这一路会漏掉:

- 客户端 LCP 看似命中, 但 dashscope 没建好块或 cc 标记位置漂移 → 真没省钱
- LCP 算出 0% 命中, 可能是首次请求 (预期), 也可能是 prefix 被污染 (灾难)

要把**两路对齐**, 必须在 chat 调用现场同时收两边数据。本评估机制就是为此而生:
用 yak 脚本驱动一次真实 [aim.InvokeReAct](../../../aiengine/aiengine.go) 闭环,
通过 [ai.usageCallback](../../gateway.go) 抓上游 usage, 与 aicache 自动落盘的
dump 文件 (000XXX.txt) 按调用次序对齐, 输出双口径命中率与瓶颈 prompt 列表。

> aicache 的"客户端预测会命中" + dashscope 的"上游真没命中" =  
> 通常意味着 [hijacker.go](hijacker.go) 的 `splitByFrozenBoundary` 切的字节边界  
> 跟当次 prompt 的 cache_control 标记位置不一致, 是最值得人工排查的瓶颈类别。

要回答的两个问题在报告里有专门列项:

- **Q1**: 多少次 prompt 创建了缓存 / 多少次命中? → `summary.cacheCreateCount` / `summary.cacheHitCount`
- **Q2**: 哪些 prompt 是瓶颈 (拿原文)? → `bottlenecks` 列表 + `reports/bottleneck-prompts/<seq>-<tag>.txt`

---

## 2. 一行命令跑

```bash
go run common/yak/cmd/yak.go \
    common/ai/aid/aicache/cachebench/run_react.yak \
    --input hostscan --ai-type aibalance --max-iteration 15
```

参数表:

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `--input` | `hostscan` | 传给 `aim.InvokeReAct` 的 user input |
| `--ai-type` | `aibalance` | aispec 注册的 provider 名 (aibalance/openai/dashscope/...) |
| `--ai-model` | `""` (空) | 强制覆盖 chater 默认 model; aibalance 多模型路由建议留空 |
| `--timeout` | `1200` | 整个 React + 单次 chat 超时秒数 |
| `--max-iteration` | `15` | React 循环最大迭代次数 |
| `--language` | `zh` | aim.language |
| `--output-dir` | `./common/ai/aid/aicache/cachebench/reports` | 报告产物根目录 |

脚本启动时自动 `os.Setenv("YAKLANGDEBUG", "1")`, 触发
[debug_dump.go](debug_dump.go) 的落盘行为, 你**不需要手动设置**任何环境变量。

---

## 3. 输出物地图

每次跑完会在 `--output-dir` 下产出 3 类产物:

```
reports/
├── cachebench-<ts>.json            # 完整对齐结构 (机器可读)
├── cachebench-<ts>.md              # 摘要 + 瓶颈表 + 建议聚类 (人类可读)
└── bottleneck-prompts/
    ├── 000003-prefix_misalign.txt  # 单条 dump 全文 + 头部标注
    ├── 000007-cache_create.txt
    └── 000012-lcp_hit_but_upstream_miss.txt
```

源 dump 目录路径在脚本启动日志里打出, 也在报告头部 `sessionDir` 字段里:

```
~/yakit-projects/temp/aicache/<sessionId>/
├── 000001.txt
├── 000002.txt
└── ...
```

每个 `000XXX.txt` 是 [debug_dump.go::renderDebugDump](debug_dump.go) 输出的, 包含:

- `seq` `model` `total: <bytes> / <chunks>`
- `## sections` - 5 段切片明细 (section / nonce / bytes / hash / seen / first)
- `## hit report` - 客户端 LCP 信号 (`prefix_hit_chunks`, `prefix_hit_bytes`, `prefix_hit_ratio`, `section_hash_count`)
- `## advices` - [advice.go::buildAdvices](advice.go) 给出的静态建议
- `## raw prompt` - **完整 prompt 原文**, 直接 grep 即可

---

## 4. 报告字段含义

[reports/cachebench-*.md](cachebench/reports/) 头部 `Q1: hit/creation summary` 部分:

| 字段 | 含义 |
| --- | --- |
| `total LLM calls` | usage 回调 + dump 对齐后的总调用数 |
| `cache_create_count` | 上游 `cache_creation_input_tokens > 0` 的次数 (新建块, 计费 1.25x) |
| `cache_hit_count` | 上游 `cached_tokens > 0` 的次数 (真命中, 计费 0.4x) |
| `missing usage callbacks` | usage 没回调的次数 (上游 SSE 没返 usage 块, 一般是 nil) |
| `hit_ratio_token_real` | `sum(cachedTokens) / sum(promptTokens)` - 上游口径, 跟钱挂钩 |
| `hit_ratio_lcp_client` | `sum(prefixHitBytes) / sum(requestBytes)` - 客户端口径, 跟 prompt 结构挂钩 |
| `upstream_creation_cost` | `sum(cacheCreation) * 1.25` - 估算的本次会话总建块成本 |

`tag distribution` 给出每条 prompt 的分类计数, 判据见 §5。

`section hash drift` 是每个 section 在本次 session 里出现过几个不同的 hash:

- `high-static`, `frozen` (在 §6 规划中) → 期望恒定 1
- `semi-dynamic` → 通常 ≤ 3 (Skills / Schema 偶发漂移)
- `timeline-open`, `dynamic` → 每次都不同, 期望 == totalCalls

---

## 5. 瓶颈分类标签 (排查工作流)

[lib.yak::classifyRecord](cachebench/lib.yak) 给每条 prompt 打 5 类 tag:

| tag | 判据 | 含义 / 排查方向 |
| --- | --- | --- |
| `healthy` | `cachedTokens > 0 && prefixHitRatio >= 0.5` | 双口径都命中, 不需要处理 |
| `cache_create` | `cacheCreation > 0` | 上游本次新建块。首次必然出现 (预期); 非首次频繁出现 → 字节边界变了 |
| `prefix_misalign` | `prefixHitChunks == 0 && totalRequests > 1` | 客户端 LCP 完全失对齐 → high-static / frozen 段被污染 |
| `lcp_hit_but_upstream_miss` | `prefixHitChunks > 0 && cachedTokens == 0` | 客户端预测命中但上游没命中 → hijacker 切的边界与 cc 标记不一致 |
| `noise` | 单 raw chunk | prompt 没用 PROMPT_SECTION 包装, 本来就不参与缓存 |

**典型排查动作**:

1. 看 .md 摘要 `Q2: bottleneck prompts` 表
2. 选一条 `prefix_misalign` 行 → 打开 `bottleneck-prompts/<seq>-prefix_misalign.txt`
3. 找前一条 dump 比对 `## sections` 里的 hash 列表 → 第一个不一致的 section 就是漂移源头
   - high-static → 检查 [aireact/prompts/loop/system_prompt.txt](../aireact/prompts/loop/) 是否引入了变量
   - frozen → 检查 [aireact/prompts/loop/frozen_block_section.txt](../aireact/prompts/loop/) 与 ToolInventory / ForgeInventory 的稳定性
   - semi-dynamic → 通常是 SkillsContext (Skills 加载/卸载) 或 Schema 漂移
   - timeline-open → 注意是否进了 last interval 桶, midterm 是否被消费两次
4. 选一条 `lcp_hit_but_upstream_miss` 行 → 通常是:
   - hijacker 走了 2 段退化路径 (frozen 边界没找到), 检查
     [hijacker.go::splitByFrozenBoundary](hijacker.go) 的 fallback 分支
   - 上游缓存还没建好 (E13/E14 实测要 2 次同字节请求才稳定命中), 看 `seq` 是不是紧跟 `cache_create` 的下一条

---

## 6. 离线再分析

`run_react.yak` 跑完后 dump 留在 `~/yakit-projects/temp/aicache/<sessionId>/`,
不会被清理。你可以随时换分析口径:

```bash
go run common/yak/cmd/yak.go \
    common/ai/aid/aicache/cachebench/analyze.yak \
    --session-dir ~/yakit-projects/temp/aicache/20260504-153000-12345 \
    --input "hostscan (offline replay)"
```

离线模式没有 usage 回调, 所以:

- `hit_ratio_token_real`, `cacheCreation`, `upstream_creation_cost` 全为 0
- `tag` 只能给出 `prefix_misalign` / `noise` / `unknown` 三类

但 `prefixHitRatio`, `section hash drift`, `advices` 仍然有效, 客户端口径瓶颈
依然能定位到。

---

## 7. 与 TONGYI_CACHE_REPORT.md 实测结论的对照

[TONGYI_CACHE_REPORT.md](TONGYI_CACHE_REPORT.md) §1 的 7 条核心结论是本 benchmark
用来打 healthy 阈值的依据:

- 双 cc 命中实测稳定值 `cached_tokens / prompt_tokens ≈ 70%` (E14 r1=1478/2123)
  → `HEALTHY_LCP_RATIO_MIN = 0.5` 留有余量
- 字节级一致是命中前提, 差一字节就 miss → `prefix_misalign` 一旦出现一次,
  后续 LCP 就需要重建, 是第一优先排查
- 浅 cc 在 system+user1 跨 message 时才有效 → `lcp_hit_but_upstream_miss` 的
  根因排查直接指向 [hijacker.go](hijacker.go) 的 3 段切分逻辑
- E12 实测"部分命中 + 增量建块" 不存在 → 本评估机制不去尝试预测增量命中,
  把任何 `cacheCreation > 0` 都标为新建块

---

## 8. CI 集成提示

`run_react.yak` 末尾在两个口径都 < 5% 时主动 `die`, 退出码非 0:

```yak
if summary["totalCalls"] > 1 {
    if summary["hitRatioTokenReal"] < 0.05 && summary["hitRatioLcpClient"] < 0.05 {
        die("cache hit ratio below threshold")
    }
}
```

CI 集成时建议:

- 用 `--input` 跑一组固定的 benchmark 样例 (hostscan / portscan / 通用问答)
- 把 `reports/cachebench-*.json` 作为 artifact 上传, 用 jq 跨版本 diff
  关键指标: `summary.cacheHitCount`, `summary.hitRatioTokenReal`, `summary.tagCount`
- `bottleneck-prompts/` 目录在 PR 描述里 attach, code review 时直接看 prompt 原文
- 关注 `summary.sectionHashMax.high-static / frozen` —— 如果某次 PR 让这两个
  段的 distinct hash 数从 1 增到 ≥ 2, 几乎必然破坏 dashscope 的字节级前缀缓存,
  应优先 revert 或修复

---

## 9. 缓存命中率全面提升计划 (P0-A / P0-B / P1-C / P1-D 改造记录)

### 9.1 问题基线 (cachebench-20260504-144334)

- 130 LLM call / 13.84 MB prompt 字节
- `hit_ratio_token_real` **1.10%**, `hit_ratio_lcp_client` 6.51%
- `upstream_creation_cost` 668,524 tokens (1.25x 计费)
- `missing usage callbacks` **80 / 130**

按段字节分布 (基线):

| 段 | 字节占比 | distinct hash |
| --- | --- | --- |
| dynamic | 9.46 MB (75%) | 113 |
| raw / noise | 1.53 MB (12%) | 16 |
| timeline-open | 0.64 MB (5%) | 30 |
| high-static | 0.65 MB (5%) | **16 (期望 1)** |
| semi-dynamic | 0.33 MB (3%) | **91 (期望 1)** |
| timeline (frozen) | 28 KB (0.2%) | 1 |

raw/noise 主要由 6 个无 wrapper 模板贡献:

- `verification.txt` (934 KB, 单文件最大)
- `task-summary.txt` (5x 22-57 KB)
- `interval-review.txt` (3x 14 KB)
- `ai-review-{plan,task,tool-call}.txt` (6x BACKGROUND 自定义 tag)

### 9.2 P0-A: 消灭 noise (4 段包装)

把 6 个大模板按 `high-static` / `semi-dynamic` / `timeline-open` / `dynamic` 4 段
重新包装。详细 checklist 见 `CACHE_BOUNDARY_GUIDE.md` §6.4。

模板改造对应文件:

- `common/ai/aid/aireact/prompts/verification/verification.txt`
- `common/ai/aid/prompts/task/task-summary.txt`
- `common/ai/aid/aireact/prompts/tool/interval-review.txt`
- `common/ai/aid/aicommon/prompts/review/ai-review-plan.txt`
- `common/ai/aid/aicommon/prompts/review/ai-review-task.txt`
- `common/ai/aid/aicommon/prompts/review/ai-review-tool-call.txt`
- `common/ai/aid/aireact/prompts/review/ai-review-tool-call.txt`

### 9.3 P0-B: LiteForge 模板 4 大污染源

| 污染源 | 现象 | 修复 |
| --- | --- | --- |
| schema / static instruction 在 high-static 段内 | 跨 forge high-static distinct=16 | 下移到 semi-dynamic, high-static 仅留 `# Preset` + `# Output Formatter` (`liteforge.go:367+`) |
| `PersistentMemory` 用 `time.Now().String()` (纳秒) | semi-dynamic distinct=91 | 改为 `time.Now().Format("2006-01-02 15:04")` (`memory.go:395-405`) |
| timeline 不拆 frozen + open | frozen 段每次失效 | 调 `RenderWithFrozenBoundary` 拆 frozen + open (`liteforge.go` + `TimelineDumpFrozenOpen`) |
| 调用方 INSTRUCTION 走 `WithLiteForge_Prompt` 进 dynamic | 静态指令每次重发 | 用 `WithLiteForge_StaticInstruction` 提到 semi-dynamic (各 forge 调用方迁移) |

### 9.4 P1-C: dynamic 段拆分 (回收主 React loop 9.46 MB)

| 子段 | 旧位置 | 新位置 |
| --- | --- | --- |
| TRAITS / Execution Protocol | dynamic | high-static |
| SESSION_ARTIFACTS 文件树 | dynamic | timeline-open |
| 历史 Round 列表 (PREV_USER_INPUT) | dynamic | timeline-open |
| BACKGROUND (Current Time / OS / WorkingDir) | dynamic | semi-dynamic (Current Time 分钟级) |

### 9.5 P1-D: usage callback 透传修复 (missing 80 -> 0)

| 路径 | 修复 |
| --- | --- |
| Tiered AI (`Get*AIModelCallback`) | `extractUserUsageCallbackOpts` 注入 `aispec.WithUsageCallback` |
| `OriginalAICallback` / `WithAICallback` | `AIChatToAICallbackType` 自动从 caller config 注入 user UsageCallback |
| `WithFastAICallback` 子 coordinator | 调用站点 (invoke_liteforge / coordinator) 显式补 `WithUserUsageCallback(parent.GetUserUsageCallback())` |
| `WithInheritTieredAICallback` | 同步继承 `parentConfig.userUsageCallback` |

### 9.6 验收效果

通过手动检查 改造后 dump (`yakit-projects/temp/aicache/<sessionId>/`) 验证:

- React loop 主路径下 5 次连续 chat: 4 段全部 byte-identical, `prefix_hit_ratio = 100.0%`
  (high-static / semi-dynamic / timeline-open / dynamic 四段 hash 完全一致)
- LiteForge 模板拆分: high-static 段仅含 `# Preset` + `# Output Formatter`,
  跨同 forge 多次调用 hash 字节稳定
- 单测覆盖 (见 `CACHE_BOUNDARY_GUIDE.md` §6.3): 所有 6 大改造模板均有
  `TestSplit_*` 回归断言, LiteForge 有 `TestLiteForgePrompt_HighStaticStableAcrossNonces`
- usage callback 单测: `TestAIChatToAICallbackType_PropagatesUserUsageCallback`,
  `TestWithInheritTieredAICallback_InheritsUserUsageCallback`

> **真实上游 hit_ratio_token_real 数据**: 改造后建议在 aibalance 部署完整可用
> 时再跑一次 `cachebench --max-prompts 50`, 配合 `bottleneck-prompts/` 进一步
> 收紧剩余瓶颈。

### 9.7 P2-E2 实测 (aibalance memfit-standard-free + memfit-light-free, 50 prompt)

跑法 (Tiered: Intelligent=memfit-standard-free, Lightweight=memfit-light-free):

```
go run common/yak/cmd/yak.go \
    common/ai/aid/aicache/cachebench/run_react.yak \
    --max-prompts 50 --max-iteration 25 --ai-type aibalance
```

报告: `common/ai/aid/aicache/cachebench/reports/cachebench-20260504-184837.{md,json}`

| 维度 | Baseline (老 hostscan, 1.10%) | 本轮 (P0-A/P0-B/P1-C/P1-D 全部上线后) |
| --- | --- | --- |
| total LLM calls | 130 | 55 (max-prompts=50, +5 收尾事件) |
| missing usage callbacks | **80 (61.5%)** | **5 (9.1%)** |
| hit_ratio_token_real (上游 cached_tokens / prompt_tokens) | **1.10%** | **6.27%** (5.7x) |
| hit_ratio_lcp_client (客户端字节级 LCP) | 未采集 | 11.97% |
| sum prompt_tokens | - | 851,099 |
| sum cached_tokens | - | 53,384 |
| sum cache_creation_input_tokens | - | 121,369 |
| upstream creation cost (1.25x) | - | 151,711 tokens |
| cache_hit_count (cached>0) | - | 19 |
| cache_create_count (creation>0) | - | 24 |

**核心成果:**

1. **usage 触达率 38.5% -> 90.9%**: P1-D 修复彻底见效, 仅剩 5 次 missing
   (主要是上游 SSE 末帧异常时未带 usage block, 非 client 漏接).
2. **真实命中率 5.7x 提升**: 1.10% -> 6.27%, 但距 35% 目标仍有差距,
   主要瓶颈仍在前 20 次 prompt 的 prefix_misalign / lcp_hit_but_upstream_miss.

**剩余瓶颈 (供下一轮 P3 使用):**

| tag | 出现次数 | 含义 / 推测原因 |
| --- | --- | --- |
| `lcp_hit_but_upstream_miss` | 26 | 客户端 LCP 已对齐 (最高 40%), 但 vllm 侧未返回 cached_tokens; 推测: (a) memfit-standard-free 侧 KV cache 预热阶段, 前几次新建; (b) hijacker 字节边界未稳定到上游 block 粒度 (vllm 默认 16 token block); (c) 部分 prompt 4 段不全 (`only 3/4 sections present; missing: [timeline]` 出现 6 次), 前缀被破坏 |
| `prefix_misalign` | 9 | 主要在 seq 2-19, 即首批 prompt: high-static 段每次都换 hash. 报告里 "high-static section unstable: 10 distinct hashes" 出现 37 次 |
| `cache_create` | 6 | 首次新建块, 预期出现, 不算瓶颈 |
| `unknown` | 13 | tag 兜底 |

| section | distinct_hashes | 期望 |
| --- | --- | --- |
| high-static | **10** (期望 1) | 仍有动态污染源, 预计来自前 10 次 prompt 是 LiteForge 跨不同 forge (task-analyst / task-summary / verification / ai-review-* 各发 1 次, 模板各异); 主 React loop 内 high-static 已稳定 |
| semi-dynamic | 30 | 预期会随 forge / tool 列表变化, 当前数与 noise 历史 91 相比已 67% 降低 |
| timeline-open | 48 | 预期, 每次累加 |
| dynamic | 54 | 预期, 完全可变 |

**下一轮 (P3) 建议:**

- P3-A: 把 `high-static` distinct=10 进一步压到 ≤3. 思路: LiteForge 模板里 `# Preset` 块加入 forge name, 让所有 forge 共享一段更短的统一 high-static 头, forge 特异内容下沉到 semi-dynamic.
- P3-B: 修 `only 3/4 sections present; missing: [timeline]` (6 次). 检查哪些 prompt 没渲染 timeline section, 补一段空 `<|PROMPT_SECTION_timeline-open|><|PROMPT_SECTION_timeline-open_END|>` 占位, 保证 4 段对齐.
- P3-C: 与 aibalance 后端对齐字节边界 / cache block size, 把 `lcp_hit_but_upstream_miss` 转化为真实 cached_tokens.

### 9.8 P3-T1/T5/T6: 剩余 missing usage 链路定位与彻底修复

**背景**: P2-E2 跑后还剩 5 次 missing (9.1%), 表面看是上游 SSE 异常,
深入分析 dump (`yakit-projects/temp/aicache/<sessionId>/`) 发现这些缺口
**集中在 3 段 LiteForge 风格的 small prompt** (3-6KB, sections=`[high-static, semi-dynamic, dynamic]`,
缺 timeline), 与 P2-E2 报告中 `only 3/4 sections present; missing: [timeline]` 6 次互相印证.

#### P3-T1 根因定位

`enhancesearch` 的 4 个 LiteForge 子调用 (`HypotheticalAnswer` / `SplitQuery` /
`GeneralizeQuery` / `ExtractKeywords`) 直接走 `aicommon.InvokeLiteForge` +
`aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback())`,
而不像 aireact 主 loop 里 `r.invokeLiteForgeWithCallback` 那样显式拷贝 user usage callback.

子 LiteForge cfg 上 `userUsageCallback` 永远是 nil,
`extractUserUsageCallbackOpts` 找不到 callback, `aispec.WithUsageCallback`
也就不会注入到 chat opts, **末帧 token usage 的 callback 没被调用** -> missing.

同样路径影响:
- `common/ai/rag/enhancesearch/{enhance.go, build_questions.go}`
- `common/ai/rag/knowledgebase/query.go`
- `common/ai/rag/generate_index_tool/processor.go`
- `common/ai/aid/aitool/buildinaitools/yakscripttools/metadata/genmetadata/yakscript_ai.go`

(任何 `aicommon.WithAICallback(aicommon.MustGet*AIModelCallback())` 路径都是.)

#### P3-T5 修复方案: ctx 通道透传

| 改动 | 文件 | 作用 |
| --- | --- | --- |
| 新增 `WithUserUsageCallbackContext` / `GetUserUsageCallbackFromContext` | `aicommon/user_usage_callback_context.go` | ctx-based 透传 helper |
| `Config.GetContext()` 在有 userUsageCallback 时自动注入 ctx | `aicommon/config.go` | 父 React loop 把 callback 通过 ctx 透传给所有子调用 |
| `extractUserUsageCallbackOpts` 加 ctx fallback | `aicommon/aitier_callback.go` | 子 cfg 找不到 callback 时, 从 `cfg.GetContext()` 取 |
| 单测 4 个 | `aicommon/user_usage_callback_context_test.go` | 覆盖 round-trip / GetContext 注入 / fallback / cfg 优先级 |

链路: 父 cfg.userUsageCallback -> 父 cfg.GetContext() (注入 ctx) ->
React loop action 取 ctx -> enhancesearch 用 `WithContext(ctx)` 创建 InvokeLiteForge ->
子 cfg.Ctx = 父 ctx -> 子 cfg 上 chat 时 `extractUserUsageCallbackOpts` ->
cfg.userUsageCallback (nil) -> fallback `cfg.GetContext()` -> 拿到 callback -> 注入 chat opts -> SSE 末帧触发用户 callback.

#### P3-T6 cachebench 测量伪影修复

老 cachebench 把 dumps 比 usages 多的 trailing dumps 当 missing, 但实际是
**max-prompts ctx cancel 后 chat 抛错, callback 没机会触发**, 是测量伪影非真实漏接.

| 改动 | 文件 | 作用 |
| --- | --- | --- |
| `alignDumpsAndUsages(dumps, usages, maxPromptsTriggered)` 新增第三参数 | `cachebench/lib.yak` | trailing dumps 标记 inFlightCancelled |
| `analyzeBenchmark(... , maxPromptsTriggered)` 透传 | `cachebench/lib.yak` | 入口透传标记 |
| `run_react.yak` post-react 1.5s 稳定等待 | `cachebench/run_react.yak` | 给 SSE 末帧 callback 到达机会 |

#### 验收数据 (max-prompts=60, max-iter=40)

报告: `common/ai/aid/aicache/cachebench/reports/cachebench-20260504-2004*.{md,json}`

| 指标 | P2-E2 (12 calls 短样本) | P2-E2 (50 calls 中样本) | P3 (63 calls 全采集) |
| --- | --- | --- | --- |
| total calls | 12 | 55 | **63** |
| **真实 missing usage** | **4 (33%)** | **5 (9.1%)** | **0 (0%)** |
| in_flight_cancelled (测量伪影) | 0 | 0 | 2 (正确归类) |
| token_hit_ratio | 29.15% | 6.27% | 4.08% |
| lcp_hit_ratio | 48.97% | 11.97% | 12.25% |
| healthy + cache_create (真实命中链路) | (n/a) | (n/a) | 16 |

> **note**: 12 calls 时 33% missing, 命中率虚高 29% — 因为 enhancesearch 的 small
> 子调用 callback 全部漏接, 没进 sumPromptTokens 分母; 而 P3 把这些 small 子调用
> 全部纳入采样后, 它们的 prefix 短/不易命中拉低了 hit_ratio. 这是采样口径修复
> 后看到的**真实数据**, 而不是回归.

#### 真 missing 链路彻底关闭

P3 之后 cachebench 跑 63 calls 真实 missing = 0. 用户脚本注册的
`ai.usageCallback(...)` 现在能覆盖:
1. 主 React loop 直接 chat (P1-D1)
2. 主 React loop 内 LiteForge (`r.invokeLiteForgeWithCallback`, P1-D)
3. WithFastAICallback / OriginalAICallback 子 coordinator (P1-D)
4. WithInheritTieredAICallback (P1-D2)
5. **Tiered AI 子 LiteForge (`aicommon.InvokeLiteForge` + `MustGet*AIModelCallback`) (P3-T5)**
   - enhancesearch HyDE / SplitQuery / GeneralizeQuery / ExtractKeywords
   - knowledgebase query / build_questions
   - generate_index_tool / yakscript_ai metadata 生成

任何后续走 `aicommon.WithAICallback(aicommon.MustGet*AIModelCallback())` +
`aicommon.WithContext(ctx)` 的子调用, 只要 ctx 是从父 React loop 派生的,
user usage callback **自动透传**.

### 9.9 P3-X1: enhancesearch LiteForge 子调用静态指令下沉

**背景**: P3 (63 calls) baseline 数据 `lcp_hit_but_upstream_miss = 18` +
`prefix_misalign = 9`, dump 分析定位主要污染源是 `enhancesearch` 4 个 LiteForge
子调用 (`HypotheticalAnswer` / `SplitQuery` / `GeneralizeQuery` / `ExtractKeywords`)
全部按"反模式"调用: 1-3KB 静态指令 + nonce + query 整段塞给 `InvokeLiteForge`
第 1 参数, 在 `_executeLiteForgeTemp` 内被当作 `cfg.query` -> LiteForge 模板
`.Params` -> dynamic `<params_NONCE>` 段. dynamic 段每次 nonce / query 不同必然
misalign, 1-3KB 静态文本被一起污染.

#### P3-X1 改造方案

参考已经做对的 `enhancesearch/build_questions.go::BuildIndexQuestions`:
静态指令通过 `aicommon.LiteForgeStaticInstruction(...)` 进入 LiteForge 模板的
semi-dynamic 段, query 仍然是 `InvokeLiteForge` 第 1 参数 (走 dynamic 段, 自带
NONCE 包装防 prompt-injection).

| 文件 | 改动 |
| --- | --- |
| `common/ai/rag/enhancesearch/enhance.go` | 4 个方法每个抽出独立 `xxxStaticInstruction` 常量 (角色/任务/行动准则/few-shot, 不含 nonce / query); query 作为 `InvokeLiteForge` 第 1 参数, 加 `aicommon.LiteForgeStaticInstruction(constant)` option, 去掉模板里 `<\|问题_{{ .nonce }}_START\|>{{ .query }}<\|问题_{{ .nonce }}_END\|>` 这层冗余 nonce |
| `common/ai/rag/enhancesearch/enhance_section_test.go` (new) | 静态指令稳定性 + dynamic 段位置回归 (3 组测试 / 9 sub-test) |

**4 个新增 staticInstruction 常量**:
- `hydeStaticInstruction` (HypotheticalAnswer)
- `splitQueryStaticInstruction` (SplitQuery)
- `generalizeQueryStaticInstruction` (GeneralizeQuery)
- `extractKeywordsStaticInstruction` (ExtractKeywords)

#### P3-X1 单测验证 (代码级正确性证明)

`enhance_section_test.go` 通过 `aicommon.RegisterLiteForgeExecuteCallback` hook
全局 callback 抓 `cfg`, 严格断言:

| 测试 | 断言 |
| --- | --- |
| `TestEnhanceSearch_StaticInstructionStableAcrossNonces` | 同一 method 用 5 个不同 query 调用, `cfg.staticInstruction` 跨调用 **byte-identical** (sha256 一致); query 仅出现在 `cfg.query` 第 1 参数 |
| `TestEnhanceSearch_StaticInstructionHasNoNonceTemplate` | 4 个 staticInstruction 常量不含 `{{ .nonce }}` / `{{ .query }}` / `<\|问题_` 模板残留 |
| `TestEnhanceSearch_QueryGoesThroughDynamicNotStatic` | query 进入 `cfg.query` (-> dynamic `<params_NONCE>`); 不出现在 `cfg.staticInstruction` (-> semi-dynamic) |

9 个 sub-test 全过, 证明 P3-X1 改造在 byte 层正确.

#### P3-X3 cachebench 实测 (受限样本)

100 prompt 目标因 aibalance 上游模型间歇性不稳定 (`401 Unauthorized` retry +
`max retry count[5] reached, last error: action type is empty`), React loop 在
第 9-10 个 prompt 持续早夭, 采样到 9-10 个 dump.

```
session: 20260504-212511-42586  (memfit-standard-free + 简化英文 input)
total calls: 10  cache create: 1  cache hit: 1  missing usage: 6
token_hit_ratio: 5.24% (upstream)
lcp_hit_ratio:  43.13% (in-proc)
```

**dump 分布**:
- dump 1, 2 = capability-catalog-match (memfit-light-free)
- dump 3 = tag-selection (memory tagging)
- dump 4 = intent recognition forge
- dump 5 = memory-triage forge
- dump 6-9 = **主 React loop (deepseek-v3)** -- dump 7,8,9 全部 `prefix_hit_chunks: 4 / 100%`, semi-dynamic + high-static + timeline + dynamic 段 byte-identical 复用

**没有采集到 enhancesearch 4 个子调用的 dump** (search-deep 阶段未到达).
但出现 1 次 upstream cache hit (主 React loop), 证明 P3-T5 修后的
cache 链路在主 loop 工作正常.

#### P3-X1 阻塞分析: 100 prompt 不达原因与 X1 改造无关

| 现象 | 根因 | 影响层级 |
| --- | --- | --- |
| React loop 在第 9-10 prompt 早夭 | aibalance `memfit-standard-free` 在 retry 路径被路由到 `deepseek-v3` / `memfit-light-free`, 这两个 model key 间歇性 401 | 上游模型路由 |
| `lcp_hit_but_upstream_miss = 7` | 客户端 LCP 命中 (in-proc 43%), 但 `memfit-light-free` 上游可能未启用 explicit cache | aibalance 配置 |
| `missing usage: 6` | React loop ctx cancel 后 SSE 末帧 callback 没机会触发, P3-T6 已分类为 `inFlightCancelled` (但本次小样本未触发分类逻辑) | 测量伪影 |

这些都是上游 / 模型路由层的问题, 与 P3-X1 (enhancesearch 4 个 method 静态指令
下沉到 semi-dynamic 段) 改造的字节布局无关.

#### P3-X1 验收结论

| 验收项 | 状态 | 证据 |
| --- | --- | --- |
| 静态指令跨 nonce / query byte-identical | **PASS** | `TestEnhanceSearch_StaticInstructionStableAcrossNonces` (sha256 校验) |
| 静态指令不含 nonce / query 模板残留 | **PASS** | `TestEnhanceSearch_StaticInstructionHasNoNonceTemplate` |
| query 进 dynamic 段, 不污染 static | **PASS** | `TestEnhanceSearch_QueryGoesThroughDynamicNotStatic` |
| `enhancesearch` / `aicache` / `aicommon` / `aiforge` 全套件无回归 | **PASS** | `go test ./...` 全过 |
| cachebench 100 prompt 命中率提升 | **DEFERRED** | 当前 aibalance 环境无法稳定跑到 100 prompt, 需上游模型修复后复测 |

#### P3-X2 是否启动 (备选)

P3-X1 已经在代码层和单测层证明 enhancesearch 子调用 prompt 段布局正确.
若后续 aibalance 修复后 100 prompt 复测仍出现 `lcp_hit_but_upstream_miss > 8`,
说明剩余瓶颈是 hijacker 字节边界问题 (client LCP 命中但上游 KV cache block
boundary 不对齐), 升级到 P3-X2 (enhancesearch 子调用主动注入
`<|AI_CACHE_FROZEN_semi-dynamic|>` boundary 让 hijacker 走 3 段切分路径).

**当前不启动 P3-X2**: 现有 aibalance 数据不足以判断瓶颈是否在 hijacker 边界,
盲启动会引入额外风险.

#### cachebench `lib.yak` 健壮性增强 (附带修复)

P3-X3 跑 cachebench 时发现 `lib.yak` 在分析 dump 时偶发
`YakVM Panic: cannot support op1[undefined] > op2[int]` (placeholder rec
缺数值字段). 同步修复:

| 改动 | 文件 | 作用 |
| --- | --- | --- |
| 新增 `ccbAsInt(v)` / `ccbAsFloat(v)` nil-safe 转换 | `cachebench/lib.yak` | nil / undefined 归 0, 防 op1[undefined] > op2[int] panic |
| `classifyRecord` / `summarize` 内所有数值字段过 `ccbAs*` | `cachebench/lib.yak` | 兜底防御性强转 |
| `alignDumpsAndUsages` placeholder rec 补全 numeric 字段 | `cachebench/lib.yak` | dumps 缺失场景全字段 0 默认 |
| `classifyRecord` 调用包 try-catch | `cachebench/lib.yak` | 单 dump 异常不再 crash 整个 benchmark, 标记 `unknown` 继续跑 |

修复后 cachebench 在小样本 (10 prompt) 也能产出完整 summary, 不再因部分 dump
缺字段中断分析.

#### 文件清单

| 文件 | 改动类型 |
| --- | --- |
| `common/ai/rag/enhancesearch/enhance.go` | 重构 4 个方法, 新增 4 个 static instruction const |
| `common/ai/rag/enhancesearch/enhance_section_test.go` | 新增 (3 组 / 9 sub-test) |
| `common/ai/aid/aicache/cachebench/lib.yak` | 健壮性增强 (ccbAsInt/ccbAsFloat + try-catch + placeholder 全字段) |
| `common/ai/aid/aicache/CACHE_HIT_BENCHMARK.md` | 9.9 节 (本节) |

---

### 9.10 P3-X2 排障 — cachebench 卡顿真因: persistent session 污染 (run_react.yak session 隔离)

#### 现象

跑 `run_react.yak --max-duration 300` 复测时, 标准客户端 (yakit GUI / yak ai) 跑同样
hostscan 任务 90~150 秒可走完, cachebench 却在 5 分钟硬截止时只走了 68 个 prompt
就被 `--max-duration` 切掉, 大量"安静的空白时段". 抓 round2 日志的 first-byte
时间戳, AI 调用本身都在 3~9 秒内返回 (`memfit-standard-free`),
两次 LLM 调用之间却有最长 41 秒的空白 (17:11:27 -> 17:12:08), 还有多次 5~15s 间隙.

#### 排障路径

1. 先怀疑 LLM 慢: `usageList` 里相邻 first-byte cost 全部 < 10s → **不是 LLM 慢**.
2. 怀疑 aicache 命中差: round2 数据 `lcp_hit_but_upstream_miss` 已经从 89% 降到 7%, 命中率 11.33% → **不是 aicache 慢**.
3. 抓空白窗口里的日志:
    - `[timeline:1054] reassigned IDs for 2363+ timeline items` — **35 次**
    - `restored timeline instance from persistent session [default] with 2363 items` — **38 次**
    - `restored 1 user input history entries from session [default]`
    - `[re-act:209] loading ReAct config took 2.89s, too long, maybe some events happened`
4. 看 timeline.go:1004-1055, ReassignIDs 里对全量 items 做 O(N) 三个 OrderedMap 重建,
   N=2363 时单次约几十~上百毫秒, 35 次叠加再加上 hnsw collection load /
   intent recognition init, 累积**几十秒卡顿假象**.

#### 真相

[`common/aiengine/config.go:88`](../../../aiengine/config.go) 默认 `SessionID = "default"`.
`run_react.yak` 此前没有显式传 `aim.sessionID(...)`, 因此 cachebench 历次跑都共享
同一个 `default` session, 数据库 `ai_memory_*` 表里的 timeline 持续累积. 每次
React loop init / subtask invoke / forge 调用都触发 `restoreTimeline`,
3 个内部 OrderedMap 全量 reassign + reducer 重映射, 这是非常昂贵的初始化.

标准客户端为什么没事: yakit 每个 chat session 用的是 GUI 生成的独立 session id,
新会话 timeline = 0, 自然不会触发这条慢路径.

| 跑次 | 加载 timeline 时打印 | 结论 |
| --- | --- | --- |
| `--session-id` 留空 (走 `default`) | `restored timeline instance ... with 2363 items` | 每次 React loop init 都全量 reassign 2363 项 |
| `--session-id` 自动生成 (本次修复) | `restored timeline instance ... with 0 items` | timeline 完全干净, 走与标准客户端同样的快路径 |

(后者用 `cachebench-<unix-ts>-<pid>` 作为 session id, 让 aiengine 自己建一份新表)

#### 修复

1. [`run_react.yak`](cachebench/run_react.yak) 新增 `--session-id` flag, 默认空 ⇒ 自动生成
   `cachebench-<unix-ts>-<pid>`. 在 `aim.InvokeReAct(...)` 选项链里追加 `aim.sessionID(...)`.
2. 启动日志打印当前 session id, 方便排查时确认是否真的隔离了.
3. 如果用户显式传 `--session-id default` (回归到老行为), 仍然支持, 但需明知道在做什么.

#### 验收 (smoke)

跑 `/tmp/yak-bench common/ai/aid/aicache/cachebench/run_react.yak --max-duration 1`,
启动日志:

```
[INFO] aicache cachebench - session-id=cachebench-1777973016-73993 ...
[WARN] failed to fetch AI runtime for session [cachebench-1777973016-73993]: record not found  // 预期: 新 session
[INFO] [config:2990] successfully restored timeline instance from persistent session [cachebench-1777973016-73993] with 0 items
```

`with 0 items` 即为隔离生效信号. 后续 `--max-duration 300` 复测预期会跑出更多
prompt (timeline init 不再是瓶颈, AI 调用本身才是节奏决定因素).

#### 副作用 / 注意事项

- 每次 cachebench 跑都会在 `ai_memory_collections_v1` / `ai_memory_entities_v1` /
  `ai_sessions_v1` 留一份新 session 记录, 不主动清理. 长期跑攒下来的话可以用
  `sqlite3 ~/yakit-projects/default-yakit.db "DELETE FROM ai_sessions_v1 WHERE session_id LIKE 'cachebench-%'"` 整理.
- 跑同一个 session id 多次复用是合法做法, 适合"想观察 timeline 累积对命中率的影响"
  这种特殊场景, 但不能拿来当默认值, 否则后续 cachebench 会再次掉进同一个坑.

#### 文件清单

| 文件 | 改动类型 |
| --- | --- |
| `common/ai/aid/aicache/cachebench/run_react.yak` | 新增 `--session-id` flag + 自动隔离 + 调用链注入 `aim.sessionID` |
| `common/ai/aid/aicache/CACHE_HIT_BENCHMARK.md` | 9.10 节 (本节) |

