//go:build irify_exclude

package sfbuildin

import (
	"embed"
	"errors"
	"io/fs"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfrisk"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed buildin/bash/***
var embeddedBashRuleFS embed.FS

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

// GenerateRuleVersionsFromLocalFS 在 irify_exclude 模式下不可用
func GenerateRuleVersionsFromLocalFS(dirs []string, baselinePath string) ([]sfdb.RuleInfo, error) {
	return nil, ErrSyntaxFlowNotAvailable
}

// CheckBuiltinRiskTypes 在 irify_exclude 模式下不可用
func CheckBuiltinRiskTypes(dir string, checker *sfrisk.Checker) (*BuiltinRiskCheckResult, error) {
	return nil, ErrSyntaxFlowNotAvailable
}

// GetRuleFS 在 irify_exclude 模式下，返回 nil
func GetRuleFS() *embed.FS {
	return nil
}

// LoadEmbeddedRulesByLanguage loads the canonical bash rule snapshot in the
// slim build. Full SyntaxFlow rule sets are excluded, but the bash guard needs
// these rules to keep high-severity preflight working.
func LoadEmbeddedRulesByLanguage(language string, severities ...schema.SyntaxFlowSeverity) ([]*schema.SyntaxFlowRule, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if language != "bash" {
		return nil, nil
	}
	wanted := normalizeEmbeddedRuleSeverities(severities)
	var rules []*schema.SyntaxFlowRule
	err := fs.WalkDir(embeddedBashRuleFS, "buildin/bash", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".sf") {
			return nil
		}
		raw, err := embeddedBashRuleFS.ReadFile(path)
		if err != nil {
			return err
		}
		rule, err := compileEmbeddedRuleForLanguage(path, string(raw), language, wanted)
		if err != nil {
			return err
		}
		if rule != nil {
			rules = append(rules, rule)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].RuleName < rules[j].RuleName
	})
	return rules, nil
}
