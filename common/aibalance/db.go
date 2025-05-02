package aibalance

import (
	"fmt"
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

// SaveAiApiKey 保存API密钥到数据库
func SaveAiApiKey(apiKey string, allowedModels string) error {
	key := &schema.AiApiKeys{
		APIKey:        apiKey,
		AllowedModels: allowedModels,
	}
	return GetDB().Create(key).Error
}

// GetAiApiKey 根据API密钥获取数据库记录
func GetAiApiKey(apiKey string) (*schema.AiApiKeys, error) {
	var key schema.AiApiKeys
	if err := GetDB().Where("api_key = ?", apiKey).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// GetAllAiApiKeys 获取所有API密钥
func GetAllAiApiKeys() ([]*schema.AiApiKeys, error) {
	var keys []*schema.AiApiKeys
	if err := GetDB().Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteAiApiKey 删除API密钥
func DeleteAiApiKey(apiKey string) error {
	return GetDB().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{}).Error
}

// UpdateAiApiKey 更新API密钥的允许模型
func UpdateAiApiKey(apiKey string, allowedModels string) error {
	return GetDB().Model(&schema.AiApiKeys{}).Where("api_key = ?", apiKey).
		Update("allowed_models", allowedModels).Error
}

// GetAiProviderByID 根据ID获取单个AI提供者
func GetAiProviderByID(id uint) (*schema.AiProvider, error) {
	var provider schema.AiProvider
	if err := GetDB().Where("id = ?", id).First(&provider).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteAiProviderByID 根据ID删除AI提供者
func DeleteAiProviderByID(id uint) error {
	// 先获取提供者信息，便于日志记录
	provider, err := GetAiProviderByID(id)
	if err != nil {
		return fmt.Errorf("获取提供者信息失败: %v", err)
	}

	// 执行删除操作
	if err := GetDB().Delete(&schema.AiProvider{}, id).Error; err != nil {
		return fmt.Errorf("删除提供者失败: %v", err)
	}

	// 记录删除日志
	log.Infof("成功删除AI提供者 (ID: %d, 名称: %s, 模型: %s)",
		id, provider.WrapperName, provider.ModelName)

	return nil
}
