package aibalance

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
)

// defaultModelDowngradeRules 是轻量模型降级的内置默认规则：当客户端上报 tier=lightweight
// 且请求 memfit-standard-free 时，自动降级到 memfit-light-free 以保护用量。
// 老配置行（ModelDowngradeRules 为空）会回退到该默认值；管理员若想彻底关闭降级，
// 在 portal 写入 "[]" 即可（空字符串视为未配置）。
// 关键词: defaultModelDowngradeRules, 轻量降级内置规则, memfit-standard-free → memfit-light-free
const defaultModelDowngradeRules = `[{"tier":"lightweight","from":"memfit-standard-free","to":"memfit-light-free"}]`

// EnsureRateLimitConfigTable ensures the AiBalanceRateLimitConfig table exists.
func EnsureRateLimitConfigTable() error {
	db := GetDB()
	return db.AutoMigrate(&AiBalanceRateLimitConfig{}).Error
}

// GetRateLimitConfig returns the singleton rate-limit config (ID=1), creating with defaults if absent.
func GetRateLimitConfig() (*AiBalanceRateLimitConfig, error) {
	var config AiBalanceRateLimitConfig
	db := GetDB()
	if err := db.Where("id = ?", 1).First(&config).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to query rate limit config: %v", err)
		}
		config = AiBalanceRateLimitConfig{
			DefaultRPM:                  600,
			FreeUserDelaySec:            3,
			ModelRPMOverrides:           "{}",
			ModelDelayOverrides:         "{}",
			FreeUserTokenLimitM:         1200,
			FreeUserTokenModelOverrides: "{}",
			FreeUserDelayMaxSec:         0,
			FreeUserOutputTPS:           0,
			ModelOutputTPSOverrides:     "{}",
			FreeUserTokenSoftLimitM:     0,
			FreeUserSoftLimitTPS:        0,
			// memfit-* 客户端版本控流默认关闭, 默认无最低 BuildTime
			// 关键词: GetRateLimitConfig 默认值 MemfitVersionGate
			MemfitVersionGateEnabled:  false,
			MemfitVersionMinBuildTime: "",
			// 自定义 429 文案默认关闭（关闭时完全保持现有文案）
			// 关键词: GetRateLimitConfig 默认值 Custom429
			Custom429Enabled:       false,
			Custom429Notice:        "",
			Custom429KindOverrides: "{}",
			// 轻量降级默认带内置规则（memfit-standard-free → memfit-light-free）
			// 关键词: GetRateLimitConfig 默认值 ModelDowngradeRules
			ModelDowngradeRules: defaultModelDowngradeRules,
			// 一键限流 IP 默认参数（RPM=3 / TPS=15）
			// 关键词: GetRateLimitConfig 默认值 ThrottledIPDefault
			ThrottledIPDefaultRPM: 3,
			ThrottledIPDefaultTPS: 15,
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
	// 老行兼容：新字段为空时给出安全默认（不写库）。Custom429KindOverrides 至少为 "{}"，
	// ModelDowngradeRules 为空回退内置降级规则（"[]" 表示管理员显式关闭，不回退）。
	// 关键词: GetRateLimitConfig 老行兼容, Custom429KindOverrides 默认, ModelDowngradeRules 回退
	if strings.TrimSpace(config.Custom429KindOverrides) == "" {
		config.Custom429KindOverrides = "{}"
	}
	if strings.TrimSpace(config.ModelDowngradeRules) == "" {
		config.ModelDowngradeRules = defaultModelDowngradeRules
	}
	// 老行兼容：一键限流默认参数 <=0 视作未配置，按 3/15 兜底（不写库）。
	// 关键词: GetRateLimitConfig 老行兼容, ThrottledIPDefault 兜底
	if config.ThrottledIPDefaultRPM <= 0 {
		config.ThrottledIPDefaultRPM = 3
	}
	if config.ThrottledIPDefaultTPS <= 0 {
		config.ThrottledIPDefaultTPS = 15
	}
	return &config, nil
}

// SaveRateLimitConfig saves the global rate-limit config.
func SaveRateLimitConfig(config *AiBalanceRateLimitConfig) error {
	config.ID = 1
	return GetDB().Save(config).Error
}
