package consts

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
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
}

func GetThirdPartyApplicationConfig(t string) *thirdPartyApplicationConfig {
	if v, ok := thirdPartyConfig.Load(t); ok {
		return v.(*thirdPartyApplicationConfig)
	}
	return &thirdPartyApplicationConfig{}
}

func AllThirdPartyApplicationConfig() []*ypb.ThirdPartyApplicationConfig {
	var configs []*ypb.ThirdPartyApplicationConfig
	thirdPartyConfig.Range(func(key, value interface{}) bool {
		rawConfig := value.(*thirdPartyApplicationConfig)
		configs = append(configs, &ypb.ThirdPartyApplicationConfig{
			Type:           rawConfig.Type,
			APIKey:         rawConfig.APIKey,
			UserIdentifier: rawConfig.UserIdentifier,
			UserSecret:     rawConfig.UserSecret,
			Namespace:      rawConfig.Namespace,
			Domain:         rawConfig.Domain,
			WebhookURL:     rawConfig.WebhookURL,
		})
		return true
	})
	return configs
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
	}
	thirdPartyConfig.Store(config.Type, c)
}
