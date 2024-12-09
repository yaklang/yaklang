package sfdb

import (
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

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

// AddGroupsForBuildInRule 为内置规则添加默认分组
// 默认分组为：语言、严重程度、规则类型
func AddGroupsForBuildInRule(db *gorm.DB, buildInRule *schema.SyntaxFlowRule) error {
	if buildInRule == nil {
		return utils.Errorf("add build in rule group failed:rule is empty")
	}
	var groups []string
	groups = append(groups, buildInRule.Language)
	groups = append(groups, string(buildInRule.Severity))
	groups = append(groups, string(buildInRule.Severity))

	_, err := BatchAddGroupsForRules(db, []string{buildInRule.RuleName}, groups)
	return err
}

// BatchAddGroupsForRules 为多个规则添加多个组
// 如果要添加的组不存在，会自动创建
func BatchAddGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var count int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, groupName := range groupNames {
			for _, ruleName := range ruleNames {
				if ruleName == "" || groupName == "" {
					continue
				}
				err := AddGroupForRule(tx, ruleName, groupName)
				if err != nil {
					return err
				} else {
					count++
				}
			}
		}
		return nil
	})
	return count, err
}

// BatchRemoveGroupsForRules 为多个规则移除多个组
func BatchRemoveGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var count int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, groupName := range groupNames {
			for _, ruleName := range ruleNames {
				err := RemoveGroupForRule(tx, ruleName, groupName)
				if err != nil {
					return err
				} else {
					count++
				}
			}
		}
		return nil
	})

	return count, err
}

// AddGroupForRule 为规则添加组
// 如果要添加的组不存在，会自动创建
func AddGroupForRule(db *gorm.DB, ruleName, groupName string) error {
	group, err := QueryGroupByName(db, groupName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if group, err = CreateGroup(db, groupName); err != nil {
			return err
		}
	}
	rule, err := QueryRuleByName(db, ruleName)
	if err != nil {
		return err
	}

	if err = db.Model(group).Association("Rules").Append(rule).Error; err != nil {
		return err
	}
	return nil
}

// RemoveGroupForRule 为规则移除组
func RemoveGroupForRule(db *gorm.DB, ruleName, groupName string) error {
	rule, err := QueryRuleByName(db, ruleName)
	if err != nil {
		return err
	}
	group, err := QueryGroupByName(db, groupName)
	if err != nil {
		return err
	}

	if err := db.Model(group).Association("Rules").Delete(rule).Error; err != nil {
		return err
	}
	return nil
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

// ImportBuildInGroup 导入规则内置分组,默认使用language,purpose,severity作为内置分组
func ImportBuildInGroup(db *gorm.DB) {
	var buildInGroups []string
	buildInGroups = append(buildInGroups, schema.GetAllSFSupportLanguage()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFPurposeTypes()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFSeverityTypes()...)

	for _, groupName := range buildInGroups {
		_, err := QueryGroupByName(db, groupName)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if _, err = CreateGroup(db, groupName, true); err != nil {
			log.Errorf("create group %s failed: %s", groupName, err)
		}
	}
}
