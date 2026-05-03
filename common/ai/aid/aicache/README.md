# aicache

aicache 在 `aispec.ChatBase` 入口接管 mirror observer，承担两件事：

1. **缓存分析**：把 prompt 按 `PROMPT_SECTION` 外层标签切片，登记到全局缓存表，
   按字节级 LCP 计算前缀命中率，节流打印诊断行；DEBUG 模式下把每次 prompt
   完整落盘，便于事后定位"哪儿污染了缓存"。
2. **可选 messages 改写（hijack）**：当 prompt 中存在 high-static 段时，把它
   切出来单独包成 `<|AI_CACHE_SYSTEM_high-static|>...<|AI_CACHE_SYSTEM_END_high-static|>`，
   作为 `role:system` 单独消息发给上游 LLM；其余 prompt 内容继续作为
   `role:user` 消息发出。意图是用 role 边界让上游隐式缓存识别"系统级稳定
   内容"，提升命中概率。

> Mirror 与 hijack 共用一个注册表入口 `RegisterChatBaseMirrorObserver`，通过
> 返回值 `*ChatBaseMirrorResult{IsHijacked, Messages}` 区分两职。详见下文
> §3"Observe + Hijack 合一接口"。

---

## 1. PROMPT_SECTION 4 段切片：是什么、为什么这么切

业务侧（aireact / liteforge / aimem / ...）按 4 段框架渲染 prompt，外层用
`<|PROMPT_SECTION_<section>|>...<|PROMPT_SECTION_END_<section>|>` 包裹：

| 段 | 含义 | 跨调用稳定性 |
|---|---|---|
| `high-static` | 系统侧静态指令、Schema、Output Formatter 等"跨调用永远不变"的内容 | 极稳定 |
| `semi-dynamic` | 工具/forge 清单、Schema、PersistentMemory 等"短窗口稳定"的内容 | 一般稳定 |
| `timeline` | 历史时间线，按时间分桶；通常窗口移动一次才变 | 中等不稳定 |
| `dynamic` | 用户最新输入、AutoContext、注入的 memory 等"每次都变"的内容；外层带 nonce 防 prompt-injection | 极易变 |

dynamic 段使用 `<|PROMPT_SECTION_dynamic_<NONCE>|>...<|PROMPT_SECTION_dynamic_END_<NONCE>|>`
形态，以 nonce 隔离不同请求；其余 3 段不带 nonce，以保证哈希跨调用稳定。

**为什么这么切**：上游 LLM（阿里云百炼、OpenAI 等）对同一账号 + 同一模型
的请求做"字节级前缀缓存"。把"跨调用稳定的内容"集中到 prompt 头部，让最稳
定的 high-static 在最前面，能让上游缓存命中并按 20% 单价计费（隐式缓存的
账单折扣）。这是 aicache 切片器的核心约束。

---

## 2. 两层"缓存"

aicache 名字里带"cache"，但实际涉及两个不同的缓存层，不要混淆：

### 2.1 上游隐式缓存（Upstream Implicit Cache）

- 由模型供应商内部维护，aicache 触不到也改不了。
- 按 messages 数组的字节级前缀做最长公共前缀匹配。
- 5 分钟 TTL，**账号级 + 模型级强隔离**：跨账号跨模型缓存不共享。
- 命中部分按标准 input token 单价的 20% 计费（折扣 80%）。
- aicache 通过 hijack 把 high-static 搬到 `role:system`，企图让上游"看到"
  稳定的 system 段；同时通过观测帮助开发者定位"为什么前缀对不齐"。

### 2.2 本地分析表（Local Analysis Table）

- aicache 进程内的 `globalCache`（见 `cache.go`），保存最近 256 次 ChatBase
  调用的 chunk hash 序列。
- **不真实缓存任何 prompt 内容**，只做命中率测算与 advice 输出。
- LCP 算法：把当前请求的 chunk hash 序列与历史所有请求做最长公共前缀比对，
  取最大值作为本次"命中前缀长度"，按字节折算命中率。
- 输出去向：节流打印（`log.Infof`）+ DEBUG 模式落盘到
  `<YakitTemp>/aicache/<sessionId>/000NNN.txt`。

