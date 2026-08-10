package sfdb

import (
	"errors"

	"github.com/samber/lo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var buildInGroupsMap map[string]struct{}

func init() {
	buildInGroupsMap = make(map[string]struct{})
	var buildInGroups []string
	// reserved rule groups
	buildInGroups = append(buildInGroups,
		schema.SyntaxFlowGroupBuiltin,
		schema.SyntaxFlowGroupAgent,
	)
	// legacy auto-group names (compat for isBuildIn checks only)
	buildInGroups = append(buildInGroups, schema.GetAllSFSupportLanguage()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFPurposeTypes()...)
	buildInGroups = append(buildInGroups, schema.GetAllSFSeverityTypes()...)
	lo.ForEach(buildInGroups, func(item string, _ int) {
		if item != "" {
			buildInGroupsMap[item] = struct{}{}
		}
	})
}

// pickExclusiveGroup returns the single group name for a rule.
// With the one-group-per-rule model, multiple names collapse to the last non-empty.
func pickExclusiveGroup(groupNames []string) string {
	var last string
	for _, g := range groupNames {
		if g != "" {
			last = g
		}
	}
	return last
}

// CreateGroup creates a SyntaxFlow rule-group catalog entry.
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
	updateBuildInGroup := func(group *schema.SyntaxFlowGroup, isBuildIn bool) (*schema.SyntaxFlowGroup, error) {
		if group.IsBuildIn != isBuildIn {
			group.IsBuildIn = isBuildIn
			err := db.Save(group).Error
			return group, err
		}
		return group, nil
	}
	for _, groupName := range groupNames {
		if groupName == "" {
			continue
		}
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
	if ok {
		return true
	}
	return ssaconfig.IsStandardGroupName(groupName)
}

// QueryGroupByName returns a catalog group by name (no rule preload).
func QueryGroupByName(db *gorm.DB, groupName string) (*schema.SyntaxFlowGroup, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	i := &schema.SyntaxFlowGroup{}
	if db = db.Where("group_name = ?", groupName).First(i); db.Error != nil {
		return nil, db.Error
	}
	return i, nil
}

func QueryGroupsByName(db *gorm.DB, groupNames []string) ([]*schema.SyntaxFlowGroup, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	var groups []*schema.SyntaxFlowGroup
	if db = db.Where("group_name IN (?)", groupNames).Find(&groups); db.Error != nil {
		return nil, db.Error
	}
	return groups, nil
}

// CountRulesInGroup counts rules whose RuleGroup equals groupName.
func CountRulesInGroup(db *gorm.DB, groupName string) int64 {
	var count int64
	db.Model(&schema.SyntaxFlowRule{}).Where("rule_group = ?", groupName).Count(&count)
	return count
}

// GetIntersectionGroup returns groups present on every rule (single-group model → equal RuleGroup).
func GetIntersectionGroup(db *gorm.DB, groups [][]*schema.SyntaxFlowGroup) []*schema.SyntaxFlowGroup {
	var groupNames [][]string
	lo.ForEach(groups, func(group []*schema.SyntaxFlowGroup, _ int) {
		var names []string
		lo.ForEach(group, func(item *schema.SyntaxFlowGroup, _ int) {
			names = append(names, item.GroupName)
		})
		groupNames = append(groupNames, names)
	})

	if len(groupNames) == 0 {
		return []*schema.SyntaxFlowGroup{}
	}

	groupCount := make(map[string]int)
	for _, names := range groupNames {
		seen := make(map[string]struct{})
		for _, name := range names {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			groupCount[name]++
		}
	}

	var resultName []string
	for name, count := range groupCount {
		if count == len(groupNames) {
			resultName = append(resultName, name)
		}
	}
	result, _ := QueryGroupsByName(db, resultName)
	return result
}

func addGroupsForRule(db *gorm.DB, rule *schema.SyntaxFlowRule, needDefaultGroup bool, groups ...string) error {
	if rule == nil {
		return utils.Errorf("add default group for rule failed:rule is empty")
	}
	if needDefaultGroup {
		// language/severity/purpose auto-groups are abandoned; keep flag for call-site compat only
		_ = needDefaultGroup
	}
	groups = lo.Filter(groups, func(item string, _ int) bool {
		return item != ""
	})
	_, err := BatchAddGroupsForRules(db, []string{rule.RuleName}, groups)
	db.Where("rule_name = ?", rule.RuleName).First(&rule)
	return err
}

// BatchAddGroupsForRules sets each rule's exclusive RuleGroup (last name wins).
func BatchAddGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	ruleNames = utils.RemoveRepeatedWithStringSlice(ruleNames)
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)
	exclusive := pickExclusiveGroup(groupNames)
	if exclusive == "" || len(ruleNames) == 0 {
		return 0, utils.Errorf("batch add groups for rules failed: groups or rules is empty")
	}

	var count int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		_ = GetOrCreateGroups(tx, []string{exclusive})
		rules, err := QueryRulesByName(tx, ruleNames)
		if err != nil {
			return err
		}
		if len(ruleNames) != len(rules) {
			return utils.Errorf("batch add groups for rules failed: rules not found")
		}
		for _, rule := range rules {
			if err = tx.Model(rule).Update("rule_group", exclusive).Error; err != nil {
				return err
			}
			rule.RuleGroup = exclusive
			count++
		}
		return nil
	})
	return count, err
}

