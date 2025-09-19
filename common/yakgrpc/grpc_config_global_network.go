package yakgrpc

import (
	"context"
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/amap"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
	INIT:
		if yakit.Get(consts.GLOBAL_NETWORK_CONFIG_INIT) == "" {
			log.Info("initialize global network config")
			defaultConfig := yakit.GetDefaultNetworkConfig()
			raw, err := json.Marshal(defaultConfig)
			if err != nil {
				return err
			}
			log.Infof("use config: %v", string(raw))
			yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
			yakit.ConfigureNetWork(defaultConfig)
			yakit.Set(consts.GLOBAL_NETWORK_CONFIG_INIT, "1")
			return nil
		} else {
			config := yakit.GetNetworkConfig()
			if config == nil {
				yakit.Set(consts.GLOBAL_NETWORK_CONFIG_INIT, "")
				goto INIT
			}
			log.Debugf("load global network config from database user config")
			log.Debugf("disable system dns: %v", config.DisableSystemDNS)
			log.Debugf("dns fallback tcp: %v", config.DNSFallbackTCP)
			log.Debugf("dns fallback doh: %v", config.DNSFallbackDoH)
			log.Debugf("custom dns servers: %v", config.CustomDNSServers)
			log.Debugf("custom doh servers: %v", config.CustomDoHServers)
			log.Debugf("disallow ip address: %v", config.DisallowIPAddress)
			log.Debugf("disallow domain: %v", config.DisallowDomain)
			log.Debugf("global proxy: %v", config.GlobalProxy)
			yakit.ConfigureNetWork(config)
			return nil
		}
	}, "sync-global-config-from-db")
}

func (s *Server) GetGlobalNetworkConfig(ctx context.Context, req *ypb.GetGlobalNetworkConfigRequest) (*ypb.GlobalNetworkConfig, error) {
	var config *ypb.GlobalNetworkConfig
	defer func() {
		raw, err := json.Marshal(config)
		if err != nil {
			log.Errorf("marshal config error: %v", err)
		}
		yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
	}()
	config = yakit.GetNetworkConfig()
	if config == nil {
		config = yakit.GetDefaultNetworkConfig()
		return config, nil
	}
	for _, appConfig := range config.AppConfigs {
		consts.ConvertCompatibleConfig(appConfig)
	}
	total := aispec.RegisteredAIGateways()
	canUseAiApiPriority := make([]string, 0)
	for _, s := range config.AiApiPriority { // remove deprecated ai type
		if utils.StringArrayContains(total, s) {
			canUseAiApiPriority = append(canUseAiApiPriority, s)
		}
	}
	config.AiApiPriority = canUseAiApiPriority
	for _, s := range total { // add new ai type
		if !utils.StringArrayContains(config.AiApiPriority, s) {
			config.AiApiPriority = append(config.AiApiPriority, s)
		}
	}
	return config, nil
}

func (s *Server) SetGlobalNetworkConfig(ctx context.Context, req *ypb.GlobalNetworkConfig) (*ypb.Empty, error) {
	defaultBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	yakit.ConfigureNetWork(req)
	yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(defaultBytes))
	return &ypb.Empty{}, nil
}

func (s *Server) ResetGlobalNetworkConfig(ctx context.Context, req *ypb.ResetGlobalNetworkConfigRequest) (*ypb.Empty, error) {
	defaultConfig := yakit.GetDefaultNetworkConfig()
	raw, err := json.Marshal(defaultConfig)
	if err != nil {
		return nil, err
	}
	yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
	yakit.ConfigureNetWork(defaultConfig)
	return &ypb.Empty{}, nil
}

func (s *Server) ValidP12PassWord(ctx context.Context, req *ypb.ValidP12PassWordRequest) (*ypb.ValidP12PassWordResponse, error) {
	data := true
	if len(req.GetPkcs12Bytes()) > 0 {
		_, _, _, err := tlsutils.LoadP12ToPEM(req.GetPkcs12Bytes(), string(req.GetPkcs12Password()))
		if err != nil {
			data = false
		}
	}
	return &ypb.ValidP12PassWordResponse{IsSetPassWord: data}, nil
}

