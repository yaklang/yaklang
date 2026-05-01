package aispec

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

type ChatMessage struct {
	Model          string       `json:"model"`
	Messages       []ChatDetail `json:"messages"`
	Stream         bool         `json:"stream"`
	EnableThinking bool         `json:"enable_thinking,omitempty"`
	// Tools defines the available tools that the model may call
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice controls which (if any) tool is called by the model
	// Can be "none", "auto", "required", or {"type": "function", "function": {"name": "my_function"}}
	ToolChoice any `json:"tool_choice,omitempty"`
	// Modalities 用于 Qwen Omni 等多模态模型声明输出模态，
	// 例如 ["text"] 或 ["text","audio"]；非 omni 模型可不设置（omitempty）。
	// 关键词: modalities, omni 输出模态
	Modalities []string `json:"modalities,omitempty"`
	// StreamOptions 用于 omni 模型必填项 stream_options.include_usage=true，
	// 非 omni 模型可不设置（omitempty）。
	// 关键词: stream_options, include_usage
	StreamOptions map[string]any `json:"stream_options,omitempty"`
}

// Tool represents a tool that the model may call
type Tool struct {
	Type     string       `json:"type"` // Currently only "function" is supported
	Function ToolFunction `json:"function"`
}

// ToolFunction represents the function details for a tool
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Parameters is a JSON Schema object describing the function parameters
	Parameters any `json:"parameters,omitempty"`
}

type ChatDetail struct {
	Role         string        `json:"role"`
	Name         string        `json:"name,omitempty"`
	Content      any           `json:"content"`
	ToolCalls    []*ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
}

type ChatContent struct {
	Type     string `json:"type"` // text / image_url / video_url
	Text     string `json:"text,omitempty"`
	ImageUrl any    `json:"image_url,omitempty"`
	// VideoUrl 用于 Qwen Omni 等多模态模型直接喂入视频文件
	// 关键词: video_url, omni 视频输入
	VideoUrl any `json:"video_url,omitempty"`
}

type ChatDetails []ChatDetail

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}
type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type ChatCompletion struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Choices []ChatChoice `json:"choices"`
	Usage   ChatUsage    `json:"usage"`
	Error   *ChatError   `json:"error,omitempty"`
}

