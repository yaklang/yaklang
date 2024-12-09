package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func createGroups(client ypb.YakClient, groupNames []string) error {
	for _, group := range groupNames {
		req := &ypb.CreateSyntaxFlowGroupRequest{
			GroupName: group,
		}
		_, err := client.CreateSyntaxFlowRuleGroup(context.Background(), req)
		if err != nil {
			return err
		}
	}
	return nil
}

func addGroups(client ypb.YakClient, ruleNames []string, groupNames []string) error {
	req := &ypb.UpdateSyntaxFlowRuleAndGroupRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: ruleNames,
		},
		AddGroups: groupNames,
	}
	_, err := client.UpdateSyntaxFlowRuleAndGroup(context.Background(), req)
	return err
}

func removeGroups(client ypb.YakClient, ruleNames []string, groupNames []string) error {
	req := &ypb.UpdateSyntaxFlowRuleAndGroupRequest{
		Filter: &ypb.SyntaxFlowRuleFilter{
			RuleNames: ruleNames,
		},
		RemoveGroups: groupNames,
	}
	_, err := client.UpdateSyntaxFlowRuleAndGroup(context.Background(), req)
	return err
}

func queryRuleGroupCount(client ypb.YakClient, groupName string) (int, error) {
	req := &ypb.QuerySyntaxFlowRuleGroupRequest{
		Filter: &ypb.SyntaxFlowRuleGroupFilter{
			KeyWord: groupName,
		},
	}
	rsp, err := client.QuerySyntaxFlowRuleGroup(context.Background(), req)
	if err != nil {
		return 0, err
	}
	if len(rsp.GetGroup()) == 0 {
		return 0, nil
	} else if len(rsp.GetGroup()) == 1 {
		return int(rsp.GetGroup()[0].Count), nil
	} else {
		return 0, errors.New("query group count failed")
	}
}

func deleteRuleGroup(client ypb.YakClient, groupNames []string) (int64, error) {
	req := &ypb.DeleteSyntaxFlowRuleGroupRequest{
		Filter: &ypb.SyntaxFlowRuleGroupFilter{
			GroupNames: groupNames,
		},
	}
	rsp, err := client.DeleteSyntaxFlowRuleGroup(context.Background(), req)
	if err != nil {
		return 0, err
	}
	return rsp.EffectRows, nil
}

func TestGRPCMUSTPASS_SyntaxFlow_Rule_Group(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("test create and delete syntax flow rule group", func(t *testing.T) {
		var groupNames []string

		for i := 0; i < 10; i++ {
			groupName := fmt.Sprintf("group_%s", uuid.NewString())
			groupNames = append(groupNames, groupName)
		}
		err = createGroups(client, groupNames)
		require.NoError(t, err)
		for _, groupName := range groupNames {
			afterSaveCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, 0, afterSaveCount)
		}
		_, err := deleteRuleGroup(client, groupNames)
		require.NoError(t, err)
		count := sfdb.GetRuleCountByGroupName(consts.GetGormProfileDatabase(), groupNames...)
		require.Equal(t, int32(0), count)
		for _, groupName := range groupNames {
			afterDeleteCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, afterDeleteCount, 0)
		}
	})

	t.Run("test update: add  rule group relation ship", func(t *testing.T) {
		var groupNames []string
		var ruleNames []string
		for i := 0; i < 10; i++ {
			groupName := fmt.Sprintf("group_%s", uuid.NewString())
			require.NoError(t, err)
			groupNames = append(groupNames, groupName)
		}
		for i := 0; i < 10; i++ {
			ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
			createSfRule(client, ruleName)
			ruleNames = append(ruleNames, ruleName)
		}
		err = createGroups(client, groupNames)
		require.NoError(t, err)
		err = addGroups(client, ruleNames, groupNames)
		require.NoError(t, err)

		for _, groupName := range groupNames {
			afterSaveCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, 10, afterSaveCount)
		}

		count, err := deleteRuleGroup(client, groupNames)
		require.NoError(t, err)
		require.Equal(t, int64(10), count)
		for _, groupName := range groupNames {
			afterDeleteCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
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
		err = createGroups(client, groupNames)
		require.NoError(t, err)
		err = addGroups(client, ruleNames, groupNames)
		require.NoError(t, err)
		for _, groupName := range groupNames {
			afterSaveCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, 10, afterSaveCount)
		}
		err = removeGroups(client, ruleNames, groupNames)
		require.NoError(t, err)
		for _, groupName := range groupNames {
			afterSaveCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, 0, afterSaveCount)
		}
		_, err := deleteRuleGroup(client, groupNames)
		require.NoError(t, err)
		count := yakit.QuerySyntaxFlowGroupCount(consts.GetGormProfileDatabase(), groupNames)
		require.Equal(t, int64(0), count)
		for _, groupName := range groupNames {
			afterDeleteCount, err := queryRuleGroupCount(client, groupName)
			require.NoError(t, err)
			require.Equal(t, afterDeleteCount, 0)
		}
	})

}
