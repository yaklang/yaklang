package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
)

// chatJSONChunkWriter handles the streaming of chat completion responses
// It implements chunked transfer encoding for streaming responses
type chatJSONChunkWriter struct {
	// notStream 标记本次响应是否走「非流式」路径（客户端 stream=false）。
	// true 时所有 writerWrapper.Write / WriteToolCalls 仅累积到内部缓冲，
	// 不再向 writerClose 发送 SSE 增量帧；最终由 GetNotStreamBody 一次性
	// 拼出完整 chat.completion JSON 由 server.go 主流程写出。
	// 关键词: chatJSONChunkWriter notStream, 非流式不发 SSE
	notStream bool

	wg              *sync.WaitGroup
	writerClose     io.WriteCloser
	reasonBufWriter *bytes.Buffer
	outputBufWriter *bytes.Buffer
	mu              sync.Mutex
	closed          bool // Track if writer has been closed to prevent double-close

	uid     string    // Unique identifier for the chat session
	created time.Time // Timestamp when the chat session was created
	model   string    // Name of the AI model being used

	// lastUsage 由 WriteUsage 写入，保存上游 LLM 在 SSE 末帧返回的 token 用量
	// （prompt_tokens / completion_tokens / total_tokens / prompt_tokens_details）。
	// Close 时会按 OpenAI stream_options.include_usage=true 规范在 finish_reason="stop"
	// 帧之后、[DONE] 之前单独发一帧 choices=[] + usage={...} 给客户端，
	// 让客户端能感知隐式缓存命中（cached_tokens 等关键计费指标）。
	// 关键词: aibalance writer usage 透传, cached_tokens, include_usage 末帧
	lastUsage *aispec.ChatUsage

	// accumulatedToolCalls 按 index 合并所有上游回调过来的 tool_calls 增量，
	// 同时服务于两条链路：
	//   1. 流式模式: Close() 据此把 finish_reason 从 "stop" 修正为 "tool_calls"，
	//      让 OpenAI 兼容客户端 (OpenAI SDK / LangChain / litellm) 正确触发
	//      工具执行；deepseek-v4-pro thinking + tool_calls 必须依赖此判定。
	//   2. 非流式模式: GetNotStreamBody 据此把完整 tool_calls 字段写入返回的
	//      assistant message，避免 tool_calls 在非流式响应中被静默吞掉。
	//
	// 关键词: aibalance tool_calls 增量累积, finish_reason tool_calls, 非流式 tool_calls 还原
	accumulatedToolCalls map[int]*aispec.ToolCall
	toolCallOrder        []int

	// reactExtractor 在 round1 react 模式下被 EnableReactExtractor 实例化:
	// content 流字节先过 extractor, 由它分离出 [tool_call ...][/tool_call] 文本片段
	// 转换为 OpenAI tool_calls delta, 同时把普通 content 透传给客户端.
	// 关键词: chatJSONChunkWriter reactExtractor, ReAct -> tool_calls 反解析
	reactExtractor *ReactToolExtractor
}

// NewChatJSONChunkWriter creates a new chat JSON chunk writer
// writer: The underlying writer to write the chunks to
// uid: Unique identifier for the chat session
// model: Name of the AI model being used
func NewChatJSONChunkWriter(writer io.WriteCloser, uid string, model string) *chatJSONChunkWriter {
	return NewChatJSONChunkWriterEx(writer, uid, model, false)
}

// NewChatJSONChunkWriterEx 与 NewChatJSONChunkWriter 等价，但允许显式声明
// 本次响应是否「非流式」。当 notStream=true 时，writerWrapper.Write 与
// WriteToolCalls 都不会向客户端管道写 SSE 帧，仅把内容累积到 reasonBufWriter /
// outputBufWriter / accumulatedToolCalls 中，由 server.go 主流程在最后调用
// GetNotStreamBody 拼出完整的 chat.completion JSON。
//
// 关键词: NewChatJSONChunkWriterEx, 非流式 writer, 显式 notStream
func NewChatJSONChunkWriterEx(writer io.WriteCloser, uid string, model string, notStream bool) *chatJSONChunkWriter {
	pr, pw := bufpipe.NewPipe()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
		}()
		//io.Copy(writer, io.TeeReader(pr, os.Stdout))
		io.Copy(writer, pr)
		utils.FlushWriter(writer)
	}()
	return &chatJSONChunkWriter{
		notStream:       notStream,
		wg:              wg,
		writerClose:     pw,
		reasonBufWriter: bytes.NewBuffer(nil),
		outputBufWriter: bytes.NewBuffer(nil),
		uid:             uid,
		created:         time.Now(),
		model:           model,
	}
}

