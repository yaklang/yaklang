package ai

import (
	"errors"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/ai/aibalance"

	"github.com/yaklang/yaklang/common/ai/dashscopebase"
	"github.com/yaklang/yaklang/common/ai/deepseek"
	"github.com/yaklang/yaklang/common/ai/gemini"
	"github.com/yaklang/yaklang/common/ai/openrouter"
	"github.com/yaklang/yaklang/common/ai/siliconflow"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/chatglm"
	"github.com/yaklang/yaklang/common/ai/comate"
	"github.com/yaklang/yaklang/common/ai/moonshot"
	"github.com/yaklang/yaklang/common/ai/ollama"
	"github.com/yaklang/yaklang/common/ai/openai"
	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/volcengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	aispec.Register("openai", func() aispec.AIClient {
		return &openai.GatewayClient{}
	})
	aispec.Register("chatglm", func() aispec.AIClient {
		return &chatglm.GLMClient{}
	})
	aispec.Register("moonshot", func() aispec.AIClient {
		return &moonshot.GatewayClient{}
	})
	aispec.Register("tongyi", func() aispec.AIClient {
		return &tongyi.GatewayClient{}
	})
	aispec.Register("volcengine", func() aispec.AIClient {
		return &volcengine.GatewayClient{
			ExtraOptions: []aispec.AIConfigOption{
				aispec.WithEnableThinkingEx("thinking", map[string]any{
					"type": "disabled",
				}),
			},
		}
	})
	aispec.Register("comate", func() aispec.AIClient {
		return &comate.Client{}
	})
	aispec.Register("deepseek", func() aispec.AIClient {
		return &deepseek.GatewayClient{}
	})
	aispec.Register("siliconflow", func() aispec.AIClient {
		return &siliconflow.GatewayClient{}
	})
	aispec.Register("ollama", func() aispec.AIClient {
		return &ollama.GatewayClient{}
	})
	aispec.Register("openrouter", func() aispec.AIClient {
		return &openrouter.GatewayClient{}
	})
	aispec.Register("gemini", func() aispec.AIClient {
		return &gemini.Client{}
	})
	aispec.Register("yaklang-writer", func() aispec.AIClient {
		return dashscopebase.CreateDashScopeGateway("a51e9af5a60f40c983dac6ed50dba15b")
	})
	aispec.Register("yaklang-rag", func() aispec.AIClient {
		return dashscopebase.CreateDashScopeGateway("e3acc5f1c8ea4995aeac7618bc543ad5")
	})
	aispec.Register("yaklang-com-search", func() aispec.AIClient {
		return dashscopebase.CreateDashScopeGateway("5d880c5d33484343b5b08a66c4d5ee77")
	})
	aispec.Register("yakit-plugin-search", func() aispec.AIClient {
		return dashscopebase.CreateDashScopeGateway("e8be1ba351dc44568728bcb46e36aac2")
	})
	aispec.Register("aibalance", func() aispec.AIClient {
		return &aibalance.GatewayClient{}
	})
}

type Gateway struct {
	Config    *aispec.AIConfig
	TargetUrl string
	aispec.AIClient
}

func (g *Gateway) GetTypeName() string {
	if g.Config == nil {
		return ""
	}
	return g.Config.Type
}

func (g *Gateway) GetModelName() string {
	if g.Config == nil {
		return ""
	}
	return g.Config.Model
}

func (g *Gateway) Chat(s string, f ...any) (string, error) {
	return aispec.ChatBase(g.TargetUrl, g.Config.Model, s,
		aispec.WithChatBase_Function(f),
		aispec.WithChatBase_PoCOptions(g.AIClient.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.Config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.Config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.Config.HTTPErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.Config.Images...),
		aispec.WithChatBase_ToolCallCallback(g.Config.ToolCallCallback),
		aispec.WithChatBase_Tools(g.Config.Tools),
		aispec.WithChatBase_ToolChoice(g.Config.ToolChoice),
	)
}

func (g *Gateway) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.TargetUrl, g.Config.Model, msg, fields, g.AIClient.BuildHTTPOptions, g.Config.StreamHandler, g.Config.ReasonStreamHandler, g.Config.HTTPErrorHandler, g.Config.Images...)
}

func (g *Gateway) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.TargetUrl, g.Config.Model, s, g.Config.HTTPErrorHandler, g.Config.StreamHandler, g.AIClient.BuildHTTPOptions)
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

