package schema

import (
	"fmt"
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
	EnableThinking bool            `json:"enable_thinking" gorm:"default:false"`
	// 以下为可选模型参数（nil 表示未配置）
	MaxTokens          *int64   `json:"max_tokens,omitempty" gorm:"column:max_tokens"`
	Temperature        *float64 `json:"temperature,omitempty" gorm:"column:temperature"`
	TopP               *float64 `json:"top_p,omitempty" gorm:"column:top_p"`
	TopK               *int64   `json:"top_k,omitempty" gorm:"column:top_k"`
	FrequencyPenalty   *float64 `json:"frequency_penalty,omitempty" gorm:"column:frequency_penalty"`
	ReasoningEffort    string   `json:"reasoning_effort,omitempty" gorm:"column:reasoning_effort"`
	WebhookURL         string   `json:"webhook_url"`
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
		c.EnableThinking,
		optionalInt64ForHash(c.MaxTokens),
		optionalFloat64ForHash(c.Temperature),
		optionalFloat64ForHash(c.TopP),
		optionalInt64ForHash(c.TopK),
		optionalFloat64ForHash(c.FrequencyPenalty),
		c.ReasoningEffort,
	)
}

func optionalInt64ForHash(p *int64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

func optionalFloat64ForHash(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%g", *p)
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
		EnableThinking: c.EnableThinking,
		WebhookURL:     c.WebhookURL,
		Disabled:       c.Disabled,
		Proxy:          c.Proxy,
		NoHttps:        c.NoHttps,
		APIType:        c.APIType,
	}
	if c.MaxTokens != nil {
		v := *c.MaxTokens
		cfg.MaxTokens = &v
	}
	if c.Temperature != nil {
		v := *c.Temperature
		cfg.Temperature = &v
	}
	if c.TopP != nil {
		v := *c.TopP
		cfg.TopP = &v
	}
	if c.TopK != nil {
		v := *c.TopK
		cfg.TopK = &v
	}
	if c.FrequencyPenalty != nil {
		v := *c.FrequencyPenalty
		cfg.FrequencyPenalty = &v
	}
	if strings.TrimSpace(c.ReasoningEffort) != "" {
		s := strings.TrimSpace(c.ReasoningEffort)
		cfg.ReasoningEffort = &s
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

	out := &AIThirdPartyConfig{
		Type:           cfg.GetType(),
		APIKey:         cfg.GetAPIKey(),
		UserIdentifier: cfg.GetUserIdentifier(),
		UserSecret:     cfg.GetUserSecret(),
		Namespace:      cfg.GetNamespace(),
		Domain:         cfg.GetDomain(),
		BaseURL:        cfg.GetBaseURL(),
		Endpoint:       cfg.GetEndpoint(),
		EnableEndpoint: cfg.GetEnableEndpoint(),
		EnableThinking: cfg.GetEnableThinking(),
		WebhookURL:     cfg.GetWebhookURL(),
		ExtraParams:    extra,
		Disabled:       cfg.GetDisabled(),
		Proxy:          cfg.GetProxy(),
		NoHttps:        cfg.GetNoHttps(),
		APIType:        cfg.GetAPIType(),
	}
	if cfg.MaxTokens != nil {
		v := *cfg.MaxTokens
		out.MaxTokens = &v
	}
	if cfg.Temperature != nil {
		v := *cfg.Temperature
		out.Temperature = &v
	}
	if cfg.TopP != nil {
		v := *cfg.TopP
		out.TopP = &v
	}
	if cfg.TopK != nil {
		v := *cfg.TopK
		out.TopK = &v
	}
	if cfg.FrequencyPenalty != nil {
		v := *cfg.FrequencyPenalty
		out.FrequencyPenalty = &v
	}
	if cfg.ReasoningEffort != nil {
		out.ReasoningEffort = strings.TrimSpace(*cfg.ReasoningEffort)
	}
	return out
}
