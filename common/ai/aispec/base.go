package aispec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/davecgh/go-spew/spew"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func ListChatModels(url string, opt func() ([]poc.PocConfigOption, error)) ([]*ModelMeta, error) {
	opts, err := opt()
	if err != nil {
		return nil, utils.Errorf("build config failed: %v", err)
	}
	opts = append(
		opts, poc.WithTimeout(30), poc.WithConnectTimeout(8), poc.WithRetryTimes(3), // 30s is enough for listing models
		poc.WithSave(false),
		poc.WithConnPool(true), // enable connection pool for better performance
		poc.WithAppendHeader("Accept-Encoding", "gzip, deflate, br"), // enable compression for better network performance
	)

	if strings.HasSuffix(url, "/") {
		// remove /
		url = url[:len(url)-1]
	}
	if strings.HasSuffix(url, "/chat/completions") {
		// remove /chat/completions
		url = url[:len(url)-len("/chat/completions")]
		url += "/models"
	} else if strings.HasSuffix(url, "/responses") {
		url = url[:len(url)-len("/responses")]
		url += "/models"
	}

	log.Infof("requtest GET to %v to find available models", url)
	rsp, _, err := poc.DoGET(url, opts...)
	if err != nil {
		return nil, utils.Errorf("request get to %v：%v", url, err)
	}

	// body like  {"object":"list","data":[{"id":"qwq:latest","object":"model","created":1741877931,"owned_by":"library"},{"id":"gemma3:27b","object":"model","created":1741875247,"owned_by":"library"},{"id":"deepseek-r1:32b","object":"model","created":1738946811,"owned_by":"library"},{"id":"deepseek-r1:70b","object":"model","created":1738939603,"owned_by":"library"},{"id":"qwen2.5:32b","object":"model","created":1727615210,"owned_by":"library"},{"id":"qwen2.5:latest","object":"model","created":1727613786,"owned_by":"library"}]}
	body := rsp.GetBody()
	if len(body) <= 0 {
		return nil, utils.Errorf("empty response")
	}

	var resp struct {
		Object string       `json:"object"`
		Data   []*ModelMeta `json:"data"`
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		return nil, utils.Errorf("unmarshal models failed: %v raw:\n%v", err, spew.Sdump(body))
	}

	return resp.Data, nil
}

type streamToStructuredStream struct {
	isReason bool
	id       func() int
	idInc    func()
	mutex    *sync.Mutex
	r        chan *StructuredData
}

