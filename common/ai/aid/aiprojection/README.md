# aiprojection

`aiprojection` 负责把已渲染的 prompt 投影成 Provider 可见的消息，并解析同一份
prompt 的缓存切片。外部 Go 代码统一导入
`github.com/yaklang/yaklang/common/ai/aid/aiprojection`。

## 从完整字符串开始

`Parse` 读取外层 AITAG 一次，产出投影和缓存观察共用的解析结果；`Project`
消费该结果并生成请求消息。以下代码可直接处理一整段 prompt：

```go
package main

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

func main() {
	prompt := `<|AI_CACHE_SYSTEM_high-static|>稳定的系统指令<|AI_CACHE_SYSTEM_END_high-static|>
<|PROMPT_SECTION_dynamic_request1|>本次用户问题<|PROMPT_SECTION_dynamic_END_request1|>`

	parsed := aiprojection.Parse(prompt)
	split := parsed.CacheSplit()
	for _, chunk := range split.Chunks {
		fmt.Printf("section=%s nonce=%s bytes=%d hash=%s\n",
			chunk.Section, chunk.Nonce, chunk.Bytes, chunk.Hash)
	}

	result := aiprojection.Project(aiprojection.ProjectionInput{Sections: parsed.Sections()})
	fmt.Printf("cache projected: %v\n", result.Metadata.CacheProjected)
	for _, message := range result.Messages {
		fmt.Printf("role=%s content=%v\n", message.Role, message.Content)
	}
}
```

这个例子产生 `high-static` 和 `dynamic` 两个切片，并投影成 `system` 与
`user` 两条消息。`ChatDetail.Content` 可能是字符串，也可能是带
`cache_control` 的内容数组；调用方应直接把消息传给现有请求序列化路径。

## 实际发送链路

普通 AI 调用会在 `aispec.ChatBase` 发送前触发
`aiprojection.ProjectAndObserve`：

```text
原始 prompt
  → Parse（一次外层 AITAG 解析；旧 timeline 格式按需解析内层标签）
  → 缓存观察 + Project（消费 Parse 产出的 ProjectionSections）
  → ChatBase 将投影消息写入 RawMessages
  → chat-completions 的 messages / responses 的 input
  → HTTP 请求
```

这个发送前入口注册在 `aispec.RegisterChatBaseHijackHook`。投影需要修改
消息时返回 `ChatBaseHijackResult{IsHijacked: true, Messages: ...}`；
`ChatBase` 才会以这些消息发送。没有可投影的缓存边界时，hook 只记录统计，
`ChatBase` 保持原来的请求消息。调用方明确设置 `RawMessages` 时优先使用
调用方的消息。

直接调用 `Project` 只计算结果，不会自动发请求；需要自行将
`result.Messages` 传给 `aispec.WithChatBase_RawMessages`，并将
`result.Tools` 传给 `aispec.WithChatBase_Tools`。当前自动 hook 只投影消息，
业务方已选定的工具仍由 `ChatBaseContext.Tools` 透传。

## Split 的解析规则

- 识别 `AI_CACHE_SYSTEM_high-static`、旧版
  `PROMPT_SECTION_high-static`、其他 `PROMPT_SECTION_<section>`，以及
  `PROMPT_SECTION_dynamic_<nonce>`。标签中的正文成为 `Chunk.Content`。
- `Chunks` 保持外层标签的出现顺序。`Section` 是类别；动态段的 `Nonce`
  包含本次请求的 nonce，但其 `Hash` 只计算
  `sha256(Section + "|" + Content)`，因此 nonce 变化不会单独造成 hash 漂移。
- 存在可解析的外层标签时，标签间的普通文本不计入 `Chunks`；它仍保留在
  `Project` 的请求消息中。没有标签或解析失败时，整段字符串是一个 `raw`
  切片。`Bytes` 按 UTF-8 字节计数。

`Split(prompt)` 是只需要缓存切片时的兼容快捷调用，等价于
`Parse(prompt).CacheSplit()`。它不生成 Provider 消息，也不会更新全局
统计。需要发送完整 prompt 时，应把 `Parse` 的结果交给 `Project`。

## Project 的输入和边界

`Parse(prompt).Sections()` 返回 `ProjectionSections`：`Items` 保留外层片段的
原始顺序与字节，缓存边界候选也在其中生成。cache policy 只消费这份投影
sections，不读取 `CacheSplit()` 的观察结果。未提供 `Sections` 时，`Project`
会解析 `Prompt`。手动提供 `Sections.Items` 时，`Project` 按顺序拼接每项
`Raw` 后统一解析；`Sections` 优先于 `Prompt`。`ProjectionSection.Kind`
仅描述片段用途，未知 kind 仍作为普通文本保留。

```go
result := aiprojection.Project(aiprojection.ProjectionInput{
	Sections: &aiprojection.ProjectionSections{Items: []aiprojection.ProjectionSection{
		{Kind: aiprojection.ProjectionSectionCache, Raw: staticBlock},
		{Kind: aiprojection.ProjectionSectionTimeline, Raw: timelineBlock},
	}},
	ActionTools: selectedTools,
})
messages, tools := result.Messages, result.Tools
```

`ActionTools` 是调用方已选定的工具定义，按原顺序透传。若已构造
`RawMessages`，它们优先于 `Prompt` 和 `Sections`，不会再次拆分。
`Project` 不选择工具、不执行 action，也不更新缓存统计。

`aiprojection` 只改变发给 Provider 的消息布局和缓存标记，不改变业务语义；
它不决定 finish、tool、skill 的权限，也不成为业务状态源。调用方仍负责
选择可用工具、执行结果与对话状态。旧 `AI_CACHE_*` 标签、`Split` 输出、
缓存控制位置及推理回放在兼容测试中锁定；脚本和 dump 的对外名称保持兼容。

有 `high-static` 且仍有用户内容时，缓存投影把稳定段放入 `system` 消息。
完整的 frozen、semi 和 semi2 边界可进一步把用户内容拆成 3、4 或 5 条
消息，并在适用的稳定前缀上设置 `cache_control`；边界缺失或残缺时退回
2 条消息。无法确认可切分的 `high-static` 时，原文作为单条 `user` 消息。
未知 AITAG 原样保留。

稳定前缀的正文应跨请求保持相同字节。修改 `high-static` 模板时，不要在普通散文
中写具体的 action enum 字面量；需要列出 enum 时放进对应角色的 schema 块，
否则基于字面量的 prompt 匹配可能误判任务类型。

## 自动观测与调试

`aiprojection` 在包初始化时注册 `ProjectAndObserve`。现有 AI 调用
自动经过这个 hook；通常无需手动调用。返回的 `CorrelationID` 会写入
usage 回调的 `MirrorCorrelationID`，用于对齐调试 dump。

设置 `YAKLANGDEBUG=1` 后，观测记录异步写入
`<YakitTemp>/aicache/<sessionId>/000001.txt` 等文件。外部 Go 代码可用
`aiprojection.SessionDir()` 获取本进程的目录；Yak 脚本继续使用
`ai.aicacheSession()`。这些对外名称和 dump 格式保持兼容。

离线检查 dump 可运行：

```sh
go run common/yak/cmd/yak.go \
  common/ai/aid/aiprojection/cachebench/analyze.yak \
  --session-dir /path/to/aicache/session \
  --output-dir /tmp/cachebench-report
```

`cachebench/selftest.yak` 验证 dump 与 usage 的关联逻辑；
`cachebench/run_react.yak` 用真实模型运行 ReAct 并生成缓存报告。
