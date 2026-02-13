package aibalance

import (
	"errors"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// EnsureAmapApiKeyTable ensures the AmapApiKey and AmapConfig tables exist
func EnsureAmapApiKeyTable() error {
	db := GetDB()
	if err := db.AutoMigrate(&schema.AmapApiKey{}).Error; err != nil {
		return err
	}
	if err := db.AutoMigrate(&schema.AmapConfig{}).Error; err != nil {
		return err
	}
	return nil
}

// GetAmapConfig returns the singleton amap config (ID=1), creating it if not exists
func GetAmapConfig() (*schema.AmapConfig, error) {
	var config schema.AmapConfig
	db := GetDB()
	if err := db.Where("id = ?", 1).First(&config).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to query amap config: %v", err)
		}
		// Record not found, create default
		config = schema.AmapConfig{}
		config.ID = 1
		config.AllowFreeUserAmap = true
		if createErr := db.Create(&config).Error; createErr != nil {
			return nil, createErr
		}
	}
	return &config, nil
}

// SaveAmapConfig saves the global amap config
func SaveAmapConfig(config *schema.AmapConfig) error {
	config.ID = 1 // Always use singleton ID
	return GetDB().Save(config).Error
}

// SaveAmapApiKey creates a new amap API key record
func SaveAmapApiKey(key *schema.AmapApiKey) error {
	return GetDB().Create(key).Error
}

// GetAllAmapApiKeys returns all amap API keys
func GetAllAmapApiKeys() ([]*schema.AmapApiKey, error) {
	var keys []*schema.AmapApiKey
	if err := GetDB().Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetActiveAmapApiKeys returns active and healthy amap API keys
func GetActiveAmapApiKeys() ([]*schema.AmapApiKey, error) {
	var keys []*schema.AmapApiKey
	if err := GetDB().Where("active = ? AND is_healthy = ?", true, true).Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetAllActiveAmapApiKeys returns all active amap API keys regardless of health
func GetAllActiveAmapApiKeys() ([]*schema.AmapApiKey, error) {
	var keys []*schema.AmapApiKey
	if err := GetDB().Where("active = ?", true).Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetAmapApiKeyByID returns an amap API key by its ID
func GetAmapApiKeyByID(id uint) (*schema.AmapApiKey, error) {
	var key schema.AmapApiKey
	if err := GetDB().Where("id = ?", id).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// UpdateAmapApiKey updates an amap API key record
func UpdateAmapApiKey(key *schema.AmapApiKey) error {
	return GetDB().Save(key).Error
}

// DeleteAmapApiKeyByID deletes an amap API key by its ID
func DeleteAmapApiKeyByID(id uint) error {
	key, err := GetAmapApiKeyByID(id)
	if err != nil {
		return fmt.Errorf("failed to get amap api key: %v", err)
	}

	if err := GetDB().Delete(&schema.AmapApiKey{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete amap api key: %v", err)
	}

	log.Infof("successfully deleted amap api key (ID: %d, Key: %s***)", id, maskAPIKeyShort(key.APIKey))
	return nil
}

// UpdateAmapApiKeyStats updates statistics for an amap API key (used during proxy requests)
func UpdateAmapApiKeyStats(id uint, success bool, latencyMs int64) error {
	var key schema.AmapApiKey
	if err := GetDB().Where("id = ?", id).First(&key).Error; err != nil {
		return fmt.Errorf("failed to find amap api key: %v", err)
	}

	key.TotalRequests++
	key.LastUsedTime = time.Now()
	key.LastLatency = latencyMs

	if success {
		key.SuccessCount++
		key.ConsecutiveFailures = 0
		key.IsHealthy = true
	} else {
		key.FailureCount++
		key.ConsecutiveFailures++
		if key.ConsecutiveFailures >= 3 {
			key.IsHealthy = false
			log.Warnf("amap api key (ID: %d) marked as unhealthy after %d consecutive failures", id, key.ConsecutiveFailures)
		}
	}

	return GetDB().Save(&key).Error
}

// UpdateAmapApiKeyHealthStatus updates the health check status for an amap API key
func UpdateAmapApiKeyHealthStatus(id uint, healthy bool, latencyMs int64, checkError string) error {
	return GetDB().Model(&schema.AmapApiKey{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_healthy":        healthy,
			"health_check_time": time.Now(),
			"last_check_error":  checkError,
			"last_latency":      latencyMs,
		}).Error
}

// UpdateAmapApiKeyStatus updates the active status of an amap API key
func UpdateAmapApiKeyStatus(id uint, active bool) error {
	return GetDB().Model(&schema.AmapApiKey{}).Where("id = ?", id).
		Update("active", active).Error
}

// ResetAmapApiKeyHealth resets the health status of an amap API key to healthy
func ResetAmapApiKeyHealth(id uint) error {
	return GetDB().Model(&schema.AmapApiKey{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_healthy":           true,
			"failure_count":        0,
			"consecutive_failures": 0,
			"last_check_error":     "",
		}).Error
}

// IncrementAmapConfigTotalRequests atomically increments the persistent total amap request counter.
func IncrementAmapConfigTotalRequests() error {
	db := GetDB()
	_, err := GetAmapConfig()
	if err != nil {
		return fmt.Errorf("failed to ensure amap config: %v", err)
	}
	return db.Model(&schema.AmapConfig{}).Where("id = ?", 1).
		UpdateColumn("total_amap_requests", gorm.Expr("total_amap_requests + ?", 1)).Error
}

// GetTotalAmapRequests returns the persistent cumulative amap request count from the database.
func GetTotalAmapRequests() int64 {
	config, err := GetAmapConfig()
	if err != nil {
		log.Errorf("failed to get amap config for total requests: %v", err)
		return 0
	}
	return config.TotalAmapRequests
}

// maskAPIKeyShort returns first 4 chars of the key for logging
func maskAPIKeyShort(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[:4]
}
