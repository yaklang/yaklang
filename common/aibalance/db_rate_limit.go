package aibalance

import (
	"errors"
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// EnsureRateLimitConfigTable ensures the AiBalanceRateLimitConfig table exists.
func EnsureRateLimitConfigTable() error {
	db := GetDB()
	return db.AutoMigrate(&schema.AiBalanceRateLimitConfig{}).Error
}

// GetRateLimitConfig returns the singleton rate-limit config (ID=1), creating with defaults if absent.
func GetRateLimitConfig() (*schema.AiBalanceRateLimitConfig, error) {
	var config schema.AiBalanceRateLimitConfig
	db := GetDB()
	if err := db.Where("id = ?", 1).First(&config).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to query rate limit config: %v", err)
		}
		config = schema.AiBalanceRateLimitConfig{
			DefaultRPM:                  600,
			FreeUserDelaySec:            3,
			ModelRPMOverrides:           "{}",
			ModelDelayOverrides:         "{}",
			FreeUserTokenLimitM:         1200,
			FreeUserTokenModelOverrides: "{}",
		}
		config.ID = 1
		if createErr := db.Create(&config).Error; createErr != nil {
			return nil, createErr
		}
	}
	// 老行兼容：FreeUserTokenLimitM == 0 视作未配置，按默认 1200 兜底（不写库）。
	// 关键词: GetRateLimitConfig 老行兼容, FreeUserTokenLimitM 默认值兜底
	if config.FreeUserTokenLimitM <= 0 {
		config.FreeUserTokenLimitM = 1200
	}
	return &config, nil
}

// SaveRateLimitConfig saves the global rate-limit config.
func SaveRateLimitConfig(config *schema.AiBalanceRateLimitConfig) error {
	config.ID = 1
	return GetDB().Save(config).Error
}
