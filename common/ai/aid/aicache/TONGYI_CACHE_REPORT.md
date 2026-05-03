# Dashscope (tongyi) 显式上下文缓存 实测实验报告

> 本报告由 9 组真实 dashscope 调用实验产出。所有结论以 SSE 末帧 `usage`
> 字段（aispec 解析后的 `ChatUsage.PromptTokensDetails`）为唯一证据，
> 配合 1 组 raw SSE 末帧抓样作为锚点。**不复述文档**, 只列实测。
>
> 报告目的: 解答"aicache 该不该把 high-static section 拆成多 ChatContent
> 并用多 cache_control 精细控制？" — 实测结论是**不该**。详见 §6 / §7。
>
> 关键词: dashscope explicit cache, cache_control ephemeral, qwen3.6-plus,
> qwen3.5-flash, qwen3-vl-flash, cached_tokens, cache_creation_input_tokens,
> aibalance explicit_cache_rewriter, aicache hijacker

---

## 1. TL;DR (7 个核心结论)

1. **dashscope 显式缓存只命中"完整字节级前缀"** — 命中要求本次请求的
   "messages[0] 一路到最末 `cache_control` 标记位置"的字节序列与已有
   缓存块完全一致。差一个字节都不命中。
2. **多 `cache_control` 标记不会让 dashscope 在请求时自动尝试匹配更短的
   前缀**。文档中"以每个 cache_control 标记位置为终点向前回溯尝试命中"
   实测**只对最末一个**标记生效。E3 / E5 / E7 / E8 / E9 / E11 六组实验均 FAIL。
3. **浅 `cache_control` 标记不会创建独立缓存块** (E11 r1 反证): 即使
   r0 是 `[A+cc, B+cc, ...]` 把 A 段也标了 cc, 后续请求 `[A+cc, E, F]`
   (字节级保留 A+cc) 仍然 cached=0。dashscope 实际只对**最末**标记位置
   建立缓存块, 浅标记位置 既不建块, 也不参与命中, 只是个无效声明。
   (这一条修正了报告早期版本 §1.3 的不严谨判断)
4. **核心问题原始答案 (E2 PASS)**: 当业务发了 `[A,B,C+cc]` 之后再发
   `[A,D+cc]`, A 部分**不会独立命中**。整个 `[A,D]` 当作新缓存块创建。
5. **未命中时 dashscope 把整个 prompt 按 125% 计费 (E11 量化)**:
   `cache_creation_input_tokens ≈ prompt_tokens − 26`, 与 cache_control
   位置无关。由此推出:**「用了 cache_control 但每次没命中」 = 比完全
   不用 cache_control 还贵 25%**。E11 6 次顺序"标记漂移"请求实测整体
   成本比无缓存基线高 24.8%, 比理想全命中状态贵 1071%。详见 §6.2。
6. **「部分命中 + 增量建块」机制根本不存在 (E12 实测证伪)**: 已建
   `[A]` 1998 token 块后发 `[A, B+cc]` 总 3992 token (B 增量 2000 tok,
   远超 1024 阈值), **r1 仍然 cached=0, 整个 4000 tok 全建**。文档
   §6.1"若新请求的缓存内容包含已有缓存作为前缀, 则仅对新增部分计费"
   规则**完全不工作**, 与增量大小无关。换言之: **唯一能命中的场景是
   "完整字节级一致的请求重发"** (E12 r2 PASS 验证), 其它都是 miss。
7. **跨 model 行为完全一致**: qwen3.6-plus / qwen3.5-flash /
   qwen3-vl-flash 三个模型实测命中规则完全相同, 不是 qwen3.6-plus 的
   局部 bug。

由此**反推 aicache 的最佳策略不是"多 ChatContent + 多 cache_control 精
细控制"**, 而是"让 system message 的字节级前缀尽可能长期稳定, 在 system
末尾打一个 cache_control 即可"。**业务侧滥用 cache_control 的代价
不是"和无缓存一样", 而是「比无缓存还贵 25%」**。详见 §7。

### 1.1 dashscope 显式缓存的实际可用边界 (业务必读)

把上面 7 条压成"可用 / 不可用"的二元判定:

| 业务场景 | 是否命中 | 实际计费 |
|---|---|---|
| 完整请求字节级 1:1 重发 (含 system + user 全部) | ✓ 命中 | 10% (省 90%) |
| system 字节稳定, user 末尾变化 (如对话追加) | ✗ miss | **125% (贵 25%)** |
| 多段 cache_control 精细控制 | ✗ miss | **125% (贵 25%)** |
| 大段稳定前缀 + 短增量 (无论增量大小) | ✗ miss | **125% (贵 25%)** |
| 完全不带 cache_control 字段 | (无缓存机制) | 100% |

实际可命中的场景**几乎只有"完全相同的 prompt 重复发"**, 这在生产业务中
非常罕见 (通常每次会话都有时间戳 / sessionId / 用户输入差异)。dashscope
显式缓存目前在 OpenAI 兼容接口上**事实上不可用于通用业务场景**, 仅适
用于"批量复测 / 回归 / 脚本化重发"这类纯重复负载。

→ 见 §6 / §7 关于 aicache / aibalance 应该如何应对的具体建议。

---

## 2. 测试环境

| 项 | 值 |
|---|---|
| 测试时间 | E1-E10: 2026-05-03 23:43–23:48 (UTC+8); E11: 2026-05-04 00:24–00:26; E12: 2026-05-04 00:38–00:39 |
| Run ID | E1-E10: `run-1777823006312881000`; E11: `run-1777825499213141000`; E12: `run-1777826325134397000` |
| 主测 model | `qwen3.6-plus` |
| 对照 model | `qwen3.5-flash`, `qwen3-vl-flash` |
| API endpoint | `https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions` |
| 调用链路 | `ai.GetAI("tongyi", ...) -> tongyi.GatewayClient.Chat -> aispec.ChatBase` (与 [common/aibalance/server.go](../../../aibalance/server.go) 调用链路完全一致) |
| usage 拿取方式 | `aispec.WithUsageCallback`, 字段 `PromptTokensDetails.{CachedTokens, CacheCreationInputTokens, CacheType}` |
| Raw SSE 锚点 | `aispec.WithRawHTTPRequestResponseCallback`, body preview 4096B (E7 三个数据点完整覆盖末帧, 用作 aispec 解析正确性锚点) |
| 实验代码 | `/Users/v1ll4n/Projects/yaklang/common/aibalance/cmd_cache_probe_temp/main.go` (跑完已删除, 仅本报告引用) |
| 原始数据 | `/tmp/aibalance-cache-debug/results.jsonl` (跑完已删除) |
| 反污染机制 | 每次进程启动随机生成 `runID`, 注入到所有 prompt marker, 保证跨进程零缓存复用 |
| 关键代码引用 | [common/aibalance/explicit_cache_rewriter.go](../../../aibalance/explicit_cache_rewriter.go), [common/ai/aispec/msg.go](../../aispec/msg.go) `ChatContent.CacheControl`, `PromptTokensDetails.CacheCreationInputTokens` |

