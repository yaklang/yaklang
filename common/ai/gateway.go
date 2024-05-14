package ai

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

import (
	_ "github.com/yaklang/yaklang/common/ai/chatglm"
	_ "github.com/yaklang/yaklang/common/ai/moonshot"
	_ "github.com/yaklang/yaklang/common/ai/openai"
)

func tryCreateAIGateway(t string, cb func(string, aispec.AIGateway) bool) error {
	createAIGatewayByType := func(typ string) aispec.AIGateway {
		gw, ok := aispec.Lookup(typ)
		if !ok {
			return nil
		}
		return gw
	}
	if utils.StringArrayContains(aispec.RegisteredAIGateways(), t) {
		gw := createAIGatewayByType(t)
		if gw != nil {
			if cb(t, gw) {
				return nil
			}
		}
	}
	if t != "" {
		log.Warnf("unsupported ai type: %s, use default config ai type", t)
	}
	cfg := yakit.GetNetworkConfig()
	if cfg == nil {
		return nil
	}
	for _, typ := range cfg.AiApiPriority {
		agent := createAIGatewayByType(typ)
		if agent != nil {
			if cb(typ, agent) {
				return nil
			}
		} else {
			log.Warnf("create ai agent by type %s failed", typ)
		}
	}
	return errors.New("not found valid ai agent")
}
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

func GetPrimaryAgent() aispec.AIGateway {
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
	return agent
}

func Chat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	config := &aispec.AIConfig{}
	for _, p := range opts {
		p(config)
	}
	var responseRsp string
	var err error
	err = tryCreateAIGateway(config.Type, func(typ string, gateway aispec.AIGateway) bool {
		gateway.LoadOption(append([]aispec.AIConfigOption{aispec.WithType(typ)}, opts...)...)
		if err := gateway.CheckValid(); err != nil {
			log.Warnf("check valid by %s failed: %s", typ, err)
			return false
		}
		responseRsp, err = gateway.Chat(msg)
		if err != nil {
			log.Warnf("chat by %s failed: %s", typ, err)
			return false
		}
		return true
	})
	if err != nil {
		return "", err
	}
	return responseRsp, nil
}

func FunctionCall(input string, funcs any, opts ...aispec.AIConfigOption) (map[string]any, error) {
	config := &aispec.AIConfig{}
	for _, p := range opts {
		p(config)
	}
	agent := createAIGateway(config.Type)
	if agent == nil {
		return nil, utils.Error("not found valid ai agent config")
	}
	agent.LoadOption(opts...)
	return agent.ExtractData(input, "", utils.InterfaceToGeneralMap(funcs))
}

var Exports = map[string]any{
	"OpenAI":   OpenAI,
	"ChatGLM":  ChatGLM,
	"Moonshot": Moonshot,

	"Chat":         Chat,
	"FunctionCall": FunctionCall,

	"timeout":     aispec.WithTimeout,
	"proxy":       aispec.WithProxy,
	"model":       aispec.WithModel,
	"apiKey":      aispec.WithAPIKey,
	"noHttps":     aispec.WithNoHttps,
	"domain":      aispec.WithDomain,
	"baseURL":     aispec.WithBaseURL,
	"onStream":    aispec.WithStreamHandler,
	"debugStream": aispec.WithDebugStream,
	"type":        aispec.WithType,
}
