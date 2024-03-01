package consts

import (
	"sync"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// thirdparty app config
var thirdPartyConfig = new(sync.Map)

type thirdPartyApplicationConfig struct {
	Type           string
	APIKey         string
	UserIdentifier string
	UserSecret     string
	Namespace      string
	Domain         string
	WebhookURL     string
	ExtraParams    map[string]string
}

// GetExtraParam 获取 Yakit 第三方应用配置的其他信息值，如: OpenAI的model,domain,proxy
// Example:
// ```
// config = yakit.GetThirdPartyAppConfig("openai")
// model = config.GetExtraParam("model") // 获取配置 openai 模型名称
// ```
func (c *thirdPartyApplicationConfig) GetExtraParam(name string) string {
	if c.ExtraParams == nil {
		return ""
	}
	if v, ok := c.ExtraParams[name]; ok {
		return v
	}
	return ""
}

// GetOpenAIModel 获取 OpenAI类型第三方应用配置的模型名称
// Example:
// ```
// config = yakit.GetThirdPartyAppConfig("openai")
// model = config.GetOpenAIModel() // 获取openai的模型名称
// ```
func (c *thirdPartyApplicationConfig) GetOpenAIModel() string {
	return c.GetExtraParam("model")
}

// GetOpenAIDomain 获取 OpenAI类型第三方应用配置的第三方加速域名
// Example:
// ```
// config = yakit.GetThirdPartyAppConfig("openai")
// domain = config.GetOpenAIDomain() // 获取openai的第三方加速域名
// ```
func (c *thirdPartyApplicationConfig) GetOpenAIDomain() string {
	return c.GetExtraParam("domain")
}

// GetOpenAIProxy 获取 OpenAI类型第三方应用配置的代理
// Example:
// ```
// config = yakit.GetThirdPartyAppConfig("openai")
// proxy = config.GetOpenAIProxy() // 获取openai的代理
// ```
func (c *thirdPartyApplicationConfig) GetOpenAIProxy() string {
	return c.GetExtraParam("proxy")
}

// GetThirdPartyAppConfig 获取对应类型的 Yakit 第三方应用配置
func GetThirdPartyApplicationConfig(typ string) *thirdPartyApplicationConfig {
	if v, ok := thirdPartyConfig.Load(typ); ok {
		return v.(*thirdPartyApplicationConfig)
	}
	return &thirdPartyApplicationConfig{ExtraParams: make(map[string]string)}
}

func AllThirdPartyApplicationConfig() []*ypb.ThirdPartyApplicationConfig {
	var configs []*ypb.ThirdPartyApplicationConfig
	thirdPartyConfig.Range(func(key, value interface{}) bool {
		rawConfig := value.(*thirdPartyApplicationConfig)
		config := &ypb.ThirdPartyApplicationConfig{
			Type:           rawConfig.Type,
			APIKey:         rawConfig.APIKey,
			UserIdentifier: rawConfig.UserIdentifier,
			UserSecret:     rawConfig.UserSecret,
			Namespace:      rawConfig.Namespace,
			Domain:         rawConfig.Domain,
			WebhookURL:     rawConfig.WebhookURL,
			ExtraParams:    make([]*ypb.KVPair, 0, len(rawConfig.ExtraParams)),
		}
		for k, v := range rawConfig.ExtraParams {
			config.ExtraParams = append(config.ExtraParams, &ypb.KVPair{Key: k, Value: v})
		}

		configs = append(configs, config)
		return true
	})
	return configs
}

func ClearThirdPartyApplicationConfig() {
	thirdPartyConfig.Range(func(key, value interface{}) bool {
		thirdPartyConfig.Delete(key)
		return true
	})
}

func UpdateThirdPartyApplicationConfig(config *ypb.ThirdPartyApplicationConfig) {
	if config.Type == "" {
		return
	}

	c := &thirdPartyApplicationConfig{
		Type:           config.Type,
		APIKey:         config.APIKey,
		UserIdentifier: config.UserIdentifier,
		UserSecret:     config.UserSecret,
		Namespace:      config.Namespace,
		Domain:         config.Domain,
		WebhookURL:     config.WebhookURL,
		ExtraParams:    make(map[string]string, len(config.ExtraParams)),
	}
	for _, kv := range config.ExtraParams {
		c.ExtraParams[kv.Key] = kv.Value
	}

	thirdPartyConfig.Store(config.Type, c)
}
