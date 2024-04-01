package ai

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

import (
	_ "github.com/yaklang/yaklang/common/ai/chatglm"
	_ "github.com/yaklang/yaklang/common/ai/moonshot"
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

func Moonshot(opts ...aispec.AIConfigOption) aispec.AIGateway {
	agent := createAIGateway("moonshot")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func Chat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	var agent aispec.AIGateway

	t := consts.GetAIPrimaryType()
	if t == "" {
		for _, defaultType := range []string{
			"openai", "chatglm",
		} {
			agent = createAIGateway(defaultType)
			if agent == nil {
				continue
			}
			break
		}
	} else {
		agent = createAIGateway(t)
	}

	if agent == nil {
		if t == "" {
			return "", utils.Error("no primary and configged ai agent found")
		}
		return "", utils.Errorf("primary type: %v is not config", t)
	}
	agent.LoadOption(opts...)
	return agent.Chat(msg)
}

func FunctionCall(input string, funcs map[string]any, opts ...aispec.AIConfigOption) (map[string]any, error) {
	var agent aispec.AIGateway
	t := consts.GetAIPrimaryType()
	if t == "" {
		for _, defaultType := range []string{
			"openai", "chatglm",
		} {
			agent = createAIGateway(defaultType)
			if agent == nil {
				continue
			}
			break
		}
	} else {
		agent = createAIGateway(t)
	}

	if agent == nil {
		if t == "" {
			return nil, utils.Error("no primary and configged ai agent found")
		}
		return nil, utils.Errorf("primary type: %v is not config", t)
	}
	agent.LoadOption(opts...)
	return agent.ExtractData(input, "", funcs)
}

var Exports = map[string]any{
	"OpenAI":       OpenAI,
	"ChatGLM":      ChatGLM,
	"Moonshot":     Moonshot,
	"Chat":         Chat,
	"FunctionCall": FunctionCall,

	"timeout": aispec.WithTimeout,
	"proxy":   aispec.WithProxy,
	"model":   aispec.WithModel,
	"apiKey":  aispec.WithAPIKey,
	"noHttps": aispec.WithNoHttps,
	"domain":  aispec.WithDomain,
	"baseURL": aispec.WithBaseURL,
}
