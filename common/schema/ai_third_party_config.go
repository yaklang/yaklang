package schema

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const aiThirdPartyConfigTableName = "ai_third_party_configs"

// AIThirdPartyConfig stores AI provider configuration synced with ypb.ThirdPartyApplicationConfig.
type AIThirdPartyConfig struct {
	gorm.Model

	Hash           string          `json:"hash" gorm:"unique_index"`
	Type           string          `json:"type" gorm:"index"`
	APIKey         string          `json:"api_key"`
	UserIdentifier string          `json:"user_identifier"`
	UserSecret     string          `json:"user_secret"`
	Namespace      string          `json:"namespace"`
	Domain         string          `json:"domain"`
	BaseURL        string          `json:"base_url"`
	Endpoint       string          `json:"endpoint"`
	EnableEndpoint bool            `json:"enable_endpoint" gorm:"default:false"`
	WebhookURL     string          `json:"webhook_url"`
	ExtraParams    MapStringString `json:"extra_params" gorm:"type:text"`
	APIType        string          `json:"api_type"`
	Disabled       bool            `json:"disabled" gorm:"default:false"`
	Proxy          string          `json:"proxy"`
	NoHttps        bool            `json:"no_https" gorm:"default:false"`
}

func (c *AIThirdPartyConfig) CalcHash() string {
	if c == nil {
		return ""
	}
	keys := make([]string, 0, len(c.ExtraParams))
	for k := range c.ExtraParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, k := range keys {
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(c.ExtraParams[k])
		builder.WriteString(";")
	}
	return utils.CalcSha256(
		c.Type,
		c.APIKey,
		c.UserIdentifier,
		c.UserSecret,
		c.Namespace,
		c.Domain,
		c.BaseURL,
		c.Endpoint,
		c.WebhookURL,
		builder.String(),
		c.Proxy,
		c.NoHttps,
		c.EnableEndpoint,
	)
}

func (c *AIThirdPartyConfig) BeforeSave() error {
	c.Hash = c.CalcHash()
	return nil
}

func (c *AIThirdPartyConfig) TableName() string {
	return aiThirdPartyConfigTableName
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
		BaseURL:        c.BaseURL,
		Endpoint:       c.Endpoint,
		EnableEndpoint: c.EnableEndpoint,
		WebhookURL:     c.WebhookURL,
		Disabled:       c.Disabled,
		Proxy:          c.Proxy,
		NoHttps:        c.NoHttps,
		APIType:        c.APIType,
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

	err := utils.ImportAppConfigToStruct(cfg, extra)
	if err != nil {
		log.Errorf("ImportAppConfigToStruct failed: %v", err)
	}

	return &AIThirdPartyConfig{
		Type:           cfg.GetType(),
		APIKey:         cfg.GetAPIKey(),
		UserIdentifier: cfg.GetUserIdentifier(),
		UserSecret:     cfg.GetUserSecret(),
		Namespace:      cfg.GetNamespace(),
		Domain:         cfg.GetDomain(),
		BaseURL:        cfg.GetBaseURL(),
		Endpoint:       cfg.GetEndpoint(),
		EnableEndpoint: cfg.GetEnableEndpoint(),
		WebhookURL:     cfg.GetWebhookURL(),
		ExtraParams:    extra,
		Disabled:       cfg.GetDisabled(),
		Proxy:          cfg.GetProxy(),
		NoHttps:        cfg.GetNoHttps(),
		APIType:        cfg.GetAPIType(),
	}
}
