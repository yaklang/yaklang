package sfdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestRule_OP(t *testing.T) {
	db := consts.GetGormProfileDatabase()

	t.Run("create and delete rule", func(t *testing.T) {
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		_, err := CreateRule(rule)
		require.NoError(t, err)
		got, err := QueryRuleByName(db, ruleName)
		require.NoError(t, err)
		require.Equal(t, ruleName, got.RuleName)

		err = DeleteRuleByRuleName(ruleName)
		require.NoError(t, err)
		_, err = QueryRuleByName(db, ruleName)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})
	t.Run("create and update rule", func(t *testing.T) {
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		_, err := CreateRule(rule)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteRuleByRuleName(ruleName)
			require.NoError(t, err)
		})

		rule.Language = "java"
		err = UpdateRule(rule)
		require.NoError(t, err)

		got, err := QueryRuleByName(db, ruleName)
		require.NoError(t, err)
		require.Equal(t, "java", got.Language)
	})
}

func TestRule_Group_OP(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	t.Run("test create and delete group", func(t *testing.T) {
		groupName := uuid.NewString()
		_, err := CreateGroup(db, groupName)
		require.NoError(t, err)

		got, err := QueryGroupByName(db, groupName)
		require.NoError(t, err)
		require.Equal(t, groupName, got.GroupName)

		err = DeleteGroup(db, groupName)
		require.NoError(t, err)
		_, err = QueryGroupByName(db, groupName)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("test add remove rule group", func(t *testing.T) {
		// create group
		groupName := uuid.NewString()
		_, err := CreateGroup(db, groupName)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteGroup(db, groupName)
			require.NoError(t, err)
		})
		// create rule
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		_, err = CreateRule(rule)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteRuleByRuleName(ruleName)
			require.NoError(t, err)
		})
		// add rule to group
		err = AddGroupForRule(db, ruleName, groupName)
		require.NoError(t, err)
		require.Equal(t, int32(1), GetRuleCountByGroupName(db, groupName))

		newGroupName := uuid.NewString()
		err = AddGroupForRule(db, ruleName, newGroupName)
		t.Cleanup(func() {
			err = DeleteGroup(db, newGroupName)
			require.NoError(t, err)
		})
		require.NoError(t, err)
		require.Equal(t, int32(1), GetRuleCountByGroupName(db, newGroupName))

		gotRule, err := QueryRuleByName(db, ruleName)
		require.NoError(t, err)
		require.Equal(t, 2, len(gotRule.Groups))

		// remove rule from group
		err = RemoveGroupForRule(db, ruleName, groupName)
		require.NoError(t, err)
		require.Equal(t, int32(1), GetGroupCountByRuleName(db, ruleName))
	})

	t.Run("test create and delete group for rule", func(t *testing.T) {
		// create group
		groupName := uuid.NewString()
		_, err := CreateGroup(db, groupName)
		require.NoError(t, err)

		// create rule
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		_, err = CreateRule(rule)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteRuleByRuleName(ruleName)
			require.NoError(t, err)
		})
		// add rule to group
		err = AddGroupForRule(db, ruleName, groupName)
		require.NoError(t, err)
		require.Equal(t, int32(1), GetRuleCountByGroupName(db, groupName))

		// delete group
		err = DeleteGroup(db, groupName)
		require.NoError(t, err)

		queryRule, err := QueryRuleByName(db, ruleName)
		require.NoError(t, err)
		require.Equal(t, 0, len(queryRule.Groups))
	})
	t.Run("test GetIntersectionGroups", func(t *testing.T) {
		creatGroup := func(groupName string) *schema.SyntaxFlowGroup {
			group := &schema.SyntaxFlowGroup{GroupName: groupName}
			return group
		}

		groupA := creatGroup(uuid.NewString())
		groupB := creatGroup(uuid.NewString())
		groupC := creatGroup(uuid.NewString())

		groups := []*schema.SyntaxFlowGroup{groupA, groupB, groupC}
		ret := GetIntersectionGroups(groups)
		require.Nil(t, ret)

		groups = []*schema.SyntaxFlowGroup{groupA, groupB, groupA}
		ret = GetIntersectionGroups(groups)
		require.Equal(t, 1, len(ret))
		require.Equal(t, groupA, ret[0])

		groups = []*schema.SyntaxFlowGroup{groupA, groupB, groupC, groupA, groupB}
		ret = GetIntersectionGroups(groups)
		require.Equal(t, 2, len(ret))
		require.Contains(t, ret, groupA)
		require.Contains(t, ret, groupB)
	})

	t.Run("test GetOrCreatGroups", func(t *testing.T) {
		groupName1 := uuid.NewString()
		_, err := CreateGroup(db, groupName1)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteGroup(db, groupName1)
			require.NoError(t, err)
		})

		groupName2 := uuid.NewString()
		ret := GetOrCreatGroups(db, []string{groupName1, groupName2})
		require.Equal(t, 2, len(ret))
		groupNames := []string{ret[0].GroupName, ret[1].GroupName}
		require.Contains(t, groupNames, groupName1)
		require.Contains(t, groupNames, groupName2)
	})

	t.Run("test rename group", func(t *testing.T) {
		groupName1 := uuid.NewString()
		_, err := CreateGroup(db, groupName1)
		require.NoError(t, err)

		group1, err := QueryGroupByName(db, groupName1)
		require.NoError(t, err)

		groupName2 := uuid.NewString()
		err = RenameGroup(db, groupName1, groupName2)
		require.NoError(t, err)

		t.Cleanup(func() {
			err = DeleteGroup(db, groupName2)
			require.NoError(t, err)
		})
		group2, err := QueryGroupByName(db, groupName2)
		require.NoError(t, err)
		require.Equal(t, group1.ID, group2.ID)
	})
}
