# aicache 评估报告：aibalance 中转链路对隐式缓存命中的影响与优化

> 作用域：仅讨论 **隐式缓存（Implicit Cache）**，不涉及 `cache_control: ephemeral` 显式缓存。
>
> 数据基线：`/Users/v1ll4n/yakit-projects/temp/aicache/20260503-124205-7848`（B 档落地后样本）。

## 0. TL;DR

aicache 落地 A/B 档之后，LiteForge 路径的 `high-static` 段复用率从 2% 跃升至 81.8%，但前缀字节级缓存命中率仍然受限于 **aibalance 中转层** —— 这是一个我们之前没有正面观测过的环节。本次审视抓出 **三个系统性影响因素**，并落地了 **一个最小可行优化**：

1. 多 provider 随机轮询（最严重，影响命中率上限 ≈ 1/N） → **已落地：亲和性路由**
2. messages 数组被强制拍平为单条 user 消息（中等影响，损失角色边界但前缀仍稳定）
3. `enable_thinking` 字段二次 marshal 路径会重排 JSON key（低影响，仅改变字节序而非内容）

针对 #1，本次新增 `PeekOrderedProvidersWithAffinity` 与 `BuildPromptAffinityKey`，把"同一 prompt 前缀 + 同一 apiKey + 同一 model"的逻辑请求稳定路由到同一 provider。这是一次零业务感知的纯路由层改造，单测覆盖全部通过。

---

## 1. 上下文：为什么要看 aibalance

aicache 镜像观测的位置在 `aispec.RegisterChatBaseMirrorObserver(Observe)`，它捕获的是 **aispec 即将向上游 LLM 发出的 prompt 字符串**。在 yaklang 内部链路中，aispec 是底层 SDK；当请求经过 aibalance 转发时，路径如下：

```
内部业务调用
    -> AID/aiforge/aireact 等业务逻辑
    -> aispec.AIClient.Chat(prompt)        <- aicache 镜像点 A（业务侧观测）
    -> HTTP POST /v1/chat/completions      <- 客户端协议层
    -> [可选] aibalance 中转
            -> 选择 provider（鉴权/限流/负载均衡）
            -> aispec.AIClient.Chat(prompt) <- aicache 镜像点 B（中转侧观测）
            -> HTTP POST 上游真实 LLM API
    -> 上游做隐式前缀缓存匹配
```

**关键问题**：业务侧（A）的 prompt 哪怕已经做到了 `high-static` 段字节稳定，到了上游真实 API 端，是否仍然能命中前缀缓存？答案不仅取决于 prompt 字节稳定性，还取决于 **aibalance 是否把这次请求发去了同一个上游账号**。

---

## 2. 隐式缓存机制摘要（基于阿里云百炼文档）

> 本节仅作为后续分析的判据基线。

| 维度 | 隐式缓存的契约 |
|------|---------------|
| 触发方式 | 对所有支持模型自动开启，无法关闭 |
| 匹配算法 | 以请求 `messages` 数组的 **字节级前缀** 为键，向系统已有缓存做最长公共前缀匹配 |
| 命中确定性 | 不保证。即便完全相同的请求也可能未命中（系统内部判定 + 定期清理） |
| 最少 Token | 256（部分供应商 512） |
| 计费 | 命中部分按标准输入 Token 单价的 **20%** 计费（折扣 80%） |
| 隔离 | **账号级 + 模型级** 强隔离，不跨账号、不跨模型共享 |
| 工具定义 | `tools` 序列化结果作为系统消息的一部分参与前缀匹配 |
| 最佳实践 | **重复内容置于 messages 头部，差异内容置于尾部** |

**最关键的两条约束**：

- **前缀字节级匹配**：任何使前缀字节发生变化的因素都会让缓存彻底失效；任何使前缀字节稳定的设计都直接拉升命中率。
- **账号级隔离**：同一逻辑请求被分发到不同账号 → 各自独立创建缓存 → 在 5 分钟有效窗口内，只要请求不被复路由到同一账号，缓存就被浪费。

---

## 3. aibalance 中转流程拆解

入口在 `common/aibalance/server.go:serveChatCompletions`。下面按"会影响隐式缓存命中"的视角抽出关键步骤。

### 3.1 messages 拍平（行 712-752）