---

## 3. Observe + Hijack 合一接口

`aispec` 提供单一注册入口 `RegisterChatBaseMirrorObserver(fn)`，回调签名：

```go
type ChatBaseMirrorObserver func(model string, msg string) *ChatBaseMirrorResult

type ChatBaseMirrorResult struct {
    IsHijacked bool
    Messages   []ChatDetail
}
```

返回语义：

| 返回值 | 行为 |
|---|---|
| `nil` | 纯观测，ChatBase 走默认路径（msg 包成单 user 消息） |
| `&ChatBaseMirrorResult{IsHijacked: false}` | 同上 |
| `&ChatBaseMirrorResult{IsHijacked: true, Messages: [...]}` | ChatBase 把 Messages 写入 `ctx.RawMessages`，自动复用现有 RawMessages 透传链路 |

调度策略（`dispatchChatBaseMirror`）：

- **同步**调用所有已注册 observer。observer 自身慢操作（文件 I/O 等）应自
  己 `go` 出去；CPU 操作要保证够快，否则会拖慢 ChatBase 主流程。
- 多 observer 时取**最后一个 `IsHijacked==true`** 的结果。
- 任何 observer panic 通过 `recover` 隔离，不影响其他 observer 与主流程。

aicache 提供唯一一个 observer：`aicache.Observe`。它先做完整缓存分析，再
调用 `hijackHighStatic` 决定是否返回 hijack 结果：

```go
func Observe(model, msg string) *aispec.ChatBaseMirrorResult {
    if msg == "" {
        return nil
    }
    split := Split(msg)
    rep := gCache.Record(split, model)
    rep.Advices = buildAdvices(rep, split)
    gPrinter.Trigger(rep)
    utils.Debug(func() { go dumpDebug(rep, split, gCache) })
    return hijackHighStatic(msg)
}
```

### 3.1 Hijack 输出形态

```
[
  {role: "system", content: "<|AI_CACHE_SYSTEM_high-static|>\n...原 high-static 字节...\n<|AI_CACHE_SYSTEM_END_high-static|>"},
  {role: "user",   content: "<原 prompt 去掉 high-static 段后剩余的 semi-dynamic + timeline + dynamic>"}
]
```

要点：

- system 包装层 `<|AI_CACHE_SYSTEM_high-static|>` 是 AITAG 兼容的，aicache
  自身后续仍可用 `aitag.SplitViaTAG` 重新识别。
- user 中保留所有非 high-static 段的原 PROMPT_SECTION 标签，aibalance flatten
  后字节序仍然稳定。
- 当 prompt 中没有 high-static 段、或剥离 high-static 后 user 内容为空时，
  hijack 返回 `nil`，ChatBase 走默认路径。

---

## 4. 如何使用

### 4.1 自动接入

```go
import (
    _ "github.com/yaklang/yaklang/common/ai/aid/aicache" // 副作用 import
)
```

`init()` 把 `Observe` 注册到 aispec mirror observer 链；任何 `aispec.ChatBase`
调用都会自动经过缓存分析与可选 hijack。

### 4.2 节流日志

aicache 把每次请求的 hit report 节流为单行 INFO 日志：

```
[aicache] reqs=42 model=memfit-light-free chunks=4 prefix_hit=2/4(64.5%) bytes=12345/19000 cache_uniq=87 cache_bytes=210000 advice="hit ratio fair (64.5%) - room for improvement"
```

最低打印间隔由 `printer.go:minPrintInterval`（默认 3s）控制，避免高 QPS 场
景刷屏。

### 4.3 DEBUG 落盘

设置环境变量启用 yaklang debug 模式后（`utils.InDebugMode() == true`），每
次 ChatBase 调用都会把完整 prompt + section 元数据 + hit report + advices
写到：

```
<YakitTemp>/aicache/<sessionId>/<seqId>.txt
```

dump 文件可作为本仓库 testdata fixtures 的来源；详见 `testdata/fixtures/README.md`。

### 4.4 关闭/隔离 hijack（测试场景）

```go
import "github.com/yaklang/yaklang/common/ai/aispec"

// 清空所有已注册 observer（包含 aicache.Observe），后续 ChatBase 走默认路径
aispec.ResetChatBaseMirrorObserversForTest()
```