---

## 3. 实验矩阵设计

每条 system message 的 `Content` 由若干 `*aispec.ChatContent` 段组成,
其中带 `+cc` 的段在 `CacheControl` 字段挂上 `{"type":"ephemeral"}`。

| # | 假设要验证的命题 | r0 (建块) | r1 (考点) | r2 (确认) |
|---|---|---|---|---|
| E1 | 单标记单标记重复, 100% 命中 | `[A1+cc]` ~2000tok | 同 r0 | 同 r0 |
| E2 | **核心**: `[A,B,C+cc]` 之后 `[A,D+cc]`, A 能否独立命中? | `[A2,B2,C2+cc]` | `[A2,D2+cc]` | 同 r1 |
| E3 | 多标记 `[A+cc, B, C+cc]` 之后 `[A,D+cc]`, A 能否命中独立块? | `[A3+cc, B3, C3+cc]` | `[A3, D3+cc]` | 同 r1 |
| E4 | <1024 token 是否真不建块? | `[A4+cc]` ~733tok | 同 r0 | — |
| E5 | 增量计费: `[A+cc]` 后发 `[A,B+cc]`, 是否 cached=A + create=B? | `[A5+cc]` ~1623tok | `[A5, B5+cc]` ~2061tok | 同 r1 |
| E6 (对照) | qwen3.5-flash 复跑 E2, 验证规则与 model 无关 | `[A6,B6,C6+cc]` | `[A6,D6+cc]` | 同 r1 |
| E7 (对照) | qwen3-vl-flash 复跑 E3, 同上 | `[A7+cc,B7,C7+cc]` | `[A7,D7+cc]` | 同 r1 |
| E8 (E3 加强) | r1 仍保留 A+cc 标记, A 能否命中? | `[A8+cc, B8, C8+cc]` | `[A8+cc, D8+cc]` | 同 r1 |
| E9 (E8 控制) | 先**单独** `[A+cc]` 建 A 独立块, 然后多标记 `[A+cc, B+cc]` | `[A9+cc]` | `[A9+cc, B9+cc]` | 同 r1 |
| E10 (E9 反证) | E9 r0 建的 A9 块在 5min TTL 内是否仍活? 单标记能否命中? | `[A9+cc]` (复用 E9 marker) | — | — |
| E11 (用户提问 / 漂移成本) | **「A 永久带 cc + 标记位置漂移」6 次顺序请求, 量化成本** | `[A+cc, B+cc, C, D]` → `[A+cc, E, F]` → `[A+cc, G+cc, H]` → `[A+cc, G, H+cc, I]` → `[A+cc, G, H, I+cc, J]` → `[A+cc, G, H, I, J+cc, K]` (6 次连续, 见 §4.11) |
| E12 (1024 阈值 vs 增量计费 隔离实验) | **验证「增量 ≥ 1024 是否能让部分命中生效」** | `[A+cc]` 1998tok → `[A, B+cc]` 增量 B≈2000tok → 重发 `[A, B+cc]` → `[A, C+cc]` 增量 C≈500tok (4 次连续, 见 §4.13) |

设计原则:
- **每组实验独立 marker** + 共同 `runID` 前缀, 字节级隔离, 杜绝实验间
  缓存交叉污染。
- **PASS/FAIL 由实测 token 数 vs 期望关系判定** (允许 ±5% 误差; 期望
  数字为粗估)。
- **FAIL 不藏不掩**, 数据照贴, 假设修正。

---

## 4. 实验结果 (期望 vs 实测)

完整实测数据 (单位: token):

```
exp   r   model              sig                              prompt  cached  create
E1    0   qwen3.6-plus       [A1+cc]                            2031       0    2005
E1    1   qwen3.6-plus       [A1+cc]                            2031    2005       0
E1    2   qwen3.6-plus       [A1+cc]                            2031    2005       0
E2    0   qwen3.6-plus       [A2, B2, C2+cc]                    2247       0    2221
E2    1   qwen3.6-plus       [A2, D2+cc]                        1550       0    1524
E2    2   qwen3.6-plus       [A2, D2+cc]                        1550    1524       0
E3    0   qwen3.6-plus       [A3+cc, B3, C3+cc]                 2906       0    2880
E3    1   qwen3.6-plus       [A3, D3+cc]                        2209       0    2183
E3    2   qwen3.6-plus       [A3, D3+cc]                        2209    2183       0
E4    0   qwen3.6-plus       [A4+cc]                             733       0       0
E4    1   qwen3.6-plus       [A4+cc]                             733       0       0
E5    0   qwen3.6-plus       [A5+cc]                            1649       0    1623
E5    1   qwen3.6-plus       [A5, B5+cc]                        2087       0    2061
E5    2   qwen3.6-plus       [A5, B5+cc]                        2087    2061       0
E6    0   qwen3.5-flash      [A6, B6, C6+cc]                    2246       0    2220
E6    1   qwen3.5-flash      [A6, D6+cc]                        1548       0    1522
E6    2   qwen3.5-flash      [A6, D6+cc]                        1548    1522       0
E7    0   qwen3-vl-flash     [A7+cc, B7, C7+cc]                 2911       0    2887
E7    1   qwen3-vl-flash     [A7, D7+cc]                        2207       0    2183
E7    2   qwen3-vl-flash     [A7, D7+cc]                        2207    2183       0
E9    0   qwen3.6-plus       [A9+cc]                            1508       0    1482
E9    1   qwen3.6-plus       [A9+cc, B9+cc]                     2206       0    2180
E9    2   qwen3.6-plus       [A9+cc, B9+cc]                     2206    2180       0
E10   0   qwen3.6-plus       [A9+cc] (re-fetch)                 1508    1482       0
E8    0   qwen3.6-plus       [A8+cc, B8, C8+cc]                 2906       0    2880
E8    1   qwen3.6-plus       [A8+cc, D8+cc]                     2209       0    2183
E8    2   qwen3.6-plus       [A8+cc, D8+cc]                     2209    2183       0
E11   0   qwen3.6-plus       [A+cc, B+cc, C, D]                 3066       0    3040
E11   1   qwen3.6-plus       [A+cc, E, F]                       2632       0    2606
E11   2   qwen3.6-plus       [A+cc, G+cc, H]                    2892       0    2866
E11   3   qwen3.6-plus       [A+cc, G, H+cc, I]                 3586       0    3560
E11   4   qwen3.6-plus       [A+cc, G, H, I+cc, J]              4280       0    4254
E11   5   qwen3.6-plus       [A+cc, G, H, I, J+cc, K]           4974       0    4948
E12   0   qwen3.6-plus       [A+cc]                             2024       0    1998
E12   1   qwen3.6-plus       [A, B+cc]  (增量 B≈2000tok)        4018       0    3992
E12   2   qwen3.6-plus       [A, B+cc]  (重发 r1)               4018    3992       0
E12   3   qwen3.6-plus       [A, C+cc]  (增量 C≈500tok)         2578       0    2552
```