// 创建一个 OpenAI 客户端实例，支持 OpenAI 官方 API 及兼容的第三方服务。
//
// 参数：
// - opts(...aispec.AIConfigOption): 配置选项（必须包含 apiKey）
//
// 返回值：
// - r1: AI 客户端实例
//
// Example:
// ```go
// // 创建 OpenAI 客户端
// client = ai.OpenAI(
//
//	ai.apiKey("sk-xxx"),
//	ai.model("gpt-3.5-turbo"),
//
// )
//
// // 发送消息
// response, err = client.Chat("你好")
// die(err)
// println(response)
//
// // 使用自定义 API 地址
// client = ai.OpenAI(
//	ai.apiKey("sk-xxx"),
//	ai.baseURL("https://api.openai-proxy.com/v1"),
//	ai.model("gpt-4"),
// )
// ```
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

// 创建一个 ChatGLM 客户端实例，用于调用智谱 AI 的 ChatGLM 系列模型。
//
// 参数:
// - opts(...aispec.AIConfigOption): 配置选项（必须包含 apiKey）
//
// 返回值:
// - r1: AI 客户端实例
//
// Example:
// ```go
// // 创建 ChatGLM 客户端
// client = ai.ChatGLM(
//	ai.apiKey("your-api-key"),
//	ai.model("chatglm_turbo"),
// )
//
// // 调用对话
// response, err = client.Chat("介绍一下你自己")
// die(err)
// println(response)
// ```
func ChatGLM(opts ...aispec.AIConfigOption) aispec.AIClient {
	agent := createAIGateway("chatglm")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

// 创建一个 Moonshot 客户端实例，用于调用 Moonshot AI 服务。
//
// 参数：
// - opts(...aispec.AIConfigOption): 配置选项（必须包含 apiKey）
//
// 返回值：
// - r1: AI 客户端实例
//
// Example:
// ```go
// // 创建 Moonshot 客户端
// client = ai.Moonshot(
//	ai.apiKey("sk-xxx"),
//	ai.model("moonshot-v1-8k"),
// )
//
// // 使用客户端
// response, err = client.Chat("帮我分析这段代码")
// die(err)
// println(response)
// ```
func Moonshot(opts ...aispec.AIConfigOption) aispec.AIClient {
	agent := createAIGateway("moonshot")
	if agent != nil {
		agent.LoadOption(opts...)
	}
	return agent
}

func Volcengine(opts ...aispec.AIConfigOption) aispec.AIClient {
	agent := createAIGateway("volcengine")
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
			"openai", "chatglm", "moonshot", "tongyi", "volcengine", "comate",
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

// Chat 快速调用 AI 服务进行对话，这是最简单的调用方式。
//
// 参数：
// - msg(string): 要发送给 AI 的消息内容
// - opts(...aispec.AIConfigOption): AI 配置选项（如 apiKey、model 等）
//
// 返回值：
// - string(string): AI 返回的回复内容
// - error(error): 错误信息
//
// Example:
// ```go
// response, err = ai.Chat("介绍一下 Yakit",
//	ai.baseURL("https://api.openai-proxy.com/v1"),
//	ai.apiKey("sk-xxx"),
//	ai.type("openai"),
//	ai.model("gpt-4"),
//	ai.proxy("http://127.0.0.1:7890"),
//	ai.timeout(60),
// )
//
// if err != nil {
//	panic(err)
// }
//
// println(response)
// ```
func Chat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	// Parse options to check if user explicitly specified a type
	config := aispec.NewDefaultAIConfig(opts...)

	// If user explicitly specified a type, use legacy chat to respect their choice
	if config.Type != "" {
		return legacyChat(msg, opts...)
	}

	// Check if tiered AI model configuration is enabled
	if consts.IsTieredAIModelConfigEnabled() {
		return tieredChat(msg, opts...)
	}
	// Fall back to legacy chat logic
	return legacyChat(msg, opts...)
}

// legacyChat is the original Chat implementation for backward compatibility
func legacyChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	config := aispec.NewDefaultAIConfig(opts...)
	var responseRsp string
	var err error
	err = tryCreateAIGateway(config.Type, func(typ string, gateway aispec.AIClient) bool {
		gateway.LoadOption(append([]aispec.AIConfigOption{aispec.WithType(typ)}, opts...)...)
		if err := gateway.CheckValid(); err != nil {
			log.Debugf("check valid by %s failed: %s", typ, err)
			return false
		}
		log.Infof("start to chat completions by %v", typ)
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

// tieredChat handles chat using the tiered AI model configuration
func tieredChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	// Get the current routing policy
	policy := consts.GetTieredAIRoutingPolicy()

	// Select the appropriate tier based on policy
	var configs []*ypb.ThirdPartyApplicationConfig
	switch policy {
	case consts.PolicyPerformance:
		configs = consts.GetIntelligentAIConfigs()
	case consts.PolicyCost:
		configs = consts.GetLightweightAIConfigs()
	case consts.PolicyBalance, consts.PolicyAuto:
		// Balance mode: use lightweight by default
		configs = consts.GetLightweightAIConfigs()
	default:
		configs = consts.GetLightweightAIConfigs()
	}

	if len(configs) == 0 {
		log.Warnf("No tiered AI config available for policy %s, falling back to legacy chat", policy)
		return legacyChat(msg, opts...)
	}

	// Try each config in order
	for _, cfg := range configs {
		result, err := chatWithThirdPartyConfig(msg, cfg, opts...)
		if err == nil {
			return result, nil
		}
		log.Debugf("Chat with config type=%s failed: %v, trying next", cfg.Type, err)
	}

	// Fallback to lightweight if not already using it
	if policy != consts.PolicyCost && !consts.IsTieredAIFallbackDisabled() {
		log.Debugf("Falling back to lightweight model")
		lightweightConfigs := consts.GetLightweightAIConfigs()
		for _, cfg := range lightweightConfigs {
			result, err := chatWithThirdPartyConfig(msg, cfg, opts...)
			if err == nil {
				return result, nil
			}
		}
	}

	// Final fallback to legacy chat
	log.Warnf("All tiered configs failed, falling back to legacy chat")
	return legacyChat(msg, opts...)
}

// chatWithThirdPartyConfig performs chat using a specific ThirdPartyApplicationConfig
func chatWithThirdPartyConfig(msg string, cfg *ypb.ThirdPartyApplicationConfig, opts ...aispec.AIConfigOption) (string, error) {
	if cfg == nil {
		return "", errors.New("config is nil")
	}

	// Build options from config
	configOpts := buildOptionsFromThirdPartyConfig(cfg)
	allOpts := append(configOpts, opts...)

	// Create gateway and chat
	agent := createAIGateway(cfg.Type)
	if agent == nil {
		return "", errors.New("failed to create AI gateway for type: " + cfg.Type)
	}

	agent.LoadOption(allOpts...)
	if err := agent.CheckValid(); err != nil {
		return "", err
	}

	log.Debugf("Start tiered chat with type=%s", cfg.Type)
	return agent.Chat(msg)
}

// buildOptionsFromThirdPartyConfig builds AIConfigOption slice from ThirdPartyApplicationConfig
func buildOptionsFromThirdPartyConfig(cfg *ypb.ThirdPartyApplicationConfig) []aispec.AIConfigOption {
	var opts []aispec.AIConfigOption

	if cfg.Type != "" {
		opts = append(opts, aispec.WithType(cfg.Type))
	}
	if cfg.APIKey != "" {
		opts = append(opts, aispec.WithAPIKey(cfg.APIKey))
	}
	if cfg.Domain != "" {
		opts = append(opts, aispec.WithDomain(cfg.Domain))
	}

	// Extract model from ExtraParams
	for _, param := range cfg.ExtraParams {
		if param.Key == "model" {
			opts = append(opts, aispec.WithModel(param.Value))
			break
		}
	}

	return opts
}

// TieredChat allows explicit selection of a model tier for chat
type ModelTier string

const (
	TierIntelligent ModelTier = "intelligent"
	TierLightweight ModelTier = "lightweight"
	TierVision      ModelTier = "vision"
)

// TieredChatWithTier performs chat with a specific model tier
func TieredChatWithTier(tier ModelTier, msg string, opts ...aispec.AIConfigOption) (string, error) {
	if !consts.IsTieredAIModelConfigEnabled() {
		log.Debugf("Tiered AI config not enabled, using legacy chat")
		return legacyChat(msg, opts...)
	}

	var configs []*ypb.ThirdPartyApplicationConfig
	switch tier {
	case TierIntelligent:
		configs = consts.GetIntelligentAIConfigs()
	case TierLightweight:
		configs = consts.GetLightweightAIConfigs()
	case TierVision:
		configs = consts.GetVisionAIConfigs()
	default:
		log.Warnf("Unknown tier %s, using intelligent", tier)
		configs = consts.GetIntelligentAIConfigs()
	}

	if len(configs) == 0 {
		return "", errors.New("no configuration available for tier: " + string(tier))
	}

	// Try the first config
	return chatWithThirdPartyConfig(msg, configs[0], opts...)
}

// IntelligentChat uses the intelligent (high-quality) model
func IntelligentChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	return TieredChatWithTier(TierIntelligent, msg, opts...)
}

// LightweightChat uses the lightweight (fast) model
func LightweightChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	return TieredChatWithTier(TierLightweight, msg, opts...)
}