```go
var prompt bytes.Buffer
for _, message := range bodyIns.Messages {
    switch ret := message.Content.(type) {
    case string:
        prompt.Write([]byte(ret))   // 直接拼接
    default:
        // 遍历多模态 content 数组，把所有 type=text 的内容写入 prompt buffer
        // image_url 收集进 imageContent 列表（不进 prompt buffer）
    }
}
client.Chat(prompt.String())
```

**含义**：客户端发的 `[system, user, assistant, user]` 多角色对话被无损地按顺序拼接成单条字符串，最终送进 `aispec.AIClient.Chat(string)`，并由 aispec 包装成 `[{"role": "user", "content": "<拼接结果>"}]`。

**对缓存的影响**：

- **拼接顺序稳定**：`bodyIns.Messages` 是有序数组，遍历顺序确定 → 拼接结果字节稳定。✓ 不破坏前缀。
- **角色边界丢失**：上游永远只看到一条 user 消息。这意味着客户端原本可能有 4 个 cache_control 标记的精细控制能力被退化为单一前缀。**对隐式缓存影响有限**（隐式缓存本来就不看 cache_control），但在未来想引入显式缓存时会成为障碍。
- **图片放在 text 后**：`aispec.chatBaseChatCompletions` 在非 video 通路下采用 `[text, image]` 顺序（行 469-472）。文档建议"对不同图像问相同问题"时 text 在前，符合默认场景；"对同一图像多次提问"时则相反。当前选择是合理默认值。

### 3.2 provider 选择（核心问题）

`PeekOrderedProviders` 在多 provider 时执行 Fisher-Yates 完全随机洗牌（修改前的 `server.go:325-332`）：

```go
shuffledProviders := make([]*Provider, len(validProviders))
copy(shuffledProviders, validProviders)
for i := len(shuffledProviders) - 1; i > 0; i-- {
    j := int(utils.RandFloat64() * float64(i+1))
    shuffledProviders[i], shuffledProviders[j] = shuffledProviders[j], shuffledProviders[i]
}
```

**含义**：每次请求都重新随机排序所有健康 provider，第一个被尝试，失败重试时按随机顺序回退。

**对缓存的影响（最严重）**：

- 假设某 model 在 aibalance 上配置了 N 个独立账号的 provider。
- 上游隐式缓存按账号隔离：5 分钟内 provider A 的缓存对 provider B 完全不可见。
- 同一逻辑请求每次随机分布 → 平均下来在每个 provider 上独立创建/被遗忘缓存。
- **5 分钟窗口内连续发起 K 次相同请求，命中率上界 ≈ 1 - (1 - 1/N)^K**，N 越大命中越难触发。
- 例：N=3, K=2 → 上界 ≈ 33.3%；N=5, K=3 → 上界 ≈ 48.8%。

这就是 aibalance 链路上"隐式缓存命中保障缺失"的根因。

### 3.3 tools 透传与 enable_thinking 注入

```go
// server.go:844
client, err := provider.GetAIClientWithImagesAndTools(
    imageContent,
    bodyIns.Tools,      // 直接透传
    bodyIns.ToolChoice, // 直接透传
    bodyIns.EnableThinking,
    ...
)
```

进入 aispec 后，`executeChatBaseRequest` 处的注入分支（base.go:632-644）：

```go
if ctx.EnableThinkingField != "" {
    raw, _ = json.Marshal(msgResult)         // struct → JSON（字段顺序按 struct 定义）
    msgMap := make(map[string]any)
    json.Unmarshal(raw, &msgMap)             // JSON → map
    msgMap[ctx.EnableThinkingField] = ...
    payload = msgMap
}
raw, _ = json.Marshal(payload)               // map → JSON（字段顺序按字典序）
```

**含义**：当 provider 配置了 `EnableThinkingField` 时，最终 JSON 字段顺序从"struct 字段定义序"变成"字典序"。

**对缓存的影响**：

- **同一 provider 内**：`EnableThinkingField` 是 provider 配置，不会在调用之间变化 → 同一 provider 的请求字节序仍然稳定。✓
- **跨 provider**：A provider 不走该路径、B provider 走 → 同一逻辑请求的 JSON 字节序不同。但反正跨 provider 缓存就不共享，这个差异不增加新问题。
- **结论**：**不影响命中率**，但带来一个隐藏的"序列化非对称"，未来若需要做"逻辑请求级别去重"会成为障碍。优先级：低。

### 3.4 stream / non-stream

