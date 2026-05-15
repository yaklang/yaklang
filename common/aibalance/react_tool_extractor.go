package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// react_tool_extractor.go 实现流式 ReAct -> tool_calls 反解析器.
//
// 关键词: aibalance ReactToolExtractor, react -> tool_calls 反解析, 流式状态机
//
// 协议 (与 round1_react_inject.go / FlattenToolCallsForRoundTrip 严格一致):
//   - 工具调用文本格式: [tool_call name=NAME]ARGS_JSON[/tool_call]
//   - 工具结果文本格式: [tool_result tool_call_id=ID]CONTENT[/tool_result]
//     (extractor 仅解析 tool_call, tool_result 是 round2 -> 上游方向)
//
// 输入: 上游 wrapper 写过来的 content 字节流 (流式 chunk by chunk).
// 输出:
//   - 普通 content 文本 -> OnContent 回调, 按 OpenAI delta.content 透传给客户端
//   - 完整 [tool_call ...][/tool_call] -> OnToolCall 回调, 按 OpenAI delta.tool_calls 透传给客户端
//
// 设计要点:
//  1. 跨 chunk 边界缓冲: buffer 累积所有未消费字节, 反复尝试匹配完整 tool_call.
//  2. 容错坏 JSON: 解析失败的内容仍作为 text emit 给 OnContent, 避免误吞普通文本.
//  3. 并行 multi tool_call: 一次 Flush 可 emit 多个独立 tool_call (index 递增).
//  4. partial prefix 保留: buffer 尾部若可能是 "[tool_call" 的前缀, 暂不 emit, 等下次 Write.
//  5. 并发安全: 所有公开方法持单锁, 与 writerWrapper.Write 在同一线程也是安全的.

const (
	// extractorBufferLimit 是单条工具调用文本的最长允许长度.
	// 超过该长度仍未匹配到 [/tool_call] 则视为坏数据, 兜底当成 text emit.
	// 关键词: extractorBufferLimit, react 防超长溢出
	extractorBufferLimit = 64 * 1024
)

// ReactToolExtractor 流式解析 [tool_call name=...]args[/tool_call] 文本 -> 结构化 tool_calls.
// 关键词: ReactToolExtractor, react -> tool_calls 流式状态机
type ReactToolExtractor struct {
	mu sync.Mutex

	// buf 累积所有未消费的输入字节
	buf bytes.Buffer

	// nextToolCallIndex 用来生成新 tool_call 的 index (与 OpenAI streaming tool_calls index 对齐)
	nextToolCallIndex int

	// finished 表示 Flush 已经被调用过
	finished bool

	// OnContent 接收纯文本片段 (零拷贝 byte slice). 必须非 nil.
	OnContent func(p []byte) error

	// OnToolCall 接收完整解析出的一次 tool_call. 必须非 nil.
	OnToolCall func(tc *aispec.ToolCall) error
}

// NewReactToolExtractor 构造一个新的 extractor 实例.
// 关键词: NewReactToolExtractor
func NewReactToolExtractor(onContent func(p []byte) error, onToolCall func(tc *aispec.ToolCall) error) *ReactToolExtractor {
	return &ReactToolExtractor{
		OnContent:  onContent,
		OnToolCall: onToolCall,
	}
}

// Write 处理一段新输入字节. 状态机会反复尝试在 buffer 里抽出完整的 tool_call.
// 不立即可判定的部分 (前缀 / partial open tag) 会保留在 buf 等下一次 Write.
// 关键词: ReactToolExtractor.Write, 流式字节状态机入口
func (e *ReactToolExtractor) Write(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.finished {
		// 已 Flush, 后续输入按 text 透传
		return e.emitContentLocked(p)
	}
	e.buf.Write(p)
	return e.drainLocked()
}

// Flush 在流结束时调用, 把 buffer 中剩余字节 (可能含半开的 [tool_call... 没闭合) 当作 text emit.
// 关键词: ReactToolExtractor.Flush, 流末兜底
func (e *ReactToolExtractor) Flush() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.finished {
		return nil
	}
	e.finished = true
	if e.buf.Len() == 0 {
		return nil
	}
	leftover := e.buf.Bytes()
	e.buf.Reset()
	return e.emitContentLocked(leftover)
}