// buildDelta constructs a delta message for streaming responses
// reason: Whether this is a reason message (true) or content message (false)
// content: The actual content to be sent
func (w *chatJSONChunkWriter) buildDelta(reason bool, content string) ([]byte, error) {
	outputField := "content"
	if reason {
		outputField = "reasoning_content"
	}
	result := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"delta": map[string]any{outputField: content},
				"index": 0,
				//"finish_reason": "stop",
			},
		},
	}
	return json.Marshal(result)
}

// buildMessage constructs a full message for streaming responses
// reason: Whether this is a reason message (true) or content message (false)
// content: The actual content to be sent
//
// 关键修复：
//   - 当 accumulatedToolCalls 非空时把完整 tool_calls 数组写入 message，避免
//     非流式响应里 tool_calls 字段被静默丢失（旧实现只透传 content/reasoning_content）。
//   - finish_reason 与 tool_calls 状态联动：有 tool_calls 时改用 "tool_calls"，
//     与 OpenAI / DeepSeek 官方 chat completions 规范对齐，让 SDK 触发工具执行。
//
// 关键词: buildMessage tool_calls 还原, 非流式 finish_reason 联动
func (w *chatJSONChunkWriter) buildMessage(reasonContent string, content string) ([]byte, error) {
	r := map[string]any{
		"role": "assistant",
	}
	if reasonContent != "" {
		r["reasoning_content"] = reasonContent
	}
	r["content"] = content

	finishReason := "stop"
	if toolCalls := w.snapshotToolCallsForOutput(); len(toolCalls) > 0 {
		r["tool_calls"] = toolCalls
		finishReason = "tool_calls"
	}

	result := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"message":       r,
				"index":         0,
				"finish_reason": finishReason,
			},
		},
	}
	return json.Marshal(result)
}

// snapshotToolCallsForOutput 把 accumulatedToolCalls 按追加顺序拍平成
// 适合 JSON marshaling 的 []map[string]any 形态。读侧调用，写侧（accumulate）
// 已被 mu 保护，因此本方法假定调用者已持有 mu，避免在 GetNotStreamBody 等
// 已经持锁的路径上重复 Lock 引发死锁。
//
// 关键词: snapshotToolCallsForOutput, tool_calls JSON 输出形态
func (w *chatJSONChunkWriter) snapshotToolCallsForOutput() []map[string]any {
	if len(w.accumulatedToolCalls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(w.accumulatedToolCalls))
	for _, idx := range w.toolCallOrder {
		tc, ok := w.accumulatedToolCalls[idx]
		if !ok || tc == nil {
			continue
		}
		fn := map[string]any{
			"name":      tc.Function.Name,
			"arguments": tc.Function.Arguments,
		}
		out = append(out, map[string]any{
			"index":    tc.Index,
			"id":       tc.ID,
			"type":     tc.Type,
			"function": fn,
		})
	}
	return out
}

// accumulateToolCalls 把这一帧 tool_calls 增量按 index 合并到 writer 内部状态：
//
//   - 同一 index 的多帧（deepseek/OpenAI 流式 incremental arguments）会合并为一个
//     完整 ToolCall，arguments 字符串按到达顺序拼接。
//   - 不同 index 视为独立 tool call，按首次出现顺序在 toolCallOrder 中追加，
//     保证最终输出顺序稳定。
//
// 调用方需自行加锁（外层 WriteToolCalls 已持有 mu）。
//
// 关键词: accumulateToolCalls, tool_calls 按 index 累积, arguments 拼接
func (w *chatJSONChunkWriter) accumulateToolCalls(toolCalls []*aispec.ToolCall) {
	if len(toolCalls) == 0 {
		return
	}
	if w.accumulatedToolCalls == nil {
		w.accumulatedToolCalls = make(map[int]*aispec.ToolCall)
	}
	for _, tc := range toolCalls {
		if tc == nil {
			continue
		}
		existing, ok := w.accumulatedToolCalls[tc.Index]
		if !ok {
			existing = &aispec.ToolCall{
				Index: tc.Index,
				Type:  "function",
			}
			w.accumulatedToolCalls[tc.Index] = existing
			w.toolCallOrder = append(w.toolCallOrder, tc.Index)
		}
		if tc.ID != "" {
			existing.ID = tc.ID
		}
		if tc.Type != "" {
			existing.Type = tc.Type
		}
		if tc.Function.Name != "" {
			existing.Function.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			existing.Function.Arguments += tc.Function.Arguments
		}
	}
}