func (s *streamToStructuredStream) Write(p []byte) (n int, err error) {
	if s.r == nil {
		return 0, utils.Error("streamToStructuredStream is not initialized")
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.idInc != nil {
		s.idInc()
	}
	id := s.id()

	data := &StructuredData{
		Id:           fmt.Sprint(id),
		Event:        "data",
		OutputText:   "",
		OutputReason: "",
	}
	if s.isReason {
		data.OutputReason = string(p)
	} else {
		data.OutputText = string(p)
	}
	s.r <- data
	return len(p), nil
}

func StructuredStreamBase(
	url string,
	model string,
	msg string,
	opt func() ([]poc.PocConfigOption, error),
	streamHandler func(io.Reader),
	reasonHandler func(io.Reader),
	errHandler func(error),
) (chan *StructuredData, error) {
	var schan = make(chan *StructuredData, 1000)
	id := 0
	getId := func() int {
		return id
	}
	idInc := func() {
		id++
	}
	m := new(sync.Mutex)
	go func() {
		defer close(schan)
		_, err := ChatBase(url, model, msg, WithChatBase_PoCOptions(opt), WithChatBase_StreamHandler(func(reader io.Reader) {
			structured := &streamToStructuredStream{
				isReason: false,
				id:       getId,
				idInc:    idInc,
				mutex:    m,
				r:        schan,
			}
			if streamHandler == nil {
				// read from reader
				io.Copy(structured, reader)
				return
			}
			// tee reader to mirror streamHandler
			r, w := utils.NewPipe()
			defer w.Close()
			newReader := io.TeeReader(reader, w)
			go func() { streamHandler(r) }()

			// read from newReader
			io.Copy(structured, newReader)
		}), WithChatBase_ReasonStreamHandler(func(reader io.Reader) {
			structured := &streamToStructuredStream{
				isReason: true,
				id:       getId,
				idInc:    idInc,
				mutex:    m,
				r:        schan,
			}
			if reasonHandler == nil {
				io.Copy(structured, reader)
				return
			}
			// tee reader to mirror streamHandler
			r, w := utils.NewPipe()
			defer w.Close()
			newReader := io.TeeReader(reader, w)
			go func() { streamHandler(r) }()
			// read from newReader
			io.Copy(structured, newReader)
		}), WithChatBase_ErrHandler(errHandler))
		if err != nil {
			log.Errorf("structured stream error: %v", err)
		}
	}()
	return schan, nil
}

type ImageDescription struct {
	Url string `json:"url"`
}

// VideoDescription 视频输入承载体，用于 Qwen Omni 等多模态模型 video_url 通路
// 关键词: VideoDescription, video_url, omni 视频输入
type VideoDescription struct {
	Url string `json:"url"`
}

type ChatBaseInterfaceType string

const (
	// ChatBaseInterfaceTypeChatCompletions uses OpenAI-compatible /v1/chat/completions format.
	ChatBaseInterfaceTypeChatCompletions ChatBaseInterfaceType = "chat_completions"
	// ChatBaseInterfaceTypeResponses uses /v1/responses format.
	ChatBaseInterfaceTypeResponses ChatBaseInterfaceType = "responses"
)

type ChatBaseContext struct {
	PoCOptionGenerator func() ([]poc.PocConfigOption, error)
	// ExtraBody is merged into the outbound JSON body (top-level keys) after marshaling msgResult.
	ExtraBody      map[string]any
	ThinkingBudget int64
	// 模型采样/推理参数（来自 ThirdPartyApplicationConfig / AIConfig，未设置则不写入请求体）
	MaxTokens           *int64
	Temperature         *float64
	TopP                *float64
	TopK                *int64
	FrequencyPenalty    *float64
	ReasoningEffort     string
	StreamHandler       func(io.Reader)
	ReasonStreamHandler func(reader io.Reader)
	ErrHandler          func(err error)
	// InterfaceType controls request/response protocol shape.
	// Default is chat_completions for backward compatibility.
	InterfaceType ChatBaseInterfaceType
	ImageUrls     []*ImageDescription
	// VideoUrls 视频输入列表，用于 Qwen Omni 等多模态模型
	// 关键词: VideoUrls, omni 视频输入通道
	VideoUrls []*VideoDescription
	// Modalities 输出模态，omni 模型必填（如 ["text"]）；其他模型可留空
	// 关键词: modalities, omni 模态
	Modalities    []string
	DisableStream bool
	// ToolCallCallback is called when the AI response contains tool_calls.
	// If set, tool_calls will NOT be converted to <|TOOL_CALL...|> format in the output stream.
	// If not set, the original behavior (converting to <|TOOL_CALL...|> format) is preserved.
	ToolCallCallback func([]*ToolCall)
	// Tools defines the available tools that the model may call
	Tools []Tool
	// ToolChoice controls which (if any) tool is called by the model
	ToolChoice any
	// RawHTTPResponseCallback is called after the AI HTTP response is fully consumed,
	// providing the raw HTTP response header and a body preview for debugging.
	RawHTTPResponseCallback func(headerBytes []byte, bodyPreview []byte)
	// RawHTTPResponseHeaderCallback is called as soon as the raw HTTP response header
	// is fully available, before the response body is consumed.
	RawHTTPResponseHeaderCallback RawHTTPResponseHeaderCallback
	// RawHTTPRequestResponseCallback is called after the AI HTTP response is fully consumed,
	// providing the raw request bytes, response debug data, and final usage info.
	RawHTTPRequestResponseCallback RawHTTPRequestResponseCallback
	// UsageCallback is invoked exactly once after the streaming response is
	// fully consumed, carrying the last non-empty `usage` block from the SSE
	// stream (OpenAI-compatible stream_options.include_usage=true). The
	// callback may receive nil if no usage block was observed.
	// 关键词: ChatBaseContext.UsageCallback, token 用量回调
	UsageCallback func(*ChatUsage)

	// RawMessages 用于完整透传客户端原始 messages 数组到上游 LLM。
	// 当 len(RawMessages) > 0 时，chatBaseChatCompletions 不再做「单 user 包装」，
	// 直接以本字段作为请求 messages 的基础；若同时存在 ImageUrls/VideoUrls
	//（例如 gateway.LoadOption 注入的 WithImageRaw），会合并进**最后一条 user**
	// 消息的 content（去重），与无 RawMessages 时的多模态顺序规则一致。
	//
	// 关键词: ChatBaseContext.RawMessages, messages 完整透传
	RawMessages []ChatDetail

	// MirrorCorrelationID 由 ChatBase 在 dispatchChatBaseMirror 后填充,
	// 取自 mirrorResult.MirrorCorrelationID. processAIResponse 在调用
	// UsageCallback 前会通过本字段把 ID 复制到 ChatUsage.MirrorCorrelationID.
	// 关键词: ChatBaseContext.MirrorCorrelationID, mirror dump usage 关联
	MirrorCorrelationID string
}

type ChatBaseOption func(c *ChatBaseContext)

func WithChatBase_InterfaceType(interfaceType ChatBaseInterfaceType) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.InterfaceType = interfaceType
	}
}

func WithChatBase_DisableStream(b bool) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.DisableStream = b
	}
}

func WithChatBase_ThinkingBudget(budget int64) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.ThinkingBudget = budget
	}
}

// WithChatBase_EnableThinkingEx merges a single top-level key into ExtraBody (e.g. custom reasoning_effort).
func WithChatBase_EnableThinkingEx(key string, value any) ChatBaseOption {
	return func(c *ChatBaseContext) {
		if key == "" {
			return
		}
		mergeChatBaseExtraBody(c, map[string]any{key: value})
	}
}

func mergeChatBaseExtraBody(c *ChatBaseContext, m map[string]any) {
	if len(m) == 0 {
		return
	}
	if c.ExtraBody == nil {
		c.ExtraBody = make(map[string]any, len(m))
	}
	for k, v := range m {
		c.ExtraBody[k] = v
	}
}

// ChatBaseThinkingOptions merges ThinkingExtraBodyForProvider into ExtraBody when cfg.EnableThinking != nil.
func ChatBaseThinkingOptions(cfg *AIConfig, resolvedTargetURL string) ChatBaseOption {
	return func(c *ChatBaseContext) {
		if cfg == nil || cfg.EnableThinking == nil {
			return
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = resolvedTargetURL
		}
		m := ThinkingExtraBodyForProvider(cfg.Type, cfg.Model, baseURL, cfg.Domain, *cfg.EnableThinking)
		mergeChatBaseExtraBody(c, m)
	}
}

// WithChatBase_AISamplingFromConfig copies optional model sampling fields from AIConfig
// into the chat request context (chat/completions JSON and responses API where applicable).
func WithChatBase_AISamplingFromConfig(cfg *AIConfig) ChatBaseOption {
	return func(c *ChatBaseContext) {
		if cfg == nil {
			return
		}
		c.MaxTokens = cloneInt64Ptr(cfg.MaxTokens)
		c.Temperature = cloneFloat64Ptr(cfg.Temperature)
		c.TopP = cloneFloat64Ptr(cfg.TopP)
		c.TopK = cloneInt64Ptr(cfg.TopK)
		c.FrequencyPenalty = cloneFloat64Ptr(cfg.FrequencyPenalty)
		if s := strings.TrimSpace(cfg.ReasoningEffort); s != "" {
			c.ReasoningEffort = s
		}
	}
}

