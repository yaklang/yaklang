package aibalance

// db_mirror.go - aibalance 镜像规则 (AiMirrorRule) 的 CRUD 包装
//
// 设计取舍:
//   - 累计计数器 (TotalTriggered / Success / Failed / Dropped) 用 gorm.Expr 自增,
//     避免读-改-写竞争; 写库失败仅 logWarn, 不影响热路径.
//   - 所有函数返回 (*schema.AiMirrorRule, error), nil 语义清晰.
//   - LastTriggeredAt 由 IncrementMirrorCounters 一次性 UPDATE 写入, 不再单独函数.
//
// 关键词: aibalance mirror CRUD, AiMirrorRule, gorm.Expr 原子自增

import (
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// EnsureMirrorRuleTable ensures the AiMirrorRule table exists.
func EnsureMirrorRuleTable() error {
	return GetDB().AutoMigrate(&schema.AiMirrorRule{}).Error
}

// CreateMirrorRule 写入一条新规则; ID 由数据库回填到入参指针.
func CreateMirrorRule(rule *schema.AiMirrorRule) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if rule.Concurrency <= 0 {
		rule.Concurrency = 4
	}
	if rule.QueueSize <= 0 {
		rule.QueueSize = 1024
	}
	if rule.TimeoutMs <= 0 {
		rule.TimeoutMs = 30000
	}
	if err := GetDB().Create(rule).Error; err != nil {
		return fmt.Errorf("create mirror rule failed: %w", err)
	}
	log.Infof("mirror: created rule id=%d name=%q condition=%s", rule.ID, rule.Name, rule.ConditionType)
	return nil
}

// GetMirrorRuleByID 通过主键获取规则; 不存在返回 nil, nil.
func GetMirrorRuleByID(id uint) (*schema.AiMirrorRule, error) {
	var rule schema.AiMirrorRule
	err := GetDB().Where("id = ?", id).First(&rule).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// ListMirrorRules 返回全部规则 (含禁用), 按 ID 升序排列.
func ListMirrorRules() ([]*schema.AiMirrorRule, error) {
	var rules []*schema.AiMirrorRule
	if err := GetDB().Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ListEnabledMirrorRules 仅返回启用中的规则, 供 MirrorManager.LoadRules 使用.
func ListEnabledMirrorRules() ([]*schema.AiMirrorRule, error) {
	var rules []*schema.AiMirrorRule
	if err := GetDB().Where("enabled = ?", true).Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// UpdateMirrorRule 全字段更新 (除 ID / CreatedAt / 计数器外).
// 计数器走 IncrementMirrorCounters 独立通道, 不在此函数中触碰.
func UpdateMirrorRule(rule *schema.AiMirrorRule) error {
	if rule == nil || rule.ID == 0 {
		return fmt.Errorf("rule is nil or id is zero")
	}
	updates := map[string]interface{}{
		"name":            rule.Name,
		"enabled":         rule.Enabled,
		"condition_type":  rule.ConditionType,
		"action_name":     rule.ActionName,
		"tool_name":       rule.ToolName,
		"callback_script": rule.CallbackScript,
		"concurrency":     rule.Concurrency,
		"queue_size":      rule.QueueSize,
		"timeout_ms":      rule.TimeoutMs,
	}
	result := GetDB().Model(&schema.AiMirrorRule{}).Where("id = ?", rule.ID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update mirror rule failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	log.Infof("mirror: updated rule id=%d name=%q condition=%s", rule.ID, rule.Name, rule.ConditionType)
	return nil
}

// DeleteMirrorRule 物理删除 (gorm soft delete 同样会留 deleted_at).
func DeleteMirrorRule(id uint) error {
	result := GetDB().Delete(&schema.AiMirrorRule{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete mirror rule failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	log.Infof("mirror: deleted rule id=%d", id)
	return nil
}

// ToggleMirrorRule 切换 enabled 标记并返回新值.
func ToggleMirrorRule(id uint, enabled bool) error {
	result := GetDB().Model(&schema.AiMirrorRule{}).Where("id = ?", id).Update("enabled", enabled)
	if result.Error != nil {
		return fmt.Errorf("toggle mirror rule failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MirrorCounterDelta 描述一次回调结束后要写回 DB 的增量.
type MirrorCounterDelta struct {
	Triggered int64
	Success   int64
	Failed    int64
	Dropped   int64
	TouchTime bool // true 时 同步把 last_triggered_at 写为 now
}

// IncrementMirrorCounters 原子自增计数器; 任何字段 = 0 时跳过该字段写入,
// 避免不必要的 SQL 写操作. 设计成可批量 (一次调用代表 N 个增量) 也可单次.
//
// 关键词: aibalance mirror counters 原子自增, gorm.Expr
func IncrementMirrorCounters(id uint, delta MirrorCounterDelta) error {
	if id == 0 {
		return fmt.Errorf("id is zero")
	}
	updates := map[string]interface{}{}
	if delta.Triggered != 0 {
		updates["total_triggered"] = gorm.Expr("total_triggered + ?", delta.Triggered)
	}
	if delta.Success != 0 {
		updates["total_success"] = gorm.Expr("total_success + ?", delta.Success)
	}
	if delta.Failed != 0 {
		updates["total_failed"] = gorm.Expr("total_failed + ?", delta.Failed)
	}
	if delta.Dropped != 0 {
		updates["total_dropped"] = gorm.Expr("total_dropped + ?", delta.Dropped)
	}
	if delta.TouchTime {
		updates["last_triggered_at"] = time.Now()
	}
	if len(updates) == 0 {
		return nil
	}
	if err := GetDB().Model(&schema.AiMirrorRule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("increment mirror counters failed: %w", err)
	}
	return nil
}
