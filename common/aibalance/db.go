package aibalance

import "github.com/yaklang/yaklang/common/schema"

func SaveAiProvider(provider *schema.AiProvider) error {
	return schema.GetGormProfileDatabase().Create(provider).Error
}

func GetOrCreateAiProvider(wrapperName string, apiKey string) (*schema.AiProvider, error) {
	var provider schema.AiProvider
	if err := schema.GetGormProfileDatabase().Where("wrapper_name = ? AND api_key = ?", wrapperName, apiKey).First(&provider).Error; err != nil {
		// 如果找不到记录，创建一个新的
		provider = schema.AiProvider{
			WrapperName: wrapperName,
			APIKey:      apiKey,
		}
		if err := schema.GetGormProfileDatabase().Create(&provider).Error; err != nil {
			return nil, err
		}
	}
	return &provider, nil
}

func GetAllAiProviders() ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := schema.GetGormProfileDatabase().Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func UpdateAiProvider(provider *schema.AiProvider) error {
	return schema.GetGormProfileDatabase().Save(provider).Error
}

func DeleteAiProvider(wrapperName string) error {
	return schema.GetGormProfileDatabase().Where("wrapper_name = ?", wrapperName).Delete(&schema.AiProvider{}).Error
}