// HasEmittedToolCall 报告 extractor 至今是否成功 emit 过至少一次 tool_call.
// 关键词: HasEmittedToolCall, react extractor stats
func (e *ReactToolExtractor) HasEmittedToolCall() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nextToolCallIndex > 0
}

// drainLocked 反复扫描 buf, 抽出所有完整 [tool_call ...] tool_call.
// 不能消费的 partial 前缀保留在 buf 里. 假定调用方持锁.
// 关键词: drainLocked, react extractor state machine
func (e *ReactToolExtractor) drainLocked() error {
	for {
		raw := e.buf.Bytes()
		if len(raw) == 0 {
			return nil
		}
		openIdx := bytes.Index(raw, []byte(reactToolCallOpen))
		if openIdx == -1 {
			// 没有 [tool_call 开头标签. 但要保留尾部可能是 partial prefix 的字节
			safeLen := safeEmitTextLen(raw)
			if safeLen > 0 {
				if err := e.emitContentLocked(raw[:safeLen]); err != nil {
					return err
				}
				e.buf.Next(safeLen)
			}
			return nil
		}

		// [tool_call 之前的纯文本可以立即 emit
		if openIdx > 0 {
			if err := e.emitContentLocked(raw[:openIdx]); err != nil {
				return err
			}
			e.buf.Next(openIdx)
			continue // 重新从 buf 头部开始扫描 (现在 buf 以 [tool_call 开头)
		}

		// buf 现在 [tool_call... 开头, 找闭合标签 [/tool_call]
		closeIdx := bytes.Index(raw, []byte(reactToolCallClose))
		if closeIdx == -1 {
			// 没找到 close tag. 若 buf 已经超长, 视为坏数据 fall back to text emit.
			if e.buf.Len() > extractorBufferLimit {
				if err := e.emitContentLocked(raw); err != nil {
					return err
				}
				e.buf.Reset()
				return nil
			}
			// 否则等下一次 Write
			return nil
		}

		// 完整 tool_call 区段: raw[:closeIdx+len(close)]
		segment := raw[:closeIdx+len(reactToolCallClose)]
		tc, parseErr := parseReactToolCall(segment, e.nextToolCallIndex)
		if parseErr != nil {
			// 解析失败 (坏 JSON / name 缺失等), 兜底当 text emit
			if err := e.emitContentLocked(segment); err != nil {
				return err
			}
		} else {
			if err := e.OnToolCall(tc); err != nil {
				return err
			}
			e.nextToolCallIndex++
		}
		e.buf.Next(len(segment))
		continue
	}
}

// emitContentLocked 把字节作为纯文本 content 透传给 OnContent.
// 关键词: emitContentLocked, react extractor text passthrough
func (e *ReactToolExtractor) emitContentLocked(p []byte) error {
	if len(p) == 0 || e.OnContent == nil {
		return nil
	}
	return e.OnContent(p)
}

// safeEmitTextLen 返回可以立即 emit 的 text 长度.
// 尾部如果可能是 "[tool_call" 的前缀, 要保留, 等下一次 Write 来填齐.
// 关键词: safeEmitTextLen, partial prefix protection
func safeEmitTextLen(raw []byte) int {
	maxPrefix := len(reactToolCallOpen)
	n := len(raw)
	// 检查 raw 末尾最多 maxPrefix-1 字节是否是 "[tool_call" 的某个前缀
	for k := maxPrefix - 1; k > 0; k-- {
		if k > n {
			continue
		}
		tail := raw[n-k:]
		if bytes.HasPrefix([]byte(reactToolCallOpen), tail) {
			// 末尾 k 字节是 prefix, 不能 emit
			return n - k
		}
	}
	return n
}