func BatchAddGroupsForRulesByRuleId(db *gorm.DB, ruleIds, groupNames []string) (int64, error) {
	ruleIds = utils.RemoveRepeatedWithStringSlice(ruleIds)
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)
	exclusive := pickExclusiveGroup(groupNames)
	if exclusive == "" || len(ruleIds) == 0 {
		return 0, utils.Errorf("batch add groups for rules failed: groups or rules is empty")
	}

	var count int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		_ = GetOrCreateGroups(tx, []string{exclusive})
		rules, err := QueryRulesById(tx, ruleIds)
		if err != nil {
			return err
		}
		if len(ruleIds) != len(rules) {
			return utils.Errorf("batch add groups for rules failed: rules not found")
		}
		for _, rule := range rules {
			if err = tx.Model(rule).Update("rule_group", exclusive).Error; err != nil {
				return err
			}
			rule.RuleGroup = exclusive
			count++
		}
		return nil
	})
	return count, err
}

// BatchRemoveGroupsForRules clears RuleGroup when it matches a removed name (→ custom).
func BatchRemoveGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	var count int64
	ruleNames = utils.RemoveRepeatedWithStringSlice(ruleNames)
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)
	removeSet := make(map[string]struct{}, len(groupNames))
	for _, g := range groupNames {
		removeSet[g] = struct{}{}
	}

	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		rules, err := QueryRulesByName(tx, ruleNames)
		if err != nil {
			return utils.Errorf("batch remove groups for rules failed: %s", err)
		}
		if len(rules) == 0 || len(groupNames) == 0 {
			return utils.Errorf("batch remove groups for rules failed: rules or groups is empty")
		}
		if len(ruleNames) != len(rules) {
			return utils.Errorf("batch remove groups for rules failed: rules not found")
		}
		for _, rule := range rules {
			if _, ok := removeSet[rule.RuleGroup]; !ok {
				continue
			}
			if err = tx.Model(rule).Update("rule_group", schema.SyntaxFlowGroupCustom).Error; err != nil {
				return err
			}
			rule.RuleGroup = schema.SyntaxFlowGroupCustom
			_ = GetOrCreateGroups(tx, []string{schema.SyntaxFlowGroupCustom})
			count++
		}
		return nil
	})

	return count, err
}

func BatchRemoveGroupsForRulesById(db *gorm.DB, ruleIds, groupNames []string) (int64, error) {
	var count int64
	ruleIds = utils.RemoveRepeatedWithStringSlice(ruleIds)
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)
	removeSet := make(map[string]struct{}, len(groupNames))
	for _, g := range groupNames {
		removeSet[g] = struct{}{}
	}

	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		rules, err := QueryRulesById(tx, ruleIds)
		if err != nil {
			return utils.Errorf("batch remove groups for rules failed: %s", err)
		}
		if len(rules) == 0 || len(groupNames) == 0 {
			return utils.Errorf("batch remove groups for rules failed: rules or groups is empty")
		}
		if len(ruleIds) != len(rules) {
			return utils.Errorf("batch remove groups for rules failed: rules not found")
		}
		for _, rule := range rules {
			if _, ok := removeSet[rule.RuleGroup]; !ok {
				continue
			}
			if err = tx.Model(rule).Update("rule_group", schema.SyntaxFlowGroupCustom).Error; err != nil {
				return err
			}
			rule.RuleGroup = schema.SyntaxFlowGroupCustom
			_ = GetOrCreateGroups(tx, []string{schema.SyntaxFlowGroupCustom})
			count++
		}
		return nil
	})

	return count, err
}

