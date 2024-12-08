package sfdb

import (
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestRule_OP(t *testing.T) {
	t.Run("create and delete rule", func(t *testing.T) {
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		err := CreateRule(rule)
		require.NoError(t, err)
		got, err := QueryRuleByName(ruleName)
		require.Equal(t, ruleName, got.RuleName)

		err = DeleteRuleByRuleName(ruleName)
		require.NoError(t, err)
		_, err = QueryRuleByName(ruleName)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})
	t.Run("create and update rule", func(t *testing.T) {
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		err := CreateRule(rule)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteRuleByRuleName(ruleName)
			require.NoError(t, err)
		})

		rule.Language = "java"
		err = UpdateRule(rule)
		require.NoError(t, err)

		got, err := QueryRuleByName(ruleName)
		require.NoError(t, err)
		require.Equal(t, "java", got.Language)
	})
}

func TestRule_Group_OP(t *testing.T) {
	t.Run("test create and delete group", func(t *testing.T) {
		groupName := uuid.NewString()
		err := CreateGroupByName(groupName)
		require.NoError(t, err)

		got, err := QueryGroupByName(groupName)
		require.NoError(t, err)
		require.Equal(t, groupName, got.GroupName)

		err = DeleteGroupByName(groupName)
		require.NoError(t, err)
		_, err = QueryGroupByName(groupName)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("test add remove rule group", func(t *testing.T) {
		// create group
		groupName := uuid.NewString()
		err := CreateGroupByName(groupName)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteGroupByName(groupName)
			require.NoError(t, err)
		})
		// create rule
		ruleName := uuid.NewString()
		rule := &schema.SyntaxFlowRule{RuleName: ruleName}
		err = CreateRule(rule)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteRuleByRuleName(ruleName)
			require.NoError(t, err)
		})
		// add rule to group
		err = AddGroupForRuleByName(ruleName, groupName)
		require.NoError(t, err)
		require.Equal(t, int32(1), QueryRuleCountInGroup(groupName))

		newGroupName := uuid.NewString()
		err = AddGroupForRuleByName(ruleName, newGroupName)
		t.Cleanup(func() {
			err = DeleteGroupByName(newGroupName)
			require.NoError(t, err)
		})
		require.NoError(t, err)
		require.Equal(t, int32(1), QueryRuleCountInGroup(newGroupName))

		gotRule, err := QueryRuleByName(ruleName)
		require.NoError(t, err)
		require.Equal(t, 2, len(gotRule.Groups))

		// remove rule from group
		err = RemoveGroupForRuleByName(ruleName, groupName)
		require.NoError(t, err)
		require.Equal(t, int32(1), QueryGroupCountInRule(ruleName))
	})

}