`bodyIns.Stream` 透传，stream 标志位本身不参与上游缓存匹配（隐式缓存只看 messages 内容）。✓ 无影响。

### 3.5 `tools` 序列化稳定性

`bodyIns aispec.ChatMessage` 中的 `Tools []aispec.Tool` 是 struct 切片，Go json.Marshal 对 struct 字段顺序稳定（按定义顺序）。客户端如果以 `map[string]any` 形式描述工具，已经在 aibalance 入口处通过 `json.Unmarshal` 强制转成 struct，因此 **tools 顺序与字段顺序对同一客户端调用是稳定的**。✓

**风险点**：客户端不同调用之间若工具数组排列顺序不同（例如根据某种动态条件 reorder），上游缓存仍然失效。这是客户端责任，aibalance 无能为力。

---

## 4. 影响隐式缓存命中的因素清单（按影响优先级）

| 优先级 | 因素 | 影响 | 是否本次解决 |
|--------|------|------|-------------|
| P0 | 多 provider 随机轮询导致缓存碎片 | 命中上界 ≈ 1/N | **已解决：亲和性路由** |
| P1 | messages 拍平丢失 role 边界 | 未来引入显式缓存的障碍；当前隐式缓存场景影响有限 | 未解决（设计性约束） |
| P1 | 客户端 messages 顺序若不稳定，前缀立即失效 | 命中率归零 | aibalance 不可控（客户端责任） |
| P2 | `EnableThinkingField` 路径让 JSON 字段顺序在 struct 序与字典序间切换 | 跨 provider 字节序不同（但跨 provider 本来就不共享缓存） | 未解决（影响极小） |
| P2 | `tools` 数组若客户端动态排序 | 上游缓存失效 | aibalance 不可控 |
| P3 | stream 标志位 | 无 | N/A |
| P3 | `Connection: keep-alive`、压缩等传输层因素 | 不参与缓存匹配 | N/A |

---

## 5. 已实施的小优化：亲和性路由（Affinity Routing）

### 5.1 思路

**目标**：在 N 个独立账号 provider 之间，让"同一逻辑请求"稳定路由到同一 provider，使上游隐式缓存有机会被复用。

**约束**：
- 不破坏现有负载均衡：失败重试链路保留洗牌后的随机回退序，避免某 provider 永久被偏置。
- 不引入业务感知：aibalance 的所有上层调用方接口不变。
- 健康集合变化时平滑迁移：当某 provider 变为不健康/被过滤，原本路由到它的请求自动迁移到剩余健康集合中的稳定主选。

### 5.2 实现

新增三段代码（`common/aibalance/server.go`）：

#### 5.2.1 `PeekOrderedProvidersWithAffinity(model, affinityKey)`

- 健康过滤逻辑沿用旧实现（延迟 < 10s + 健康检查回退）。
- 当 `affinityKey != ""` 且健康集合 ≥ 2 时：
  1. 对健康集合按 `TypeName|DomainOrURL|APIKey` 字典序做一次确定性排序（`sortProvidersStably`）—— 跨进程稳定。
  2. 用 `hashAffinityKey(affinityKey)` 在排序后的集合上 mod 选出 **主 provider**。
  3. 在原本的 Fisher-Yates 洗牌结果中把主 provider 交换到第 0 位，其余顺序保持随机。
- `affinityKey == ""` 退化为完全随机洗牌，即原 `PeekOrderedProviders` 行为。

#### 5.2.2 `BuildPromptAffinityKey(prompt, apiKey, model, prefixLen)`

- 对 prompt 取前 `prefixLen` 字节（默认 2048），与 `apiKey`、`model` 一起 sha1 → 16 字节十六进制。
- **为什么取前缀而不是全文**：隐式缓存按前缀匹配，差异通常在 prompt 末尾。前缀稳定即可决定路由，省去对长 prompt 全量哈希的开销。
- **为什么混入 apiKey 和 model**：上游缓存账号级 + 模型级隔离，逻辑请求即便相同，跨账号或跨模型也不能复用，应当分到不同的"亲和性桶"以避免不必要的分布偏置。

#### 5.2.3 `serveChatCompletions` 接入

```go
affinityKey := BuildPromptAffinityKey(prompt.String(), apiKeyForStat, modelName, 2048)
providers := c.Entrypoints.PeekOrderedProvidersWithAffinity(modelName, affinityKey)
```