func cloneInt64Ptr(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneFloat64Ptr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func WithChatBase_Function(b []any) ChatBaseOption {
	return func(c *ChatBaseContext) {
		//c.FS = b
	}
}

// WithChatBase_Tools sets the available tools for the model to call
func WithChatBase_Tools(tools []Tool) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.Tools = tools
	}
}

// WithChatBase_ToolChoice controls which tool is called by the model
// Can be "none", "auto", "required", or {"type": "function", "function": {"name": "my_function"}}
func WithChatBase_ToolChoice(choice any) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.ToolChoice = choice
	}
}

func WithChatBase_StreamHandler(b func(io.Reader)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.StreamHandler = b
	}
}

func WithChatBase_ReasonStreamHandler(b func(reader io.Reader)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.ReasonStreamHandler = b
	}
}

func WithChatBase_ErrHandler(b func(error)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.ErrHandler = b
	}
}

func WithChatBase_PoCOptions(b func() ([]poc.PocConfigOption, error)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.PoCOptionGenerator = b
	}
}

func WithChatBase_ImageRawInstance(images ...*ImageDescription) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.ImageUrls = append(c.ImageUrls, images...)
	}
}

// WithChatBase_VideoRawInstance 把 VideoDescription 添加到 ChatBaseContext，
// 用于把视频文件（http(s) URL 或 data:video/mp4;base64,... 形式）喂给 omni 类模型。
// 关键词: WithChatBase_VideoRawInstance, video_url 注入
func WithChatBase_VideoRawInstance(videos ...*VideoDescription) ChatBaseOption {
	return func(c *ChatBaseContext) {
		for _, v := range videos {
			if v == nil || v.Url == "" {
				continue
			}
			c.VideoUrls = append(c.VideoUrls, v)
		}
	}
}

// WithChatBase_Modalities 设置输出模态（omni 模型必填，例如 ["text"]）。
// 关键词: WithChatBase_Modalities, omni 输出模态
func WithChatBase_Modalities(modalities ...string) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.Modalities = append(c.Modalities, modalities...)
	}
}

// WithChatBase_ToolCallCallback sets a callback function that will be called when the AI response contains tool_calls.
// If set, tool_calls will NOT be converted to <|TOOL_CALL...|> format in the output stream.
// Instead, the callback will be invoked with the parsed ToolCall objects.
// If not set, the original behavior (converting to <|TOOL_CALL...|> format) is preserved for backward compatibility.
func WithChatBase_ToolCallCallback(cb func([]*ToolCall)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.ToolCallCallback = cb
	}
}

func WithChatBase_RawHTTPResponseCallback(cb func(headerBytes []byte, bodyPreview []byte)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.RawHTTPResponseCallback = cb
	}
}

func WithChatBase_RawHTTPResponseHeaderCallback(cb RawHTTPResponseHeaderCallback) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.RawHTTPResponseHeaderCallback = cb
	}
}

func WithChatBase_RawHTTPRequestResponseCallback(cb RawHTTPRequestResponseCallback) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.RawHTTPRequestResponseCallback = cb
	}
}

// WithChatBase_UsageCallback registers a callback that fires once after the
// AI streaming response is fully consumed, carrying the last `usage` block
// observed in the OpenAI-compatible SSE stream. nil is delivered if the
// upstream did not return a usage block at all.
//
// 关键词: WithChatBase_UsageCallback, ChatBase token 用量回调
func WithChatBase_UsageCallback(cb func(*ChatUsage)) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.UsageCallback = cb
	}
}

// WithChatBase_RawMessages 让 chatBaseChatCompletions 跳过「仅 prompt 的单 user 包装」，
// 直接使用调用方提供的 messages 作为请求体 messages；若同时存在 ImageUrls/VideoUrls，
// 仍会合并进最后一条 user（见 mergeRawMessagesWithGatewayMedia）。
// 配合 gateway.Chat(s) 中的 g.config.RawMessages 透传使用。
//
// 关键词: WithChatBase_RawMessages, messages 完整透传
func WithChatBase_RawMessages(msgs []ChatDetail) ChatBaseOption {
	return func(c *ChatBaseContext) {
		c.RawMessages = msgs
	}
}

func NewChatBaseContext(opts ...ChatBaseOption) *ChatBaseContext {
	ctx := &ChatBaseContext{
		InterfaceType: ChatBaseInterfaceTypeChatCompletions,
	}
	for _, i := range opts {
		i(ctx)
	}
	return ctx
}

func ChatBase(url string, model string, msg string, chatOpts ...ChatBaseOption) (string, error) {
	ctx := NewChatBaseContext(chatOpts...)
	// RawMessages 非空时，让 mirror observer 收到稳定的 messages 序列化字符串
	// 而不是 prompt 拍平字符串，保持 aicache 等观测者的前缀字节序与上游 LLM
	// 实际收到的 messages 数组一致，提升缓存命中率统计准确性。
	// 关键词: ChatBase mirror RawMessages 序列化, 镜像观测前缀对齐
	mirrorMsg := msg
	if len(ctx.RawMessages) > 0 {
		mirrorMsg = serializeRawMessagesForMirror(ctx.RawMessages)
	}
	// 同步分发 mirror observer，可能拿到 hijack 决策。仅当 caller 没显式
	// 给 RawMessages 时才允许 hijack 接管：caller 已构造好 messages 时尊
	// 重其意图，不二次猜测。
	// 关键词: ChatBase mirror hijack apply, RawMessages 优先级
	mirrorResult := dispatchChatBaseMirror(model, mirrorMsg)
	if len(ctx.RawMessages) == 0 && mirrorResult != nil && mirrorResult.IsHijacked && len(mirrorResult.Messages) > 0 {
		ctx.RawMessages = mirrorResult.Messages
	}
	// 把 mirror observer 自定义的关联 ID 透传到 ctx, 并用闭包包装 UsageCallback,
	// 让 SSE 末帧 usage 在抵达上层订阅者前自动盖上同一个 ID, 离线分析就能
	// 用 ID 在 mirror 落盘 (例如 aicache dump) 与 token usage 之间做精确 join.
	// 关键词: ChatBase mirror correlation plumb, UsageCallback wrap, dump usage 对齐
	if mirrorResult != nil && mirrorResult.MirrorCorrelationID != "" {
		ctx.MirrorCorrelationID = mirrorResult.MirrorCorrelationID
		correlationID := ctx.MirrorCorrelationID
		origUsageCallback := ctx.UsageCallback
		ctx.UsageCallback = func(usage *ChatUsage) {
			if usage != nil {
				usage.MirrorCorrelationID = correlationID
			}
			if origUsageCallback != nil {
				origUsageCallback(usage)
			}
		}
	}
	interfaceType := ctx.InterfaceType
	if interfaceType == ChatBaseInterfaceTypeChatCompletions && strings.HasSuffix(strings.TrimRight(strings.ToLower(url), "/"), "/responses") {
		interfaceType = ChatBaseInterfaceTypeResponses
	}
	switch interfaceType {
	case ChatBaseInterfaceTypeResponses:
		return chatBaseResponses(url, model, msg, ctx)
	default:
		return chatBaseChatCompletions(url, model, msg, ctx)
	}
}

