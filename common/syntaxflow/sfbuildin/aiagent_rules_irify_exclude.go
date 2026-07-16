//go:build irify_exclude

package sfbuildin

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

// SyncAIAgentEmbedRule 在 irify_exclude 模式下，返回错误
func SyncAIAgentEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	return ErrSyntaxFlowNotAvailable
}

// ForceSyncAIAgentEmbedRule 在 irify_exclude 模式下，返回错误
func ForceSyncAIAgentEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	return ErrSyntaxFlowNotAvailable
}

// ForceSyncAIAgentEmbedRuleToDB 在 irify_exclude 模式下，返回错误
func ForceSyncAIAgentEmbedRuleToDB(db *gorm.DB, notifies ...func(process float64, ruleName string)) (err error) {
	return ErrSyntaxFlowNotAvailable
}

// AIAgentRuleHash 在 irify_exclude 模式下，返回错误
func AIAgentRuleHash() (string, error) {
	return "", ErrSyntaxFlowNotAvailable
}

// NeedSyncAIAgentEmbedRule 在 irify_exclude 模式下，始终返回 false
func NeedSyncAIAgentEmbedRule() bool {
	return false
}

// DoneAIAgentEmbedRule 在 irify_exclude 模式下，不执行任何操作
func DoneAIAgentEmbedRule() {}

// GetAIAgentRuleFS 在 irify_exclude 模式下，返回 nil
func GetAIAgentRuleFS() resources_monitor.ResourceMonitor {
	return nil
}

// InitAIAgentEmbedFS 在 irify_exclude 模式下，不执行任何操作
func InitAIAgentEmbedFS() {}
