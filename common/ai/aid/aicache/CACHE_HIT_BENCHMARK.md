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
