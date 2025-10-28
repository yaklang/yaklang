package sfdb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// DBRuleLoader 数据库规则加载器
// 适配现有的数据库查询实现，保持向后兼容
type DBRuleLoader struct {
	db *gorm.DB
}

// NewDBRuleLoader 创建数据库规则加载器
// 如果db为nil，将使用默认的profile数据库
func NewDBRuleLoader(db *gorm.DB) *DBRuleLoader {
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}
	return &DBRuleLoader{db: db}
}

// LoadRules 根据筛选条件加载规则列表
// 复用现有的yakit.AllSyntaxFlowRule实现
func (l *DBRuleLoader) LoadRules(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 使用sfdb本地的实现，避免循环依赖
	rules := make([]*schema.SyntaxFlowRule, 0)
	db := applyRuleFilter(l.db, filter)
	if err := db.Find(&rules).Error; err != nil {
		return nil, utils.Wrapf(err, "load rules from database failed")
	}

	return rules, nil
}

// LoadRuleByName 根据规则名称加载单个规则
// 复用现有的GetRule实现
func (l *DBRuleLoader) LoadRuleByName(ctx context.Context, ruleName string) (*schema.SyntaxFlowRule, error) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 复用现有实现
	rule, err := GetRule(ruleName)
	if err != nil {
		return nil, utils.Wrapf(err, "load rule %s from database failed", ruleName)
	}

	return rule, nil
}

// YieldRules 流式加载规则（通过channel）
// 复用现有的YieldSyntaxFlowRules实现
func (l *DBRuleLoader) YieldRules(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) <-chan *RuleItem {
	ch := make(chan *RuleItem, 10)

	go func() {
		defer close(ch)

		// 应用筛选条件
		db := applyRuleFilter(l.db, filter)

		// 使用现有的yield实现
		for rule := range YieldSyntaxFlowRules(db, ctx) {
			select {
			case ch <- &RuleItem{Rule: rule}:
			case <-ctx.Done():
				log.Infof("context cancelled, stop yielding rules")
				return
			}
		}
	}()

	return ch
}

// GetLoaderType 返回加载器类型
func (l *DBRuleLoader) GetLoaderType() RuleLoaderType {
	return LoaderTypeDatabase
}

// Close 关闭加载器
// 数据库由外部管理，这里不关闭
func (l *DBRuleLoader) Close() error {
	return nil
}

// String 返回加载器的字符串表示
func (l *DBRuleLoader) String() string {
	return "DBRuleLoader"
}
