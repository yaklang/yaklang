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
//   - 工具调用文本格式 (规范): [tool_call name=NAME]ARGS_JSON[/tool_call]
//   - 工具调用文本格式 (兼容): <tool_call name="NAME" id="...">ARGS_JSON</tool_call>
//     (deepseek-v4-pro thinking + react 模式下的 hallucinate 漂移格式,
//      用户在 opencode TUI 实测复现, system prompt 仍只声明方括号一种)
//   - 工具结果文本格式: [tool_result tool_call_id=ID]CONTENT[/tool_result]
//     (extractor 仅解析 tool_call, tool_result 是 round2 -> 上游方向)
//
// 输入: 上游 wrapper 写过来的 content 字节流 (流式 chunk by chunk).
// 输出:
//   - 普通 content 文本 -> OnContent 回调, 按 OpenAI delta.content 透传给客户端
//   - 完整 [tool_call ...][/tool_call] 或 <tool_call ...></tool_call>
//     -> OnToolCall 回调, 按 OpenAI delta.tool_calls 透传给客户端
//
// 设计要点:
//  1. 跨 chunk 边界缓冲: buffer 累积所有未消费字节, 反复尝试匹配完整 tool_call.
//  2. 容错坏 JSON: 解析失败的内容仍作为 text emit 给 OnContent, 避免误吞普通文本.
//  3. 并行 multi tool_call: 一次 Flush 可 emit 多个独立 tool_call (index 递增).
//  4. partial prefix 保留: buffer 尾部若可能是 "[tool_call" 或 "<tool_call"
//     的前缀, 暂不 emit, 等下次 Write.
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

// reactToolCallOpenTokens / reactToolCallCloseTokens 是反解析时的全部候选
// open / close 字面量. 三段独立扫描: 找最早 open -> 找最早 close -> 切 segment.
// 这是 deepseek-v4-pro thinking + react 模式下"格式漂移可以是任意混合"
// 这一事实下唯一能稳定工作的策略: 不把 open/close 绑成 variant 二元组,
// 而是允许 open 与 close 自由组合.
//
// 关键词: reactToolCallOpenTokens, reactToolCallCloseTokens, 三段独立扫描,
//        多变体 hallucinate 防御
var (
	reactToolCallOpenTokens = []string{
		reactToolCallOpen,
		reactToolCallOpenAngle,
	}
	reactToolCallCloseTokens = []string{
		reactToolCallClose,
		reactToolCallCloseAngle,
		reactToolCallCloseMixedBA,
		reactToolCallCloseMixedAB,
	}
)

// findEarliestToken 在 raw 中找任一 candidate 最早出现的位置, 返回
// (idx, candidate). 都没找到返回 (-1, "").
// 关键词: findEarliestToken, multi-token scanner
func findEarliestToken(raw []byte, candidates []string) (int, string) {
	bestIdx := -1
	bestCand := ""
	for _, c := range candidates {
		idx := bytes.Index(raw, []byte(c))
		if idx == -1 {
			continue
		}
		if bestIdx == -1 || idx < bestIdx {
			bestIdx = idx
			bestCand = c
		}
	}
	return bestIdx, bestCand
}

// findHeaderEndQuoteAware 在 inner 里找第一个 ']' 或 '>' 当 header 终止符,
// 跳过 "..." 与 '...' 引号包裹内的字节, 防止 attr value 内嵌的特殊字符被
// 误识别为 header end. 返回 -1 表示没有 header end.
// 关键词: findHeaderEndQuoteAware, attr value > ] 字符防误切
func findHeaderEndQuoteAware(inner []byte) int {
	inQuote := false
	quoteCh := byte(0)
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if inQuote {
			if c == quoteCh {
				inQuote = false
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = true
			quoteCh = c
			continue
		}
		if c == ']' || c == '>' {
			return i
		}
	}
	return -1
}