// VisionChat uses the vision model
func VisionChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	return TieredChatWithTier(TierVision, msg, opts...)
}

// 获取结构化的流式输出，支持实时接收 AI 返回的数据。
//
// 参数：
// - input(string): 输入消息
// - opts(...aispec.AIConfigOption): 配置选项
//
// 返回值：
// - r1(chan *aispec.StructuredData): 结构化数据通道
// - r2(error): 错误信息
//
// Example:
// ```go
// // 获取流式输出
// stream, err = ai.StructuredStream(
//	"生成一个端口扫描脚本",
//	ai.apiKey("sk-xxx"),
//	ai.type("openai"),
// )
// die(err)
//
// // 读取流数据
//
// for data = range stream {
//	printf("收到数据: %v\n", data)
// }
// ```
func StructuredStream(input string, opts ...aispec.AIConfigOption) (chan *aispec.StructuredData, error) {
	config := aispec.NewDefaultAIConfig(opts...)
	var selectedGateway aispec.AIClient
	tryCreateAIGateway(config.Type, func(typ string, gateway aispec.AIClient) bool {
		gateway.LoadOption(append([]aispec.AIConfigOption{aispec.WithType(typ)}, opts...)...)
		if err := gateway.CheckValid(); err != nil {
			log.Debugf("check valid by %s failed: %s", typ, err)
			return false
		}

		if gateway.SupportedStructuredStream() {
			selectedGateway = gateway
		}
		return true
	})
	if selectedGateway == nil {
		return nil, errors.New("not found valid ai agent")
	}

	for i := 0; i < config.FunctionCallRetryTimes; i++ {
		ch, err := selectedGateway.StructuredStream(input)
		if err != nil {
			log.Warnf("structured stream by %s failed: %s, retry times: %d", config.Type, err, i)
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}
		return ch, nil
	}
	return nil, errors.New("not found valid ai agent or retry times is over")
}

