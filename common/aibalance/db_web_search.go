package aibalance

import (
	"errors"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// EnsureWebSearchApiKeyTable ensures the WebSearchApiKey table exists
func EnsureWebSearchApiKeyTable() error {
	db := GetDB()
	if err := db.AutoMigrate(&WebSearchApiKey{}).Error; err != nil {
		return err
	}
	if err := db.AutoMigrate(&WebSearchConfig{}).Error; err != nil {
		return err
	}
	return nil
}

// GetWebSearchConfig returns the singleton web search config (ID=1), creating it if not exists
func GetWebSearchConfig() (*WebSearchConfig, error) {
	var config WebSearchConfig
	db := GetDB()
	if err := db.Where("id = ?", 1).First(&config).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Real DB error (connection issue, etc.), return it directly
			return nil, fmt.Errorf("failed to query web search config: %v", err)
		}
		// Record not found, create default
		config = WebSearchConfig{}
		config.ID = 1
		if createErr := db.Create(&config).Error; createErr != nil {
			return nil, createErr
		}
	}
	return &config, nil
}

// SaveWebSearchConfig saves the global web search config
func SaveWebSearchConfig(config *WebSearchConfig) error {
	config.ID = 1 // Always use singleton ID
	return GetDB().Save(config).Error
}

// SaveWebSearchApiKey creates a new web search API key record
func SaveWebSearchApiKey(key *WebSearchApiKey) error {
	return GetDB().Create(key).Error
}

// GetAllWebSearchApiKeys returns all web search API keys
func GetAllWebSearchApiKeys() ([]*WebSearchApiKey, error) {
	var keys []*WebSearchApiKey
	if err := GetDB().Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetWebSearchApiKeysByType returns all web search API keys of a given type
func GetWebSearchApiKeysByType(searcherType string) ([]*WebSearchApiKey, error) {
	var keys []*WebSearchApiKey
	if err := GetDB().Where("searcher_type = ?", searcherType).Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetActiveWebSearchApiKeysByType returns active and healthy web search API keys of a given type
func GetActiveWebSearchApiKeysByType(searcherType string) ([]*WebSearchApiKey, error) {
	var keys []*WebSearchApiKey
	if err := GetDB().Where("searcher_type = ? AND active = ? AND is_healthy = ?", searcherType, true, true).Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetAllActiveWebSearchApiKeys returns all active web search API keys regardless of type
func GetAllActiveWebSearchApiKeys() ([]*WebSearchApiKey, error) {
	var keys []*WebSearchApiKey
	if err := GetDB().Where("active = ?", true).Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// GetWebSearchApiKeyByID returns a web search API key by its ID
func GetWebSearchApiKeyByID(id uint) (*WebSearchApiKey, error) {
	var key WebSearchApiKey
	if err := GetDB().Where("id = ?", id).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// UpdateWebSearchApiKey updates a web search API key record
func UpdateWebSearchApiKey(key *WebSearchApiKey) error {
	return GetDB().Save(key).Error
}

// DeleteWebSearchApiKeyByID deletes a web search API key by its ID
func DeleteWebSearchApiKeyByID(id uint) error {
	key, err := GetWebSearchApiKeyByID(id)
	if err != nil {
		return fmt.Errorf("failed to get web search api key: %v", err)
	}

	if err := GetDB().Delete(&WebSearchApiKey{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete web search api key: %v", err)
	}

	log.Infof("successfully deleted web search api key (ID: %d, Type: %s)", id, key.SearcherType)
	return nil
}

// UpdateWebSearchApiKeyStats updates statistics for a web search API key
func UpdateWebSearchApiKeyStats(id uint, success bool, latencyMs int64) error {
	var key WebSearchApiKey
	if err := GetDB().Where("id = ?", id).First(&key).Error; err != nil {
		return fmt.Errorf("failed to find web search api key: %v", err)
	}

	key.TotalRequests++
	key.LastUsedTime = time.Now()
	key.LastLatency = latencyMs

	if success {
		key.SuccessCount++
		key.ConsecutiveFailures = 0 // Reset consecutive failure counter on success
		key.IsHealthy = true
	} else {
		key.FailureCount++
		key.ConsecutiveFailures++
		// Mark as unhealthy after 3 consecutive failures
		if key.ConsecutiveFailures >= 3 {
			key.IsHealthy = false
			log.Warnf("web search api key (ID: %d, Type: %s) marked as unhealthy after %d consecutive failures", id, key.SearcherType, key.ConsecutiveFailures)
		}
	}

	return GetDB().Save(&key).Error
}

// UpdateWebSearchApiKeyStatus updates the active status of a web search API key
func UpdateWebSearchApiKeyStatus(id uint, active bool) error {
	return GetDB().Model(&WebSearchApiKey{}).Where("id = ?", id).
		Update("active", active).Error
}

// ResetWebSearchApiKeyHealth resets the health status of a web search API key to healthy
func ResetWebSearchApiKeyHealth(id uint) error {
	return GetDB().Model(&WebSearchApiKey{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_healthy":           true,
			"failure_count":        0,
			"consecutive_failures": 0,
		}).Error
}

// IncrementWebSearchConfigTotalRequests atomically increments the persistent total web search request counter.
// This counter survives process restarts (stored in WebSearchConfig singleton row).
func IncrementWebSearchConfigTotalRequests() error {
	db := GetDB()
	// Ensure the config row exists first
	_, err := GetWebSearchConfig()
	if err != nil {
		return fmt.Errorf("failed to ensure web search config: %v", err)
	}
	return db.Model(&WebSearchConfig{}).Where("id = ?", 1).
		UpdateColumn("total_web_search_requests", gorm.Expr("total_web_search_requests + ?", 1)).Error
}

// GetTotalWebSearchRequests returns the persistent cumulative web search request count from the database.
func GetTotalWebSearchRequests() int64 {
	config, err := GetWebSearchConfig()
	if err != nil {
		log.Errorf("failed to get web search config for total requests: %v", err)
		return 0
	}
	return config.TotalWebSearchRequests
}
