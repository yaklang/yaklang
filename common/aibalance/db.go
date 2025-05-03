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
		// If record not found, create a new one
		if err := GetDB().Create(provider).Error; err != nil {
			return nil, err
		}
		return provider, nil
	}
	// Return existing record (with ID)
	return &existingProvider, nil
}

func GetAllAiProviders() ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := schema.GetGormProfileDatabase().Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// GetAiProvidersByModelName gets all providers with specified model name (WrapperName) from database
func GetAiProvidersByModelName(modelName string) ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := GetDB().Where("wrapper_name = ?", modelName).Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// GetAiProvidersByModelType gets all providers with specified model type (TypeName) from database
func GetAiProvidersByModelType(typeName string) ([]*schema.AiProvider, error) {
	var providers []*schema.AiProvider
	if err := GetDB().Where("type_name = ?", typeName).Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// RegisterAiProvider registers a new AI provider to the database
// wrapperName: model name displayed to users
// modelName: actual model name used internally
// typeName: provider type, such as openai, chatglm, etc.
// domainOrUrl: API domain or URL
// apiKey: API key
// noHTTPS: whether to disable HTTPS
func RegisterAiProvider(wrapperName, modelName, typeName, domainOrUrl, apiKey string, noHTTPS bool) (*schema.AiProvider, error) {
	// Create provider object
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

	// Check if provider with same details exists
	var existingProvider schema.AiProvider
	if err := GetDB().Where("wrapper_name = ? AND model_name = ? AND api_key = ?",
		wrapperName, modelName, apiKey).First(&existingProvider).Error; err == nil {
		// If exists, return existing provider
		log.Infof("Provider already exists: WrapperName=%s, ModelName=%s, TypeName=%s",
			wrapperName, modelName, typeName)
		return &existingProvider, nil
	}

	// Create new provider
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

// SaveAiApiKey saves API key to database
func SaveAiApiKey(apiKey string, allowedModels string) error {
	key := &schema.AiApiKeys{
		APIKey:        apiKey,
		AllowedModels: allowedModels,
		InputBytes:    0,
		OutputBytes:   0,
		UsageCount:    0,
		SuccessCount:  0,
		FailureCount:  0,
		LastUsedTime:  time.Now(),
	}
	return GetDB().Create(key).Error
}

// GetAiApiKey gets database record by API key
func GetAiApiKey(apiKey string) (*schema.AiApiKeys, error) {
	var key schema.AiApiKeys
	if err := GetDB().Where("api_key = ?", apiKey).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// GetAllAiApiKeys gets all API keys
func GetAllAiApiKeys() ([]*schema.AiApiKeys, error) {
	var keys []*schema.AiApiKeys
	if err := GetDB().Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteAiApiKey deletes API key
func DeleteAiApiKey(apiKey string) error {
	return GetDB().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{}).Error
}

// UpdateAiApiKey updates allowed models for API key
func UpdateAiApiKey(apiKey string, allowedModels string) error {
	return GetDB().Model(&schema.AiApiKeys{}).Where("api_key = ?", apiKey).
		Update("allowed_models", allowedModels).Error
}

// GetAiProviderByID gets single AI provider by ID
func GetAiProviderByID(id uint) (*schema.AiProvider, error) {
	var provider schema.AiProvider
	if err := GetDB().Where("id = ?", id).First(&provider).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteAiProviderByID deletes AI provider by ID
func DeleteAiProviderByID(id uint) error {
	// Get provider info first for logging
	provider, err := GetAiProviderByID(id)
	if err != nil {
		return fmt.Errorf("Failed to get provider info: %v", err)
	}

	// Execute delete operation
	if err := GetDB().Delete(&schema.AiProvider{}, id).Error; err != nil {
		return fmt.Errorf("Failed to delete provider: %v", err)
	}

	// Log deletion
	log.Infof("Successfully deleted AI provider (ID: %d, Name: %s, Model: %s)",
		id, provider.WrapperName, provider.ModelName)

	return nil
}

// UpdateAiApiKeyStats 更新 API Key 的使用统计信息
// apiKey：API密钥
// inputBytes：本次请求的输入字节数
// outputBytes：本次请求的输出字节数
// success：请求是否成功
func UpdateAiApiKeyStats(apiKey string, inputBytes, outputBytes int64, success bool) error {
	// 获取数据库中的 API Key 记录
	var key schema.AiApiKeys
	if err := GetDB().Where("api_key = ?", apiKey).First(&key).Error; err != nil {
		return fmt.Errorf("Failed to find API key: %v", err)
	}

	// 更新统计信息
	key.UsageCount++
	key.InputBytes += inputBytes
	key.OutputBytes += outputBytes
	key.LastUsedTime = time.Now()

	if success {
		key.SuccessCount++
	} else {
		key.FailureCount++
	}

	// 保存到数据库
	return GetDB().Save(&key).Error
}

// UpdateAiApiKeyStatus 更新单个 API Key 的激活状态
func UpdateAiApiKeyStatus(id uint, active bool) error {
	result := GetDB().Model(&schema.AiApiKeys{}).Where("id = ?", id).Update("active", active)
	if result.Error != nil {
		return fmt.Errorf("failed to update status for API key ID %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		// 如果没有行受影响，可能是因为 ID 不存在
		return gorm.ErrRecordNotFound // 返回 GORM 的标准错误
	}
	action := "deactivated"
	if active {
		action = "activated"
	}
	log.Infof("Successfully %s API key (ID: %d)", action, id)
	return nil
}

// BatchUpdateAiApiKeyStatus 批量更新 API Key 的激活状态
func BatchUpdateAiApiKeyStatus(ids []uint, active bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil // 没有 ID 需要更新
	}
	result := GetDB().Model(&schema.AiApiKeys{}).Where("id IN (?)", ids).Update("active", active)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to batch update status for %d API keys: %w", len(ids), result.Error)
	}
	action := "deactivated"
	if active {
		action = "activated"
	}
	log.Infof("Successfully %s %d API keys (requested %d)", action, result.RowsAffected, len(ids))
	return result.RowsAffected, nil
}