// serializeRawMessagesForMirror 将 RawMessages 稳定序列化为 JSON 字符串，
// 用于 dispatchChatBaseMirror 的 msg 参数。失败时回退为空字符串。
//
// 实现选择 encoding/json：默认使用结构体 JSON tag 的字段顺序，多次序列化
// 同一输入产生相同字节序，满足 aicache 等观测者基于字符串前缀做 LCP 计算
// 的稳定性要求。
//
// 关键词: serializeRawMessagesForMirror, RawMessages JSON 序列化, mirror 前缀稳定
func serializeRawMessagesForMirror(msgs []ChatDetail) string {
	if len(msgs) == 0 {
		return ""
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		return ""
	}
	return string(b)
}

// dedupGatewayMedia 对 ctx 中的 VideoUrls/ImageUrls 做 URL 级去重，顺序保留首次出现。
// 关键词: RawMessages 合并 gateway 多模态, multimodal dedup
func dedupGatewayMedia(ctx *ChatBaseContext) (uniqVideos []*VideoDescription, uniqImages []*ImageDescription) {
	seenVideo := map[string]bool{}
	seenImage := map[string]bool{}
	for _, v := range ctx.VideoUrls {
		if v == nil || v.Url == "" || seenVideo[v.Url] {
			continue
		}
		seenVideo[v.Url] = true
		uniqVideos = append(uniqVideos, v)
	}
	for _, im := range ctx.ImageUrls {
		if im == nil || im.Url == "" || seenImage[im.Url] {
			continue
		}
		seenImage[im.Url] = true
		uniqImages = append(uniqImages, im)
	}
	return uniqVideos, uniqImages
}

func extractOpenAICompatMediaURL(v any) string {
	switch m := v.(type) {
	case map[string]any:
		if u, ok := m["url"].(string); ok {
			return u
		}
	case string:
		return m
	}
	return ""
}

func collectMediaURLsFromChatContents(arr []*ChatContent) (seenVideo, seenImage map[string]bool) {
	seenVideo, seenImage = map[string]bool{}, map[string]bool{}
	for _, c := range arr {
		if c == nil {
			continue
		}
		switch c.Type {
		case "image_url":
			if u := extractOpenAICompatMediaURL(c.ImageUrl); u != "" {
				seenImage[u] = true
			}
		case "video_url":
			if u := extractOpenAICompatMediaURL(c.VideoUrl); u != "" {
				seenVideo[u] = true
			}
		}
	}
	return seenVideo, seenImage
}

func filterNewGatewayVideos(uniq []*VideoDescription, seen map[string]bool) []*VideoDescription {
	out := make([]*VideoDescription, 0, len(uniq))
	for _, v := range uniq {
		if v == nil || v.Url == "" || seen[v.Url] {
			continue
		}
		out = append(out, v)
	}
	return out
}

func filterNewGatewayImages(uniq []*ImageDescription, seen map[string]bool) []*ImageDescription {
	out := make([]*ImageDescription, 0, len(uniq))
	for _, im := range uniq {
		if im == nil || im.Url == "" || seen[im.Url] {
			continue
		}
		out = append(out, im)
	}
	return out
}

func resolveRawMergeUserText(existing string, legacyMsg string, hasVid bool, hasImg bool) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	if strings.TrimSpace(legacyMsg) != "" {
		return legacyMsg
	}
	if hasVid {
		return "请描述视频内容"
	}
	if hasImg {
		return "请描述图片内容"
	}
	return ""
}

// buildGatewayMultimodalUserParts 与 chatBaseChatCompletions 非 Raw 分支一致：
// 有视频时 video → image → text；仅图时 text → image。
func buildGatewayMultimodalUserParts(uniqVideos []*VideoDescription, uniqImages []*ImageDescription, text string) []*ChatContent {
	var contents []*ChatContent
	if len(uniqVideos) > 0 {
		for _, video := range uniqVideos {
			contents = append(contents, NewUserChatContentVideoUrl(video.Url))
		}
		for _, image := range uniqImages {
			contents = append(contents, NewUserChatContentImageUrl(image.Url))
		}
		contents = append(contents, NewUserChatContentText(text))
	} else {
		contents = append(contents, NewUserChatContentText(text))
		for _, image := range uniqImages {
			contents = append(contents, NewUserChatContentImageUrl(image.Url))
		}
	}
	return contents
}

