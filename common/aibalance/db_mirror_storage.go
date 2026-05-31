package aibalance

// db_mirror_storage.go - 镜像数据落盘配置 (AiMirrorStorageConfig) 的单例读写
//
// 与 AiBalanceRateLimitConfig 一样是 ID=1 的单例行; 缺失时按默认值创建。
// 读出后会对 <=0 的容量字段做默认值兜底 (不写库), 兼容老行。
//
// 关键词: db_mirror_storage, AiMirrorStorageConfig 单例, 落盘配置读写

import (
	"errors"
	"fmt"

	"github.com/jinzhu/gorm"
)

// EnsureMirrorStorageConfigTable ensures the AiMirrorStorageConfig table exists.
func EnsureMirrorStorageConfigTable() error {
	return GetDB().AutoMigrate(&AiMirrorStorageConfig{}).Error
}

// GetMirrorStorageConfig 返回落盘配置单例 (ID=1), 不存在则按默认值创建。
// 关键词: GetMirrorStorageConfig, 单例 + 默认值兜底
func GetMirrorStorageConfig() (*AiMirrorStorageConfig, error) {
	var cfg AiMirrorStorageConfig
	db := GetDB()
	if err := db.Where("id = ?", 1).First(&cfg).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to query mirror storage config: %v", err)
		}
		cfg = AiMirrorStorageConfig{
			Enabled:          false,
			MaxBytes:         defaultDataSinkMaxBytes,
			ReclaimBytes:     defaultDataSinkReclaimBytes,
			CheckIntervalSec: defaultDataSinkCheckSec,
		}
		cfg.ID = 1
		if createErr := db.Create(&cfg).Error; createErr != nil {
			return nil, createErr
		}
	}
	// 老行 / 异常值兜底（不写库）。
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultDataSinkMaxBytes
	}
	if cfg.ReclaimBytes <= 0 {
		cfg.ReclaimBytes = defaultDataSinkReclaimBytes
	}
	if cfg.CheckIntervalSec <= 0 {
		cfg.CheckIntervalSec = defaultDataSinkCheckSec
	}
	return &cfg, nil
}

// SaveMirrorStorageConfig 写回落盘配置单例 (ID=1)。
// 关键词: SaveMirrorStorageConfig, 单例 upsert
func SaveMirrorStorageConfig(cfg *AiMirrorStorageConfig) error {
	if cfg == nil {
		return fmt.Errorf("nil mirror storage config")
	}
	cfg.ID = 1
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultDataSinkMaxBytes
	}
	if cfg.ReclaimBytes <= 0 {
		cfg.ReclaimBytes = defaultDataSinkReclaimBytes
	}
	if cfg.CheckIntervalSec <= 0 {
		cfg.CheckIntervalSec = defaultDataSinkCheckSec
	}
	db := GetDB()
	var existing AiMirrorStorageConfig
	if err := db.Where("id = ?", 1).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return db.Create(cfg).Error
		}
		return err
	}
	return db.Model(&AiMirrorStorageConfig{}).Where("id = ?", 1).Updates(map[string]interface{}{
		"enabled":            cfg.Enabled,
		"max_bytes":          cfg.MaxBytes,
		"reclaim_bytes":      cfg.ReclaimBytes,
		"check_interval_sec": cfg.CheckIntervalSec,
	}).Error
}

// applyMirrorStorageConfig 把配置热应用到全局落盘器 (装配 + 启用/容量参数)。
// 关键词: applyMirrorStorageConfig, 热应用落盘配置
func applyMirrorStorageConfig(cfg *AiMirrorStorageConfig) {
	if cfg == nil {
		return
	}
	initDataSink(cfg.Enabled, cfg.MaxBytes, cfg.ReclaimBytes, cfg.CheckIntervalSec)
}
