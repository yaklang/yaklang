package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Rule_Group(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	addGroupForRule := func(ruleName string, groupName string) {
		saveData := &schema.SyntaxFlowRuleGroup{RuleName: ruleName, GroupName: groupName}
		err := yakit.AddSyntaxFlowRulesGroup(consts.GetGormProfileDatabase(), saveData)
		require.NoError(t, err)
	}
	queryRuleGroupCount := func(groupName string) int {
		req := &ypb.QuerySyntaxFlowRuleGroupRequest{
			Filter: &ypb.SyntaxFlowRuleGroupFilter{
				KeyWord: groupName,
			},
		}
		rsp, err := client.QuerySyntaxFlowRuleGroup(context.Background(), req)
		require.NoError(t, err)
		if len(rsp.GetGroup()) == 0 {
			return 0
		} else if len(rsp.GetGroup()) == 1 {
			return int(rsp.GetGroup()[0].Count)
		} else {
			require.Fail(t, "query group count failed")
			return 0
		}
	}
	deleteRuleGroup := func(ruleName string, groupName string) {
		data := &schema.SyntaxFlowRuleGroup{
			RuleName:  ruleName,
			GroupName: groupName,
		}
		err := yakit.DeleteSyntaxFlowRuleGroup(consts.GetGormProfileDatabase(), data)
		require.NoError(t, err)
	}

	t.Run("test add and delete syntax flow rule group", func(t *testing.T) {
		groupName := fmt.Sprintf("group_%s", uuid.NewString())
		var ruleNames []string
		for i := 0; i < 10; i++ {
			ruleName := fmt.Sprintf("test_rule_%d_%s.sf", i, uuid.NewString())
			addGroupForRule(ruleName, groupName)
			ruleNames = append(ruleNames, ruleName)
		}
		afterSaveCount := queryRuleGroupCount(groupName)
		require.Equal(t, 10, afterSaveCount)
		for _, ruleName := range ruleNames {
			deleteRuleGroup(ruleName, groupName)
		}
		afterDeleteCount := queryRuleGroupCount(groupName)
		require.Equal(t, afterDeleteCount, 0)
	},
	)
}