func mergeContentArrayWithGateway(arr []*ChatContent, newVideos []*VideoDescription, newImages []*ImageDescription) []*ChatContent {
	if len(newVideos) > 0 {
		prefix := make([]*ChatContent, 0, len(newVideos)+len(newImages))
		for _, v := range newVideos {
			if v != nil && v.Url != "" {
				prefix = append(prefix, NewUserChatContentVideoUrl(v.Url))
			}
		}
		for _, im := range newImages {
			if im != nil && im.Url != "" {
				prefix = append(prefix, NewUserChatContentImageUrl(im.Url))
			}
		}
		return append(prefix, arr...)
	}
	out := make([]*ChatContent, 0, len(arr)+len(newImages))
	out = append(out, arr...)
	for _, im := range newImages {
		if im != nil && im.Url != "" {
			out = append(out, NewUserChatContentImageUrl(im.Url))
		}
	}
	return out
}

// mergeRawMessagesWithGatewayMedia 在保留 RawMessages 对话结构的前提下，把 gateway 注入的
// ImageUrls/VideoUrls 并入最后一条 user（无 user 则追加一条 user）。
// 返回值 hasVideos/hasImages 表示**合并后**本条请求是否带对应模态，供 stream_options 等逻辑使用。
// 关键词: RawMessages ImageUrls 合并, gateway 多模态透传
func mergeRawMessagesWithGatewayMedia(msgs []ChatDetail, legacyMsg string, ctx *ChatBaseContext) ([]ChatDetail, bool, bool) {
	uniqVideos, uniqImages := dedupGatewayMedia(ctx)
	if len(uniqVideos) == 0 && len(uniqImages) == 0 {
		return msgs, false, false
	}

	out := make([]ChatDetail, len(msgs))
	copy(out, msgs)

	lastUser := -1
	for i := len(out) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(out[i].Role), "user") {
			lastUser = i
			break
		}
	}

	if lastUser < 0 {
		text := resolveRawMergeUserText("", legacyMsg, len(uniqVideos) > 0, len(uniqImages) > 0)
		parts := buildGatewayMultimodalUserParts(uniqVideos, uniqImages, text)
		out = append(out, NewUserChatDetailEx(parts))
		return out, len(uniqVideos) > 0, len(uniqImages) > 0
	}

	d := out[lastUser]
	switch content := d.Content.(type) {
	case string:
		seenV, seenI := map[string]bool{}, map[string]bool{}
		newV := filterNewGatewayVideos(uniqVideos, seenV)
		newI := filterNewGatewayImages(uniqImages, seenI)
		if len(newV) == 0 && len(newI) == 0 {
			return msgs, false, false
		}
		text := resolveRawMergeUserText(content, legacyMsg, len(newV) > 0, len(newI) > 0)
		parts := buildGatewayMultimodalUserParts(newV, newI, text)
		d.Content = parts
		out[lastUser] = d
		return out, len(newV) > 0, len(newI) > 0

	case []*ChatContent:
		seenV, seenI := collectMediaURLsFromChatContents(content)
		newV := filterNewGatewayVideos(uniqVideos, seenV)
		newI := filterNewGatewayImages(uniqImages, seenI)
		if len(newV) == 0 && len(newI) == 0 {
			return msgs, false, false
		}
		d.Content = mergeContentArrayWithGateway(content, newV, newI)
		out[lastUser] = d
		return out, len(newV) > 0, len(newI) > 0

	default:
		text := resolveRawMergeUserText("", legacyMsg, len(uniqVideos) > 0, len(uniqImages) > 0)
		parts := buildGatewayMultimodalUserParts(uniqVideos, uniqImages, text)
		out = append(out, NewUserChatDetailEx(parts))
		return out, len(uniqVideos) > 0, len(uniqImages) > 0
	}
}

