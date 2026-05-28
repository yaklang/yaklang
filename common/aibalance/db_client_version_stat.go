package aibalance

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// EnsureClientVersionStatTable ensures the AiBalanceClientVersionStat table exists.
// 关键词: EnsureClientVersionStatTable 客户端版本统计表
func EnsureClientVersionStatTable() error {
	return GetDB().AutoMigrate(&schema.AiBalanceClientVersionStat{}).Error
}

// RecordClientVersion upserts a client-version statistics record.
// 行为:
//   - 同一 version 已存在: count++, last_seen 更新; 若 buildTime 非空则覆盖
//   - 新 version: 创建记录, first_seen 与 last_seen 都设为当前时间
//
// 该函数永不返回错误给调用者: 数据库写入异常一律降级为日志, 防止阻塞 chat 主链路。
// 关键词: RecordClientVersion upsert 客户端版本, last_seen 更新, first_seen 写入
func RecordClientVersion(version string, buildTime string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "unknown"
	}
	buildTime = strings.TrimSpace(buildTime)

	nowUnix := time.Now().Unix()

	db := GetDB()
	var existing schema.AiBalanceClientVersionStat
	err := db.Where("version = ?", version).First(&existing).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warnf("RecordClientVersion: query failed for version %s: %v", version, err)
			return err
		}
		// 新增记录
		fresh := schema.AiBalanceClientVersionStat{
			Version:       version,
			BuildTime:     buildTime,
			FirstSeenUnix: nowUnix,
			LastSeenUnix:  nowUnix,
			RequestCount:  1,
		}
		if createErr := db.Create(&fresh).Error; createErr != nil {
			log.Warnf("RecordClientVersion: create failed for version %s: %v", version, createErr)
			return createErr
		}
		return nil
	}

	// 已存在: 更新计数与最近时间; 若有新 buildTime 上报则覆盖
	updates := map[string]interface{}{
		"last_seen_unix": nowUnix,
		"request_count":  existing.RequestCount + 1,
	}
	if buildTime != "" {
		updates["build_time"] = buildTime
	}
	if updErr := db.Model(&schema.AiBalanceClientVersionStat{}).
		Where("id = ?", existing.ID).Updates(updates).Error; updErr != nil {
		log.Warnf("RecordClientVersion: update failed for version %s: %v", version, updErr)
		return updErr
	}
	return nil
}

// QueryTopClientVersions 按 last_seen_unix DESC, request_count DESC 排序取前 limit 条。
// limit <= 0 时按 20 兜底, > 200 钳到 200, 避免 portal 误填爆库。
// 关键词: QueryTopClientVersions Top N 版本, portal 客户端版本展示
func QueryTopClientVersions(limit int) ([]schema.AiBalanceClientVersionStat, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var items []schema.AiBalanceClientVersionStat
	if err := GetDB().Order("last_seen_unix DESC, request_count DESC").
		Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("query top client versions: %w", err)
	}
	return items, nil
}
