package sfdb

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// QueryGroupCountInRule 统计规则中的组数量
func QueryGroupCountInRule(ruleName string) int32 {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowRule{})
	var rule schema.SyntaxFlowRule
	db.Preload("Groups").Where("rule_name = ?", ruleName).First(&rule)
	return int32(len(rule.Groups))
}

// QueryRuleCountInGroup 统计某个组中的规则数量
func QueryRuleCountInGroup(groupName string) int32 {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	var group schema.SyntaxFlowGroup
	db.Preload("Rules").Where("group_name = ?", groupName).First(&group)
	return int32(len(group.Rules))
}

// QueryRuleCountInGroups 统计多个组中的规则数量
func QueryRuleCountInGroups(groupNames []string) int32 {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	var groups []schema.SyntaxFlowGroup
	db.Preload("Rules").
		Where("group_name IN (?)", groupNames).
		Find(&groups)
	var count int32
	for _, group := range groups {
		count += int32(len(group.Rules))
	}
	return count
}

// CreateGroupsByName 通过多个组名创建多个SyntaxFlow规则组
func CreateGroupsByName(groupNames []string, isBuildIn ...bool) (int64, error) {
	var count int64
	var errs error
	for _, groupName := range groupNames {
		if _, err := CreateGroupByName(groupName, isBuildIn...); err != nil {
			errs = utils.JoinErrors(errs, err)
			continue
		} else {
			count++
		}
	}
	return count, errs
}

// CreateGroupByName 通过组名创建SyntaxFlow规则组
func CreateGroupByName(groupName string, isBuildIn ...bool) (*schema.SyntaxFlowGroup, error) {
	buildIn := false
	if len(isBuildIn) > 0 {
		buildIn = isBuildIn[0]
	}

	db := consts.GetGormProfileDatabase()
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

// QueryAllGroups 查询所有的SyntaxFlow规则组
func QueryAllGroups() ([]schema.SyntaxFlowGroup, error) {
	db := consts.GetGormProfileDatabase()
	var groups []schema.SyntaxFlowGroup
	db = db.Model(&schema.SyntaxFlowGroup{})
	if db = db.Find(&groups); db.Error != nil {
		return nil, db.Error
	}
	return groups, nil
}

// QueryGroupByName 根据组名查询组
func QueryGroupByName(groupName string) (*schema.SyntaxFlowGroup, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	i := &schema.SyntaxFlowGroup{}
	if db = db.Preload("Rules").Where("group_name = ?", groupName).First(i); db.Error != nil {
		return nil, db.Error
	}
	return i, nil
}

// AddGroupsForBuildInRule 为内置规则添加默认分组
// 默认分组为：语言、严重程度、规则类型
func AddGroupsForBuildInRule(buildInRule *schema.SyntaxFlowRule) error {
	if buildInRule == nil {
		return utils.Errorf("add build in rule group failed:rule is empty")
	}
	var groups []string
	groups = append(groups, buildInRule.Language)
	groups = append(groups, string(buildInRule.Severity))
	groups = append(groups, string(buildInRule.Severity))

	_, err := AddGroupsForRulesByName([]string{buildInRule.RuleName}, groups)
	return err
}

// AddGroupsForRulesByName 为多个规则添加多个组
// 如果要添加的组不存在，会自动创建
func AddGroupsForRulesByName(ruleNames, groupNames []string) (int64, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	var errs error
	var count int64
	for _, groupName := range groupNames {
		for _, ruleName := range ruleNames {
			err := AddGroupForRuleByName(ruleName, groupName)
			if err != nil {
				errs = utils.JoinErrors(errs, err)
				continue
			} else {
				count++
			}
		}
	}
	return count, errs
}

// RemoveGroupsForRulesByName 为多个规则移除多个组
func RemoveGroupsForRulesByName(ruleNames, groupNames []string) (int64, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	var errs error
	var count int64
	for _, groupName := range groupNames {
		for _, ruleName := range ruleNames {
			err := RemoveGroupForRuleByName(ruleName, groupName)
			if err != nil {
				errs = utils.JoinErrors(errs, err)
				continue
			} else {
				count++
			}
		}
	}
	return count, errs
}

// AddGroupForRuleByName 为规则添加组
// 如果要添加的组不存在，会自动创建
func AddGroupForRuleByName(ruleName, groupName string) error {
	db := consts.GetGormProfileDatabase()
	rule, err := QueryRuleByName(ruleName)
	if err != nil {
		return err
	}
	group, err := QueryGroupByName(groupName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if _, err = CreateGroupByName(groupName); err != nil {
			return err
		}
		group, err = QueryGroupByName(groupName)
	}
	if err != nil {
		return err
	}
	if rule != nil && group != nil {
		if err = db.Model(group).Association("Rules").Append(rule).Error; err != nil {
			return err
		}
	}
	return nil
}

// RemoveGroupForRuleByName 为规则移除组
func RemoveGroupForRuleByName(ruleName, groupName string) error {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	rule, err := QueryRuleByName(ruleName)
	if err != nil {
		return err
	}
	group, err := QueryGroupByName(groupName)
	if err != nil {
		return err
	}
	if rule != nil && group != nil {
		if err = db.Model(group).Association("Rules").Delete(rule).Error; err != nil {
			return err
		}
	}
	return nil
}

// DeleteGroupsByName 通过多个组名删除多个SyntaxFlow规则组
func DeleteGroupsByName(groupNames []string) (int64, error) {
	var count int64
	var errs error
	for _, groupName := range groupNames {
		if err := DeleteGroupByName(groupName); err != nil {
			errs = utils.JoinErrors(errs, err)
			continue
		} else {
			count++
		}
	}
	return count, errs
}

// DeleteGroupByName 通过组名删除SyntaxFlow规则组
func DeleteGroupByName(groupName string) error {
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowGroup{})
	db = db.Where("group_name = ?", groupName).Unscoped().Delete(&schema.SyntaxFlowGroup{})
	return db.Error
}

// ImportBuildInGroup 导入规则内置分组,默认使用language,purpose,severity作为内置分组
func ImportBuildInGroup() {
	var buildInGroups []string
	buildInGroups = append(buildInGroups, schema.GetAllSFSupportLanguage()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFPurposeTypes()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFSeverityTypes()...)

	for _, groupName := range buildInGroups {
		_, err := QueryGroupByName(groupName)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if _, err = CreateGroupByName(groupName, true); err != nil {
			log.Errorf("create group %s failed: %s", groupName, err)
		}
	}
}
