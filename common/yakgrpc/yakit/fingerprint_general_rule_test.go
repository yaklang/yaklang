package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCURD_GeneralRule_base(t *testing.T) {
	ruleName1 := utils.RandStringBytes(10)
	ruleExpr1 := utils.RandStringBytes(10)
	ruleName2 := utils.RandStringBytes(10)
	ruleExpr2 := utils.RandStringBytes(10)

	db := consts.GetGormProfileDatabase()
	// 清理测试数据，确保测试环境干净
	ClearGeneralRule(db)

	generalRule := &schema.GeneralRule{
		MatchExpression: ruleExpr1,
		RuleName:        ruleName1,
	}
	err := CreateGeneralRule(db, generalRule)
	require.NoError(t, err)

	rule, err := GetGeneralRuleByID(db, int64(generalRule.ID))
	require.NoError(t, err)
	require.Equal(t, ruleExpr1, rule.MatchExpression)

	generalRule.RuleName = ruleName2
	_, err = UpdateGeneralRule(db, generalRule)
	require.NoError(t, err)

	rule, err = GetGeneralRuleByID(db, int64(generalRule.ID))
	require.NoError(t, err)
	require.Equal(t, ruleName2, rule.RuleName)

	generalRule.MatchExpression = ruleExpr2
	_, err = UpdateGeneralRuleByRuleName(db, generalRule.RuleName, generalRule)
	require.NoError(t, err)

	rule, err = GetGeneralRuleByID(db, int64(generalRule.ID))
	require.NoError(t, err)
	require.Equal(t, ruleExpr2, rule.MatchExpression)

	count, err := DeleteGeneralRuleByFilter(db, &ypb.FingerprintFilter{IncludeId: []int64{int64(generalRule.ID)}})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

//func TestSssssaaa(t *testing.T) {
//	db := consts.GetGormProfileDatabase()
//	ClearGeneralRule(db)
//	err := InsertBuiltinGeneralRules(db)
//	require.NoError(t, err)
//}