// hasAccumulatedToolCalls 是 Close() 判定 finish_reason 是否应当切换到
// "tool_calls" 的核心依据。需要持有 mu。
// 关键词: hasAccumulatedToolCalls, finish_reason 切换
func (w *chatJSONChunkWriter) hasAccumulatedToolCalls() bool {
	return len(w.accumulatedToolCalls) > 0
}

// HasToolCalls reports whether this response has emitted or accumulated any
// OpenAI-compatible tool_calls. It is used by the server success detector:
// tool-call-only responses are successful even when no content/reasoning bytes
// flowed through the text pipes.
//
// 关键词: HasToolCalls, tool_call-only success detector
func (w *chatJSONChunkWriter) HasToolCalls() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hasAccumulatedToolCalls()
}

func (w *chatJSONChunkWriter) flushReactExtractor() {
	if w.reactExtractor != nil {
		_ = w.reactExtractor.Flush()
	}
}

// writerWrapper wraps the chatJSONChunkWriter to handle different types of messages
type writerWrapper struct {
	notStream bool
	buf       *bytes.Buffer

	reason bool                 // Whether this is a reason writer
	writer *chatJSONChunkWriter // The underlying chat writer
}

// Write implements io.Writer interface for streaming responses
// It formats the data into chunked transfer encoding format
//
// 关键修复 (round1 react 模式):
//   - 当 writer 启用 reactExtractor 且本 wrapper 写的是 content 流 (非 reason),
//     字节先过 extractor: 普通文本走 emitReactContent, [tool_call ...] 文本块走 emitReactToolCall.
//   - reasoning_content 流保持原行为 (上游 thinking 思考链不应被 ReAct 抠出),
//     因此 reactExtractor 不接管 reason wrapper.
//
// 关键词: writerWrapper.Write react 接入, content vs reasoning_content
func (w *writerWrapper) Write(p []byte) (n int, err error) {
	if !w.reason && w.writer.reactExtractor != nil {
		if werr := w.writer.reactExtractor.Write(p); werr != nil {
			return 0, werr
		}
		return len(p), nil
	}

	if w.notStream {
		return w.buf.Write(p)
	}

	delta, err := w.writer.buildDelta(w.reason, string(p))
	if err != nil {
		return 0, err
	}

	w.writer.mu.Lock()
	defer w.writer.mu.Unlock()
	buf := bytes.Buffer{}
	buf.WriteString("data: ")
	buf.Write(delta)
	buf.WriteString("\n\n")
	w.writer.writerClose.Write([]byte(fmt.Sprintf("%x\r\n", buf.Len())))
	w.writer.writerClose.Write(buf.Bytes())
	w.writer.writerClose.Write([]byte("\r\n"))
	utils.FlushWriter(w.writer.writerClose)
	return len(p), nil
}

// EnableReactExtractor 开启 ReAct -> tool_calls 反解析模式.
// 调用后, output (content) 流字节会先过 ReactToolExtractor:
//   - 普通文本 -> 走 OpenAI delta.content 路径透传给客户端;
//   - [tool_call name=...]ARGS_JSON[/tool_call] -> 转成 OpenAI delta.tool_calls 透传给客户端,
//     同时累积到 accumulatedToolCalls 让 Close / GetNotStreamBody 把 finish_reason 联动为 tool_calls.
//
// 关键词: EnableReactExtractor, react extractor 接入 writer
func (w *chatJSONChunkWriter) EnableReactExtractor() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.reactExtractor != nil {
		return
	}
	w.reactExtractor = NewReactToolExtractor(
		func(p []byte) error { return w.emitReactContent(p) },
		func(tc *aispec.ToolCall) error { return w.emitReactToolCall(tc) },
	)
}

