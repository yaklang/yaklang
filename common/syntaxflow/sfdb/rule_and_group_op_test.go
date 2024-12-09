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
}