// 列出当前配置下所有可用的 AI 模型。
//
// 参数：
// - opts(...aispec.AIConfigOption): 配置选项（如 apiKey、type）
//
// 返回值：
// - r1([]*aispec.ModelMeta): 模型元数据列表
// - r2(error): 错误信息
//
// Example:
// ```go
// // 列出 OpenAI 的所有模型
// models, err = ai.ListModels(
//	ai.apiKey("sk-xxx"),
//	ai.type("openai"),
// )
// die(err)
//
// for _, model = range models {
//	printf("模型: %s\n", model.Name)
// }
// ```
func ListModels(opts ...aispec.AIConfigOption) ([]*aispec.ModelMeta, error) {
	config := aispec.NewDefaultAIConfig(opts...)
	client := GetAI(config.Type, opts...)
	if utils.IsNil(client) {
		return nil, utils.Error("List AI model failed:unknown AI type")
	}
	return client.GetModelList()
}

// 根据提供商类型列出可用的 AI 模型。
//
// 参数：
// - providerType(string): 提供商类型（如 "openai"、"chatglm"、"moonshot"）
// - opts(...aispec.AIConfigOption): 配置选项
//
// 返回值：
//
// - r1([]*aispec.ModelMeta): 模型元数据列表
// - r2(error): 错误信息
//
// Example:
// ```go
// // 列出 ChatGLM 的模型
// models, err = ai.ListModelByProviderType(
//	"chatglm",
//	ai.apiKey("your-key"),
// )
// die(err)
//
// for _, model = range models {
//  printf("ChatGLM 模型: %s\n", model.Name)
// }
// ```
func ListModelByProviderType(providerType string, opts ...aispec.AIConfigOption) ([]*aispec.ModelMeta, error) {
	config := aispec.NewDefaultAIConfig(opts...)
	config.Type = providerType
	client := GetAI(config.Type, opts...)
	return client.GetModelList()
}

