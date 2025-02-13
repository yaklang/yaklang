package ai

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/deepseek"
	"github.com/yaklang/yaklang/common/ai/siliconflow"
	"io"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/chatglm"
	"github.com/yaklang/yaklang/common/ai/comate"
	"github.com/yaklang/yaklang/common/ai/moonshot"
	"github.com/yaklang/yaklang/common/ai/openai"
	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	aispec.Register("openai", func() aispec.AIClient {
		return &openai.GetawayClient{}
	})
	aispec.Register("chatglm", func() aispec.AIClient {
		return &chatglm.GLMClient{}
	})
	aispec.Register("moonshot", func() aispec.AIClient {
		return &moonshot.GatewayClient{}
	})
	aispec.Register("tongyi", func() aispec.AIClient {
		return &tongyi.GetawayClient{}
	})
	aispec.Register("comate", func() aispec.AIClient {
		return &comate.Client{}
	})
	aispec.Register("deepseek", func() aispec.AIClient {
		return &deepseek.GetawayClient{}
	})
	aispec.Register("siliconflow", func() aispec.AIClient {
		return &siliconflow.GetawayClient{}
	})
}

type Gateway struct {
	Config    *aispec.AIConfig
	TargetUrl string
	aispec.AIClient
}

func (g *Gateway) Chat(s string, f ...aispec.Function) (string, error) {
	return aispec.ChatBase(g.TargetUrl, g.Config.Model, s, f, g.AIClient.BuildHTTPOptions, g.Config.StreamHandler)
}

func (g *Gateway) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.TargetUrl, g.Config.Model, details, function, g.AIClient.BuildHTTPOptions, g.Config.StreamHandler)
}

func (g *Gateway) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.TargetUrl, g.Config.Model, msg, fields, g.AIClient.BuildHTTPOptions, g.Config.StreamHandler)
}

func (g *Gateway) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.TargetUrl, g.Config.Model, s, g.Config.HTTPErrorHandler, g.AIClient.BuildHTTPOptions)
}

func NewGateway() *Gateway {
	return &Gateway{}
}

func tryCreateAIGateway(t string, cb func(string, aispec.AIClient) bool) error {
	createAIGatewayByType := func(typ string) aispec.AIClient {
		gw, ok := aispec.Lookup(typ)
		if !ok {
			return nil
		}
		return gw
	}

	total := aispec.RegisteredAIGateways()
	if utils.StringArrayContains(total, t) {
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

	// update database if registered ai type is not in config or configured ai type is not in registered
	updateCfg := false
	cfg.AiApiPriority = lo.Filter(cfg.AiApiPriority, func(s string, _ int) bool {
		reserve := utils.StringArrayContains(total, s)
		if !reserve {
			updateCfg = true
		}
		return reserve
	})

	for _, s := range total {
		if !utils.StringArrayContains(cfg.AiApiPriority, s) {
			cfg.AiApiPriority = append(cfg.AiApiPriority, s)
			updateCfg = true
		}
	}
	if updateCfg {
		yakit.ConfigureNetWork(cfg)
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

func createAIGateway(t string) aispec.AIClient {
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

func OpenAI(opts ...aispec.AIConfigOption) aispec.AIClient {
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

func GetAI(t string, opts ...aispec.AIConfigOption) aispec.AIClient {
	agent := createAIGateway(t)
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func ChatGLM(opts ...aispec.AIConfigOption) aispec.AIClient {
	agent := createAIGateway("chatglm")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func Moonshot(opts ...aispec.AIConfigOption) aispec.AIClient {
	agent := createAIGateway("moonshot")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func GetPrimaryAgent() aispec.AIClient {
	var agent aispec.AIClient

	t := consts.GetAIPrimaryType()
	if t == "" {
		for _, defaultType := range []string{
			"openai", "chatglm", "moonshot", "tongyi", "comate",
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
	config := aispec.NewDefaultAIConfig(opts...)
	var responseRsp string
	var err error
	err = tryCreateAIGateway(config.Type, func(typ string, gateway aispec.AIClient) bool {
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
	config := aispec.NewDefaultAIConfig(opts...)
	var responseRsp map[string]any
	var err error
	err = tryCreateAIGateway(config.Type, func(typ string, gateway aispec.AIClient) bool {
		gateway.LoadOption(append([]aispec.AIConfigOption{aispec.WithType(typ)}, opts...)...)
		if err := gateway.CheckValid(); err != nil {
			log.Warnf("check valid by %s failed: %s", typ, err)
			return false
		}
		var ok bool
		for i := 0; i < config.FunctionCallRetryTimes; i++ {
			responseRsp, err = gateway.ExtractData(input, "", utils.InterfaceToGeneralMap(funcs))
			if err != nil {
				log.Warnf("chat by %s failed: %s, retry times: %d", typ, err, i)
			} else {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return responseRsp, nil
}

var Exports = map[string]any{
	"OpenAI":   OpenAI,
	"ChatGLM":  ChatGLM,
	"Moonshot": Moonshot,

	"Chat":         Chat,
	"FunctionCall": FunctionCall,

	"timeout":            aispec.WithTimeout,
	"proxy":              aispec.WithProxy,
	"model":              aispec.WithModel,
	"apiKey":             aispec.WithAPIKey,
	"noHttps":            aispec.WithNoHttps,
	"funcCallRetryTimes": aispec.WithFunctionCallRetryTimes,
	"domain":             aispec.WithDomain,
	"baseURL":            aispec.WithBaseURL,
	"onStream":           aispec.WithStreamHandler,
	"debugStream":        aispec.WithDebugStream,
	"type":               aispec.WithType,
}
