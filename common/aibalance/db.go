package aibalance

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func GetDB() *gorm.DB {
	return schema.GetGormProfileDatabase()
}

func SaveAiProvider(provider *schema.AiProvider) error {
	return GetDB().Create(provider).Error
}

func GetOrCreateAiProvider(provider *schema.AiProvider) (*schema.AiProvider, error) {
	var existingProvider schema.AiProvider
	if err := GetDB().Where("wrapper_name = ? AND model_name = ? AND api_key = ?",
		provider.WrapperName, provider.ModelName, provider.APIKey).First(&existingProvider).Error; err != nil {
		// 如果找不到记录，创建一个新的
		if err := GetDB().Create(provider).Error; err != nil {
			return nil, err
		}
		return provider, nil
	}
	// 返回已存在的记录（带有ID）
	return &existingProvider, nil
}

func GetAllAiProviders() ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := schema.GetGormProfileDatabase().Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func UpdateAiProvider(provider *schema.AiProvider) error {
	return GetDB().Save(provider).Error
}
