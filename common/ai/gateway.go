package ai

import "github.com/yaklang/yaklang/common/ai/aispec"

import (
	_ "github.com/yaklang/yaklang/common/ai/chatglm"
	_ "github.com/yaklang/yaklang/common/ai/openai"
)

func createAIGateway(t string) aispec.AIGateway {
	gw, ok := aispec.Lookup(t)
	if !ok {
		return nil
	}
	return gw
}

/*
ai mod

client = ai.Client()
*/

func OpenAI(opts ...aispec.AIConfigOption) aispec.AIGateway {
	agent := createAIGateway("openai")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func HaveAI(t string) bool {
	_, ok := aispec.Lookup(t)
	return ok
}

func GetAI(t string, opts ...aispec.AIConfigOption) aispec.AIGateway {
	agent := createAIGateway(t)
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func ChatGLM(opts ...aispec.AIConfigOption) aispec.AIGateway {
	agent := createAIGateway("chatglm")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

var Exports = map[string]any{
	"OpenAI":  OpenAI,
	"ChatGLM": ChatGLM,

	"timeout": aispec.WithTimeout,
	"proxy":   aispec.WithProxy,
	"model":   aispec.WithModel,
	"apiKey":  aispec.WithAPIKey,
	"noHttps": aispec.WithNoHttps,
	"domain":  aispec.WithDomain,
	"baseURL": aispec.WithBaseURL,
}
