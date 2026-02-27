package yakit

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	aiProviderTypeAIBalance = "aibalance"
	aiProviderDefaultAPIKey = "free-user"
	aiProviderDefaultDomain = "aibalance.yaklang.com"
)

func CreateAIProvider(db *gorm.DB, provider *schema.AIThirdPartyConfig) error {
	if db == nil {
		return utils.Error("no set database")
	}
	if provider == nil {
		return utils.Error("provider is nil")
	}
	return db.Create(provider).Error
}

func UpdateAIProvider(db *gorm.DB, id int64, provider *schema.AIThirdPartyConfig) error {
	if db == nil {
		return utils.Error("no set database")
	}
	if provider == nil {
		return utils.Error("provider is nil")
	}
	updates := map[string]interface{}{
		"type":            provider.Type,
		"api_key":         provider.APIKey,
		"user_identifier": provider.UserIdentifier,
		"user_secret":     provider.UserSecret,
		"namespace":       provider.Namespace,
		"domain":          provider.Domain,
		"webhook_url":     provider.WebhookURL,
		"extra_params":    provider.ExtraParams,
		"disabled":        provider.Disabled,
	}
	return db.Model(&schema.AIThirdPartyConfig{}).Where("id = ?", id).Updates(updates).Error
}

func UpsertAIProvider(db *gorm.DB, provider *schema.AIThirdPartyConfig) (*schema.AIThirdPartyConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if provider == nil {
		return nil, utils.Error("provider is nil")
	}
	if provider.ID > 0 {
		if err := UpdateAIProvider(db, int64(provider.ID), provider); err != nil {
			return nil, err
		}
		return GetAIProvider(db, int64(provider.ID))
	}
	if err := CreateAIProvider(db, provider); err != nil {
		return nil, err
	}
	return provider, nil
}

func GetAIProvider(db *gorm.DB, id int64) (*schema.AIThirdPartyConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	var provider schema.AIThirdPartyConfig
	if err := db.Model(&schema.AIThirdPartyConfig{}).Where("id = ?", id).First(&provider).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

func ListAIProviders(db *gorm.DB) ([]*schema.AIThirdPartyConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	var providers []*schema.AIThirdPartyConfig
	if err := db.Model(&schema.AIThirdPartyConfig{}).Order("id asc").Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func DeleteAIProvider(db *gorm.DB, id int64) error {
	if db == nil {
		return utils.Error("no set database")
	}
	return db.Model(&schema.AIThirdPartyConfig{}).Where("id = ?", id).Unscoped().Delete(&schema.AIThirdPartyConfig{}).Error
}

func LoadAIProviderMap(db *gorm.DB) (map[int64]*schema.AIThirdPartyConfig, error) {
	providers, err := ListAIProviders(db)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]*schema.AIThirdPartyConfig, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		result[int64(p.ID)] = p
	}
	return result, nil
}

// EnsureAIBalanceProviderConfig ensures the default aibalance provider exists.
// This keeps fresh installs usable with free AI services.
func EnsureAIBalanceProviderConfig(db *gorm.DB) int64 {
	if db == nil {
		return 0
	}
	var existConfig schema.AIThirdPartyConfig
	if err := db.Model(&schema.AIThirdPartyConfig{}).Where("type = ?", aiProviderTypeAIBalance).First(&existConfig).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Warnf("query aibalance provider failed: %v", err)
		return 0
	}
	if existConfig.ID != 0 {
		return int64(existConfig.ID)
	}

	provider := &schema.AIThirdPartyConfig{
		Type:   aiProviderTypeAIBalance,
		APIKey: aiProviderDefaultAPIKey,
		Domain: aiProviderDefaultDomain,
	}
	if err := db.Create(provider).Error; err != nil {
		log.Warnf("create default aibalance provider failed: %v", err)
		return 0
	}
	log.Infof("Added default AIBalance provider config (key: %s)", aiProviderDefaultAPIKey)
	return int64(provider.ID)
}
