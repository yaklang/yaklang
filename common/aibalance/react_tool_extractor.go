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
	// reactToolCallOpenTokens 是 drainLocked 通用扫描的 open 候选集.
	// 注意 mistral / deepseek-fullwidth 命中后 drainLocked 走特例分支(brace-balanced
	// JSON 数组扫描 / 内部子帧切分), 不走 reactToolCallCloseTokens 通用 close 扫描.
	reactToolCallOpenTokens = []string{
		reactToolCallOpen,
		reactToolCallOpenAngle,
		reactToolCallOpenChinese,
		reactToolCallOpenDeepseekFW,
		reactToolCallOpenMistral,
	}
	// reactToolCallCloseTokens 仅供 bracket / angle / chinese / anthropic-xml-param
	// 这类"成对 open/close 标签"形态使用.
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
//
// v2 raw passthrough 新增两类特例分支 (不走通用 close 候选扫描):
//   - mistral: [TOOL_CALLS] 后面是 JSON 数组自闭合, 用 brace-balanced 扫描
//   - deepseek-fullwidth: <｜tool_calls_begin｜>...<｜tool_calls_end｜> 外层包裹,
//     内部多个 <｜tool_call_begin｜>NAME<｜tool_sep｜>ARGS<｜tool_call_end｜> 子帧
//
// 关键词: drainLocked, react extractor state machine, mistral 数组扫描,
//
//	deepseek-fullwidth 子帧切分, raw passthrough
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

		// 特例 1: mistral [TOOL_CALLS] [...] JSON 数组自闭合, brace-balanced 扫描
		if openTok == reactToolCallOpenMistral {
			rtcs, consumed, complete := extractMistralArrayRaw(raw)
			if !complete {
				// 数组还没接收完整. buf 超长视为坏数据 fall back to text emit.
				if e.buf.Len() > extractorBufferLimit {
					if err := e.emitContentLocked(raw); err != nil {
						return err
					}
					e.buf.Reset()
					return nil
				}
				return nil
			}
			if len(rtcs) == 0 {
				// 数组结构能识别但 element 都没有合法 name, 整段当 text 兜底
				if err := e.emitContentLocked(raw[:consumed]); err != nil {
					return err
				}
			} else {
				for _, rtc := range rtcs {
					tc := buildToolCallFromRaw(rtc, e.nextToolCallIndex)
					if err := e.OnToolCall(tc); err != nil {
						return err
					}
					e.nextToolCallIndex++
				}
			}
			e.buf.Next(consumed)
			continue
		}

		// 特例 2: deepseek 全角外层 <｜tool_calls_begin｜>...<｜tool_calls_end｜>
		if openTok == reactToolCallOpenDeepseekFW {
			afterOpen := raw[len(openTok):]
			closeRel := bytes.Index(afterOpen, []byte(reactToolCallCloseDeepseekFW))
			if closeRel == -1 {
				if e.buf.Len() > extractorBufferLimit {
					if err := e.emitContentLocked(raw); err != nil {
						return err
					}
					e.buf.Reset()
					return nil
				}
				return nil
			}
			segmentEnd := len(openTok) + closeRel + len(reactToolCallCloseDeepseekFW)
			segment := raw[:segmentEnd]
			rtcs := extractDeepseekFullwidthAll(segment)
			if len(rtcs) == 0 {
				if err := e.emitContentLocked(segment); err != nil {
					return err
				}
			} else {
				for _, rtc := range rtcs {
					tc := buildToolCallFromRaw(rtc, e.nextToolCallIndex)
					if err := e.OnToolCall(tc); err != nil {
						return err
					}
					e.nextToolCallIndex++
				}
			}
			e.buf.Next(segmentEnd)
			continue
		}

		// 通用 (bracket / angle / chinese / anthropic-xml-param / hermes-body): 找 close 切 segment
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
			// name 抠不出来 / hermes-body 也解不出, 整段当 text 兜底.
			// 这一兜底重要: 当 args body 里恰好嵌入了 close token 子串导致提前切割时,
			// 整段 fall back, 不会给客户端发残缺 tool_call.
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
// 尾部如果可能是任何已知 open 标签的 partial prefix, 要保留, 等下一次 Write 来填齐,
// 否则会把跨 chunk 拆开的 open tag 误 emit 给客户端, 让本来能识别的 tool_call
// 退化成残缺文本.
//
// v2 raw passthrough 已扩展 reactToolCallOpenTokens 含 5 类 variant open token:
//   - [tool_call            (ASCII, 10 bytes)
//   - <tool_call            (ASCII, 10 bytes)
//   - [调用                 (UTF-8, 7 bytes:  '[' + '调'(3B) + '用'(3B))
//   - <｜tool_calls_begin｜> (UTF-8, 24 bytes: '<' + 全角'｜'(3B) + 'tool_calls_begin' + 全角'｜'(3B) + '>')
//   - [TOOL_CALLS]          (ASCII, 12 bytes)
//
// 本函数迭代该 slice, 自动覆盖所有新 token 的尾部 partial prefix 保护. 不需要每次
// 加新 variant 都改这里 - 只要把 open token 加进 reactToolCallOpenTokens 即可.
// holdBack 取所有 token 的最大可能命中值, 因此即便用户输入恰好是某个 token 的真前缀,
// 也会被保留到下一次 Write 填齐.
//
// 关键词: safeEmitTextLen, partial prefix protection, 多 open token,
//
//	跨 chunk 边界保护, UTF-8 多字节 token (中文 / 全角分隔符) 自动覆盖
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

