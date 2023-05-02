package openai

import "yaklang.io/yaklang/common/log"

func chat(data string, opts ...ConfigOption) string {
	msg, err := NewOpenAIClient(opts...).Chat(data)
	if err != nil {
		log.Errorf("openai chatgpt failed: %s", err)
		return ""
	}
	return msg
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
	"Chat":               chat,
	"apiKey":             WithAPIKey,
	"localAPIKey":        WithAPIKeyFromYakitHome,
	"proxy":              WithProxy,
	"domain":             WithDomain,
	"yakDomain":          WithYakProxy,
	"model":              WithModel,
}