// emitReactContent 把 react extractor 抽出的纯文本片段透传给客户端.
// 自己持锁, 与 writerWrapper.Write 在不同调用栈上不会死锁 (writerWrapper.Write 入口已不再持锁).
// 关键词: emitReactContent, react extractor text passthrough
func (w *chatJSONChunkWriter) emitReactContent(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if w.notStream {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.outputBufWriter.Write(p)
		return nil
	}
	delta, err := w.buildDelta(false, string(p))
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	buf := bytes.Buffer{}
	buf.WriteString("data: ")
	buf.Write(delta)
	buf.WriteString("\n\n")
	w.writerClose.Write([]byte(fmt.Sprintf("%x\r\n", buf.Len())))
	w.writerClose.Write(buf.Bytes())
	w.writerClose.Write([]byte("\r\n"))
	utils.FlushWriter(w.writerClose)
	return nil
}

// emitReactToolCall 把 react extractor 抽出的完整 tool_call 透传给客户端,
// 同时累积到 accumulatedToolCalls 让 finish_reason 联动.
// 关键词: emitReactToolCall, react tool_call -> OpenAI delta
func (w *chatJSONChunkWriter) emitReactToolCall(tc *aispec.ToolCall) error {
	if tc == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accumulateToolCalls([]*aispec.ToolCall{tc})
	if w.notStream {
		// 非流式: 已累积, GetNotStreamBody 一次性输出
		return nil
	}
	if w.closed {
		return nil
	}
	delta, err := w.buildToolCallsDelta([]*aispec.ToolCall{tc})
	if err != nil {
		return err
	}
	buf := bytes.Buffer{}
	buf.WriteString("data: ")
	buf.Write(delta)
	buf.WriteString("\n\n")
	w.writerClose.Write([]byte(fmt.Sprintf("%x\r\n", buf.Len())))
	w.writerClose.Write(buf.Bytes())
	w.writerClose.Write([]byte("\r\n"))
	utils.FlushWriter(w.writerClose)
	log.Infof("emitReactToolCall: forwarded react tool_call name=%s index=%d", tc.Function.Name, tc.Index)
	return nil
}

// GetOutputWriter returns a writer for content messages
func (w *chatJSONChunkWriter) GetOutputWriter() *writerWrapper {
	return &writerWrapper{
		notStream: w.notStream,
		buf:       w.outputBufWriter,
		reason:    false,
		writer:    w,
	}
}

// GetReasonWriter returns a writer for reason messages
func (w *chatJSONChunkWriter) GetReasonWriter() *writerWrapper {
	return &writerWrapper{
		notStream: w.notStream,
		buf:       w.reasonBufWriter,
		reason:    true,
		writer:    w,
	}
}

func (w *chatJSONChunkWriter) WriteError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	defer utils.FlushWriter(w.writerClose)

	rawmsg := map[string]any{
		"error": err,
	}
	msgBytes, err := json.Marshal(rawmsg)
	if err != nil {
		log.Printf("Failed to marshal error: %v", err)
		return
	}
	msg := fmt.Sprintf("data: %s\n\n", string(msgBytes))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		log.Printf("Failed to write error: %v", err)
	}
}

// buildToolCallsDelta constructs a delta message for tool calls streaming responses
// toolCalls: The tool calls to be sent
func (w *chatJSONChunkWriter) buildToolCallsDelta(toolCalls []*aispec.ToolCall) ([]byte, error) {
	// Convert to OpenAI format tool_calls structure
	// Use ToolCall.Index if set, otherwise fall back to array position
	var formattedToolCalls []map[string]any
	for i, tc := range toolCalls {
		index := tc.Index
		if index == 0 && i > 0 {
			// If Index is 0 but this isn't the first element, use array position as fallback
			// This handles cases where Index wasn't explicitly set
			index = i
		}
		formattedToolCalls = append(formattedToolCalls, map[string]any{
			"index": index,
			"id":    tc.ID,
			"type":  tc.Type,
			"function": map[string]any{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		})
	}

	result := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"delta": map[string]any{
					"role":       "assistant",
					"tool_calls": formattedToolCalls,
				},
				"index":         0,
				"finish_reason": nil, // tool_calls chunk doesn't have finish_reason yet
			},
		},
	}
	return json.Marshal(result)
}

