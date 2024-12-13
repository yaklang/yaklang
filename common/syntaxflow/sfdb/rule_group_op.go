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

func GetOrCreateGroups(db *gorm.DB, groupNames []string) []*schema.SyntaxFlowGroup {
	var groups []*schema.SyntaxFlowGroup
	// 更新内置组
	updateBuildInGroup := func(group *schema.SyntaxFlowGroup, isBuildIn bool) (*schema.SyntaxFlowGroup, error) {
		if group.IsBuildIn != isBuildIn {
			group.IsBuildIn = isBuildIn
			err := db.Update(group).Error
			return group, err
		}
		return group, nil
	}
	for _, groupName := range groupNames {
		isBuildIn := isBuildInGroup(groupName)
		group, err := QueryGroupByName(db, groupName)
		if err == nil && group != nil {
			group, err = updateBuildInGroup(group, isBuildIn)
			if err != nil {
				log.Errorf("update group %s failed: %s", groupName, err)
				continue
			}
			groups = append(groups, group)
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorf("get group %s failed: %s", groupName, err)
			continue
		}
		// if not found, create it
		group, err = CreateGroup(db, groupName, isBuildIn)
		if err != nil {
			log.Errorf("create group %s failed: %s", groupName, err)
			continue
		}
		groups = append(groups, group)
	}
	return groups
}

func isBuildInGroup(groupName string) bool {
	_, ok := buildInGroupsMap[groupName]
	return ok
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
		return item != ""
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
		groups := GetOrCreateGroups(tx, groupNames)
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

	err = db.Where("group_name = ?", oldName).Update("group_name", newName).Error
	if err != nil {
		return utils.Errorf("rename group failed: %s", err)
	}
	return nil
}
