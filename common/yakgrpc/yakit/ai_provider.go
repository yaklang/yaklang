package yakit

import (
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	provider.Hash = provider.CalcHash()
	return db.Create(provider).Error
}

func UpdateAIProvider(db *gorm.DB, id int64, provider *schema.AIThirdPartyConfig) error {
	if db == nil {
		return utils.Error("no set database")
	}
	if provider == nil {
		return utils.Error("provider is nil")
	}
	provider.Hash = provider.CalcHash()
	updates := map[string]interface{}{
		"hash":            provider.Hash,
		"type":            provider.Type,
		"api_key":         provider.APIKey,
		"user_identifier": provider.UserIdentifier,
		"user_secret":     provider.UserSecret,
		"namespace":       provider.Namespace,
		"domain":          provider.Domain,
		"webhook_url":     provider.WebhookURL,
		"extra_params":    provider.ExtraParams,
		"disabled":        provider.Disabled,
		"proxy":           provider.Proxy,
		"no_https":        provider.NoHttps,
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
	provider.Hash = provider.CalcHash()

	var existProvider schema.AIThirdPartyConfig
	err := db.Model(&schema.AIThirdPartyConfig{}).Where("hash = ?", provider.Hash).First(&existProvider).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		if err := UpdateAIProvider(db, int64(existProvider.ID), provider); err != nil {
			return nil, err
		}
		return GetAIProvider(db, int64(existProvider.ID))
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

func FilterAIProvider(db *gorm.DB, filter *ypb.AIProviderFilter) *gorm.DB {
	db = db.Model(&schema.AIThirdPartyConfig{})
	if filter == nil {
		return db
	}

	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetIds())
	db = bizhelper.ExactQueryStringArrayOr(db, "type", filter.GetAIType())
	return db
}

func QueryAIProviders(db *gorm.DB, filter *ypb.AIProviderFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AIThirdPartyConfig, error) {
	if db == nil {
		return nil, nil, utils.Error("no set database")
	}

	db = FilterAIProvider(db, filter)

	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"}
	}
	if paging.GetPage() <= 0 {
		paging.Page = 1
	}
	if paging.GetLimit() == 0 {
		paging.Limit = 10
	}
	if paging.GetRawOrder() == "" && paging.GetOrderBy() == "" {
		paging.OrderBy = "id"
	}
	if paging.GetRawOrder() == "" && paging.GetOrder() == "" {
		paging.Order = "asc"
	}

	var providers []*schema.AIThirdPartyConfig
	pag, db := bizhelper.YakitPagingQuery(db, paging, &providers)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, providers, nil
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
