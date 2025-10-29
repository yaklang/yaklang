//go:build no_syntaxflow
// +build no_syntaxflow

package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Stub functions when SyntaxFlow support is excluded

func FilterSyntaxFlowRule(db *gorm.DB, filter *ypb.SyntaxFlowRuleFilter, opt ...interface{}) *gorm.DB {
	return db
}

func ParseSyntaxFlowInput(ruleInput *ypb.SyntaxFlowRuleInput) (*schema.SyntaxFlowRule, error) {
	return &schema.SyntaxFlowRule{}, nil
}

func QuerySyntaxFlowRule(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleRequest) (*interface{}, []*schema.SyntaxFlowRule, error) {
	return nil, nil, nil
}

func UpdateSyntaxFlowRule(db *gorm.DB, params *ypb.UpdateSyntaxFlowRuleRequest) error {
	return nil
}

func DeleteSyntaxFlowRule(db *gorm.DB, params *ypb.DeleteSyntaxFlowRuleRequest) (int64, error) {
	return 0, nil
}

func AllSyntaxFlowRule(db *gorm.DB) ([]*schema.SyntaxFlowRule, error) {
	return nil, nil
}

func QuerySyntaxFlowRuleGroup(db *gorm.DB, params *ypb.QuerySyntaxFlowRuleGroupRequest) (*interface{}, []*interface{}, error) {
	return nil, nil, nil
}

const (
	FilterLibRuleTrue  string = "lib"
	FilterLibRuleFalse string = "noLib"

	FilterBuiltinRuleTrue  string = "buildIn"
	FilterBuiltinRuleFalse string = "unBuildIn"
)
