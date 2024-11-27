package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Rule_Group(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	createGroups := func(groupNames []string) {
		group := &ypb.UpdateSyntaxFlowRuleAndGroupRequest{
			AddGroups: groupNames,
		}
		_, err := client.UpdateSyntaxFlowRuleAndGroup(context.Background(), group)
		require.NoError(t, err)
	}
	_ = createGroups

	addGroups := func(ruleNames []string, groupNames []string) {
		req := &ypb.UpdateSyntaxFlowRuleAndGroupRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: ruleNames,
			},
			AddGroups: groupNames,
		}
		_, err := client.UpdateSyntaxFlowRuleAndGroup(context.Background(), req)
		require.NoError(t, err)
	}

	removeGroups := func(ruleNames []string, groupNames []string) {
		req := &ypb.UpdateSyntaxFlowRuleAndGroupRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: ruleNames,
			},
			RemoveGroups: groupNames,
		}
		_, err := client.UpdateSyntaxFlowRuleAndGroup(context.Background(), req)
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

	deleteRuleGroup := func(groupNames []string) int64 {
		req := &ypb.DeleteSyntaxFlowRuleGroupRequest{
			Filter: &ypb.SyntaxFlowRuleGroupFilter{
				GroupNames: groupNames,
			},
		}
		m, err := client.DeleteSyntaxFlowRuleGroup(context.Background(), req)
		require.NoError(t, err)
		return m.EffectRows
	}

	t.Run("test create and delete syntax flow rule group", func(t *testing.T) {
		var groupNames []string

		for i := 0; i < 10; i++ {
			groupName := fmt.Sprintf("group_%s", uuid.NewString())
			groupNames = append(groupNames, groupName)
		}
		createGroups(groupNames)
		for _, groupName := range groupNames {
			afterSaveCount := queryRuleGroupCount(groupName)
			require.Equal(t, 1, afterSaveCount)
		}
		count := deleteRuleGroup(groupNames)
		require.Equal(t, count, int64(10))
		for _, groupName := range groupNames {
			afterDeleteCount := queryRuleGroupCount(groupName)
			require.Equal(t, afterDeleteCount, 0)
		}
	})

	t.Run("test update: add  rule group relation ship", func(t *testing.T) {
		var groupNames []string
		var ruleNames []string
		for i := 0; i < 10; i++ {
			groupName := fmt.Sprintf("group_%s", uuid.NewString())
			err = createSfRule(client, groupName)
			require.NoError(t, err)
			groupNames = append(groupNames, groupName)
		}
		for i := 0; i < 10; i++ {
			ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
			ruleNames = append(groupNames, ruleName)
		}
		createGroups(groupNames)
		addGroups(ruleNames, groupNames)

		for _, groupName := range groupNames {
			afterSaveCount := queryRuleGroupCount(groupName)
			require.Equal(t, 11, afterSaveCount) // 10条rule-group relation,和1条空rule的group
		}

		count := deleteRuleGroup(groupNames)
		require.Equal(t, count, int64(110))
		for _, groupName := range groupNames {
			afterDeleteCount := queryRuleGroupCount(groupName)
			require.Equal(t, afterDeleteCount, 0)
		}
	})

	t.Run("test update: add  and remove rule group relation ship", func(t *testing.T) {
		var groupNames []string
		var ruleNames []string
		for i := 0; i < 10; i++ {
			groupName := fmt.Sprintf("group_%s", uuid.NewString())
			err = createSfRule(client, groupName)
			require.NoError(t, err)
			groupNames = append(groupNames, groupName)
		}
		for i := 0; i < 10; i++ {
			ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
			ruleNames = append(groupNames, ruleName)
		}
		createGroups(groupNames)
		addGroups(ruleNames, groupNames)
		for _, groupName := range groupNames {
			afterSaveCount := queryRuleGroupCount(groupName)
			require.Equal(t, 11, afterSaveCount) // 10条rule-group relation,和1条空rule的group
		}
		removeGroups(ruleNames, groupNames)
		for _, groupName := range groupNames {
			afterSaveCount := queryRuleGroupCount(groupName)
			require.Equal(t, 1, afterSaveCount) // 1条空rule的group
		}
		count := deleteRuleGroup(groupNames)
		require.Equal(t, count, int64(10))
		for _, groupName := range groupNames {
			afterDeleteCount := queryRuleGroupCount(groupName)
			require.Equal(t, afterDeleteCount, 0)
		}
	})

}