type ChatChoice struct {
	Index        int        `json:"index"`
	Message      ChatDetail `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`

	// PromptTokensDetails 来自 dashscope omni 在 SSE 末帧 usage 字段附带的多模态拆分。
	// dashscope qwen3.5-omni-plus 输入按"文本/图片/视频帧"与"音频"两类不同价格计费，
	// 因此把这里的 audio_tokens 与 video/image_tokens 单独保留以做精确成本核算。
	// 关键词: 多模态 token 拆分, prompt_tokens_details, dashscope omni 计费
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// PromptTokensDetails 多模态输入 token 拆分（仅 dashscope omni / openai 多模态返回）。
// 字段名沿用 dashscope SSE 帧里的下划线命名 (text_tokens / audio_tokens / image_tokens / video_tokens)。
// 关键词: 多模态 token 拆分明细
type PromptTokensDetails struct {
	TextTokens   int `json:"text_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	ImageTokens  int `json:"image_tokens,omitempty"`
	VideoTokens  int `json:"video_tokens,omitempty"`
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type ToolCall struct {
	// Index is used in streaming responses to identify which tool call this delta belongs to
	// In non-streaming responses, the array order itself serves as the implicit index
	Index    int        `json:"index,omitempty"`
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function FuncReturn `json:"function"`
}

type FuncReturn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ! 已弃用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param"`
	Code    string `json:"code"`
}

func NewChatMessage(model string, messages []ChatDetail, dummy ...any) *ChatMessage {
	return &ChatMessage{
		Model:    model,
		Messages: messages,
	}
}

// userMessage 根据传入的内容构造并返回一个 OpenAI 用户信息
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.systemMessage("The weather in Boston is 72 degrees and sunny."),
// openai.userMessage("What is the weather like today?"),
// ],
// )~
// ```
func NewUserChatDetail(content string) ChatDetail {
	return ChatDetail{
		Role:    "user",
		Content: content,
	}
}

func NewUserChatContentText(i string) *ChatContent {
	return &ChatContent{
		Type: "text",
		Text: i,
	}
}

func NewUserChatContentImageUrl(u string) *ChatContent {
	return &ChatContent{
		Type: "image_url",
		ImageUrl: map[string]any{
			"url": u,
		},
	}
}

// NewUserChatContentVideoUrl 构造 video_url 类型的 ChatContent，用于 Qwen Omni 等模型。
// 关键词: video_url, omni 视频输入
func NewUserChatContentVideoUrl(u string) *ChatContent {
	return &ChatContent{
		Type: "video_url",
		VideoUrl: map[string]any{
			"url": u,
		},
	}
}

func NewUserChatDetailEx(content any) ChatDetail {
	return ChatDetail{
		Role:    "user",
		Content: content,
	}
}

// assistantMessage 根据传入的内容构造并返回一个 OpenAI 助手信息
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.userMessage("What is the weather like today?"),
// openai.assistantMessage("72 degrees and sunny."),
// openai.userMessage("What will the temperature be tomorrow?"),
// ],
// )~
// ```
func NewAIChatDetail(content string) ChatDetail {
	return ChatDetail{
		Role:    "assistant",
		Content: content,
	}
}

// systemMessage 根据传入的内容构造并返回一个 OpenAI 系统信息
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.systemMessage("The weather in Boston is 72 degrees and sunny."),
// openai.userMessage("What is the weather like today?"),
// ],
// )~
// ```
func NewSystemChatDetail(content string) ChatDetail {
	return ChatDetail{
		Role:    "system",
		Content: content,
	}
}

// toolMessage 根据传入的函数名,内容构造并返回一个 OpenAI 工具信息，用于指示工具返回结果
// Example:
// ```
// session = openai.NewSession(
// openai.proxy("http://127.0.0.1:7890")
// )
// result = session.Chat(openai.userMessage("What is the weather like in Boston?"),
// openai.newFunction(
// "get_current_weather",
// "Get the current weather in a given location",
// openai.functionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
// openai.functionRequired("location"),
// ),
// )~
// result = session.Chat(openai.toolMessage("get_current_weather", `{"degree":72,"weather":"sunny"}`))~
// println(result.String())
// ```
func NewToolChatDetail(name, content string) ChatDetail {
	return ChatDetail{
		Role:    "tool",
		Name:    name,
		Content: content,
	}
}

// toolMessageWithID 根据传入的ID,函数名,内容构造并返回一个 OpenAI 工具信息，用于指示工具返回结果
// Example:
// ```
// session = openai.NewSession(
// openai.proxy("http://127.0.0.1:7890")
// )
// result = session.Chat(openai.userMessage("What is the weather like in Boston?"),
// openai.newFunction(
// "get_current_weather",
// "Get the current weather in a given location",
// openai.functionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
// openai.functionRequired("location"),
// ),
// )~
// result = session.Chat(openai.toolMessage("get_current_weather", `{"degree":72,"weather":"sunny"}`))~
// println(result.String())
// ```
func NewToolChatDetailWithID(id, name, content string) ChatDetail {
	return ChatDetail{
		Role:       "tool",
		Name:       name,
		ToolCallID: id,
		Content:    content,
	}
}

func (t *ToolCall) Clone() *ToolCall {
	if t == nil {
		return nil
	}
	return &ToolCall{
		ID:       t.ID,
		Type:     t.Type,
		Function: t.Function,
	}
}

func (f *FunctionCall) Clone() *FunctionCall {
	if f == nil {
		return nil
	}
	return &FunctionCall{
		Name:      f.Name,
		Arguments: f.Arguments,
	}
}

func (detail ChatDetail) Clone() ChatDetail {
	return ChatDetail{
		Role:         detail.Role,
		Name:         detail.Name,
		Content:      detail.Content,
		ToolCalls:    lo.Map(detail.ToolCalls, func(tool *ToolCall, _ int) *ToolCall { return tool.Clone() }),
		ToolCallID:   detail.ToolCallID,
		FunctionCall: detail.FunctionCall.Clone(),
	}
}

func (details ChatDetails) Clone() ChatDetails {
	return lo.Map(details, func(detail ChatDetail, _ int) ChatDetail { return detail.Clone() })
}

// ChatMessages 返回一个 ChatDetail 切片
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.userMessage("What is the weather like today?"),
// openai.assistantMessage("72 degrees and sunny."),
// openai.userMessage("What will the temperature be tomorrow?"),
// ],
// )~
// println(d.ChatMessages())
// ```
func (details ChatDetails) ChatMessages() []ChatDetail {
	return details
}

// String 返回消息切片中包含的所有消息
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.userMessage("What is the weather like today?"),
// openai.assistantMessage("72 degrees and sunny."),
// openai.userMessage("What will the temperature be tomorrow?"),
// ],
// )~
// println(d.String())
// ```
func (details ChatDetails) String() string {
	return DetailsToString(details)
}

// String 返回第一个消息
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.userMessage("What is the weather like today?"),
// openai.assistantMessage("72 degrees and sunny."),
// openai.userMessage("What will the temperature be tomorrow?"),
// ],
// )~
// println(d.String())
// ```
func (details ChatDetails) FirstString() string {
	return DetailsToString([]ChatDetail{details[0]})
}

// FunctionCallResult 返回函数调用的结果
// Example:
// ```
// d = openai.ChatEx(
// [
// openai.userMessage("What is the weather like in Boston?")
// ],
// openai.newFunction(
// "get_current_weather",
// "Get the current weather in a given location",
// openai.functionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
// openai.functionRequired("location"),
// ),
// openai.proxy("http://127.0.0.1:7890"),
// )~
// println(d.FunctionCallResult())
// ```
func (details ChatDetails) FunctionCallResult() map[string]any {
	var result map[string]any

	err := json.Unmarshal([]byte(details.FirstString()), &result)
	if err != nil {
		log.Errorf("OpenAI function call failed: %s", err)
		return result
	}

	return result
}

func DetailsToString(details []ChatDetail) string {
	var list []string

	hasFunctionCallResults := false
	for _, d := range details {
		if len(d.ToolCalls) > 0 {
			hasFunctionCallResults = true
			break
		}
	}
	if hasFunctionCallResults {
		list = lo.Map(details, func(d ChatDetail, _ int) string {
			return strings.Join(
				lo.Map(d.ToolCalls, func(tool *ToolCall, _ int) string {
					if tool == nil {
						return ""
					}
					return strings.TrimSpace(tool.Function.Arguments)
				}),
				"\n")
		})
	} else {
		list = lo.Map(details, func(d ChatDetail, _ int) string {
			switch ret := d.Content.(type) {
			case []*ChatContent:
				var txt bytes.Buffer
				for _, i := range ret {
					n, _ := txt.WriteString(i.Text)
					if n > 0 {
						txt.WriteString("\n")
					}
				}
				return strings.TrimSpace(txt.String())
			}
			return strings.TrimSpace(utils.InterfaceToString(d.Content))
		})
	}

	list = utils.StringArrayFilterEmpty(list)
	return strings.Join(list, "\n\n")
}