func chatBaseChatCompletions(url string, model string, msg string, ctx *ChatBaseContext) (string, error) {
	var msgs []ChatDetail
	hasImages := len(ctx.ImageUrls) > 0
	hasVideos := len(ctx.VideoUrls) > 0
	// RawMessages 优先：透传对话结构；若 gateway 另注入了 ImageUrls/VideoUrls，
	// 合并进最后一条 user，避免 RawMessages 路径丢失多模态输入。
	// 关键词: chatBaseChatCompletions RawMessages 分支, messages 完整透传
	if len(ctx.RawMessages) > 0 {
		msgs = append(msgs, ctx.RawMessages...)
		if len(ctx.ImageUrls) > 0 || len(ctx.VideoUrls) > 0 {
			msgs, hasVideos, hasImages = mergeRawMessagesWithGatewayMedia(msgs, msg, ctx)
		} else {
			hasImages = false
			hasVideos = false
		}
	} else if !hasImages && !hasVideos {
		msgs = append(msgs, NewUserChatDetail(msg))
	} else {
		var contents []*ChatContent
		if msg == "" {
			if hasVideos {
				// 视频默认提示，用户可通过 query 自行覆盖
				// 关键词: omni 视频默认提示
				msg = "请描述视频内容"
			} else {
				msg = "请描述图片内容"
			}
		}
		// 拼装顺序: 通义 omni 官方示例要求 video_url 出现在 text 之前，
		// 否则会报 "Multiple inputs of the same modality or mixed modality inputs are currently not applicable to the omni model"。
		// image_url 路径保持原"text 在前"的顺序，避免影响既有图像通路行为。
		// 关键词: 多模态 content 拼装顺序, omni video_url 顺序
		// 注意: NewDefaultAIConfig 内部会对 opts 应用两次（先 type 后 user opts），
		// 加上 ai.Chat -> legacyChat -> gateway.LoadOption 三层各调一次，
		// 同一个 video/image 可能被 append 多次，会触发 omni 模型 "Multiple inputs of the same modality" 报错。
		// 这里在拼装 content 时按 URL 去重，确保单视频单图像的稳定输出。
		// 关键词: omni 多模态去重, multimodal dedup
		seenVideo := map[string]bool{}
		seenImage := map[string]bool{}
		uniqVideos := make([]*VideoDescription, 0, len(ctx.VideoUrls))
		for _, v := range ctx.VideoUrls {
			if v == nil || v.Url == "" || seenVideo[v.Url] {
				continue
			}
			seenVideo[v.Url] = true
			uniqVideos = append(uniqVideos, v)
		}
		uniqImages := make([]*ImageDescription, 0, len(ctx.ImageUrls))
		for _, im := range ctx.ImageUrls {
			if im == nil || im.Url == "" || seenImage[im.Url] {
				continue
			}
			seenImage[im.Url] = true
			uniqImages = append(uniqImages, im)
		}
		hasVideos = len(uniqVideos) > 0
		hasImages = len(uniqImages) > 0

		if hasVideos {
			for _, video := range uniqVideos {
				contents = append(contents, NewUserChatContentVideoUrl(video.Url))
			}
			for _, image := range uniqImages {
				contents = append(contents, NewUserChatContentImageUrl(image.Url))
			}
			contents = append(contents, NewUserChatContentText(msg))
		} else {
			contents = append(contents, NewUserChatContentText(msg))
			for _, image := range uniqImages {
				contents = append(contents, NewUserChatContentImageUrl(image.Url))
			}
		}
		msgs = append(msgs, NewUserChatDetailEx(contents))
	}
	msgIns := NewChatMessage(model, msgs)
	msgIns.Stream = !ctx.DisableStream
	msgIns.MaxTokens = ctx.MaxTokens
	msgIns.Temperature = ctx.Temperature
	msgIns.TopP = ctx.TopP
	msgIns.TopK = ctx.TopK
	msgIns.FrequencyPenalty = ctx.FrequencyPenalty
	if ctx.ReasoningEffort != "" {
		msgIns.ReasoningEffort = ctx.ReasoningEffort
	}

	// Add tools if provided
	if len(ctx.Tools) > 0 {
		msgIns.Tools = ctx.Tools
	}
	if ctx.ToolChoice != nil {
		msgIns.ToolChoice = ctx.ToolChoice
	}

	// 透传 modalities；视频通路下若用户未显式设置，自动补 ["text"]
	// 关键词: omni modalities 默认值, video_url 自动 modalities
	if len(ctx.Modalities) > 0 {
		msgIns.Modalities = ctx.Modalities
	} else if hasVideos {
		msgIns.Modalities = []string{"text"}
	}

	// stream_options.include_usage=true 自动注入条件（OpenAI / dashscope omni 规范）：
	//   1) 视频通路（dashscope omni 强制要求）
	//   2) 上层注册了 ctx.UsageCallback —— 调用方明确想要 token 用量
	//      （包括 prompt_tokens、cached_tokens 等隐式缓存命中信息）
	// 任一条件满足且当前为流式请求时即注入；保持已设的 StreamOptions 不被覆盖。
	// 关键词: stream_options, include_usage 自动注入, UsageCallback 触发, cached_tokens 暴露
	if msgIns.Stream && (hasVideos || ctx.UsageCallback != nil) {
		if msgIns.StreamOptions == nil {
			msgIns.StreamOptions = map[string]any{"include_usage": true}
		} else if _, ok := msgIns.StreamOptions["include_usage"]; !ok {
			msgIns.StreamOptions["include_usage"] = true
		}
	}

	return executeChatBaseRequest(url, msgIns.Stream, msgIns, ctx, appendStreamHandlerPoCOptionEx)
}

func chatBaseResponses(url string, model string, msg string, ctx *ChatBaseContext) (string, error) {
	stream := !ctx.DisableStream
	// RawMessages 优先：当 caller 或上游 hijack 已经构造好结构化 messages 时，
	// 走 RawMessages 透传分支，把 OpenAI chat-completions 风格 ChatDetail
	// 列表映射为 OpenAI responses 协议的 input 数组，保持 role 边界，
	// 让上游隐式缓存可识别 system/user 边界。
	// 关键词: chatBaseResponses RawMessages 透传, system/user 边界保留
	var input any
	if len(ctx.RawMessages) > 0 {
		rmsgs := append([]ChatDetail(nil), ctx.RawMessages...)
		if len(ctx.ImageUrls) > 0 || len(ctx.VideoUrls) > 0 {
			rmsgs, _, _ = mergeRawMessagesWithGatewayMedia(rmsgs, msg, ctx)
		}
		input = convertChatDetailsToResponsesInput(rmsgs)
	} else {
		input = buildResponsesInput(msg, ctx.ImageUrls)
	}
	req := map[string]any{
		"model":  model,
		"input":  input,
		"stream": stream,
	}
	if ctx.MaxTokens != nil {
		req["max_output_tokens"] = *ctx.MaxTokens
	}
	if ctx.Temperature != nil {
		req["temperature"] = *ctx.Temperature
	}
	if ctx.TopP != nil {
		req["top_p"] = *ctx.TopP
	}
	if strings.TrimSpace(ctx.ReasoningEffort) != "" {
		req["reasoning"] = map[string]any{"effort": strings.TrimSpace(ctx.ReasoningEffort)}
	}

	tools := convertToolsToResponses(ctx.Tools)
	if len(tools) > 0 {
		req["tools"] = tools
	}
	if ctx.ToolChoice != nil {
		req["tool_choice"] = convertToolChoiceToResponses(ctx.ToolChoice)
	}

	// stream_options.include_usage=true 自动注入：与 chatBaseChatCompletions 保持一致，
	// 当上层注册了 ctx.UsageCallback 且当前为流式请求时注入，让上游 /responses 端点
	// 在末帧返回 usage（含 prompt_tokens_details.cached_tokens）。
	// dashscope OpenAI /responses 兼容端点与 OpenAI 官方均识别该字段，未识别的上游
	// 会忽略该字段不影响请求。
	// 关键词: chatBaseResponses include_usage 注入, /responses cached_tokens 一致性
	if stream && ctx.UsageCallback != nil {
		streamOpts, _ := req["stream_options"].(map[string]any)
		if streamOpts == nil {
			streamOpts = map[string]any{}
		}
		if _, ok := streamOpts["include_usage"]; !ok {
			streamOpts["include_usage"] = true
		}
		req["stream_options"] = streamOpts
	}

	return executeChatBaseRequest(url, stream, req, ctx, appendResponsesStreamHandlerPoCOptionEx)
}