// drainLocked 反复扫描 buf, 抽出所有完整 tool_call (任意 open/close 混合).
// 不能消费的 partial 前缀保留在 buf 里. 假定调用方持锁.
// 关键词: drainLocked, react extractor state machine, 三段独立扫描
func (e *ReactToolExtractor) drainLocked() error {
	for {
		raw := e.buf.Bytes()
		if len(raw) == 0 {
			return nil
		}
		openIdx, openTok := findEarliestToken(raw, reactToolCallOpenTokens)
		if openIdx == -1 {
			// 没有任何 open 标签. 但要保留尾部可能是某种 partial prefix 的字节
			safeLen := safeEmitTextLen(raw)
			if safeLen > 0 {
				if err := e.emitContentLocked(raw[:safeLen]); err != nil {
					return err
				}
				e.buf.Next(safeLen)
			}
			return nil
		}

		// open 之前的纯文本可以立即 emit
		if openIdx > 0 {
			if err := e.emitContentLocked(raw[:openIdx]); err != nil {
				return err
			}
			e.buf.Next(openIdx)
			continue // 重新从 buf 头部开始扫描
		}

		// buf 现在以 openTok 开头. 在 openTok 之后找最早的 close (任一种)
		afterOpen := raw[len(openTok):]
		closeRel, closeTok := findEarliestToken(afterOpen, reactToolCallCloseTokens)
		if closeRel == -1 {
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

		// 完整 tool_call 区段: raw[:len(openTok)+closeRel+len(closeTok)]
		segmentEnd := len(openTok) + closeRel + len(closeTok)
		segment := raw[:segmentEnd]
		tc, parseErr := parseReactToolCallSegment(segment, e.nextToolCallIndex, openTok, closeTok)
		if parseErr != nil {
			// 解析失败 (坏 JSON / name 缺失 / header end 缺失), 兜底当 text emit.
			// 这一兜底重要: 当 args body 里恰好嵌入了 close token 子串导致提前切割时,
			// JSON 校验会失败, 整段 fall back, 不会给客户端发残缺 tool_call.
			if err := e.emitContentLocked(segment); err != nil {
				return err
			}
		} else {
			if err := e.OnToolCall(tc); err != nil {
				return err
			}
			e.nextToolCallIndex++
		}
		e.buf.Next(segmentEnd)
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
// 尾部如果可能是任何已知 open 标签 ("[tool_call" / "<tool_call") 的前缀,
// 要保留, 等下一次 Write 来填齐, 否则会把跨 chunk 拆开的 open tag 误 emit
// 给客户端, 让本来能识别的 tool_call 退化成残缺文本.
// 关键词: safeEmitTextLen, partial prefix protection, 多 open token
func safeEmitTextLen(raw []byte) int {
	n := len(raw)
	// 找最大需要保留的尾部长度: 取所有 open token 前缀检查里能命中的最大 k
	holdBack := 0
	for _, opn := range reactToolCallOpenTokens {
		opnBytes := []byte(opn)
		maxPrefix := len(opnBytes)
		for k := maxPrefix - 1; k > 0; k-- {
			if k > n {
				continue
			}
			if bytes.HasPrefix(opnBytes, raw[n-k:]) {
				if k > holdBack {
					holdBack = k
				}
				break
			}
		}
	}
	return n - holdBack
}

// parseReactToolCallSegment 解析一段以 openTok 开头、closeTok 结尾的 tool_call
// 文本块, openTok / closeTok 来自 reactToolCallOpenTokens / reactToolCallCloseTokens
// 的任意组合 (方/尖括号 + 4 种 close token).
//
// header end 用 quote-aware 扫描在 inner 里找第一个未被引号包裹的 ']' 或 '>',
// 因此:
//   - args body 中的 ']' '>' 字符 (如 shell redirect `2>/dev/null`) 不影响切分;
//   - attr value 用 "..." 或 '...' 包裹时, 内部嵌入的 ']' '>' 不会被误当 header end.
//
// 兼容 header 形态:
//   - name=foo                      (裸值)
//   - name="foo"                    (双引号)
//   - name='foo'                    (单引号)
//   - id="call_xyz" name="foo"      (顺序无关)
//
// 关键词: parseReactToolCallSegment, 三段独立解析, quote-aware header end,
//        多变体 hallucinate 防御
func parseReactToolCallSegment(segment []byte, index int, openTok, closeTok string) (*aispec.ToolCall, error) {
	if !bytes.HasPrefix(segment, []byte(openTok)) {
		return nil, fmt.Errorf("segment does not start with %s", openTok)
	}
	if !bytes.HasSuffix(segment, []byte(closeTok)) {
		return nil, fmt.Errorf("segment does not end with %s", closeTok)
	}
	inner := segment[len(openTok) : len(segment)-len(closeTok)]
	headerEndIdx := findHeaderEndQuoteAware(inner)
	if headerEndIdx == -1 {
		return nil, fmt.Errorf("missing header end (']' or '>') after %s open tag", openTok)
	}
	header := string(inner[:headerEndIdx])
	argsRaw := inner[headerEndIdx+1:]

	name := extractReactHeaderAttr(header, "name")
	if name == "" {
		return nil, fmt.Errorf("missing name= in header: %q", header)
	}

	argsStr := strings.TrimSpace(string(argsRaw))
	if argsStr == "" {
		argsStr = "{}"
	}
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

// parseReactToolCall 是 parseReactToolCallSegment 的规范方括号薄包装,
// 保留以兼容历史调用方 / 老测试.
// 关键词: parseReactToolCall, 方括号格式默认入口, 兼容历史 API
func parseReactToolCall(segment []byte, index int) (*aispec.ToolCall, error) {
	return parseReactToolCallSegment(segment, index, reactToolCallOpen, reactToolCallClose)
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
		// 双引号 / 单引号 attr value 都接受 (deepseek hallucinate 时也会
		// 用 ' 包裹 attr value). 关键词: 单双引号 attr value 兼容.
		if rest[0] == '"' || rest[0] == '\'' {
			quote := rest[0]
			end := strings.IndexByte(rest[1:], quote)
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
