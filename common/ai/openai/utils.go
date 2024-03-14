package openai

import (
	"encoding/json"
	"github.com/go-rod/rod/lib/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

func ExtractDataByAi(data string, desc string, params map[string]string, opts ...ConfigOption) (map[string]any, error) {
	functionName := utils.RandString(10)
	defaultOpts := []ConfigOption{}
	var paramNames []string
	for name, desc := range params {
		defaultOpts = append(defaultOpts, WithFunctionProperty(name, "string", desc))
		paramNames = append(paramNames, name)
	}
	defaultOpts = append(defaultOpts, WithFunctionRequired(paramNames...))
	opts = append(defaultOpts, opts...)
	result := make(map[string]any)
	aiClient := NewOpenAIClient(append(opts, WithFunction(functionName, desc, opts...))...)
	rspMsg, err := aiClient.Chat(data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(rspMsg), &result)
	if err != nil {
		return nil, utils2.Errorf("openai function call failed: %s, raw: %v", err, string(rspMsg))
	}
	return result, nil
}