仅供测试使用；生产代码不要调用。

---

## 5. 已知边界

### 5.1 aibalance 中转会拍平 system+user

`common/aibalance/server.go:serveChatCompletions` 把多角色 messages 按顺序拼
成单条 user 消息再转发。aicache hijack 出来的 `[system, user]` 经 aibalance
后会变成单条 user，失去 role 边界，但**字节序仍然稳定**——aicache 在 system
段做的 AI_CACHE_SYSTEM 包装不会因 aibalance 拍平而变化。

详见本仓库 `common/aibalance/README.md`（如已撰写）或 `aibalance/server.go`
注释里的"messages 拍平"小节。

### 5.2 多模态路径目前不 hijack

当 caller 提供 `WithChatBase_ImageRawInstance` / `WithChatBase_VideoRawInstance`
时，`chatBaseChatCompletions` 会走多模态拼装分支（带 image_url / video_url
的 user content 数组），不进入 RawMessages 透传通道。当前 hijack 只接管纯
文本 prompt 路径；多模态优化留待后续。

### 5.3 caller 显式 RawMessages 优先

当 caller 通过 `WithChatBase_RawMessages` 显式提供了 messages 数组时，aicache
hijack 自动跳过——尊重 caller 已构造好的 messages 结构，不二次猜测。

### 5.4 dynamic nonce 不进 hash

`PROMPT_SECTION_dynamic_<NONCE>` 段的内层 nonce 不参与 chunk hash 计算（详见
`splitter.go:classifyTagged`）。Section 名 `dynamic` 是稳定的，每次只是
nonce 在变；hash 源只取 `Section + "|" + Content`，所以同一逻辑请求的 dynamic
hash 也能稳定。

---

## 6. testdata fixtures

`testdata/fixtures/*.txt` 是从生产环境采下来的真实 dump，作为单元测试输入。
详见 [testdata/fixtures/README.md](testdata/fixtures/README.md)：

| 文件 | chunks | 关注点 |
|---|---|---|
| `000001.txt` | 3 | 无 timeline 的 LiteForge 早期形态 |
| `000005.txt` | 4 | 中等规模完整 4 段，hijacker 主路径 |
| `000010.txt` | 4 | 大 high-static + 大 semi-dynamic |
| `000020.txt` | 4 | dynamic 漂移、其他段稳定 |
| `000040.txt` | 4 | 中段调用，全段都已漂移过 |
| `000045.txt` | 1 | raw / 无 PROMPT_SECTION，hijacker 必须透传 |
| `000060.txt` | 4 | high-static seen=7 / semi-dynamic seen=6 真实命中 |
| `000073.txt` | 4 | 大 prompt + 部分前缀命中（16.2%） |

测试 helper `loadFixtureRawPrompt` 把 dump 头部解析出来，方便测试断言"Split
结果与 dump 头部声明一致"。

---

## 7. 历史评估报告

aicache 上线前的隐式缓存评估、aibalance 中转链路分析、亲和性路由方案等历
史报告在 git history 中。本 README 只描述当前最新形态。

旧版评估报告里关键结论摘要（仅供回顾，不再详述）：

- 多 provider 随机轮询会让上游缓存命中上限降到 1/N（已通过 aibalance 亲和
  性路由解决）。
- aibalance 把多角色 messages 拍平为单 user 是设计性选择，会让显式缓存能
  力受限，但隐式缓存仍能用前缀稳定性受益。
- `EnableThinkingField` 路径会让 JSON 字段顺序在 struct 序与字典序间切换；
  跨 provider 字节序不同但本来就不共享缓存，影响极小。

后续优化方向（按优先级）：

1. aibalance 内部缓存观测（解析 `prompt_tokens_details.cached_tokens`，按
   `(model, providerID)` 暴露统计）—— 高价值，纯观测。
2. messages 多角色透传（aibalance 不再 flatten，让显式缓存解锁）—— 中价值，
   接口面较大。
3. semi-dynamic 也搬到 system 第二条（与本次 high-static 同思路）—— 数据驱
   动决策。