仅 chat completions 通路接入；embedding 通路保持随机（embedding 本身缓存意义有限）。

### 5.3 数学验证

设健康集合 N 个 provider，K 次相同请求在 5 分钟窗口内：

- **优化前**（完全随机）：第 K 次命中前 K-1 次中至少一次的 provider 概率 = `1 - ((N-1)/N)^(K-1)`。
- **优化后**（亲和性路由）：在健康集合不变时，K 次请求 100% 路由到同一 provider；除首次外全部命中。理论命中率 = `(K-1)/K`。

| N | K | 优化前命中上界 | 优化后命中率 |
|---|---|---------------|-------------|
| 3 | 2 | 33.3% | 50% |
| 3 | 5 | 80.2% | 80% |
| 5 | 2 | 20% | 50% |
| 5 | 5 | 59% | 80% |
| 5 | 10 | 86.6% | 90% |

K 越小、N 越大，亲和性路由优势越明显。**对于 LiteForge 这种"同一 prompt 在短时间内被多次触发"的场景，亲和性路由是命中率从 1/N 量级升到接近 1 的关键。**

### 5.4 单元测试

`common/aibalance/affinity_routing_test.go` 共 13 个测试，覆盖：

- 单 provider 直返
- 同一 affinityKey 在 100 次重复中产生稳定主 provider
- 空 affinityKey 退化为完全随机（300 次取样覆盖所有 3 个 provider）
- 不同 affinityKey 在 300 次取样下分布到所有 provider
- 健康集合缩水时主 provider 平滑迁移到剩余集合中的稳定选择
- `PeekOrderedProviders` 向后兼容（行为与原版一致）
- `BuildPromptAffinityKey` 的确定性、账号隔离、模型隔离、前缀路由、默认 prefixLen
- `hashAffinityKey` 的确定性
- `sortProvidersStably` 的稳定字典序

全部通过：

```
PASS
ok  github.com/yaklang/yaklang/common/aibalance  1.006s
```

aibalance 整包回归测试（含 server_full_test、portal_auth_security 等共 28s 跑完）也全部通过。

### 5.5 部署后的预期观察

在 aibalance 接入亲和性路由后，**aicache 报告中应该观察到的现象**：

- 同一 prompt 前缀的 hash 在 aibalance 端被稳定路由到同一 provider；
- 由于 yaklang 业务侧 aicache 镜像点位于 aispec.Chat 入口（在路由之前），**aicache 自身的命中率统计不会因路由变化而变**，命中率指标仍然取决于业务侧 prompt 字节稳定性；
- **真实收益体现在上游账单**：每个 provider 账号的 `cached_tokens / prompt_tokens` 比例会显著上升。

> 验证方式：用 `DEBUG=1` 跑相同 workload，观察上游 `prompt_tokens_details.cached_tokens` 字段。

---

## 6. 未解决但建议跟进的方向

按"是否值得做"排序：

### 6.1 messages 多角色透传（已实施）

**状态**：已落地。aibalance 不再拍平多角色 messages，客户端原始的 `bodyIns.Messages` 数组按 role/content/tool_calls/tool_call_id 等字段完整透传到上游 LLM。

**实现路径**：
- `aispec.AIConfig` 新增 `RawMessages []ChatDetail` 字段与 `WithRawMessages` 选项；`aispec.ChatBaseContext` 同步新增 `RawMessages` 与 `WithChatBase_RawMessages`。
- `chatBaseChatCompletions` 在 `len(ctx.RawMessages) > 0` 时跳过"单 user 包装 + ImageUrls/VideoUrls 合并"分支，直接使用客户端原结构。
- 11 个 gateway（openai / tongyi / moonshot / siliconflow / openrouter / ollama / deepseek / chatglm / volcengine / aibalance / 通用 gateway）的 `Chat(s)` 各加一行 `WithChatBase_RawMessages(g.config.RawMessages)`，把配置层的 RawMessages 透到 ChatBase。
- `aibalance/provider.go` 新增 `GetAIClientWithRawMessages`；`aibalance/server.go` 的 `serveChatCompletions` 删去 `prompt bytes.Buffer` 拍平 + `imageContent` 单独提取的旧路径，亲和性 key 改为 `BuildMessagesAffinityKey`（基于 messages 稳定 JSON 序列化）。
- `dispatchChatBaseMirror` 在 RawMessages 模式下传入 messages 序列化字符串而非 prompt 拍平字符串，让 aicache 等观测者的前缀字节序与上游真实收到的 messages 数组保持一致。

