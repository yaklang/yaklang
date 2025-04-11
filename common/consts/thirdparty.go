package consts

import (
	"errors"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// thirdparty app config
var thirdPartyConfig = new(sync.Map)

type thirdPartyApplicationConfig struct {
	Type           string
	APIKey         string `app:"name:api_key"`
	UserIdentifier string `app:"name:user_identifier"`
	UserSecret     string `app:"name:user_secret"`
	Namespace      string `app:"name:namespace"`
	Domain         string `app:"name:domain"`
	WebhookURL     string `app:"name:webhook_url"`
	ExtraParams    map[string]string
}

func ConvertCompatibleConfig(config *ypb.ThirdPartyApplicationConfig) {
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

	err := utils.ImportAppConfigToStruct(c, c.ExtraParams)
	if err != nil {
		log.Errorf("ImportAppConfigToStruct failed: %v", err)
		return
	}

	data, err := utils.ExportAppConfigToMap(c)
	if err != nil {
		log.Errorf("ConvertCompatibleConfig failed: %v", err)
		return
	}

	for k, v := range data {
		c.ExtraParams[k] = v
	}
	config.APIKey = c.APIKey
	config.UserIdentifier = c.UserIdentifier
	config.UserSecret = c.UserSecret
	config.Namespace = c.Namespace
	config.Domain = c.Domain
	config.WebhookURL = c.WebhookURL
	config.ExtraParams = nil
	for k, v := range c.ExtraParams { // ExtraParams will be disordered
		config.ExtraParams = append(config.ExtraParams, &ypb.KVPair{
			Key:   k,
			Value: v,
		})
	}
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
	params, err := utils.ExportAppConfigToMap(c)
	if err != nil {
		log.Errorf("ExportAppConfigToMap failed: %v", err)
	}
	for k, v := range c.ExtraParams {
		params[k] = v
	}
	return params
}
func GetCommonThirdPartyApplicationConfig(t string) (*ypb.ThirdPartyApplicationConfig, error) {
	if v, ok := thirdPartyConfig.Load(t); ok {
		rawCfg := v.(*thirdPartyApplicationConfig)
		utils.ImportAppConfigToStruct(rawCfg, rawCfg.ToMap())
		config := &ypb.ThirdPartyApplicationConfig{}
		config.APIKey = rawCfg.APIKey
		config.UserIdentifier = rawCfg.UserIdentifier
		config.UserSecret = rawCfg.UserSecret
		config.Namespace = rawCfg.Namespace
		config.Domain = rawCfg.Domain
		config.WebhookURL = rawCfg.WebhookURL
		config.Type = t
		return config, nil
	}
	return nil, errors.New("third party application config not found")
}

// GetThirdPartyApplicationConfig
// first argument is the type of third party application, second argument is the config struct pointer,
// this function will fill the config struct with the third party application config
func GetThirdPartyApplicationConfig(t string, cfg any) error {
	if v, ok := thirdPartyConfig.Load(t); ok {
		rawCfg := v.(*thirdPartyApplicationConfig)
		params := rawCfg.ToMap()
		return utils.ImportAppConfigToStruct(cfg, params)
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
	ConvertCompatibleConfig(config)
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