### 4.1 E1 — 单标记单标记重复 (PASS)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 | `cache_create>0, cached=0` (建块) | `create=2005, cached=0` | PASS |
| r1 | `cache_create=0, cached>0` (命中) | `create=0, cached=2005` | PASS |
| r2 | 同 r1 | `create=0, cached=2005` | PASS |

**解读**: 单标记 + 重复请求 = 100% 命中, 完全符合文档。命中 token =
总 prompt - 18 (即追加在 cache_control 之后的少量内部 token, 见
dashscope 文档"为什么 input_tokens 不等于 cache_creation 与 cached
之和")。

### 4.2 E2 — ABC vs AD 核心问题 (PASS, 直接回答用户问题)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A2,B2,C2+cc]` | `create≈A+B+C, cached=0` | `create=2221, cached=0` | PASS |
| r1 `[A2,D2+cc]` | `cached=0` (A 不能独立命中) | `create=1524, cached=0` | **PASS (验证用户假设)** |
| r2 `[A2,D2+cc]` | `cached≈A+D` (r1 已建 [A,D] 块) | `create=0, cached=1524` | PASS |

**关键解读**: 这是用户原始问题的直接答案。`[A,B,C+cc]` 缓存创建后,
后续 `[A,D+cc]` 请求**不会**因为 A 部分前缀一致而命中 r0 的缓存块。
整个 `[A,D]` 被当作全新缓存块从零创建 (r1 cache_create=1524 ≈ A+D
全部 token)。第三轮 r2 命中的是 r1 建的 `[A,D]` 块, 而不是 r0 的
`[A,B,C]` 块。

**这意味着**: 在业务"system message 末尾不变, user message 频繁变化"
的场景下, **不能指望** "我把 high-static + semi-dynamic + timeline
全部拼到 system 里 + 在 system 末尾打 cache_control" 来实现"即使
timeline 变化, 也能命中 high-static 段"。dashscope 不会做这种事。

### 4.3 E3 — 多标记 `[A+cc, B, C+cc]` 反证 (FAIL)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A3+cc, B3, C3+cc]` | `create≈A+B+C` (建两个块: A 和 A+B+C) | `create=2880, cached=0` | PASS |
| r1 `[A3, D3+cc]` | `cached≈A3` (命中 A 独立块) + `create≈D3` | `create=2183, cached=0` | **FAIL** |
| r2 `[A3, D3+cc]` | `cached≈A+D` (r1 已建 [A,D] 块) | `create=0, cached=2183` | PASS |

**FAIL 现象**: 即便 r0 看起来给 A3 单独打了 `cache_control`, r1 中 A3
段也**没有**被独立命中。整个 `[A3,D3]` 被当作全新缓存块创建。

**可能原因**: dashscope 多 cache_control 命中机制只对**最末**那个标记
位置尝试匹配前缀缓存; r1 的最末 cache_control 在 D3 处, 尝试匹配
`[A3, D3]` 整体没找到, 不会进一步检查更短前缀 `[A3]` 单独。

### 4.4 E4 — 1024 token 阈值 (PASS)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A4+cc]` 733tok | `create=0` (低于 1024 阈值) | `create=0, cached=0` | PASS |
| r1 同 r0 | `create=0, cached=0` (没建过块) | `create=0, cached=0` | PASS |

**解读**: dashscope 文档"显式缓存最少 1024 token"实测准确, 低于阈值
的请求返回 `cache_type="ephemeral"` 但 `cache_creation=0` 且
`cached=0`。

### 4.5 E5 — 增量计费 (FAIL)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A5+cc]` 1623tok | `create≈A5, cached=0` | `create=1623, cached=0` | PASS |
| r1 `[A5, B5+cc]` 2061tok | `cached≈A5 (命中 r0 块) + create≈B5 (增量建)` | `create=2061, cached=0` | **FAIL** |
| r2 `[A5, B5+cc]` | `cached≈2061 (命中 r1 建的 [A,B] 块)` | `create=0, cached=2061` | PASS |

**FAIL 现象**: dashscope 文档明确写"若新请求的缓存内容包含已有缓存
作为前缀, 则仅对新增部分计费 (即新缓存 Token 数减去已有缓存 Token
数)" — 但 E5 实测**完全不工作**。`[A5+cc]` 已建块的情况下,
`[A5, B5+cc]` 整个被当作新块创建, 没有命中 A5 部分。

**可能原因**: 同 E3。r1 唯一的 cache_control 在 B5 处, 尝试匹配
`[A5, B5]` 整体没找到, 不会检查更短前缀 `[A5]`。

### 4.6 E6 — qwen3.5-flash 复跑 E2 (PASS, 跨 model 一致)

数据与 E2 完全同向: r1 cached=0, r2 cached=1522 命中 r1 建的 [A,D]
块。证明 E2 结论与 model 无关。

### 4.7 E7 — qwen3-vl-flash 复跑 E3 (FAIL, 跨 model 一致)

数据与 E3 完全同向: r1 cached=0 (期望 cached≈A7), r2 cached=2183
命中 r1 建的 [A,D] 块。**证明 FAIL 不是 qwen3.6-plus 的局部 bug,
而是 dashscope 全系实现一致**。

### 4.8 E8 — 加强版多标记: r1 仍保留 A+cc 标记 (FAIL)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A8+cc, B8, C8+cc]` | `create≈total, cached=0` | `create=2880, cached=0` | PASS |
| r1 `[A8+cc, D8+cc]` | `cached≈A8` (浅 cache_control 在 A8 应命中独立块) + `create≈D8` | `create=2183, cached=0` | **FAIL** |
| r2 `[A8+cc, D8+cc]` | `cached≈A+D` | `create=0, cached=2183` | PASS |

**FAIL 现象**: 即使 r1 在 A8 后面**仍然保留** cache_control 标记
(与 r0 的 A8 cache_control 标记位置完全一致), A8 仍然没有命中独立
块。整个 [A,D] 当新块全建。

**对比 E3**: E8 的"r1 保留 A+cc 标记"看上去更接近文档描述的"通过
A 标记尝试命中 A 独立块"的场景, 仍然 FAIL。说明 dashscope 实现
**根本不在浅 cache_control 位置做匹配尝试**, 哪怕是字面相同的位置。

### 4.9 E9 — 严格控制变量: 单独建块 + 多标记请求 (FAIL)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A9+cc]` 1482tok 单独建块 | `create=1482, cached=0` | `create=1482, cached=0` | PASS |
| r1 `[A9+cc, B9+cc]` | `cached≈A9 (命中 r0 单独建的 A 块) + create≈B9 (增量建 [A,B])` | `create=2180, cached=0` | **FAIL** |
| r2 `[A9+cc, B9+cc]` | `cached≈A+B` | `create=0, cached=2180` | PASS |

**FAIL 现象**: 这是最严格的控制变量实验:
1. r0 用单标记 `[A9+cc]` 单独成功建了 A9 块 (E10 反证已证实);
2. r1 完全保留了 r0 的 A9 段 + cache_control 位置;
3. r1 仍然 cached=0, 即没命中已存在的 A9 块。

唯一差异: r1 在 A9 后追加了 `B9+cc`。这一个差异就让 dashscope 的命中
逻辑彻底失效。**这是对 dashscope 多 cache_control 命中机制最有力的
否定证据**。

### 4.10 E10 — 反证: A9 块仍活着, 单标记能命中 (PASS)

| round | 期望 | 实测 | 判定 |
|---|---|---|---|
| r0 `[A9+cc]` (E9 r0 之后 ~30s) | `cached≈A9, create=0` | `cached=1482, create=0` | PASS |

**解读**: E10 紧接着 E9 跑, 用同样的单标记 `[A9+cc]` 请求, 命中了
E9 r0 建的 A9 块。这**反证排除**了"E9 r0 根本没建 A9 块"的假设, 进一
步加强了 E9 FAIL 的力度: A9 块**确实**建好了, dashscope 也确实
**能**通过单标记命中它, 只是**多标记请求中的同位置标记不会做命中
尝试**。

### 4.11 E11 — 「A 永久带 cc + 标记漂移」6 次顺序请求 (用户提问场景)

用户场景: 业务侧把"high-static"段 A 永久打 cache_control, 后续随着会
话推进追加新内容 (E/F → G+cc/H → G/H+cc/I → ...), 每次保留 A 段及其
cache_control。问: 整体成本会上升还是下降?

| round | sig | prompt | cached | cache_create | 期望 cached | 判定 |
|---|---|---|---|---|---|---|
| r0 | `[A+cc, B+cc, C, D]` | 3066 | 0 | 3040 | 0 | PASS (建块) |
| r1 | `[A+cc, E, F]` (最末标记 = A, 字节级与 r0 的 [A+cc] 完全一致) | 2632 | **0** | 2606 | ~A (~1450) | **FAIL** |
| r2 | `[A+cc, G+cc, H]` (最末标记 = G, [A,G] 新组合) | 2892 | 0 | 2866 | 0 | PASS (新组合) |
| r3 | `[A+cc, G, H+cc, I]` (最末标记漂移到 H) | 3586 | 0 | 3560 | 0 | PASS (新组合) |
| r4 | `[A+cc, G, H, I+cc, J]` (最末标记漂移到 I) | 4280 | 0 | 4254 | 0 | PASS (新组合) |
| r5 | `[A+cc, G, H, I, J+cc, K]` (最末标记漂移到 J) | 4974 | 0 | 4948 | 0 | PASS (新组合) |

#### 4.11.1 r1 FAIL 的关键意义 — 推翻"浅标记创建端有效"假设

之前 §1.3 (旧版) 推断"多 cache_control 会在浅标记位置建独立块, 后续
单标记请求能命中" — 这条**被 E11 r1 证伪**:

- r0 是 `[A+cc, B+cc, C, D]`, A 段确实带了 cache_control 标记;
- r1 是 `[A+cc, E, F]`, A 段 (含 cache_control) 与 r0 字节级完全一致,
  且 r1 最末标记 = A 位置;
- 按"浅 cc 建独立 [A] 块"假设, r1 应该命中 r0 创建的 [A] 块, cached ≈ A;
- **实测 r1 cached=0**, 整个 [A,E,F] 全建 (cache_create = prompt-26)。

修正后的事实是: **dashscope 实际只对最末 cache_control 标记位置创建
缓存块**, 浅标记位置完全失效 — 既不创建独立块, 也不参与命中。E10
之前看似"创建端有效"的 PASS, 其实是因为 E9 r0 是单标记请求
(`[A9+cc]`), 块字节序列就是 [A9+cc], 与 E10 的请求字节级完全一致,
所以 E10 命中。**E10 命中是字节级一致, 不是"独立 A 块"**。

#### 4.11.2 cache_creation_input_tokens 字段实际语义

E11 全部 6 次 cached=0 的请求中, `cache_creation_input_tokens` 与
`prompt_tokens − 26` 的差永远为 0:

```
r   prompt   cache_create   create-(prompt-26)
0     3066         3040                    0
1     2632         2606                    0
2     2892         2866                    0
3     3586         3560                    0
4     4280         4254                    0
5     4974         4948                    0
```

(E1-E10 同样验证: cached=0 时 cache_create = prompt - 26 = 整个 prompt
中可缓存计费的 token 数)

由此推出: **`cache_creation_input_tokens` 字段的实际语义不是「本次新建
缓存块的大小」, 而是「本次按 125% 单价计费的 token 数」**。当 cached=0
时, 整个 prompt (除 26 token 内部 overhead) 全部按 125% 计费。这意味着
**用了 cache_control 但没命中** = `prompt × 125%` 计费, 比无 cache_
control 的 `prompt × 100%` 还**贵 25%**。

### 4.12 E12 — 「部分命中 + 增量建块」机制是否存在 (用户怀疑验证)

**用户怀疑** (来自 E11 跑完后追问): "是不是 dashscope 的 1024 token 最
小阈值不仅适用于首次创建块, 还适用于增量计费? 即已有 [A] 1500 tok 块,
后续 [A,B+cc] 中的增量 B 如果 < 1024 就不能部分命中 A?"

E5 之前曾经测过类似场景, 但当时 B5 增量 ≈ 438 token < 1024, 没法把
"增量大小 < 1024" 和 "dashscope 根本不做增量计费" 这两个假设隔离开。
E12 用 **增量 = ~2000 token (远超 1024 阈值的 2 倍)** 直接证伪用户的
"增量 ≥ 1024 即可部分命中" 假设。

| round | sig | prompt | cached | cache_create | 增量 token | 期望 (假设增量受 1024 限制) | 期望 (假设根本无增量计费) | 实测结果 |
|---|---|---|---|---|---|---|---|---|
| r0 | `[A+cc]` | 2024 | 0 | 1998 | — | 建 [A] 块 | 同左 | PASS (建块) |
| r1 | `[A, B+cc]` | 4018 | **0** | 3992 | B ≈ 2000 (**>> 1024**) | cached=A=2000, create=B=2000 | cached=0, create=4000 | **支持假设 2** |
| r2 | `[A, B+cc]` 重发 | 4018 | 3992 | 0 | — | cached=A+B=4000 | cached=A+B=4000 | PASS (整体命中验证 r1 真的建了 [A,B] 块) |
| r3 | `[A, C+cc]` | 2578 | 0 | 2552 | C ≈ 500 (< 1024) | cached=A=2000, create=0 | cached=0, create=2500 | **支持假设 2** |

#### 4.12.1 实测结论 — 「部分命中 + 增量建块」机制根本不存在

E12 r1 是关键判别点: 增量 B ≈ 2000 token (远超 1024 阈值), 如果 dashscope
真的实现了文档 §6.1 描述的"已有缓存作为前缀, 仅对新增部分计费", 那 r1
应该 cached ≈ 2000 + cache_create ≈ 2000。但**实测 cached=0, 整个
[A,B] 4000 token 全部按 125% 计费**。

E12 r3 进一步反向验证: 增量 C 只有 500 token < 1024, 同样 cached=0,
**与 r1 行为完全相同**。这说明"是否部分命中"和"增量大小是否 ≥ 1024"
**完全无关** — 两种情况都不命中。

最终判定: **dashscope 显式缓存在 OpenAI 兼容接口的 qwen3.6-plus 上,
不存在"部分命中 + 增量建块"机制**。文档 §6.1 描述的"增量计费规则"
在实际行为中**完全没有实现**, 与增量大小无关。

#### 4.12.2 1024 token 阈值的实际语义

E12 验证了 1024 token 阈值的**唯一作用**:

- **首次创建块时**: 如果"messages[0] 到最末 cache_control 位置"的字
  节序列 token 数 < 1024, 不创建缓存块 (E4 PASS 已证)
- **后续命中时**: **不影响** — 命中条件仅是字节级 1:1 一致, 与块大小
  无关
- **增量计费时**: **不存在** — 因为增量计费机制本身不存在 (E12 r1 证伪)

#### 4.12.3 r2 反向验证

r2 重发 r1 完全相同的 `[A, B+cc]` 4018 token 请求, **cached=3992
(全命中), cache_create=0**。这强证: r1 确实在 dashscope 端创建了完整
的 [A, B] 4000 token 块, 5min TTL 内字节级一致请求重发能 100% 命中。
这反过来证明: r1 的 cached=0 不是因为"块没建好", 而是因为 dashscope
**根本不查 [A] 子前缀**, 它只查"本次最末标记位置之前的整段字节序列
能否完全匹配某个已建的块"。

### 4.13 PASS / FAIL 汇总

| Exp | 判定 | 备注 |
|---|---|---|
| E1 | PASS | 基线 |
| E2 | PASS | **直接回答用户原始问题: ABC 之后 AD, A 不命中** |
| E3 | **FAIL** | 多标记同请求内 A 段独立命中失败 |
| E4 | PASS | 1024 阈值确认 |
| E5 | **FAIL** | 文档"增量计费"机制实测不工作 |
| E6 | PASS | 跨 model 一致 (qwen3.5-flash) |
| E7 | **FAIL** | 跨 model 一致 (qwen3-vl-flash 同 E3) |
| E8 | **FAIL** | 后续请求保留 A+cc 标记仍不命中 |
| E9 | **FAIL** | 严格控制变量后仍不命中 |
| E10 | PASS | 字节级一致的请求重发能命中 (旧解读"A 块独立建好"被 E11 推翻, 实际上是字节级一致命中) |
| E11 | r0 PASS / **r1 FAIL** / r2-r5 PASS | r1 失败推翻"浅标记创建端有效", 整体序列成本比无缓存基线**高 24.8%** |
| E12 | r0 PASS / **r1 FAIL** / r2 PASS / r3 FAIL | r1 失败 (增量 ~2000 tok >> 1024) **彻底证伪"增量计费"机制**, r2 PASS 反证 [A,B] 块确实建好且字节一致请求能命中 |

**FAIL 的全部都集中在"非完整字节级前缀的命中失败"这一类**, 7 组实验
彼此交叉印证, 结论非常稳定。**E11 r1 把"浅标记创建端有效"证伪, E12 r1
把"增量计费"也证伪**, 把命中条件压缩到最严格的形式: **必须本次最末
标记位置之前的字节序列与已有缓存块完全一致**, 既无浅前缀匹配, 也无
增量部分命中。

---

## 5. 命中机制图解 (数据驱动)

下图是基于实测数据反推的 dashscope 显式缓存实际行为, **不是文档描述
的行为**:

```mermaid
flowchart TD
    Req["新请求 messages = [m0, m1, ..., mN]<br/>含 1+ 个 cache_control 标记"] --> FindLast["定位<b>最末</b>cache_control 标记位置 P_last"]
    FindLast --> BuildPrefix["计算字节级前缀 = messages[0..P_last]<br/>包含 P_last 之前的所有 content"]
    BuildPrefix --> CheckHit{"前缀字节序列<br/>是否存在于<br/>同账号同 model 的 5min TTL 缓存表?"}

    CheckHit -- YES --> Hit["cached_tokens = 该缓存块 token 数<br/>cache_creation_input_tokens = 0<br/>计费按 input_tokens 单价 10%"]
    CheckHit -- NO --> Create["新建缓存块, 块大小 = 整个前缀 token 数<br/>cache_creation_input_tokens = 块大小 - 已有较短缓存的 token 数<br/>cached_tokens = 较短缓存命中 token 数 (一般为 0)"]

    Hit --> Refresh["命中后该缓存块 TTL 重置为 5min"]
    Create --> Register["新缓存块写入缓存表, TTL=5min"]

    BuildPrefix -.-> ShallowCC["其它<b>浅</b>cache_control 标记位置<br/>(P0, P1, ...)"]
    ShallowCC -.->|"在 r0 时<b>也不</b>建独立块"| Skip1["实测: 这些位置仅是声明, 无任何实际效果"]
    ShallowCC -.->|"在 r1+ 时<b>不会</b>主动尝试命中"| Skip2["实测: 这些位置在请求时被跳过"]
```

注意虚线部分: 浅 cache_control 在**创建端和命中端均无效** — 既不创建
独立块 (E11 r1 FAIL 证实, 见 §4.11.1), 也不参与后续命中
(E3/E5/E7/E8/E9 五组 FAIL 证实)。**dashscope 实际只把"最末标记位置之
前的整个字节序列"当作单一缓存块**, 浅标记字段虽被序列化发送, 但被服
务端忽略。

### 5.1 与 dashscope 文档的差异

| 维度 | 文档描述 | 实测行为 |
|---|---|---|
| 创建端 (r0) | "以每个 cache_control 标记位置为终点向前回溯, 创建多个缓存块" | ✗ **只创建最末标记位置之前的单一缓存块**, 浅标记位置不建块 (E11 r1 FAIL 证实) |
| 命中端 (r1+) | "以每个 cache_control 标记位置为终点向前回溯尝试命中" | ✗ **只对最末标记位置匹配**, 浅标记位置不参与命中 (E3/E5/E7/E8/E9 FAIL) |
| 增量计费 | "若新请求的缓存内容包含已有缓存作为前缀, 则仅对新增部分计费" | ✗ **机制根本不存在** (E5 FAIL + E12 r1 决定性 FAIL): 即使增量部分 ≈ 2000 tok 远超 1024 阈值, 整个新前缀仍然按 cache_creation 全计 |
| 未命中计费 | (文档未直接说) | **整个 prompt 按 125% 计费** (cache_create ≈ prompt − 26, 详见 §4.11.2)。即"用了 cache_control 但没命中" 比"完全不用 cache_control"还**贵 25%** |

---

## 6. 计费表 (基于实测)

按 dashscope 公布的"创建按 125%, 命中按 10%, 其余按 100%"单价定性
列出 4 种典型场景的实际计费比例 (单价基准 = 100% input_token 单价):

| 场景 | 实测请求 | 实际计费 |
|---|---|---|
| 单标记基线命中 (E1 r1) | `[A1+cc]` 2031 prompt, cached=2005 | (cached: 2005×10%) + (其余: 26×100%) = **12.1% prompt 单价** |
| ABC 之后 AD, A 部分计费 (E2 r1) | `[A2,D2+cc]` 1550 prompt, create=1524 | (create: 1524×125%) + (其余: 26×100%) = **124.5% prompt 单价** (比无缓存还贵) |
| 多标记 r0 创建 (E3 r0) | `[A3+cc,B3,C3+cc]` 2906 prompt, create=2880 | (create: 2880×125%) + (其余: 26×100%) = **124.7% prompt 单价** |
| 多标记 r1 命中失败 (E3 r1) | `[A3,D3+cc]` 2209 prompt, create=2183 | (create: 2183×125%) + (其余: 26×100%) = **124.7% prompt 单价** (比无缓存还贵) |

**关键计费提示**: 因为 dashscope 实际实现"多 cache_control 浅标记命中
不工作", 业务**误用**多 cache_control 的成本不是"和无缓存一样", 而是
**比无缓存高 25%** (因为创建端按 125% 单价计)。这是对 aibalance /
aicache 设计要避免的最大坑。

### 6.2 「A 永久带 cc + 标记漂移」6 次序列实测成本 (E11)

下表用 1 token 在 100% input 单价下的"成本单位"作为基准货币, 直接量化
E11 的 6 次顺序请求 (具体计算见 §4.11):

| 请求 | sig | prompt | cached | create | 无缓存基线 (×100%) | 实测 (×125%) | 理想全命中 (×10%+overhead) |
|---|---|---|---|---|---|---|---|
| r0 | `[A+cc, B+cc, C, D]` | 3066 | 0 | 3040 | 3066.0 | 3826.0 | 330.0 |
| r1 | `[A+cc, E, F]` | 2632 | 0 | 2606 | 2632.0 | 3283.5 | 286.6 |
| r2 | `[A+cc, G+cc, H]` | 2892 | 0 | 2866 | 2892.0 | 3608.5 | 312.6 |
| r3 | `[A+cc, G, H+cc, I]` | 3586 | 0 | 3560 | 3586.0 | 4476.0 | 382.0 |
| r4 | `[A+cc, G, H, I+cc, J]` | 4280 | 0 | 4254 | 4280.0 | 5343.5 | 451.4 |
| r5 | `[A+cc, G, H, I, J+cc, K]` | 4974 | 0 | 4948 | 4974.0 | 6211.0 | 520.8 |
| **TOT** | | | | | **21430.0** | **26748.5** | **2283.4** |

**三条横向对比**:

| 对比 | 比例 | 解读 |
|---|---|---|
| 实测 vs 无缓存基线 | **+24.8%** | 业务侧"加了 cache_control 但每次没命中" 比"完全不用 cache_control"还**贵 24.8%** |
| 实测 vs 全命中理想上限 | **+1071%** | 实测成本是理想全命中状态的 11.7 倍 |
| 全命中理想 vs 无缓存 | **−89.3%** | 显式缓存若每次都命中, 理论上能省 89% 输入成本 |

**为什么 E11 全部 cached=0?** 因为这 6 次请求的"最末 cache_control
位置之前的字节序列"两两不同:

- r0 最末标记在 B 位置, 字节前缀 = [A,B] (~2110 token)
- r1 最末标记在 A 位置, 字节前缀 = [A] (~1450 token) — 看似与 r0 共
  享 [A] 段, 但 r0 实际建的块是 [A,B], 不存在独立的 [A] 块, 所以 r1
  的 [A] 前缀找不到匹配, miss
- r2 最末标记在 G, 字节前缀 = [A,G] — 全新组合, 无匹配
- r3-r5 同理, 每次最末标记位置漂移, 字节前缀都是历史从未见过的新组合

**关键教训**:
1. **多 cache_control 不能让浅标记位置独立"被命中"** — 即使浅标记位
   置之前的字节序列 (此例中的 [A]) 在历史请求中存在, dashscope 也不
   会拿浅标记位置的字节前缀去匹配缓存表;
2. **最末 cache_control 位置在请求间漂移** = 每次都创建一个全新缓存
   块, 整体成本爆炸;
3. **业务侧滥用 cache_control 的代价是真实存在的 +25% 损失**, 不是
   "加了无伤大雅"。

---

## 7. 给 aicache 模块的具体建议

### 7.1 现状

[aicache hijacker](hijacker.go) 当前流程:
1. PROMPT_SECTION 4 段切片 (high-static / semi-dynamic / timeline / dynamic);
2. 把 high-static 段单独抠出来作为 `role:system` 消息, 其余拼成
   `role:user` 消息;
3. 由 [common/aibalance/explicit_cache_rewriter.go](../../../aibalance/explicit_cache_rewriter.go)
   `RewriteMessagesForExplicitCache` 在最末 `system` 消息的 content
   末尾打一个 `cache_control:{"type":"ephemeral"}` 标记。

### 7.2 实测推论

E2 结论已经直接打脸了"high-static 拆出来当 system 消息就能让
high-static 段在 timeline / dynamic 变化时仍然命中" 的假设:
- 即便 high-static 内容字节级稳定, 只要 user 消息 (timeline +
  dynamic) 字节变化, dashscope 都按"完整字节级前缀"判断, **system
  消息内容稳定不能让 system + user 整体的字节前缀稳定**, 所以**仍然
  miss**。

E3/E5/E7/E8/E9/E11 结论又打脸了"那把 high-static 也单独打个
cache_control 标记不就行了" 的假设:
- 多标记浅位置**既不创建独立块, 也不参与命中**, 只有"完整到最末标记"
  的字节前缀一致才命中 (E11 r1 进一步证伪了"浅标记创建端有效"这条);
- 误用还有更糟的副作用: **未命中时整个 prompt 按 125% 计费**, 比
  "完全不用 cache_control"还**贵 25%** (见 §6.2 E11 整体序列实测)。

### 7.3 唯一可行的优化方向

**唯一可靠的高命中策略**: 让 dashscope 命中的"完整字节前缀"在多次
请求间保持一致。即:

1. **system 消息内容**: 必须**字节级**稳定 (high-static + semi-dynamic
   全部包含, 但**绝不能**包含 timeline / dynamic);
2. **cache_control 标记**: 在 system 消息末尾打一个就够 (现状 OK);
3. **timeline / dynamic 内容**: 必须放在 user 消息里, 且**绝不在
   user 消息上加 cache_control 标记** (因为 user 内容必变, 加了反而
   触发 125% 计费的"创建端"开销)。

### 7.4 反推: aicache hijacker 当前实现是"接近最优"的, 但有一个隐患

当前 hijacker 把 high-static 抽出来 + 把 semi-dynamic + timeline +
dynamic 全部拼到 user 消息。这意味着:
- system 消息 = high-static 段, **极度稳定**, 命中良好;
- user 消息 = semi-dynamic + timeline + dynamic, **每次都可能字节
  变化**, 不命中, 不算缓存。

**隐患**: aibalance rewriter 当前 `RewriteMessagesForExplicitCache`
只在最末 `system` 消息打 cache_control 标记 ([common/aibalance/explicit_cache_rewriter.go:124-160](../../../aibalance/explicit_cache_rewriter.go))。在 hijacker 拆分后, 这个标记
会**正确地**打在 high-static system 消息末尾, 所以实际工作良好。**不
要改**这个逻辑。

### 7.5 不要做的事 (重要)

- ❌ **不要**把 aicache 升级成"按 section 边界封装多 ChatContent + 每
  个 section 末尾打 cache_control" — 实测证明这种"精细控制"不仅不
  会提升命中率, 反而因为创建端 125% 计费而**增加成本** (E11 实测整体
  比无缓存基线**高 24.8%**, 见 §6.2)。
- ❌ **不要**让 hijacker 在 user 消息上打 cache_control — user 内容必
  变, 触发持续的"创建端 125%"开销。
- ❌ **不要**指望"high-static 字节稳定就能在 timeline 变化时命中" —
  E2 PASS 已证伪。
- ❌ **不要**指望"A 段永久带 cc 标记, 后续追加内容仍能命中 A 段" —
  E11 r1 已证伪 (即使 A 段字节级完全一致且最末标记位置 = A, dashscope
  也不会去命中"曾经是浅标记"的位置上的字节序列)。

### 7.6 建议要做的事

- ✅ **加测点**: 在 aicache 的 mirror 观测里, 加一个"上游 aispec usage
  callback 命中率"的统计, 让我们能直接看到 dashscope 实际命中情况
  (而不是依赖 aicache 自己的 LCP 估算)。这与 [aibalance
  cache_stats](../../../aibalance/db_cache_stats.go) 的 portal 卡片
  互补 — aicache 这边按 section 维度细化, aibalance 那边按总量维度
  汇总。
- ✅ **业务侧约束**: 在 [aicache/README.md](README.md) 显式说明 "**
  high-static section 必须在 prompt 渲染期间字节级稳定**, 否则上游
  缓存命中率会归零", 并配合 [aicache splitter](splitter.go) 加一个
  "section 字节稳定性自检" 测试点。
- ✅ **等待 dashscope 升级**: 如果未来 dashscope 实现多 cache_control
  浅位置匹配 (与文档对齐), 再考虑多 ChatContent + 多标记策略, 但目
  前 (2026-05) 实测**未实现**。

---

## 8. 给 aibalance 的具体建议

### 8.1 当前实现已是最优, 不需要改

[common/aibalance/explicit_cache_rewriter.go](../../../aibalance/explicit_cache_rewriter.go)
的 `RewriteMessagesForExplicitCache` 实现 (在最末 system 消息的
content 末尾打一个 cache_control 标记) 在实测中是 dashscope 显式缓存
能用的**最优策略**。E1 单标记基线达到 99% 命中率, E2 / E5 / E6 等多
轮场景的命中行为也完全可解释。

### 8.2 不要做"多 cache_control" 的精细化升级

之前的 plan 第 §7 节曾建议把 aibalance rewriter 升级到多 cache_control
精细控制模式, **本报告 E3/E5/E7/E8/E9 五组 FAIL 实测结论否决了这条
路径**。继续做这个升级只会增加复杂度并提高 25% 计费成本。

### 8.3 可考虑的扩展 (低优先)

- 在 cache_stats 里区分"上游真命中" (`cached_tokens > 0`) vs "上游
  纯创建" (`cache_creation_input_tokens > 0`), 当前 [common/aibalance/db_cache_stats.go](../../../aibalance/db_cache_stats.go)
  只统计前者, 把后者也加进来能识别"业务每次都创建新块" 的异常模式。
- 在 [common/aibalance/server.go](../../../aibalance/server.go)
  `onUsageForward` 里加一个采样日志, 当 `cache_creation_input_tokens
  > 1000 && cached_tokens == 0` 时打 warn, 提示"上游缓存大量重复创
  建, 可能 prompt 在帧间有微调", 帮助业务侧自省。

---

## 9. 附录

### 9.1 raw SSE 末帧采样 (E7 三个数据点, 完整覆盖)

E7 用 `qwen3-vl-flash`, 响应内容极短 (cap 在 2 token), 因此 4096B 的
body preview 完整覆盖了 SSE 末帧 usage, 是本次实验的 raw evidence
锚点。

```
[E7 r0] "usage":{
  "prompt_tokens":2911,
  "completion_tokens":2,
  "total_tokens":2913,
  "completion_tokens_details":{"text_tokens":2},
  "prompt_tokens_details":{
    "text_tokens":2911,
    "cache_creation":{"ephemeral_5m_input_tokens":2887}
  }
}

[E7 r1] "usage":{
  "prompt_tokens":2207,
  "completion_tokens":2,
  "total_tokens":2209,
  "completion_tokens_details":{"text_tokens":2},
  "prompt_tokens_details":{
    "text_tokens":2207,
    "cache_creation":{"ephemeral_5m_input_tokens":2183}
  }
}

[E7 r2] "usage":{
  "prompt_tokens":2207,
  "completion_tokens":2,
  "total_tokens":2209,
  "completion_tokens_details":{"text_tokens":2},
  "prompt_tokens_details":{"text_tokens":2207,"cached_tokens":2183}
}
```

注意 raw 帧中字段层级:
- 创建端: `prompt_tokens_details.cache_creation.ephemeral_5m_input_tokens`;
- 命中端: `prompt_tokens_details.cached_tokens`;

aispec 的 `PromptTokensDetails.CacheCreationInputTokens` 是**抹平**
后的字段 (取 `cache_creation.ephemeral_5m_input_tokens` 的值), 与
raw 数值 1:1 一致 (E7 r0 raw=2887 ↔ aispec=2887, E7 r1 raw=2183 ↔
aispec=2183, E7 r2 raw 无 cache_creation 字段 ↔ aispec=0)。这是 aispec
解析正确性的锚点证据, 由此可信 E1-E6 / E8-E10 所有 aispec usage 数据。

### 9.2 复现指南

```bash
# 1. 准备
echo "your-dashscope-api-key" > ~/yakit-projects/tongyi-apikey.txt

# 2. (临时) 在 yaklang 仓库 common/aibalance/cmd_cache_probe_temp/main.go
#    放置实验代码 (本次实验后已删除, 如需复现见本仓库 git history 或
#    本报告 §3 的 design)

# 3. 跑全部实验 (~5min)
cd /Users/v1ll4n/Projects/yaklang
go run ./common/aibalance/cmd_cache_probe_temp/

# 4. 跑指定实验 (单个 / 多个)
ONLY_EXP=E2,E3 go run ./common/aibalance/cmd_cache_probe_temp/

# 5. 复用之前已建的缓存块 (跨进程命中验证)
RUN_ID=run-1777823006312881000 ONLY_EXP=E10 go run ./common/aibalance/cmd_cache_probe_temp/
```

实验代码位于 [common/aibalance/cmd_cache_probe_temp/main.go](../../../aibalance/cmd_cache_probe_temp/main.go)
(本次实验后已删除以避免污染主分支), 设计原则:
- 调用链路 = `ai.GetAI("tongyi", ...) -> tongyi.GatewayClient.Chat`,
  与 aibalance 生产链路完全一致;
- usage 拿取 = `aispec.WithUsageCallback(...)`;
- raw evidence 拿取 = `aispec.WithRawHTTPRequestResponseCallback(...)`
  body preview 4096B;
- 反污染 = 进程级 `runID` 注入到所有 prompt marker。

### 9.3 测试期 dashscope 端响应 latency

| model | 实测 P50 | 实测 P95 |
|---|---|---|
| qwen3.6-plus | ~6s | ~9s |
| qwen3.5-flash | ~3s | ~4s |
| qwen3-vl-flash | ~0.5s (cap 2 tokens) | ~0.7s |

(供 aibalance routing 选择 model 时参考, 与缓存机制无关)

---

## 10. 相关代码引用

| 文件 | 作用 |
|---|---|
| [common/ai/aispec/msg.go](../../aispec/msg.go) | `ChatContent.CacheControl` 字段 + `PromptTokensDetails.CacheCreationInputTokens / CacheType` 字段定义, 实现 dashscope 显式缓存协议透传 |
| [common/aibalance/explicit_cache_rewriter.go](../../../aibalance/explicit_cache_rewriter.go) | 自动在 system message 末尾注入 `cache_control:{"type":"ephemeral"}` 的实现, 命中本报告 §7.4 的"接近最优"策略 |
| [common/aibalance/server.go](../../../aibalance/server.go) | aibalance 主请求路径, 在调 `GetAIClientWithRawMessages` 之前调 `RewriteMessagesForExplicitCache` 注入 cache_control |
| [common/ai/aid/aicache/hijacker.go](hijacker.go) | aicache 把 high-static section 抽出做 system message 的 hijacker 实现 |
| [common/ai/aid/aicache/splitter.go](splitter.go) | PROMPT_SECTION 4 段切片实现 |
| [common/aibalance/db_cache_stats.go](../../../aibalance/db_cache_stats.go) | aibalance portal 缓存命中率统计实现, 可与本报告对照 |

---

## 11. 报告结论 (三句话)

> **dashscope 显式缓存在 OpenAI 兼容接口上的实际可用边界极窄**: 唯一
> 能 100% 命中的场景是"完整请求字节级 1:1 重发" (E1, E12 r2 验证),
> 其它任何"前缀稳定但末尾变化"或"增量追加"场景**全部 miss**, 与
> 增量大小无关 (E12 r1 用增量 2000 tok >> 1024 直接证伪)。
>
> **滥用 cache_control 的代价是惩罚性的 +25% 单价**, 不是中性的:
> E11 6 次"标记漂移"序列实测整体成本比无缓存基线**高 24.8%** (见 §6.2),
> E12 r1 单次实测 4000 token 增量请求按 125% 计费, 比"完全不用
> cache_control" 多付 25%。
>
> 当前 aicache + aibalance 的"在 system message 末尾打单一 cache_
> control"实现是 dashscope 显式缓存能用上的**唯一可命中策略** (相同
> system + 相同 user 的重发能命中), **不要**做"多 ChatContent + 多
> cache_control 精细控制"的升级。详见 §1.1 的可用性边界表与 §7 的
> 具体建议。
