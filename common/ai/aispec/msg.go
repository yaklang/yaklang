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
	Type     string `json:"type"` // text / image_url
	Text     string `json:"text,omitempty"`
	ImageUrl any    `json:"image_url,omitempty"`
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
}

type ToolCall struct {
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
