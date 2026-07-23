package yakit

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// UpsertPluginUsageCount 在 profile 库中记录/累加插件使用次数。
// 由 SavePluginExecutionHistory 双写调用：plugin_id 已存在则 count+1，否则新建 count=1。
func UpsertPluginUsageCount(db *gorm.DB, pluginId int64, pluginName, pluginUUID, pluginType, headImg string) error {
	if db == nil {
		return utils.Error("no set database")
	}
	if pluginId <= 0 {
		return nil // 临时脚本/纯代码执行不计入使用次数
	}

	var existing schema.PluginUsageCount
	err := db.Model(&schema.PluginUsageCount{}).Where("plugin_id = ?", pluginId).First(&existing).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// 新建
			record := &schema.PluginUsageCount{
				PluginId:   pluginId,
				PluginName: pluginName,
				PluginUUID: pluginUUID,
				PluginType: pluginType,
				HeadImg:    headImg,
				Count:      1,
				LastUsedAt: time.Now().Unix(),
			}
			if err := db.Create(record).Error; err != nil {
				return utils.Errorf("create plugin usage count failed: %s", err)
			}
			return nil
		}
		return utils.Errorf("query plugin usage count failed: %s", err)
	}

	// 累加 + 更新元数据
	updates := map[string]interface{}{
		"count":        existing.Count + 1,
		"last_used_at": time.Now().Unix(),
	}
	if pluginName != "" {
		updates["plugin_name"] = pluginName
	}
	if pluginType != "" {
		updates["plugin_type"] = pluginType
	}
	if headImg != "" {
		updates["head_img"] = headImg
	}
	if pluginUUID != "" {
		updates["plugin_uuid"] = pluginUUID
	}
	if err := db.Model(&schema.PluginUsageCount{}).Where("plugin_id = ?", pluginId).Updates(updates).Error; err != nil {
		return utils.Errorf("update plugin usage count failed: %s", err)
	}
	return nil
}

// QueryPluginUsageCountRanking 从 profile 库查询插件使用次数排行（按 count 降序）。
func QueryPluginUsageCountRanking(db *gorm.DB, limit int) ([]*ypb.PluginExecutionUsageItem, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if limit <= 0 {
		limit = 50
	}

	var records []*schema.PluginUsageCount
	if err := db.Model(&schema.PluginUsageCount{}).
		Where("plugin_id > 0").
		Order("count DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, utils.Errorf("query plugin usage count ranking failed: %s", err)
	}

	result := make([]*ypb.PluginExecutionUsageItem, 0, len(records))
	for _, r := range records {
		result = append(result, &ypb.PluginExecutionUsageItem{
			PluginId:       r.PluginId,
			PluginName:     r.PluginName,
			PluginUUID:     r.PluginUUID,
			PluginType:     r.PluginType,
			HeadImg:        r.HeadImg,
			Count:          r.Count,
			LastExecutedAt: r.LastUsedAt,
		})
	}
	return result, nil
}

// GetPluginUsageCountByID 从 profile 库查单个插件的使用次数（供调试/其他用途）。
func GetPluginUsageCountByID(db *gorm.DB, pluginId int64) (int64, error) {
	if db == nil {
		return 0, utils.Error("no set database")
	}
	var record schema.PluginUsageCount
	if err := db.Model(&schema.PluginUsageCount{}).Where("plugin_id = ?", pluginId).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	return record.Count, nil
}