// parseReactToolCallSegment v2 raw passthrough: 解析一段以 openTok 开头、closeTok
// 结尾的 tool_call 文本块, openTok / closeTok 来自 reactToolCallOpenTokens /
// reactToolCallCloseTokens 的任意组合 (方/尖括号 + 中文动词 [调用 + 4 种 close).
//
// v2 关键变化 (跟 v1 的差异):
//   - args body **不再** 做 json.Unmarshal 强校验, 原文塞进 ToolCall.Function.Arguments.
//   - 设计理由: OpenAI 协议 tool_calls[].function.arguments 字段是 string, 协议层面
//     没有规定它必须是 valid JSON. JSON 是惯例, 不是契约. 各 client SDK 加的
//     "arguments 必须 JSON 校验" 是 client 侧的事, aibalance 作为中转不替客户端做这件事.
//   - 即便 args 是 Anthropic <parameter> XML / 中文文本 / 任何东西, 也原样透传.
//
// name 提取按 variant 兜底链:
//  1. chinese-invoke 特例: openTok == [调用, name 从 open 之后到第一个 ']' 之间取
//  2. canonical / angle / anthropic-xml-param: header attr name= (含单/双引号 / 裸值)
//  3. hermes-body-name: header 没 name= 时, 探测 args body 是否为 JSON 含 .name 字段
//
// header end 仍用 quote-aware 扫描在 inner 里找第一个未被引号包裹的 ']' 或 '>',
// 因此 args body 中的 ']' '>' 字符 (如 shell redirect `2>/dev/null`) 不影响切分;
// attr value 用 "..." 或 '...' 包裹时, 内部嵌入的 ']' '>' 不会被误当 header end.
//
// 兼容 header 形态:
//   - name=foo                      (裸值)
//   - name="foo"                    (双引号)
//   - name='foo'                    (单引号)
//   - id="call_xyz" name="foo"      (顺序无关)
//
// 关键词: parseReactToolCallSegment, raw passthrough, args 原文不解析,
//
//	多 variant name 兜底链, hermes body name 探测, anthropic xml param 兼容
func parseReactToolCallSegment(segment []byte, index int, openTok, closeTok string) (*aispec.ToolCall, error) {
	if !bytes.HasPrefix(segment, []byte(openTok)) {
		return nil, fmt.Errorf("segment does not start with %s", openTok)
	}
	if !bytes.HasSuffix(segment, []byte(closeTok)) {
		return nil, fmt.Errorf("segment does not end with %s", closeTok)
	}
	inner := segment[len(openTok) : len(segment)-len(closeTok)]

	var (
		name string
		args string
		id   string
	)

	switch openTok {
	case reactToolCallOpenChinese:
		// chinese-invoke 特例: open=[调用 后面是 ` NAME] args`
		n, a, ok := extractChineseInvokeName(inner)
		if !ok {
			return nil, fmt.Errorf("chinese-invoke: cannot extract name from %q", string(inner))
		}
		name = n
		args = a
	default:
		// canonical / angle / anthropic-xml-param / hermes-body 共用 header attr 提取链路
		headerEndIdx := findHeaderEndQuoteAware(inner)
		if headerEndIdx == -1 {
			// 没有 header end -> 可能整段 inner 就是 hermes body JSON
			if n, a, ok := extractHermesBodyName(bytes.TrimSpace(inner)); ok {
				name = n
				args = a
			} else {
				return nil, fmt.Errorf("missing header end and not hermes-body: %s", openTok)
			}
		} else {
			header := string(inner[:headerEndIdx])
			argsRaw := inner[headerEndIdx+1:]
			if n := extractReactHeaderAttr(header, "name"); n != "" {
				// canonical / angle / anthropic-xml-param: header 显式带 name=
				name = n
				args = string(argsRaw)
			} else {
				// header 只有 id= 之类, 试 hermes-body (argsRaw 是 JSON)
				if n2, a, ok := extractHermesBodyName(bytes.TrimSpace(argsRaw)); ok {
					name = n2
					args = a
				} else {
					return nil, fmt.Errorf("missing name= in header and not hermes-body: %q", header)
				}
			}
			id = strings.TrimSpace(extractReactHeaderAttr(header, "id"))
		}
	}

	args = strings.TrimSpace(args)
	if args == "" {
		args = "{}"
	}
	if id == "" {
		id = fmt.Sprintf("call_react_%d", index)
	}

	return &aispec.ToolCall{
		Index: index,
		ID:    id,
		Type:  "function",
		Function: aispec.FuncReturn{
			Name:      name,
			Arguments: args,
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

// ============================================================================
// v2 raw passthrough: 多 variant name 提取 + multi-call 提取 helper
// ============================================================================

// rawToolCall 是 drainLocked 特例分支 (mistral / deepseek-fullwidth) 用的轻量
// 中间结构, 不直接对外, 由 buildToolCallFromRaw 转成 *aispec.ToolCall.
// 关键词: rawToolCall, multi-call extractor 中间结构
type rawToolCall struct {
	Name string
	Args string
	ID   string // 可选, 空时由调用方按 index 生成
}

// buildToolCallFromRaw 把内部 rawToolCall 包成对外的 *aispec.ToolCall, 自动分配
// 默认 ID. args 为空时填 "{}" 保证 OpenAI client 兼容.
// 关键词: buildToolCallFromRaw, raw tool_call -> aispec.ToolCall
func buildToolCallFromRaw(rtc rawToolCall, index int) *aispec.ToolCall {
	args := strings.TrimSpace(rtc.Args)
	if args == "" {
		args = "{}"
	}
	id := strings.TrimSpace(rtc.ID)
	if id == "" {
		id = fmt.Sprintf("call_react_%d", index)
	}
	return &aispec.ToolCall{
		Index: index,
		ID:    id,
		Type:  "function",
		Function: aispec.FuncReturn{
			Name:      rtc.Name,
			Arguments: args,
		},
	}
}

// extractChineseInvokeName 处理 chinese-invoke 形态 segment 的 inner 部分.
// 调用前 segment 已剥掉 openTok `[调用`, 此时 inner 长这样:
//
//	" todowrite] {\"todos\":[...]} "
//	" web_search] {...}\n"
//
// 提取规则:
//   - 找第一个 ']' 字符截断 header
//   - header = inner[:idx], TrimSpace 后即 name (容忍 `[调用 my tool]` 这种含空白的 name)
//   - args = inner[idx+1:], 原文返回不解析
//
// 关键词: extractChineseInvokeName, chinese-invoke header 拆分
func extractChineseInvokeName(inner []byte) (name string, args string, ok bool) {
	idx := bytes.IndexByte(inner, ']')
	if idx == -1 {
		return "", "", false
	}
	name = strings.TrimSpace(string(inner[:idx]))
	if name == "" {
		return "", "", false
	}
	args = string(inner[idx+1:])
	return name, args, true
}

// extractHermesBodyName 处理 hermes-body-name 形态:
//
//	<tool_call>{"name":"bash","arguments":{"command":"ls"}}</tool_call>
//	<tool_call id="x">{"name":"bash","arguments":{"command":"ls"}}</tool_call>
//
// 调用前 body 是 args body 字节 (已剥掉 wrapper open/close + header). 解析逻辑:
//   - 用 json.Unmarshal 探测 body 是否为 {"name":...,"arguments":...} 形态
//   - 命中后: name 取 .name 字段, args 取 .arguments 子段 json.Marshal 回字符串 (恢复 native 协议语义)
//   - 没命中返回 (_, _, false), 调用方走兜底
//
// 注意 args 子段如果原始是 object/array, 用 json.RawMessage 保留原始字节避免重排序;
// 如果原始是 string, 也保留 string 形态.
//
// 关键词: extractHermesBodyName, hermes body name 探测, json.RawMessage 保序
func extractHermesBodyName(body []byte) (name string, args string, ok bool) {
	if len(body) == 0 {
		return "", "", false
	}
	var probe struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", "", false
	}
	if probe.Name == "" {
		return "", "", false
	}
	if len(probe.Arguments) == 0 {
		return probe.Name, "{}", true
	}
	// .arguments 子段如果原始是 string (双重 escape JSON), 保持 string 形态;
	// 如果原始是 object/array, 直接取 RawMessage 字节 (保序).
	if len(probe.Arguments) > 0 && probe.Arguments[0] == '"' {
		var asStr string
		if err := json.Unmarshal(probe.Arguments, &asStr); err == nil {
			return probe.Name, asStr, true
		}
	}
	return probe.Name, string(probe.Arguments), true
}

// extractDeepseekFullwidthAll 把 deepseek V3.1 全角分隔符 segment 切分成多个子帧
// rawToolCall. segment 已是完整的 <｜tool_calls_begin｜>...<｜tool_calls_end｜>
// 区间字节.
//
// 内部形态:
//
//	<｜tool_call_begin｜>NAME<｜tool_sep｜>ARGS<｜tool_call_end｜>
//
// 多个子帧可背靠背 (一个外层 calls_begin 含并行多 tool_call).
//
// 关键词: extractDeepseekFullwidthAll, deepseek v31 sub-frame 切分, parallel multi tool_call
func extractDeepseekFullwidthAll(segment []byte) []rawToolCall {
	// 剥掉外层 calls_begin / calls_end
	if !bytes.HasPrefix(segment, []byte(reactToolCallOpenDeepseekFW)) ||
		!bytes.HasSuffix(segment, []byte(reactToolCallCloseDeepseekFW)) {
		return nil
	}
	inner := segment[len(reactToolCallOpenDeepseekFW) : len(segment)-len(reactToolCallCloseDeepseekFW)]

	var out []rawToolCall
	cursor := 0
	for cursor < len(inner) {
		// 找下一个 sub-frame begin
		bIdx := bytes.Index(inner[cursor:], []byte(reactDeepseekSubFrameBegin))
		if bIdx == -1 {
			break
		}
		frameStart := cursor + bIdx + len(reactDeepseekSubFrameBegin)
		// 找对应 sub-frame end
		eIdx := bytes.Index(inner[frameStart:], []byte(reactDeepseekSubFrameEnd))
		if eIdx == -1 {
			break
		}
		frameEnd := frameStart + eIdx
		frameBody := inner[frameStart:frameEnd]
		// frameBody 形如 NAME<｜tool_sep｜>ARGS
		sepIdx := bytes.Index(frameBody, []byte(reactDeepseekSubFrameSep))
		var rtc rawToolCall
		if sepIdx == -1 {
			// 没分隔符: 整段当 name, args 空
			rtc.Name = strings.TrimSpace(string(frameBody))
		} else {
			rtc.Name = strings.TrimSpace(string(frameBody[:sepIdx]))
			rtc.Args = string(frameBody[sepIdx+len(reactDeepseekSubFrameSep):])
		}
		if rtc.Name != "" {
			out = append(out, rtc)
		}
		cursor = frameEnd + len(reactDeepseekSubFrameEnd)
	}
	return out
}

// extractMistralArrayRaw 处理 mistral [TOOL_CALLS] [...] 形态. buf 头部以
// `[TOOL_CALLS]` 开头, 后面跟一个 JSON 数组 (可能含 leading whitespace).
//
// 返回:
//   - rtcs: 解析出的 tool_call 列表 (name 从 element.name 取, args 从 element.arguments 取)
//   - consumed: 已消费的字节数 (含 `[TOOL_CALLS]` token + 空白 + JSON 数组)
//   - complete: JSON 数组是否完整接收 (用于跨 chunk 边界判定)
//
// JSON 数组用 brace/bracket balanced + quote-aware 扫描定位结尾 ']'.
//
// 关键词: extractMistralArrayRaw, mistral 数组扫描, brace-balanced JSON,
//
//	跨 chunk 边界完整性判定
func extractMistralArrayRaw(buf []byte) (rtcs []rawToolCall, consumed int, complete bool) {
	if !bytes.HasPrefix(buf, []byte(reactToolCallOpenMistral)) {
		return nil, 0, false
	}
	cursor := len(reactToolCallOpenMistral)
	// 跳过 leading whitespace
	for cursor < len(buf) {
		c := buf[cursor]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			cursor++
			continue
		}
		break
	}
	if cursor >= len(buf) {
		return nil, 0, false
	}
	if buf[cursor] != '[' {
		// 不是 JSON 数组形态, 直接判定为完整但 0 tool_call, 让 caller emit 整段 text
		return nil, cursor, true
	}
	// brace-balanced + quote-aware 扫描找匹配 ']'
	depth := 0
	inQuote := false
	escape := false
	arrStart := cursor
	arrEnd := -1
	for i := cursor; i < len(buf); i++ {
		c := buf[i]
		if escape {
			escape = false
			continue
		}
		if inQuote {
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inQuote = false
			}
			continue
		}
		switch c {
		case '"':
			inQuote = true
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 && c == ']' {
				arrEnd = i
			}
		}
		if arrEnd != -1 {
			break
		}
	}
	if arrEnd == -1 {
		// 数组未完整接收
		return nil, 0, false
	}
	arrBytes := buf[arrStart : arrEnd+1]
	// json.Unmarshal 数组 element
	var elems []json.RawMessage
	if err := json.Unmarshal(arrBytes, &elems); err != nil {
		// JSON 结构损坏, 当 0 tool_call 但 consumed 推进, 整段当 text 兜底
		return nil, arrEnd + 1, true
	}
	for _, el := range elems {
		n, a, ok := extractHermesBodyName(el)
		if !ok {
			continue
		}
		rtcs = append(rtcs, rawToolCall{Name: n, Args: a})
	}
	return rtcs, arrEnd + 1, true
}