// parseReactToolCall 解析一段完整的 [tool_call name=NAME]ARGS_JSON[/tool_call] 文本.
// 失败返回 error, 调用方 fallback 到 text emit.
//
// 兼容多种 header 形态:
//   - [tool_call name=foo]
//   - [tool_call name="foo"]
//   - [tool_call id="call_xyz" name="foo"]  (round2 flatten 后模型常 mimic 该格式)
//
// 当 header 同时携带 id="..." 时, 优先使用上游模型给出的 id, 以便:
//   - 与上游模型自己后续可能用到的 tool_call_id 强一致, 让调试/日志可追溯;
//   - 保留 round2 flatten 注入到历史里的原始 client tool_call_id, 让客户端
//     做精确的 tool_call_id 匹配 (虽然多数客户端只看 index, 但 OpenCode /
//     Codex 等会显示 id 用于 UI 关联).
// 未携带 id 时, 回落到 "call_react_N" 规则保持向后兼容.
//
// 关键词: parseReactToolCall, react tool_call serialization parse,
//        id 属性透传, OpenCode tool_call_id 关联
func parseReactToolCall(segment []byte, index int) (*aispec.ToolCall, error) {
	if !bytes.HasPrefix(segment, []byte(reactToolCallOpen)) {
		return nil, fmt.Errorf("segment does not start with %s", reactToolCallOpen)
	}
	if !bytes.HasSuffix(segment, []byte(reactToolCallClose)) {
		return nil, fmt.Errorf("segment does not end with %s", reactToolCallClose)
	}
	inner := segment[len(reactToolCallOpen) : len(segment)-len(reactToolCallClose)]
	// inner 形如 " name=NAME]ARGS_JSON" (有前导空格 / NAME 可能带引号)
	rightBracket := bytes.IndexByte(inner, ']')
	if rightBracket == -1 {
		return nil, fmt.Errorf("missing ] after [tool_call header")
	}
	header := string(inner[:rightBracket])
	argsRaw := inner[rightBracket+1:]

	name := extractReactHeaderAttr(header, "name")
	if name == "" {
		return nil, fmt.Errorf("missing name= in header: %q", header)
	}

	argsStr := strings.TrimSpace(string(argsRaw))
	if argsStr == "" {
		argsStr = "{}"
	}
	// 校验 args 是合法 JSON object
	var probe map[string]any
	if err := json.Unmarshal([]byte(argsStr), &probe); err != nil {
		return nil, fmt.Errorf("invalid args JSON: %v", err)
	}

	id := strings.TrimSpace(extractReactHeaderAttr(header, "id"))
	if id == "" {
		id = fmt.Sprintf("call_react_%d", index)
	}

	return &aispec.ToolCall{
		Index: index,
		ID:    id,
		Type:  "function",
		Function: aispec.FuncReturn{
			Name:      name,
			Arguments: argsStr,
		},
	}, nil
}

// extractReactHeaderAttr 从 header 字符串中抠出指定 key 的值.
// 支持形态:
//   - key=val     (val 直到空白 / `,` 截断)
//   - key="val"   (val 是引号包裹)
//
// 多个属性共存时按 key 精确匹配, 不会把 `id="..."` 中的 `name=` 子串误判
// (检索时要求 key 前一个字符是分隔符或字符串起点, 避免 prefix 子串误中).
// 关键词: extractReactHeaderAttr, react header attr 多属性解析
func extractReactHeaderAttr(header string, key string) string {
	if key == "" || header == "" {
		return ""
	}
	full := key + "="
	cur := 0
	for cur < len(header) {
		idx := strings.Index(header[cur:], full)
		if idx == -1 {
			return ""
		}
		pos := cur + idx
		// 前一个字符必须是空白 / 引号 / `,` / 字符串起点, 否则继续查找,
		// 避免 `xname=` 这种子串误中 `name=`.
		// 关键词: header 属性前置分隔符校验
		if pos > 0 {
			prev := header[pos-1]
			if prev != ' ' && prev != '\t' && prev != '\n' && prev != ',' && prev != '"' && prev != '\'' {
				cur = pos + len(full)
				continue
			}
		}
		rest := strings.TrimLeft(header[pos+len(full):], " \t")
		if rest == "" {
			return ""
		}
		if rest[0] == '"' {
			end := strings.IndexByte(rest[1:], '"')
			if end == -1 {
				return ""
			}
			return rest[1 : 1+end]
		}
		// 直到空白 / `,` 截断
		for i := 0; i < len(rest); i++ {
			c := rest[i]
			if c == ' ' || c == '\t' || c == '\n' || c == ',' {
				return rest[:i]
			}
		}
		return rest
	}
	return ""
}

// extractReactToolName 是 extractReactHeaderAttr 的语义薄包装, 保留旧名字
// 兼容已有调用方与单元测试.
// 关键词: extractReactToolName, react header parser
func extractReactToolName(header string) string {
	return extractReactHeaderAttr(header, "name")
}
