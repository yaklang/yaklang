package yakgrpc

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/amap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	thirdPartyTemplateTypeAI          = "ai"
	thirdPartyTemplateTypeSpaceEngine = "spaceengine"
)

type aiGatewayTemplateProfile struct {
	Verbose string
	ExtTag  map[string]string
}

var aiGatewayTemplateProfiles = map[string]aiGatewayTemplateProfile{
	"aibalance": {
		Verbose: "AIBalance",
		ExtTag: map[string]string{
			"model":   "default:memfit-light-free",
			"api_key": "default:free-user,required:false",
		},
	},
	"openai": {
		Verbose: "OpenAI",
		ExtTag: map[string]string{
			"model":  "default:gpt-3.5-turbo",
			"domain": "default:api.openai.com",
		},
	},
	"custom": {
		Verbose: "自定义AI配置",
		ExtTag: map[string]string{
			"model":  "default:memfit-light-free",
			"domain": "default:aibalance.yaklang.com",
		},
	},
	"chatglm": {
		Verbose: "ChatGLM",
		ExtTag: map[string]string{
			"model":  "default:glm-4-flash",
			"domain": "default:open.bigmodel.cn/api/paas/v4/chat/completions",
		},
	},
	"comate": {
		Verbose: "Comate",
		ExtTag: map[string]string{
			"api_key": "required:false",
			"model":   "default:ernie-bot",
			"domain":  "default:comate.baidu.com",
		},
	},
	"moonshot": {
		Verbose: "Moonshot",
		ExtTag: map[string]string{
			"model":  "default:moonshot-v1-8k",
			"domain": "default:api.moonshot.cn",
		},
	},
	"tongyi": {
		Verbose: "Tongyi",
		ExtTag: map[string]string{
			"model":  "default:qwen-turbo",
			"domain": "default:dashscope.aliyuncs.com",
		},
	},
	"deepseek": {
		Verbose: "DeepSeek",
		ExtTag: map[string]string{
			"model":  "default:deepseek-chat",
			"domain": "default:api.deepseek.com",
		},
	},
	"siliconflow": {
		Verbose: "SiliconFlow",
		ExtTag: map[string]string{
			"model":  "default:deepseek-ai/DeepSeek-V3",
			"domain": "default:api.siliconflow.cn",
		},
	},
	"ollama": {
		Verbose: "Ollama",
		ExtTag: map[string]string{
			"model":  "default:llama3",
			"domain": "default:localhost:11434",
		},
	},
	"openrouter": {
		Verbose: "OpenRouter",
		ExtTag: map[string]string{
			"model":  "default:qwen/qwq-32b:free",
			"domain": "default:openrouter.ai",
		},
	},
}

type spaceEngineTemplateProfile struct {
	Name      string
	Verbose   string
	Domain    string
	NeedEmail bool
}

var spaceEngineTemplateProfiles = []spaceEngineTemplateProfile{
	{Name: "shodan", Verbose: "Shodan", Domain: "https://api.shodan.io"},
	{Name: "fofa", Verbose: "Fofa", Domain: "https://fofa.info", NeedEmail: true},
	{Name: "quake", Verbose: "Quake", Domain: "https://quake.360.net"},
	{Name: "hunter", Verbose: "Hunter", Domain: "https://hunter.qianxin.com"},
	{Name: "zoomeye", Verbose: "ZoomEye", Domain: "https://api.zoomeye.org"},
}