func (s *Server) GetThirdPartyAppConfigTemplate(ctx context.Context, _ *ypb.Empty) (*ypb.GetThirdPartyAppConfigTemplateResponse, error) {
	//copyOpt := func(option *ypb.ThirdPartyAppConfigItemTemplate) *ypb.ThirdPartyAppConfigItemTemplate {
	//	return &ypb.ThirdPartyAppConfigItemTemplate{
	//		Name:         option.Name,
	//		Type:         option.Type,
	//		Verbose:      option.Verbose,
	//		Required:     option.Required,
	//		DefaultValue: option.DefaultValue,
	//		Desc:         option.Desc,
	//		Extra:        option.Extra,
	//	}
	//}
	newConfigTemplate := func(name, verbose, typeName string, hookOpt func(option *ypb.ThirdPartyAppConfigItemTemplate), opts ...*ypb.ThirdPartyAppConfigItemTemplate) *ypb.GetThirdPartyAppConfigTemplate {
		var copyedOpts []*ypb.ThirdPartyAppConfigItemTemplate
		for _, option := range opts {
			newOpt := &ypb.ThirdPartyAppConfigItemTemplate{
				Name:         option.Name,
				Type:         option.Type,
				Verbose:      option.Verbose,
				Required:     option.Required,
				DefaultValue: option.DefaultValue,
				Desc:         option.Desc,
				Extra:        option.Extra,
			}
			if hookOpt != nil {
				hookOpt(newOpt)
			}
			copyedOpts = append(copyedOpts, newOpt)
		}
		return &ypb.GetThirdPartyAppConfigTemplate{
			Name:    name,
			Verbose: verbose,
			Items:   copyedOpts,
			Type:    typeName,
		}
	}
	opts := make([]*ypb.GetThirdPartyAppConfigTemplate, 0)

	for _, name := range aispec.RegisteredAIGateways() {
		extTag := map[string]string{}
		hook := func(template *ypb.ThirdPartyAppConfigItemTemplate) {}
		verbose := name
		switch name {
		case "openai":
			verbose = "OpenAI"
			extTag["model"] = "default:gpt-3.5-turbo"
			extTag["domain"] = "default:api.openai.com"
		case "chatglm":
			verbose = "ChatGLM"
			extTag["model"] = "default:glm-4-flash"
			extTag["domain"] = "default:open.bigmodel.cn/api/paas/v4/chat/completions"
		case "comate":
			verbose = "Comate"
			extTag["api_key"] = "required:false"
			extTag["model"] = "default:ernie-bot"
			extTag["domain"] = "default:comate.baidu.com"
		case "moonshot":
			verbose = "Moonshot"
			extTag["model"] = "default:moonshot-v1-8k"
			extTag["domain"] = "default:api.moonshot.cn"
		case "tongyi":
			verbose = "Tongyi"
			extTag["model"] = "default:qwen-turbo"
			extTag["domain"] = "default:dashscope.aliyuncs.com"
		case "deepseek":
			verbose = "DeepSeek"
			extTag["model"] = "default:deepseek-chat"
			extTag["domain"] = "default:api.deepseek.com"
		case "siliconflow":
			verbose = "SiliconFlow"
			extTag["model"] = "default:deepseek-ai/DeepSeek-V3"
			extTag["domain"] = "default:api.siliconflow.cn"
		case "ollama":
			verbose = "Ollama"
			extTag["model"] = "default:llama3"
			extTag["domain"] = "default:localhost:11434"
		case "openrouter":
			verbose = "OpenRouter"
			extTag["model"] = "default:qwen/qwq-32b:free"
			extTag["domain"] = "default:openrouter.ai"
		}
		aiOptions, err := utils.ParseAppTagToOptions(&aispec.AIConfig{}, extTag)
		if err != nil {
			return nil, err
		}
		opts = append(opts, newConfigTemplate(name, verbose, "ai", hook, aiOptions...))
	}

	newSpaceEngineTmp := func(name string, verbose string, needEmail bool) *ypb.GetThirdPartyAppConfigTemplate {
		seOpts := []*ypb.ThirdPartyAppConfigItemTemplate{
			{
				Name:     "api_key",
				Verbose:  "ApiKey",
				Type:     "string",
				Desc:     "APIKey / Token",
				Required: true,
			},
		}
		if needEmail {
			seOpts = append(seOpts, &ypb.ThirdPartyAppConfigItemTemplate{
				Name:     "user_identifier",
				Verbose:  "用户信息",
				Type:     "string",
				Desc:     "email / username",
				Required: true,
			})
		}
		domain := ""
		switch name {
		case "shodan":
			domain = "https://api.shodan.io"
		case "fofa":
			domain = "https://fofa.info"
		case "quake":
			domain = "https://quake.360.net"
		case "hunter":
			domain = "https://hunter.qianxin.com"
		case "zoomeye":
			domain = "https://api.zoomeye.org"
		}

		seOpts = append(seOpts, &ypb.ThirdPartyAppConfigItemTemplate{
			Name:         "domain",
			Verbose:      "域名",
			Type:         "string",
			Desc:         "第三方加速域名",
			DefaultValue: domain,
			Required:     false,
		})
		return newConfigTemplate(name, verbose, "spaceengine", nil, seOpts...)
	}
	opts = append(opts, newSpaceEngineTmp("shodan", "Shodan", false))
	opts = append(opts, newSpaceEngineTmp("fofa", "Fofa", true))
	opts = append(opts, newSpaceEngineTmp("quake", "Quake", false))
	opts = append(opts, newSpaceEngineTmp("hunter", "Hunter", false))
	opts = append(opts, newSpaceEngineTmp("zoomeye", "ZoomEye", false))

	options, err := utils.ParseAppTagToOptions(&ostype.YakitOmniSearchKeyConfig{})
	if err != nil {
		log.Errorf("parse omnisearch app config tag to options failed: %v", err)
	} else {
		opts = append(opts, &ypb.GetThirdPartyAppConfigTemplate{
			Name:    "brave",
			Verbose: "Brave",
			Items:   options,
		})
		opts = append(opts, &ypb.GetThirdPartyAppConfigTemplate{
			Name:    "tavily",
			Verbose: "Tavily",
			Items:   options,
		})
	}
	amapOptions, err := utils.ParseAppTagToOptions(&amap.YakitAmapConfig{})
	if err != nil {
		log.Errorf("parse amap app config tag to options failed: %v", err)
	} else {
		opts = append(opts, &ypb.GetThirdPartyAppConfigTemplate{
			Name:    "amap",
			Verbose: "高德地图",
			Items:   amapOptions,
		})
	}
	embeddingOptions, err := utils.ParseAppTagToOptions(&plugins_rag.EmbeddingEndpointConfig{})
	if err != nil {
		log.Errorf("parse embedding endpoint app config tag to options failed: %v", err)
	} else {
		opts = append(opts, &ypb.GetThirdPartyAppConfigTemplate{
			Name:    "embedding_endpoint",
			Verbose: "Embedding Endpoint",
			Items:   embeddingOptions,
		})
	}
	//githubOpt := &ypb.GetThirdPartyAppConfigTemplate{
	//	Name:    "github",
	//	Verbose: "Github",
	//	Items: []*ypb.ThirdPartyAppConfigItemTemplate{
	//		{},
	//	},
	//	Type: "",
	//}
	////APIKey UserIdentifier
	//opts = append(opts, githubOpt)
	return &ypb.GetThirdPartyAppConfigTemplateResponse{Templates: opts}, nil
}