// WriteUsage 由上游 UsageCallback 触发，把上游 LLM 返回的 token 用量
// （包含隐式缓存命中 cached_tokens）保存下来。Close 时会按 OpenAI
// stream_options.include_usage=true 规范，在 finish_reason="stop" 帧之后、
// [DONE] 帧之前单独发一帧 choices=[] + usage={...}，把 usage 透传给客户端。
// 这是修复"aibalance 不返回 usage / cached_tokens 导致客户端无法计算缓存命中率"
// 这一问题的关键链路终点。
//
// 关键词: aibalance WriteUsage, usage 透传, cached_tokens 透传, include_usage 末帧
func (w *chatJSONChunkWriter) WriteUsage(usage *aispec.ChatUsage) {
	if usage == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastUsage = usage
}

// WriteToolCalls writes tool calls to the streaming response
// toolCalls: The tool calls received from AI provider to be forwarded to client
//
// 关键修复：
//   - 无论 stream / 非 stream 模式都先把这一帧 tool_calls 累积到内部状态
//     (accumulateToolCalls)，让 Close() 能判定 finish_reason 是否应改为
//     "tool_calls"，也让 GetNotStreamBody 能拼出完整 tool_calls 数组。
//   - notStream 模式下不再向客户端管道写 SSE 帧（与 writerWrapper.Write
//     非流式行为对齐），避免「同时输出 SSE 增量帧 + 最终 JSON」的混乱响应。
//
// 关键词: WriteToolCalls 总累积, finish_reason 联动, 非流式不发 SSE
func (w *chatJSONChunkWriter) WriteToolCalls(toolCalls []*aispec.ToolCall) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 总是累积，让 Close 与 GetNotStreamBody 都能感知 tool_calls。
	w.accumulateToolCalls(toolCalls)

	if w.notStream {
		// 非流式: tool_calls 已累积，最终通过 GetNotStreamBody 一次性输出。
		return nil
	}

	if w.closed {
		return nil
	}

	delta, err := w.buildToolCallsDelta(toolCalls)
	if err != nil {
		log.Errorf("Failed to build tool calls delta: %v", err)
		return err
	}

	buf := bytes.Buffer{}
	buf.WriteString("data: ")
	buf.Write(delta)
	buf.WriteString("\n\n")
	w.writerClose.Write([]byte(fmt.Sprintf("%x\r\n", buf.Len())))
	w.writerClose.Write(buf.Bytes())
	w.writerClose.Write([]byte("\r\n"))
	utils.FlushWriter(w.writerClose)

	log.Infof("WriteToolCalls: forwarded %d tool calls to client", len(toolCalls))
	return nil
}

// GetNotStreamBody 拼出符合 OpenAI 规范的「chat.completion」非流式响应体：
//   - object 字段使用 "chat.completion"（流式才用 "chat.completion.chunk"）。
//   - message.tool_calls 来源于全程累积的 accumulatedToolCalls，避免丢失。
//   - finish_reason 与 tool_calls 状态联动，存在 tool_calls 时为 "tool_calls"。
//   - 若上游回填了 usage（lastUsage），一并写入顶层 usage 字段，
//     与 OpenAI / DeepSeek chat completions 一致，便于上层 SDK 计费 / 限速。
//
// 关键词: GetNotStreamBody, chat.completion, 非流式 tool_calls/usage 完整体
func (w *chatJSONChunkWriter) GetNotStreamBody() []byte {
	w.flushReactExtractor()

	w.mu.Lock()
	defer w.mu.Unlock()

	reasonContent := w.reasonBufWriter.String()
	content := w.outputBufWriter.String()

	message := map[string]any{
		"role":    "assistant",
		"content": content,
	}
	if reasonContent != "" {
		message["reasoning_content"] = reasonContent
	}

	finishReason := "stop"
	if toolCalls := w.snapshotToolCallsForOutput(); len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		finishReason = "tool_calls"
	}

	result := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"message":       message,
				"index":         0,
				"finish_reason": finishReason,
			},
		},
	}

	if w.lastUsage != nil {
		result["usage"] = buildUsageMap(w.lastUsage)
	}

	body, err := json.Marshal(result)
	if err != nil {
		return []byte(utils.Errorf("GetNotStreamBody marshal failed: %v", err).Error())
	}
	return body
}