func buildThirdPartyAppConfigTemplates() ([]*ypb.GetThirdPartyAppConfigTemplate, error) {
	templates := make([]*ypb.GetThirdPartyAppConfigTemplate, 0)

	aiTemplates, err := buildAIGatewayTemplates()
	if err != nil {
		return nil, err
	}
	templates = append(templates, aiTemplates...)
	templates = append(templates, buildSpaceEngineTemplates()...)

	omniSearchOptions, err := utils.ParseAppTagToOptions(&ostype.YakitOmniSearchKeyConfig{})
	if err != nil {
		log.Errorf("parse omnisearch app config tag to options failed: %v", err)
	} else {
		templates = append(templates, newThirdPartyAppConfigTemplate("brave", "Brave", "", omniSearchOptions...))
		templates = append(templates, newThirdPartyAppConfigTemplate("tavily", "Tavily", "", omniSearchOptions...))
	}

	amapOptions, err := utils.ParseAppTagToOptions(&amap.YakitAmapConfig{})
	if err != nil {
		log.Errorf("parse amap app config tag to options failed: %v", err)
	} else {
		templates = append(templates, newThirdPartyAppConfigTemplate("amap", "高德地图", "", amapOptions...))
	}

	embeddingOptions, err := utils.ParseAppTagToOptions(&plugins_rag.EmbeddingEndpointConfig{})
	if err != nil {
		log.Errorf("parse embedding endpoint app config tag to options failed: %v", err)
	} else {
		templates = append(templates, newThirdPartyAppConfigTemplate("embedding_endpoint", "Embedding Endpoint", "", embeddingOptions...))
	}

	return templates, nil
}

func buildAIGatewayTemplates() ([]*ypb.GetThirdPartyAppConfigTemplate, error) {
	templates := make([]*ypb.GetThirdPartyAppConfigTemplate, 0)
	for _, name := range aispec.RegisteredAIGateways() {
		profile, ok := aiGatewayTemplateProfiles[name]
		verbose := name
		extTag := make(map[string]string)
		if ok {
			verbose = profile.Verbose
			extTag = profile.ExtTag
		}

		aiOptions, err := utils.ParseAppTagToOptions(&aispec.AIConfig{}, extTag)
		if err != nil {
			return nil, err
		}
		templates = append(templates, newThirdPartyAppConfigTemplate(name, verbose, thirdPartyTemplateTypeAI, aiOptions...))
	}
	return templates, nil
}

func buildSpaceEngineTemplates() []*ypb.GetThirdPartyAppConfigTemplate {
	templates := make([]*ypb.GetThirdPartyAppConfigTemplate, 0, len(spaceEngineTemplateProfiles))
	for _, profile := range spaceEngineTemplateProfiles {
		items := []*ypb.ThirdPartyAppConfigItemTemplate{
			{
				Name:     "api_key",
				Verbose:  "ApiKey",
				Type:     "string",
				Desc:     "APIKey / Token",
				Required: true,
			},
		}
		if profile.NeedEmail {
			items = append(items, &ypb.ThirdPartyAppConfigItemTemplate{
				Name:     "user_identifier",
				Verbose:  "用户信息",
				Type:     "string",
				Desc:     "email / username",
				Required: true,
			})
		}
		items = append(items, &ypb.ThirdPartyAppConfigItemTemplate{
			Name:         "domain",
			Verbose:      "域名",
			Type:         "string",
			Desc:         "第三方加速域名",
			DefaultValue: profile.Domain,
			Required:     false,
		})
		templates = append(templates, newThirdPartyAppConfigTemplate(profile.Name, profile.Verbose, thirdPartyTemplateTypeSpaceEngine, items...))
	}
	return templates
}

func newThirdPartyAppConfigTemplate(name, verbose, typeName string, items ...*ypb.ThirdPartyAppConfigItemTemplate) *ypb.GetThirdPartyAppConfigTemplate {
	copiedItems := make([]*ypb.ThirdPartyAppConfigItemTemplate, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copiedItems = append(copiedItems, &ypb.ThirdPartyAppConfigItemTemplate{
			Name:         item.Name,
			Type:         item.Type,
			Verbose:      item.Verbose,
			Required:     item.Required,
			DefaultValue: item.DefaultValue,
			Desc:         item.Desc,
			Extra:        item.Extra,
		})
	}

	return &ypb.GetThirdPartyAppConfigTemplate{
		Name:    name,
		Verbose: verbose,
		Type:    typeName,
		Items:   copiedItems,
	}
}
