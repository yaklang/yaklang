package sfdb

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func AddGroupForRule(db *gorm.DB, ruleName, groupName string) error {
	_, err := BatchAddGroupsForRules(db, []string{ruleName}, []string{groupName})
	return err
}

func RemoveGroupForRule(db *gorm.DB, ruleName, groupName string) error {
	_, err := BatchRemoveGroupsForRules(db, []string{ruleName}, []string{groupName})
	return err
}

func GetRuleCountByGroupName(db *gorm.DB, groupName ...string) int32 {
	db = db.Model(&schema.SyntaxFlowGroup{})
	if len(groupName) == 1 {
		var group schema.SyntaxFlowGroup
		db.Preload("Rules").Where("group_name = ?", groupName).First(&group)
		return int32(len(group.Rules))
	} else {
		var groups []schema.SyntaxFlowGroup
		db.Preload("Rules").Where("group_name IN (?)", groupName).Find(&groups)
		var count int32
		for _, group := range groups {
			count += int32(len(group.Rules))
		}
		return count
	}
}

func GetGroupCountByRuleName(db *gorm.DB, ruleName string) int32 {
	db = db.Model(&schema.SyntaxFlowRule{})
	var rule schema.SyntaxFlowRule
	db.Preload("Groups").Where("rule_name = ?", ruleName).First(&rule)
	return int32(len(rule.Groups))
}

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
		err = UpdateRule(db, rule)
		require.NoError(t, err)

		got, err := QueryRuleByName(db, ruleName)
		require.NoError(t, err)
		require.Equal(t, "java", string(got.Language))
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
	t.Run("test GetIntersectionGroup", func(t *testing.T) {
		groupA, err := CreateGroup(db, uuid.NewString())
		require.NoError(t, err)
		groupB, err := CreateGroup(db, uuid.NewString())
		require.NoError(t, err)
		groupC, err := CreateGroup(db, uuid.NewString())
		require.NoError(t, err)

		t.Cleanup(func() {
			err := DeleteGroup(db, groupA.GroupName)
			require.NoError(t, err)
			err = DeleteGroup(db, groupB.GroupName)
			require.NoError(t, err)
			err = DeleteGroup(db, groupC.GroupName)
			require.NoError(t, err)
		})

		groups := [][]*schema.SyntaxFlowGroup{{groupA}, {groupB}, {groupC}}
		ret := GetIntersectionGroup(db, groups)
		require.Equal(t, 0, len(ret))

		groups = [][]*schema.SyntaxFlowGroup{{groupA, groupB}, {groupB, groupC}}
		ret = GetIntersectionGroup(db, groups)
		require.Equal(t, 1, len(ret))
		require.Equal(t, groupB.GroupName, ret[0].GroupName)

		groups = [][]*schema.SyntaxFlowGroup{{groupA, groupB, groupC}, {groupA, groupB, groupC}}
		ret = GetIntersectionGroup(db, groups)
		require.Equal(t, 3, len(ret))

		groups = [][]*schema.SyntaxFlowGroup{{groupA, groupB, groupC}, {groupB, groupC}, {groupC}}
		ret = GetIntersectionGroup(db, groups)
		require.Equal(t, 1, len(ret))
	})

	t.Run("test GetOrCreateGroups", func(t *testing.T) {
		groupName1 := uuid.NewString()
		_, err := CreateGroup(db, groupName1)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = DeleteGroup(db, groupName1)
			require.NoError(t, err)
		})

		groupName2 := uuid.NewString()
		ret := GetOrCreateGroups(db, []string{groupName1, groupName2})
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

	t.Run("test create rule with default group", func(t *testing.T) {
		rule := &schema.SyntaxFlowRule{
			RuleName: uuid.NewString(),
			Language: "java",
			Severity: schema.SFR_SEVERITY_INFO,
			Purpose:  schema.SFR_PURPOSE_AUDIT,
		}
		defer func() {
			DeleteRuleByRuleName(rule.RuleName)
		}()
		newRule, err := CreateRuleWithDefaultGroup(rule)
		require.NoError(t, err)
		marshal, err := json.Marshal(newRule.Groups)
		require.NoError(t, err)
		log.Infof("new group: %v", string(marshal))
		require.Equal(t, 3, len(newRule.Groups))
	})
}

func TestRule_Group_CreateOrUpdate(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	t.Run("test create or update rule with group", func(t *testing.T) {
		rule := &schema.SyntaxFlowRule{
			RuleName: uuid.NewString(),
			Language: "java",
			Severity: schema.SFR_SEVERITY_INFO,
			Purpose:  schema.SFR_PURPOSE_AUDIT,
		}
		groupName1 := uuid.NewString()
		defer func() {
			DeleteRuleByRuleName(rule.RuleName)
			DeleteGroup(db, groupName1)
		}()
		newRule, err := CreateOrUpdateRuleWithGroup(rule, groupName1, groupName1)
		require.NoError(t, err)
		marshal, err := json.Marshal(newRule.Groups)
		require.NoError(t, err)
		log.Infof("new group: %v", string(marshal))
		require.Equal(t, 1, len(newRule.Groups))
	})

}
