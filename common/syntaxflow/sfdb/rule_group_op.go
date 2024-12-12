package sfdb

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var buildInGroupsMap map[string]struct{}

func init() {
	buildInGroupsMap = make(map[string]struct{})
	var buildInGroups []string
	buildInGroups = append(buildInGroups, schema.GetAllSFSupportLanguage()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFPurposeTypes()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFSeverityTypes()...)
	lo.ForEach(buildInGroups, func(item string, _ int) {
		if item != "" {
			buildInGroupsMap[item] = struct{}{}
		}
	})
}

// GetGroupCountByRuleName 统计规则中的组数量
func GetGroupCountByRuleName(db *gorm.DB, ruleName string) int32 {
	db = db.Model(&schema.SyntaxFlowRule{})
	var rule schema.SyntaxFlowRule
	db.Preload("Groups").Where("rule_name = ?", ruleName).First(&rule)
	return int32(len(rule.Groups))
}

// GetRuleCountByGroupName 统计某个组中的规则数量
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

// CreateGroup 通过组名创建SyntaxFlow规则组
func CreateGroup(db *gorm.DB, groupName string, isBuildIn ...bool) (*schema.SyntaxFlowGroup, error) {
	buildIn := false
	if len(isBuildIn) > 0 {
		buildIn = isBuildIn[0]
	}

	db = db.Model(&schema.SyntaxFlowGroup{})
	i := &schema.SyntaxFlowGroup{
		GroupName: groupName,
		IsBuildIn: buildIn,
	}
	if db = db.Create(&i); db.Error != nil {
		return nil, db.Error
	}
	return i, nil
}

func GetOrCreatGroups(db *gorm.DB, groupNames []string) []*schema.SyntaxFlowGroup {
	var groups []*schema.SyntaxFlowGroup
	for _, groupName := range groupNames {
		group, err := QueryGroupByName(db, groupName)
		if err == nil {
			groups = append(groups, group)
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorf("get group %s failed: %s", groupName, err)
			continue
		}
		// if not found, create it
		group, err = CreateGroup(db, groupName)
		if err != nil {
			log.Errorf("create group %s failed: %s", groupName, err)
			continue
		}
		groups = append(groups, group)
	}
	return groups
}

// QueryAllGroups 查询所有的SyntaxFlow规则组
func QueryAllGroups(db *gorm.DB) ([]schema.SyntaxFlowGroup, error) {
	var groups []schema.SyntaxFlowGroup
	db = db.Model(&schema.SyntaxFlowGroup{})
	if db = db.Find(&groups); db.Error != nil {
		return nil, db.Error
	}
	return groups, nil
}

// QueryGroupByName 根据组名查询组
func QueryGroupByName(db *gorm.DB, groupName string) (*schema.SyntaxFlowGroup, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	i := &schema.SyntaxFlowGroup{}
	if db = db.Preload("Rules").Where("group_name = ?", groupName).First(i); db.Error != nil {
		return nil, db.Error
	}
	return i, nil
}

func QueryGroupsByName(db *gorm.DB, groupNames []string) ([]*schema.SyntaxFlowGroup, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var groups []*schema.SyntaxFlowGroup
	if db = db.Preload("Rules").Where("group_name IN (?)", groupNames).Find(&groups); db.Error != nil {
		return nil, db.Error
	}
	return groups, nil
}

func GetIntersectionGroup(groups []*schema.SyntaxFlowGroup) []*schema.SyntaxFlowGroup {
	var result []*schema.SyntaxFlowGroup
	groupMap := make(map[string]struct{})
	for _, group := range groups {
		if _, ok := groupMap[group.GroupName]; !ok {
			groupMap[group.GroupName] = struct{}{}
			continue
		}
		result = append(result, group)
	}
	return result
}

