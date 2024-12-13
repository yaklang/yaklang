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

	t.Run("test intersection group of query rules", func(t *testing.T) {
		ruleName1 := fmt.Sprintf("rule_%s", uuid.NewString())
		_, err = createSfRuleEx(client, ruleName1)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName1})
			require.NoError(t, err)
		})

		ruleName2 := fmt.Sprintf("rule_%s", uuid.NewString())
		_, err = createSfRuleEx(client, ruleName2)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName2})
			require.NoError(t, err)
		})

		groupNameA := fmt.Sprintf("group_%s", uuid.NewString())
		groupNameB := fmt.Sprintf("group_%s", uuid.NewString())
		groupNameC := fmt.Sprintf("group_%s", uuid.NewString())

		t.Cleanup(func() {
			_, err = deleteRuleGroup(client, []string{groupNameA, groupNameB, groupNameC})
			require.NoError(t, err)
		})

		err = createGroups(client, []string{groupNameA, groupNameB, groupNameC})
		require.NoError(t, err)

		err = addGroups(client, []string{ruleName1}, []string{groupNameA, groupNameB})
		require.NoError(t, err)

		err = addGroups(client, []string{ruleName2}, []string{groupNameB, groupNameC})
		require.NoError(t, err)

		rule, err := queryRulesByName(client, []string{ruleName1, ruleName2})
		require.NoError(t, err)
		require.Equal(t, groupNameB, rule[0].GetGroupName()[0])
		require.Equal(t, groupNameB, rule[1].GetGroupName()[0])
	})

	t.Run("test rename group", func(t *testing.T) {
		ruleName := uuid.NewString()
		oldGroupName := uuid.NewString()
		createReq := &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName:   ruleName,
				GroupNames: []string{oldGroupName},
				Language:   "java",
			},
		}
		_, err := client.CreateSyntaxFlowRuleEx(context.Background(), createReq)
		require.NoError(t, err)

		newGroupName := uuid.NewString()
		updateReq := &ypb.UpdateSyntaxFlowRuleGroupRequest{
			OldGroupName: oldGroupName,
			NewGroupName: newGroupName,
		}
		_, err = client.UpdateSyntaxFlowRuleGroup(context.Background(), updateReq)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = deleteRuleByNames(client, []string{ruleName})
			require.NoError(t, err)
		})

		newRule, err := queryRulesByName(client, []string{ruleName})
		require.NoError(t, err)
		require.Equal(t, newGroupName, newRule[0].GetGroupName()[0])
	})

	t.Run("query buildin group", func(t *testing.T) {
		// create build in group
		groupName1 := uuid.NewString()
		db := consts.GetGormProfileDatabase()
		sfdb.CreateGroup(db, groupName1, true)
		// create not build in group
		groupName2 := uuid.NewString()
		sfdb.CreateGroup(db, groupName2, false)
		t.Cleanup(func() {
			err = sfdb.DeleteGroup(db, groupName1)
			require.NoError(t, err)
			err = sfdb.DeleteGroup(db, groupName2)
			require.NoError(t, err)
		})
		// query build in group
		group, err := client.QuerySyntaxFlowRuleGroup(context.Background(), &ypb.QuerySyntaxFlowRuleGroupRequest{
			Filter: &ypb.SyntaxFlowRuleGroupFilter{
				GroupNames: []string{groupName2, groupName1},
				IsBuildIn:  "true",
			},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(group.Group))
		require.True(t, group.Group[0].IsBuildIn)
	})
}