// 让 AI 根据用户输入自动调用预定义的函数，实现智能函数调用能力。
//
// 参数：
// - input(string): 用户输入的自然语言指令
// - funcs(any): 函数定义（支持结构体或函数列表）
// - opts(...aispec.AIConfigOption): AI 配置选项
//
// 返回值：
//
// - r1(map[string]any): 函数调用结果
// - r2(error): 错误信息
//
// Example:
// ```go
// // 定义可调用的函数
//
//	funcs = {
//	    "searchVulnerability": func(keyword) {
//	        return {"result": sprintf("搜索漏洞: %s", keyword)}
//	    },
//	    "scanTarget": func(target, port) {
//	        return {"target": target, "port": port, "status": "scanning"}
//	    },
//	}
//
// // AI 自动识别并调用函数
// result, err = ai.FunctionCall(
//	"帮我搜索 SQL 注入漏洞",
//	funcs,
//	ai.apiKey("sk-xxx"),
//	ai.type("openai"),
// )
//
// die(err)
// dump(result)
// ```
func FunctionCall(input string, funcs any, opts ...aispec.AIConfigOption) (map[string]any, error) {
	config := aispec.NewDefaultAIConfig(opts...)
	var responseRsp map[string]any
	var err error
	err = tryCreateAIGateway(config.Type, func(typ string, gateway aispec.AIClient) bool {
		gateway.LoadOption(append([]aispec.AIConfigOption{aispec.WithType(typ)}, opts...)...)
		if err := gateway.CheckValid(); err != nil {
			log.Debugf("check valid by %s failed: %s", typ, err)
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

func LoadChater(name string, defaultOpts ...aispec.AIConfigOption) (aispec.GeneralChatter, error) {
	gateway, ok := aispec.Lookup(name)
	if !ok {
		return nil, errors.New("not found valid ai chatter type: " + name)
	}
	return func(msg string, opts ...aispec.AIConfigOption) (string, error) {
		gateway.LoadOption(append(defaultOpts, append([]aispec.AIConfigOption{aispec.WithType(name)}, opts...)...)...)
		if err := gateway.CheckValid(); err != nil {
			log.Warnf("check valid by %s failed: %s", name, err)
			return "", err
		}
		return gateway.Chat(msg)
	}, nil
}

func LoadAiGatewayConfig(name string) (*aispec.AIConfig, error) {
	gateway, ok := aispec.Lookup(name)
	if !ok {
		return nil, errors.New("not found valid ai gateway type: " + name)
	}
	gateway.LoadOption(aispec.WithType(name))
	return gateway.GetConfig(), nil
}

var Exports = map[string]any{
	"OpenAI":   OpenAI,
	"ChatGLM":  ChatGLM,
	"Moonshot": Moonshot,

	"Chat":                    Chat,
	"FunctionCall":            FunctionCall,
	"StructuredStream":        StructuredStream,
	"ListModels":              ListModels,
	"ListModelByProviderType": ListModelByProviderType,

	"thinking":           aispec.WithEnableThinking,
	"timeout":            aispec.WithTimeout,
	"proxy":              aispec.WithProxy,
	"model":              aispec.WithModel,
	"apiKey":             aispec.WithAPIKey,
	"noHttps":            aispec.WithNoHttps,
	"funcCallRetryTimes": aispec.WithFunctionCallRetryTimes,
	"domain":             aispec.WithDomain,
	"baseURL":            aispec.WithBaseURL,
	"onStream":           aispec.WithStreamHandler,
	"onReasonStream":     aispec.WithReasonStreamHandler,
	"debugStream":        aispec.WithDebugStream,
	"type":               aispec.WithType,
	"imageFile":          aispec.WithImageFile,
	"imageBase64":        aispec.WithImageBase64,
	"imageRaw":           aispec.WithImageRaw,
	"toolCallCallback":   aispec.WithToolCallCallback,
}
