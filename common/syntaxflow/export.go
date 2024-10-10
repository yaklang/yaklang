package syntaxflow

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type execRuleConfig struct {
	taskID string
	debug  bool
}
type ExecRuleOption func(*execRuleConfig)

func WithExecTaskID(taskID string) ExecRuleOption {
	return func(c *execRuleConfig) {
		c.taskID = taskID
	}
}

func WithExecDebug(debug ...bool) ExecRuleOption {
	return func(c *execRuleConfig) {
		c.debug = true
		if len(debug) > 0 {
			c.debug = debug[0]
		}
	}
}

func ExecRule(r *schema.SyntaxFlowRule, prog *ssaapi.Program, opts ...ExecRuleOption) (*ssaapi.SyntaxFlowResult, error) {
	config := &execRuleConfig{}
	for _, opt := range opts {
		opt(config)
	}
	res, err := prog.SyntaxFlowRule(r, sfvm.WithEnableDebug(config.debug))
	if err != nil {
		return nil, err
	}

	if config.taskID != "" {
		if resID, err := res.Save(config.taskID); err != nil {
			_ = resID
			return res, err
		}
	}

	return res, nil
}

type QueryRulesOption func(*gorm.DB) *gorm.DB

func QuerySyntaxFlowRules(name string, opts ...QueryRulesOption) chan *schema.SyntaxFlowRule {
	db := consts.GetGormProfileDatabase()
	db = bizhelper.FuzzQueryLike(db, "rule_name", name)
	for _, opt := range opts {
		db = opt(db)
	}
	return sfdb.YieldSyntaxFlowRules(db, context.Background())
}

var Exports = map[string]any{
	"ExecRule":       ExecRule,
	"withExecTaskID": WithExecTaskID,
	"withExecDebug":  WithExecDebug,

	"QuerySyntaxFlowRules": QuerySyntaxFlowRules,
}
