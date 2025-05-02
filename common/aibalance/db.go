package aibalance

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
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

// GetAiProvidersByModelName 从数据库获取指定模型名称(WrapperName)的所有Provider
func GetAiProvidersByModelName(modelName string) ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := GetDB().Where("wrapper_name = ?", modelName).Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// GetAiProvidersByModelType 从数据库获取指定模型类型(TypeName)的所有Provider
func GetAiProvidersByModelType(typeName string) ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := GetDB().Where("type_name = ?", typeName).Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// RegisterAiProvider 注册一个新的 AI 提供者到数据库
// wrapperName: 外部展示给用户的模型名称
// modelName: 内部实际使用的模型名称
// typeName: 提供者类型，如 openai, chatglm 等
// domainOrUrl: API 域名或 URL
// apiKey: API 密钥
// noHTTPS: 是否禁用 HTTPS
func RegisterAiProvider(wrapperName, modelName, typeName, domainOrUrl, apiKey string, noHTTPS bool) (*schema.AiProvider, error) {
	// 创建提供者对象
	provider := &schema.AiProvider{
		WrapperName:       wrapperName,
		ModelName:         modelName,
		TypeName:          typeName,
		DomainOrURL:       domainOrUrl,
		APIKey:            apiKey,
		NoHTTPS:           noHTTPS,
		SuccessCount:      0,
		FailureCount:      0,
		TotalRequests:     0,
		LastRequestTime:   time.Now(),
		LastRequestStatus: true,
		LastLatency:       0,
		IsHealthy:         true,
		HealthCheckTime:   time.Now(),
	}

	// 检查是否存在相同的提供者
	var existingProvider schema.AiProvider
	if err := GetDB().Where("wrapper_name = ? AND model_name = ? AND api_key = ?",
		wrapperName, modelName, apiKey).First(&existingProvider).Error; err == nil {
		// 如果已存在，返回现有提供者
		log.Infof("Provider already exists: WrapperName=%s, ModelName=%s, TypeName=%s",
			wrapperName, modelName, typeName)
		return &existingProvider, nil
	}

	// 创建新提供者
	if err := GetDB().Create(provider).Error; err != nil {
		return nil, err
	}

	log.Infof("Successfully registered new AI provider: WrapperName=%s, ModelName=%s, TypeName=%s",
		wrapperName, modelName, typeName)
	return provider, nil
}

func UpdateAiProvider(provider *schema.AiProvider) error {
	return GetDB().Save(provider).Error
}
