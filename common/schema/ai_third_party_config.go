package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIThirdPartyConfig stores AI provider configuration synced with ypb.ThirdPartyApplicationConfig.
type AIThirdPartyConfig struct {
	gorm.Model

	Type           string          `json:"type" gorm:"index"`
	APIKey         string          `json:"api_key"`
	UserIdentifier string          `json:"user_identifier"`
	UserSecret     string          `json:"user_secret"`
	Namespace      string          `json:"namespace"`
	Domain         string          `json:"domain"`
	WebhookURL     string          `json:"webhook_url"`
	ExtraParams    MapStringString `json:"extra_params" gorm:"type:text"`
	Disabled       bool            `json:"disabled" gorm:"default:false"`
}

func (c *AIThirdPartyConfig) TableName() string {
	return "ai_third_party_configs"
}

func (c *AIThirdPartyConfig) ToThirdPartyConfig() *ypb.ThirdPartyApplicationConfig {
	if c == nil {
		return nil
	}
	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:           c.Type,
		APIKey:         c.APIKey,
		UserIdentifier: c.UserIdentifier,
		UserSecret:     c.UserSecret,
		Namespace:      c.Namespace,
		Domain:         c.Domain,
		WebhookURL:     c.WebhookURL,
		Disabled:       c.Disabled,
	}
	if len(c.ExtraParams) > 0 {
		cfg.ExtraParams = make([]*ypb.KVPair, 0, len(c.ExtraParams))
		for k, v := range c.ExtraParams {
			cfg.ExtraParams = append(cfg.ExtraParams, &ypb.KVPair{Key: k, Value: v})
		}
	}
	return cfg
}

func (c *AIThirdPartyConfig) ToAIProvider() *ypb.AIProvider {
	if c == nil {
		return nil
	}
	return &ypb.AIProvider{
		Id:     int64(c.ID),
		Config: c.ToThirdPartyConfig(),
	}
}

func AIThirdPartyConfigFromGRPC(cfg *ypb.ThirdPartyApplicationConfig) *AIThirdPartyConfig {
	if cfg == nil {
		return nil
	}
	extra := make(MapStringString)
	for _, kv := range cfg.GetExtraParams() {
		extra[kv.GetKey()] = kv.GetValue()
	}
	return &AIThirdPartyConfig{
		Type:           cfg.GetType(),
		APIKey:         cfg.GetAPIKey(),
		UserIdentifier: cfg.GetUserIdentifier(),
		UserSecret:     cfg.GetUserSecret(),
		Namespace:      cfg.GetNamespace(),
		Domain:         cfg.GetDomain(),
		WebhookURL:     cfg.GetWebhookURL(),
		ExtraParams:    extra,
		Disabled:       cfg.GetDisabled(),
	}
}
