//go:build no_syntaxflow
// +build no_syntaxflow

package syntaxflow

// Stub implementation when SyntaxFlow support is excluded
// 语言支持被排除时的桩实现 - SyntaxFlow 支持被排除

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// ExecRule 桩实现 - no_syntaxflow 版本不支持 SyntaxFlow
func ExecRule(r *schema.SyntaxFlowRule, prog *ssaapi.Program, opts ...ssaapi.QueryOption) (*ssaapi.SyntaxFlowResult, error) {
	log.Warn("SyntaxFlow support is excluded in no_syntaxflow build. Please use the full version.")
	return nil, nil
}

// QueryRulesOption 定义查询选项函数类型
type QueryRulesOption func(*gorm.DB) *gorm.DB

// QuerySyntaxFlowRules 桩实现
func QuerySyntaxFlowRules(name string, opts ...QueryRulesOption) chan *schema.SyntaxFlowRule {
	log.Warn("SyntaxFlow support is excluded in no_syntaxflow build. Please use the full version.")
	c := make(chan *schema.SyntaxFlowRule)
	close(c)
	return c
}

// Exports 桩实现 - 返回空的导出映射
var Exports = map[string]any{}

// IsSyntaxFlowSupported 返回 SyntaxFlow 是否被支持
func IsSyntaxFlowSupported() bool {
	return false
}