// buildUsageMap 把 aispec.ChatUsage 转成与 OpenAI chat.completion.usage
// 字段一致的 map（含可选的 prompt_tokens_details.cached_tokens）。
// 关键词: buildUsageMap, OpenAI usage 字段映射, cached_tokens
func buildUsageMap(u *aispec.ChatUsage) map[string]any {
	if u == nil {
		return nil
	}
	out := map[string]any{
		"prompt_tokens":     u.PromptTokens,
		"completion_tokens": u.CompletionTokens,
		"total_tokens":      u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		out["prompt_tokens_details"] = map[string]any{
			"cached_tokens": u.PromptTokensDetails.CachedTokens,
		}
	}
	return out
}

func (w *chatJSONChunkWriter) Wait() {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("wait group err: %v", err)
		}
	}()
	w.wg.Wait()
}

// Close finalizes the streaming response
// It sends the [DONE] marker and closes the underlying writer
// Safe to call multiple times - subsequent calls are no-op
func (w *chatJSONChunkWriter) Close() error {
	// 先 Flush reactExtractor (不持 mu, 它的 callback emitReactContent / emitReactToolCall
	// 内部会各自 Lock/Unlock mu, 否则会死锁).
	// 关键词: Close react extractor flush before lock, deadlock-free
	w.flushReactExtractor()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Prevent double-close
	if w.closed {
		return nil
	}
	w.closed = true

	defer utils.FlushWriter(w.writerClose)

	if w.notStream {
		// Even for non-stream, we need to close the writer to release resources
		return w.writerClose.Close()
	}
	log.Info("start to close ChatJsonChunkWriter")

	// 关键修复: 当本次响应里曾经发送过 tool_calls 时，按 OpenAI / DeepSeek
	// chat completions 规范，末帧 finish_reason 必须是 "tool_calls" 而非 "stop"。
	// 否则 OpenAI Python SDK / LangChain / litellm 等库会把响应当作普通对话结束，
	// 不会触发函数执行 —— 这是用户报告的「deepseek-v4-pro 工具调用无法启用」
	// 的核心原因。
	// 关键词: Close finish_reason tool_calls 修正, OpenAI 规范对齐
	finishReason := "stop"
	if w.hasAccumulatedToolCalls() {
		finishReason = "tool_calls"
	}

	rawmsg := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": finishReason,
			},
		},
	}
	msgBytes, err := json.Marshal(rawmsg)
	if err != nil {
		// Still need to close the writer even on error
		w.writerClose.Close()
		return err
	}
	msg := fmt.Sprintf("data: %s\n\n", string(msgBytes))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)

	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		w.writerClose.Close()
		return err
	}

	// 按 OpenAI stream_options.include_usage=true 规范：在 finish_reason="stop" 帧之后、
	// [DONE] 帧之前，单独发一帧 choices=[] + usage={...}，把上游返回的 token 用量
	// （包含 prompt_tokens / completion_tokens / total_tokens / prompt_tokens_details
	// 即 cached_tokens 等隐式缓存命中信息）透传给客户端。
	// 关键词: aibalance Close usage 帧, include_usage, cached_tokens 透传
	if w.lastUsage != nil {
		usageMsg := map[string]any{
			"id":      "chat-ai-balance-" + w.uid,
			"object":  "chat.completion.chunk",
			"created": w.created.Unix(),
			"model":   w.model,
			"choices": []map[string]any{},
			"usage":   w.lastUsage,
		}
		if usageBytes, jerr := json.Marshal(usageMsg); jerr == nil {
			usageData := fmt.Sprintf("data: %s\n\n", string(usageBytes))
			usageChunk := fmt.Sprintf("%x\r\n%s\r\n", len(usageData), usageData)
			if _, werr := w.writerClose.Write([]byte(usageChunk)); werr != nil {
				log.Warnf("write usage chunk failed: %v", werr)
			}
		} else {
			log.Warnf("marshal usage chunk failed: %v", jerr)
		}
	}

	// write data: [DONE]
	msg = "data: [DONE]\n\n"
	chunk = fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		w.writerClose.Close()
		return err
	}

	// Send chunked encoding end marker
	chunk = "0\r\n\r\n"
	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		w.writerClose.Close()
		return err
	}

	return w.writerClose.Close()
}
