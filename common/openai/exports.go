package openai

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func chat(data string, opts ...ConfigOption) string {
	msg, err := NewOpenAIClient(opts...).Chat(data)
	if err != nil {
		log.Errorf("openai chatgpt failed: %s", err)
		return ""
	}
	return msg
}

func functionCall(data, funcName, funcDesc string, opts ...ConfigOption) map[string]any {
	client := NewOpenAIClient(opts...)
	functions := Function{
		Name:        funcName,
		Description: funcDesc,
		Parameters:  client.Parameters,
	}
	result := make(map[string]any)

	msg, err := client.Chat(data, functions)
	if err != nil {
		log.Errorf("openai function call failed: %s", err)
		return result
	}
	err = json.Unmarshal(utils.UnsafeStringToBytes(msg), &result)
	if err != nil {
		log.Errorf("openai function call failed: %s", err)
		return result
	}

	return result
}

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
	"apiKey":             WithAPIKey,
	"localAPIKey":        WithAPIKeyFromYakitHome,
	"proxy":              WithProxy,
	"domain":             WithDomain,
	"yakDomain":          WithYakProxy,
	"model":              WithModel,
	"functionParamType":  WithFunctionParameterType,
	"functionProperty":   WithFunctionProperty,
	"functionRequired":   WithFunctionRequired,
}