// convertChatDetailsToResponsesInput 把 OpenAI chat-completions 风格的
// []ChatDetail 转成 OpenAI responses 协议的 input 数组。
//
// 字符串 content 渲染成 [{type:"input_text", text:...}]；[]*ChatContent
// 类型按各自 type 字段分别映射为 input_text / input_image / input_video。
//
// 关键词: convertChatDetailsToResponsesInput, ChatDetail 转 responses input
func convertChatDetailsToResponsesInput(msgs []ChatDetail) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == "" {
			role = "user"
		}
		var content []map[string]any
		switch v := m.Content.(type) {
		case string:
			content = append(content, map[string]any{
				"type": "input_text",
				"text": v,
			})
		case []*ChatContent:
			for _, c := range v {
				if c == nil {
					continue
				}
				switch c.Type {
				case "text":
					content = append(content, map[string]any{
						"type": "input_text",
						"text": c.Text,
					})
				case "image_url":
					content = append(content, map[string]any{
						"type":      "input_image",
						"image_url": c.ImageUrl,
					})
				case "video_url":
					content = append(content, map[string]any{
						"type":      "input_video",
						"video_url": c.VideoUrl,
					})
				default:
					if c.Text != "" {
						content = append(content, map[string]any{
							"type": "input_text",
							"text": c.Text,
						})
					}
				}
			}
		default:
			content = append(content, map[string]any{
				"type": "input_text",
				"text": utils.InterfaceToString(m.Content),
			})
		}
		if len(content) == 0 {
			content = append(content, map[string]any{
				"type": "input_text",
				"text": "",
			})
		}
		out = append(out, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func buildResponsesInput(msg string, images []*ImageDescription) []map[string]any {
	var content []map[string]any
	if len(images) > 0 {
		if msg == "" {
			msg = "请描述图片内容"
		}
	}
	if msg != "" {
		content = append(content, map[string]any{
			"type": "input_text",
			"text": msg,
		})
	}
	for _, image := range images {
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": image.Url,
		})
	}
	if len(content) == 0 {
		content = append(content, map[string]any{
			"type": "input_text",
			"text": "",
		})
	}
	return []map[string]any{
		{
			"role":    "user",
			"content": content,
		},
	}
}

func convertToolsToResponses(tools []Tool) []any {
	if len(tools) == 0 {
		return nil
	}
	results := make([]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" && tool.Function.Name != "" {
			t := map[string]any{
				"type":        "function",
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
			}
			if tool.Function.Parameters != nil {
				t["parameters"] = tool.Function.Parameters
			}
			results = append(results, t)
			continue
		}
		raw, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		var rawMap map[string]any
		if err := json.Unmarshal(raw, &rawMap); err != nil {
			continue
		}
		results = append(results, rawMap)
	}
	return results
}

func convertToolChoiceToResponses(choice any) any {
	choiceMap := utils.InterfaceToGeneralMap(choice)
	if len(choiceMap) == 0 {
		return choice
	}
	if utils.MapGetString(choiceMap, "type") != "function" {
		return choice
	}
	funcMap := utils.MapGetMapRaw(choiceMap, "function")
	name := utils.MapGetString(funcMap, "name")
	if name == "" {
		return choice
	}
	return map[string]any{
		"type": "function",
		"name": name,
	}
}

type chatBaseStreamHandlerAppender func(
	isStream bool,
	opts []poc.PocConfigOption,
	toolCallCallback func([]*ToolCall),
	rawResponseHeaderCallback RawHTTPResponseHeaderCallback,
	rawResponseCallback func([]byte, []byte, *ChatUsage),
	usageCallback func(*ChatUsage),
) (io.Reader, io.Reader, []poc.PocConfigOption, func())