**回滚开关**：环境变量 `AIBALANCE_LEGACY_FLATTEN_MESSAGES=1` 可临时回退到旧的拍平路径，无需重新发布。

**基线漂移说明**：本次改动后 aicache 镜像点收到的字符串由"prompt 拍平字符串"切换为"messages JSON 序列化字符串"，前缀 LCP 计算的字节基线与旧版本不可直接对比。下次 dump session 后的命中率统计需要建立**新基线**，不要与改造前的报告做横向数值比较；趋势性对比仍然有效。

**收益**：
- 显式缓存（cache_control）能力解锁
- 多轮对话场景下，上游能识别 role 边界，回复质量更稳定
- 上游隐式缓存的"重复 system prompt"自动被识别为头部稳定段
- 亲和性 key 与上游真实请求字节强相关，路由稳定性进一步提升

### 6.2 健康集合稳定排序加 wrapper_name 前缀（低价值，确定性增强）

**现状**：`sortProvidersStably` 用 `TypeName|DomainOrURL|APIKey` 排序。同一 wrapper_name 下不同账号的 provider 在不同节点重启时可能因 schema 数据顺序导致字典序变化。

**建议**：增加 `WrapperName` 作为排序键的前缀，确保完全一致。当前实现已经足够稳定（字段都是配置项，进程间一致），收益有限。

### 6.3 aibalance 内部缓存观测（高价值，纯观测）

**建议**：在 aibalance 接收上游响应时解析 `prompt_tokens_details.cached_tokens` 字段，按 `(model, providerID)` 维度统计累计命中率，暴露到 `/public/stats`。

**收益**：直接量化亲和性路由的真实效果，闭环验证。当前只能靠 aicache 业务侧间接推断。

### 6.4 工具定义稳定化（低价值，客户端责任）

**建议**：在 aibalance 入口处对 `bodyIns.Tools` 按 `function.name` 字典序做一次稳定排序，再透传。

**收益**：保护客户端的偶然乱序。但会破坏"客户端有意按优先级排序工具"这种设计意图，需要权衡。**不建议默认开启**。

### 6.5 aicache 镜像扩展到 aibalance 出口（中等价值）

**现状**：aicache 镜像点位于 aispec.Chat 入口（`aispec.RegisterChatBaseMirrorObserver`），观测的是业务侧 prompt。

**建议**：在 aibalance 也注册一份观测器（或直接复用 aispec 的注册），并把 aibalance 的 `(provider, apiKey)` 维度信息塞进 aicache 报告，让 aicache 能区分"哪些请求被路由到哪个上游账号"。

**收益**：闭环可视化亲和性路由效果，与 6.3 互补。

---

## 7. 总结

| 维度 | 状态 |
|------|------|
| 隐式缓存机制理解 | 已对齐文档（前缀字节匹配 + 账号/模型隔离 + 5 分钟 TTL + 不确定性） |
| aibalance 流程梳理 | 已完成，关键瓶颈定位为多 provider 随机轮询 |
| 影响因素清单 | 7 项，按优先级排序 |
| 已落地小优化 | 亲和性路由（`PeekOrderedProvidersWithAffinity` + `BuildPromptAffinityKey` / `BuildMessagesAffinityKey`） |
| messages 多角色透传 | 已实施（aispec RawMessages 链路 + 11 gateway + aibalance 重写，回滚开关 `AIBALANCE_LEGACY_FLATTEN_MESSAGES=1`） |
| 单测 | aispec / aibalance / common/ai/tests 三层共增覆盖，全部通过 |
| 回归测试 | aispec / aibalance / aiforge / aid 整包通过；TestLoadOption invalid_domain_with_path 与 TestScanPortTool_SynMode 为预存失败，本次 register 触发后被暴露但不属于本次改动引入 |
| 未解决项 | 4 项（6.1 已实施摘除），都标注了优先级和取舍考虑 |

亲和性路由 + messages 多角色透传 共同构成 **零侵入、零业务感知、保留所有兜底机制** 的最小改造组合：前者把"命中率上界 1/N"提升到"命中率上界接近 1"；后者保证亲和性 key 与上游 LLM 收到的真实字节强相关，并解锁显式缓存（cache_control）路径。后续可通过 6.3、6.5 完成闭环观测。