// AddDefaultGroupForRule 为规则添加默认分组
// 默认分组为：语言、严重程度、规则类型
func AddDefaultGroupForRule(db *gorm.DB, rule *schema.SyntaxFlowRule) error {
	if rule == nil {
		return utils.Errorf("add default group for rule failed:rule is empty")
	}
	var groups []string
	groups = append(groups, rule.Language)
	groups = append(groups, string(rule.Severity))
	groups = append(groups, string(rule.Purpose))
	groups = lo.Filter(groups, func(item string, _ int) bool {
		return isDefaultGroup(item) && item != ""
	})
	_, err := BatchAddGroupsForRules(db, []string{rule.RuleName}, groups)
	return err
}

// BatchAddGroupsForRules 为多个规则添加多个组
// 如果要添加的组不存在，会自动创建
func BatchAddGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	ruleNames = utils.RemoveRepeatedWithStringSlice(ruleNames)
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)

	var count int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		groups := GetOrCreatGroups(tx, groupNames)
		rules, err := QueryRulesByName(tx, ruleNames)
		if err != nil {
			return err
		}

		if len(ruleNames) != len(rules) {
			return utils.Errorf("batch add groups for rules failed: rules not found")
		}
		if len(groupNames) != len(groups) {
			return utils.Errorf("batch add groups for rules failed: groups not found")
		}
		if len(groups) == 0 || len(rules) == 0 {
			return utils.Errorf("batch add groups for rules failed: groups or rules is empty")
		}
		for _, rule := range rules {
			if err = tx.Model(rule).Association("Groups").Append(groups).Error; err != nil {
				return err
			} else {
				count += int64(len(groups))
			}
		}
		return nil
	})
	return count, err
}

// BatchRemoveGroupsForRules 为多个规则移除多个组
func BatchRemoveGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	var count int64
	ruleNames = utils.RemoveRepeatedWithStringSlice(ruleNames)
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)

	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		groups, err := QueryGroupsByName(tx, groupNames)
		if err != nil {
			return utils.Errorf("batch remove groups for rules failed: %s", err)
		}
		rules, err := QueryRulesByName(tx, ruleNames)
		if err != nil {
			return utils.Errorf("batch remove groups for rules failed: %s", err)
		}

		if len(rules) == 0 || len(groups) == 0 {
			return utils.Errorf("batch remove groups for rules failed: rules or groups is empty")
		}
		if len(ruleNames) != len(rules) {
			return utils.Errorf("batch remove groups for rules failed: rules not found")
		}
		if len(groupNames) != len(groups) {
			return utils.Errorf("batch remove groups for rules failed: groups not found")
		}
		for _, rule := range rules {
			if err = tx.Model(rule).Association("Groups").Delete(groups).Error; err != nil {
				return err
			} else {
				count += int64(len(groups))
			}
		}
		return nil
	})

	return count, err
}

// DeleteGroups 通过多个组名删除多个SyntaxFlow规则组
func DeleteGroups(db *gorm.DB, groupNames []string) (int64, error) {
	var count int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, groupName := range groupNames {
			if err := DeleteGroup(tx, groupName); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

// DeleteGroup 通过组名删除SyntaxFlow规则组
func DeleteGroup(db *gorm.DB, groupName string) error {
	db = db.Model(&schema.SyntaxFlowGroup{})
	db = db.Where("group_name = ?", groupName).Unscoped().Delete(&schema.SyntaxFlowGroup{})
	return db.Error
}

// RenameGroup 重命名组
func RenameGroup(db *gorm.DB, oldName, newName string) error {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var existingGroup schema.SyntaxFlowGroup
	err := db.Where("group_name = ? ", newName).First(&existingGroup).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.Errorf("rename group failed: %s", err)
	} else if err == nil {
		return utils.Errorf("rename group failed: new group name %s already exist.", newName)
	}

	updatedGroup, err := QueryGroupByName(db, oldName)
	if err != nil {
		return utils.Errorf("rename group failed: %s", err)
	}
	updatedGroup.GroupName = newName
	if err = db.Update(updatedGroup).Error; err != nil {
		return utils.Errorf("rename group failed: %s", err)
	}
	return nil
}

func isDefaultGroup(groupName string) bool {
	_, ok := buildInGroupsMap[groupName]
	return ok
}
