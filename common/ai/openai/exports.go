package openai

import (
	"encoding/json"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Chat 使用 OpenAI 的大语言模型进行对话，返回对话结果
// @param {string} data 用户的提问或描述
// @param {ConfigOption} ...opts 配置选项，用于配置代理、API Key、模型等
// Example:
// ```
// result = openai.Chat("Hello, world!", openai.apiKey("sk-xxx"), openai.proxy("http://127.0.0.1:7890"))
// ```
func chat(data string, opts ...ConfigOption) string {
	msg, err := NewOpenAIClient(opts...).Chat(data)
	if err != nil {
		log.Errorf("openai chatgpt failed: %s", err)
		return ""
	}
	return msg
}

// ChatEx 使用 OpenAI 的大语言模型进行对话，返回对话结果结构体与错误
// @param {[]ChatDetail} 聊天的消息上下文，可以通过openai.userMessage等创建
// @param {ConfigOption} ...opts 配置选项，用于配置代理、API Key、模型等
// @return {ChatDetails} 包含对话结果的结构体
// @return {error} 错误
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
func chatEx(messages []ChatDetail, opts ...ConfigOption) (ChatDetails, error) {
	choices, err := NewOpenAIClient(opts...).ChatEx(messages)
	if err != nil {
		return nil, err
	}
	details := lo.Map(choices, func(c ChatChoice, _ int) ChatDetail {
		return c.Message
	})
	return details, nil
}

// FunctionCall 使用 OpenAI 的大语言模型的函数调用功能，描述一个函数并让模型智能地选择输出一个包含调用函数的参数的 JSON 对象
// 详情请参考 https://platform.openai.com/docs/guides/function-calling
// @param {string} data 用户的提问或描述
// @param {string} funcName 函数名
// @param {string} funcDesc 函数描述
// @param {ConfigOption} ...opts 配置选项，用于配置代理、API Key、模型等
// @return {map[string]any} 包含调用函数的参数的映射
// Example:
// ```
// resultMap = openai.FunctionCall(
// "What is the weather like in Boston?",
// "get_current_weather",
// "Get the current weather in a given location",
// openai.apiKey("sk-xxxx"),
// openai.proxy("http://127.0.0.1:7890"),
// openai.functionProperty("location", "string", "The city and state, e.g. San Francisco, CA"),
// openai.functionRequired("location"))
// ```
func functionCall(data, funcName, funcDesc string, opts ...ConfigOption) map[string]any {
	client := NewOpenAIClient(opts...)
	functions := Function{
		Name:        funcName,
		Description: funcDesc,
		Parameters:  client.Parameters,
	}
	var result map[string]any

	msg, err := client.Chat(data, functions)
	if err != nil {
		log.Errorf("OpenAI function call failed: %s", err)
		return result
	}
	err = json.Unmarshal(utils.UnsafeStringToBytes(msg), &result)
	if err != nil {
		log.Errorf("OpenAI function call failed: %s", err)
		return result
	}

	return result
}

// TranslateToChinese 使用 OpenAI 的大语言模型将传入的字符串翻译为中文，还可以接收零个到多个配置选项，用于配置代理、API Key、模型等，返回翻译后的中文字符串
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKey("sk-xxx"), openai.proxy("http://127.0.0.1:7890"))
// ```
func translate(data string, opts ...ConfigOption) string {
	msg, err := NewOpenAIClient(opts...).TranslateToChinese(data)
	if err != nil {
		log.Errorf("openai chatgpt failed: %s", err)
		return ""
	}
	return msg
}

var Exports = map[string]interface{}{
	"TranslateToChinese": translate,
	"FunctionCall":       functionCall,
	"Chat":               chat,
	"ChatEx":             chatEx,
	"NewSession":         NewSession,

	"apiKey":      WithAPIKey,
	"localAPIKey": WithAPIKeyFromYakitHome,
	"proxy":       WithProxy,
	"domain":      WithDomain,
	"yakDomain":   WithYakDomain,
	"model":       WithModel,

	"newFunction":       WithFunction,
	"functionParamType": WithFunctionParameterType,
	"functionProperty":  WithFunctionProperty,
	"functionRequired":  WithFunctionRequired,

	"systemMessage":     NewSystemChatDetail,
	"userMessage":       NewUserChatDetail,
	"assistantMessage":  NewAIChatDetail,
	"toolMessage":       NewToolChatDetail,
	"toolMessageWithID": NewToolChatDetailWithID,
	// "functionMessage":  NewFunctionChatDetail,
}
