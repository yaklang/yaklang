//go:build irify_exclude

package sfbuildin

import (
	"embed"
	"errors"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

var (
	// ErrSyntaxFlowNotAvailable indicates that SyntaxFlow features are not available in the current build
	ErrSyntaxFlowNotAvailable = errors.New("SyntaxFlow is not available in this build. Please use the full version (build without -tags irify_exclude)")
)

// SyncEmbedRule 在 irify_exclude 模式下，返回错误提示需要使用完整版
// SyntaxFlow 内置规则已被排除
func SyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	return ErrSyntaxFlowNotAvailable
}

// ForceSyncEmbedRule 在 irify_exclude 模式下，返回错误提示需要使用完整版
func ForceSyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	return ErrSyntaxFlowNotAvailable
}

// NeedSyncEmbedRule 在 irify_exclude 模式下，始终返回 false
func NeedSyncEmbedRule() bool {
	return false
}

// DoneEmbedRule 在 irify_exclude 模式下，不执行任何操作
func DoneEmbedRule() {
	// no-op
}

// SyntaxFlowRuleHash 在 irify_exclude 模式下，返回错误提示需要使用完整版
func SyntaxFlowRuleHash() (string, error) {
	return "", ErrSyntaxFlowNotAvailable
}

// SyncRuleFromFileSystem 在 irify_exclude 模式下，返回错误提示需要使用完整版
func SyncRuleFromFileSystem(fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	return ErrSyntaxFlowNotAvailable
}

// GetRuleFS 在 irify_exclude 模式下，返回 nil
func GetRuleFS() *embed.FS {
	return nil
}
