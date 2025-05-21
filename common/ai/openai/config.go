package openai

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
)

type ConfigOption func(client *Client)

// proxy 设置调用 OpenAI 时使用的代理
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKey("sk-xxx"), openai.proxy("http://127.0.0.1:7890"))
// ```
func WithProxy(i string) ConfigOption {
	return func(client *Client) {
		client.Proxy = i
	}
}

// apiKey 设置 OpenAI的API Key
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKey("sk-xxx"))
// ```
func WithAPIKey(i string) ConfigOption {
	return func(client *Client) {
		client.APIKey = i
	}
}

// localAPIKey 从 $YAKIT_HOME/openai-key.txt 中获取 API Key
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKeyFromYakitHome())
// ```
func WithAPIKeyFromYakitHome() ConfigOption {
	return func(client *Client) {
		raw, err := os.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
		if err != nil {
			log.OnceInfoLog("check-openai-apikey", "cannot find openai-key.txt in %s", consts.GetDefaultYakitProjectsDir())
			return
		}
		client.APIKey = strings.TrimSpace(string(raw))
	}
}

// model 设置 OpenAI的大语言模型
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKey("sk-xxx"), openai.model("gpt-4-0613"))
// ```
func WithModel(i string) ConfigOption {
	return func(client *Client) {
		client.ChatModel = i
	}
}

// domain 设置 OpenAI的第三方加速域名，用于加速访问
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKey("sk-xxx"), openai.domain("api.ai.yaklang.com"))
// ```
func WithDomain(i string) ConfigOption {
	return func(client *Client) {
		client.Domain = i
	}
}

// yakDomain 设置 OpenAI的第三方加速域名为 Yaklang.io 提供的第三方加速域名，用于加速访问
// Example:
// ```
// result = openai.TranslateToChinese("Hello, world!", openai.apiKey("sk-xxx"), openai.yakDomain())
// ```
func WithYakDomain() ConfigOption {
	return func(client *Client) {
		client.Domain = "api.ai.yaklang.com"
	}
}

// functionParamType 设置函数调用时的参数类型，默认为 "object"
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
func WithFunctionParameterType(i string) ConfigOption {
	return func(client *Client) {
		client.Parameters.Type = i
	}
}

// functionProperty 设置函数调用时的单个参数属性
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
func WithFunctionProperty(name, typ, description string, enum ...[]string) ConfigOption {
	_enum := []string{}
	if len(enum) > 0 {
		_enum = enum[0]
	}

	return func(client *Client) {
		client.Parameters.Properties[name] = aispec.Property{
			Type:        typ,
			Description: description,
			Enum:        _enum,
		}
	}
}

// functionRequired 设置函数调用时的必须参数
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
func WithFunctionRequired(names ...string) ConfigOption {
	return func(client *Client) {
		client.Parameters.Required = names
	}
}