func executeChatBaseRequest(
	url string,
	handleStream bool,
	msgResult any,
	ctx *ChatBaseContext,
	appendHandler chatBaseStreamHandlerAppender,
) (string, error) {
	if ctx.PoCOptionGenerator == nil {
		return "", utils.Error("build config failed: poc option generator is nil")
	}
	opts, err := ctx.PoCOptionGenerator()
	if err != nil {
		return "", utils.Errorf("build config failed: %v", err)
	}

	payload := msgResult
	var raw []byte
	if len(ctx.ExtraBody) > 0 {
		raw, err = json.Marshal(msgResult)
		if err != nil {
			return "", utils.Errorf("marshal msg[%v] to json failed: %s", spew.Sdump(msgResult), err)
		}
		msgMap := make(map[string]any)
		err = json.Unmarshal(raw, &msgMap)
		if err != nil {
			return "", utils.Errorf("unmarshal msg[%v] to map failed: %s", string(raw), err)
		}
		for k, v := range ctx.ExtraBody {
			msgMap[k] = v
		}
		payload = msgMap
	}

	raw, err = json.Marshal(payload)
	if err != nil {
		return "", utils.Errorf("build msg[%v] to json failed: %s", string(raw), err)
	}
	if log.GetLevel() <= log.DebugLevel {
		log.Debugf("ChatBase request body preview: %s", utils.ShrinkString(string(raw), 600))
	}
	// Set default options BEFORE user options, so user can override
	defaultOpts := []poc.PocConfigOption{
		poc.WithConnectTimeout(5),
		poc.WithTimeout(600), // 10 minutes max timeout to prevent goroutine leak (can be overridden by user)
		poc.WithRetryTimes(3),
		poc.WithSave(false),
		poc.WithConnPool(true), // enable connection pool for better performance
		poc.WithAppendHeader("Accept-Encoding", "gzip, deflate, br"), // enable compression for better network performance
	}
	opts = append(defaultOpts, opts...) // User options come AFTER defaults, so they can override
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))

	var pr, reasonPr io.Reader
	var cancel func()
	var requestPacket []byte
	rawResponseCallback := func(headerBytes []byte, bodyPreview []byte, usageInfo *ChatUsage) {
		if ctx.RawHTTPResponseCallback != nil {
			ctx.RawHTTPResponseCallback(headerBytes, bodyPreview)
		}
		if ctx.RawHTTPRequestResponseCallback != nil {
			if usageInfo != nil && ctx.MirrorCorrelationID != "" {
				usageInfo.MirrorCorrelationID = ctx.MirrorCorrelationID
			}
			ctx.RawHTTPRequestResponseCallback(requestPacket, headerBytes, bodyPreview, usageInfo)
		}
	}
	pr, reasonPr, opts, cancel = appendHandler(handleStream, opts, ctx.ToolCallCallback, ctx.RawHTTPResponseHeaderCallback, rawResponseCallback, ctx.UsageCallback)
	requestPacket = poc.BuildRequest(
		lowhttp.UrlToRequestPacket(
			"POST",
			url,
			nil,
			strings.HasPrefix(strings.ToLower(url), "https://"),
		),
		opts...,
	)
	wg := new(sync.WaitGroup)

	// 统一处理reasoning stream handler
	noMerge := ctx.ReasonStreamHandler != nil

	// 启动reasoning处理协程（如果需要）
	if ctx.ReasonStreamHandler != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("reasonStreamHandler panic: %v", err)
				}
			}()
			ctx.ReasonStreamHandler(reasonPr)
		}()
	}

	// 设置流式处理handler（如果需要）
	var body bytes.Buffer
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("streamHandler panic: %v", err)
			}
		}()

		var streamReader io.Reader
		if noMerge {
			streamReader = io.TeeReader(pr, &body)
		} else {
			// 合并模式：将reasoning包装为<think>标签
			result := mergeReasonIntoOutputStream(reasonPr, pr)
			streamReader = io.TeeReader(result, &body)
		}

		if ctx.StreamHandler != nil {
			ctx.StreamHandler(streamReader)
		} else {
			utils.Debug(func() {
				streamReader = io.TeeReader(streamReader, os.Stdout)
			})
			io.Copy(io.Discard, streamReader)
		}
	}()

	_, _, err = poc.DoPOST(url, opts...)
	if err != nil {
		if ctx.ErrHandler != nil {
			ctx.ErrHandler(err)
		}
		if !utils.IsNil(cancel) {
			cancel()
		}
		wg.Wait() // 确保在错误情况下也等待goroutine完成
		return body.String(), utils.Errorf("request post to %v：%v", url, err)
	}

	// 等待所有goroutine完成数据写入，确保body.Buffer中有完整数据
	wg.Wait()
	return body.String(), nil
}

func ExtractFromResult(result string, fields map[string]any) (map[string]any, error) {
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sampleField := keys[0]

	stdjsons, raw := jsonextractor.ExtractJSONWithRaw(result)
	for _, stdjson := range stdjsons {
		var rawMap = make(map[string]any)
		err := json.Unmarshal([]byte(stdjson), &rawMap)
		if err != nil {
			fmt.Println(string(stdjson))
			log.Errorf("parse failed: %v", err)
			continue
		}
		_, ok := rawMap[sampleField]
		if ok {
			return rawMap, nil
		}
	}

	var err error
	for _, rawJson := range raw {
		stdjson := jsonextractor.FixJson([]byte(rawJson))
		var rawMap = make(map[string]any)
		err = json.Unmarshal([]byte(stdjson), &rawMap)
		if err != nil {
			fmt.Println(string(stdjson))
			log.Errorf("parse failed: %v", err)
			continue
		}
		_, ok := rawMap[sampleField]
		if ok {
			return rawMap, nil
		}
	}

	if strings.Contains(result, "，") {
		return ExtractFromResult(strings.ReplaceAll(result, "，", ","), fields)
	}

	return nil, utils.Errorf("cannot extractjson: \n%v\n", string(result))
}

func GenerateJSONPrompt(msg string, fields map[string]any) string {
	// 按字母序排列字段
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fieldsDesc strings.Builder
	for i, k := range keys {
		fieldsDesc.WriteString(fmt.Sprintf("%d. 字段名：%#v, 含义：%#v;\n", i+1, k, fields[k]))
	}

	return `# 指令
你是一个专业的数据处理助手，请严格按以下要求处理输入内容：

## 处理步骤
1. 直接提取或总结所需数据
2. 必须使用JSON格式输出
3. 不要包含推理过程
4. 不要添加额外解释

## 输入内容
` + strconv.Quote(msg) + `

## 字段定义
` + fieldsDesc.String() + `

## 输出要求
- 使用严格JSON格式（无Markdown代码块）
- 确保类型正确：
* 数值类型：不要加引号
* 字符串类型：必须加双引号
* 空值返回null
- 示例格式：
{"field1":123,"field2":"text","field3":null}

请直接输出处理后的JSON：`
}

func ChatBasedExtractData(
	url string, model string, msg string, fields map[string]any, opt func() ([]poc.PocConfigOption, error),
	streamHandler func(io.Reader),
	reasonHandler func(io.Reader),
	httpErrorHandler func(error),
	images ...*ImageDescription,
) (map[string]any, error) {
	if len(fields) <= 0 {
		return nil, utils.Error("no fields config for extract")
	}

	if fields == nil || len(fields) <= 0 {
		fields = make(map[string]any)
		fields["raw_data"] = "相关数据"
	}
	msg = GenerateJSONPrompt(msg, fields)
	result, err := ChatBase(
		url, model, msg,
		WithChatBase_PoCOptions(opt),
		WithChatBase_StreamHandler(streamHandler),
		WithChatBase_ReasonStreamHandler(reasonHandler),
		WithChatBase_ErrHandler(httpErrorHandler),
		WithChatBase_ImageRawInstance(images...))
	if err != nil {
		log.Errorf("chatbase error: %s", err)
		return nil, err
	}
	result = strings.ReplaceAll(result, "`", "")
	return ExtractFromResult(result, fields)
}
