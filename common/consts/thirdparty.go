package consts

import (
	"errors"
	"github.com/yaklang/yaklang/common/utils"
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

func (c *thirdPartyApplicationConfig) GetExtraParam(name string) string {
	if c.ExtraParams == nil {
		return ""
	}
	if v, ok := c.ExtraParams[name]; ok {
		return v
	}
	return ""
}
func (c *thirdPartyApplicationConfig) ToMap() map[string]string {
	params := map[string]string{}
	params["api_key"] = c.APIKey
	params["user_identifier"] = c.UserIdentifier
	params["user_secret"] = c.UserSecret
	params["domain"] = c.Domain
	params["webhook_url"] = c.WebhookURL
	params["namespace"] = c.Namespace
	for k, v := range c.ExtraParams {
		params[k] = v
	}
	return params
}

// GetThirdPartyApplicationConfig
// first argument is the type of third party application, second argument is the config struct pointer,
// this function will fill the config struct with the third party application config
func GetThirdPartyApplicationConfig(t string, cfg any) error {
	if v, ok := thirdPartyConfig.Load(t); ok {
		rawCfg := v.(*thirdPartyApplicationConfig)
		params := rawCfg.ToMap()
		return utils.ApplyAppConfig(cfg, params)
	}
	return errors.New("third party application config not found")
}

// GetThirdPartyApplicationConfig has deprecated
func _GetThirdPartyApplicationConfig(t string) *thirdPartyApplicationConfig {
	if v, ok := thirdPartyConfig.Load(t); ok {
		return v.(*thirdPartyApplicationConfig)
	}
	return &thirdPartyApplicationConfig{ExtraParams: make(map[string]string)}
}

// AllThirdPartyApplicationConfig has deprecated
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