// DeleteGroup deletes a catalog group by name (does not rewrite rules).
func DeleteGroup(db *gorm.DB, groupName string) error {
	db = db.Model(&schema.SyntaxFlowGroup{})
	db = db.Where("group_name = ?", groupName).Unscoped().Delete(&schema.SyntaxFlowGroup{})
	return db.Error
}

// RenameGroup renames catalog + updates matching Rule.RuleGroup values.
func RenameGroup(db *gorm.DB, oldName, newName string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := tx.Model(&schema.SyntaxFlowGroup{}).Where("group_name = ?", oldName).
			Update("group_name", newName).Error; err != nil {
			return utils.Errorf("rename group failed: %s", err)
		}
		if err := tx.Model(&schema.SyntaxFlowRule{}).Where("rule_group = ?", oldName).
			Update("rule_group", newName).Error; err != nil {
			return utils.Errorf("rename rule_group failed: %s", err)
		}
		return nil
	})
}

func CreateOrUpdateGroupsForRule(db *gorm.DB, rule *schema.SyntaxFlowRule, groups ...string) error {
	if rule == nil {
		return nil
	}
	groups = lo.Filter(groups, func(item string, _ int) bool {
		return item != ""
	})
	_, err := BatchAddOrUpdateGroupsForRules(db, []string{rule.RuleName}, groups)
	db.Where("rule_name = ?", rule.RuleName).First(&rule)
	return err
}

// BatchAddOrUpdateGroupsForRules replaces each rule's exclusive RuleGroup.
func BatchAddOrUpdateGroupsForRules(db *gorm.DB, ruleNames, groupNames []string) (int64, error) {
	return BatchAddGroupsForRules(db, ruleNames, groupNames)
}

func CreateOrUpdateGroups(db *gorm.DB, groupNames []string) []*schema.SyntaxFlowGroup {
	var groups []*schema.SyntaxFlowGroup
	for _, groupName := range groupNames {
		i := &schema.SyntaxFlowGroup{
			GroupName: groupName,
			IsBuildIn: isBuildInGroup(groupName),
		}
		group, err := CreateOrUpdateGroup(db, groupName, i)
		if err != nil {
			log.Errorf("create group %s failed: %s", groupName, err)
			continue
		}
		groups = append(groups, group)
	}
	return groups
}

func CreateOrUpdateGroup(db *gorm.DB, groupName string, i *schema.SyntaxFlowGroup) (*schema.SyntaxFlowGroup, error) {
	db = db.Model(&schema.SyntaxFlowGroup{})
	group := schema.SyntaxFlowGroup{}
	if db := db.Where("group_name = ?", groupName).Assign(i).FirstOrCreate(&group); db.Error != nil {
		return nil, utils.Errorf("create/update SyntaxFlowGroup failed: %s", db.Error)
	}

	return &group, nil
}

// groupFromRule builds a synthetic one-element group slice for intersection helpers.
func groupFromRule(db *gorm.DB, rule *schema.SyntaxFlowRule) []*schema.SyntaxFlowGroup {
	if rule == nil || rule.RuleGroup == "" {
		return nil
	}
	g, err := QueryGroupByName(db, rule.RuleGroup)
	if err != nil {
		return []*schema.SyntaxFlowGroup{{GroupName: rule.RuleGroup}}
	}
	return []*schema.SyntaxFlowGroup{g}
}

// GroupsForRule is the exported alias used by yakit same-group queries.
func GroupsForRule(db *gorm.DB, rule *schema.SyntaxFlowRule) []*schema.SyntaxFlowGroup {
	return groupFromRule(db, rule)
}
