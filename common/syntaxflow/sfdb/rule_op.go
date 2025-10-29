package sfdb

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func ExportDatabase() io.ReadCloser {
	r, w := utils.NewBufPipe(nil)
	go func() {
		defer func() {
			w.Close()
		}()
		for result := range YieldSyntaxFlowRules(consts.GetGormProfileDatabase(), context.Background()) {
			result.ID = 0
			result.IsBuildInRule = false
			raw, err := json.Marshal(result)
			if err != nil {
				log.Errorf("marshal syntax flow rule error: %s", err)
				continue
			}
			_, err = w.Write(raw)
			if err != nil {
				log.Errorf("write syntax flow rule error: %s", err)
				continue
			}
			w.Write([]byte{'\n'})
		}
	}()
	return r
}

func ImportDatabase(reader io.Reader) error {
	scanner := bufio.NewReader(reader)
	for {
		line, err := utils.BufioReadLine(scanner)
		if err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return err
		}
		var rule schema.SyntaxFlowRule
		if err := json.Unmarshal(line, &rule); err != nil {
			log.Errorf("unmarshal syntax flow rule error: %s", err)
			continue
		}

		refRule := &rule
		if refRule.IsBuildInRule {
			refRule.IsBuildInRule = false
		}
		err = MigrateSyntaxFlow(rule.CalcHash(), refRule)
		if err != nil {
			log.Errorf("create or update syntax flow rule error: %s", err)
			continue
		}
	}

	return nil
}

func MigrateSyntaxFlow(hash string, i *schema.SyntaxFlowRule) error {
	db := consts.GetGormProfileDatabase()

	if hash == "" {
		hash = i.CalcHash()
	}

	var rules []schema.SyntaxFlowRule
	if err := db.Where("rule_name = ?", i.RuleName).Find(&rules).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(i).Error
		}
		return err
	}
	if len(rules) == 1 {
		i.Hash = i.CalcHash()
		// only one rule, check and update
		rule := rules[0]
		if rule.Hash != hash {
			// if same name, but different content, update
			return db.Model(&rule).Updates(i).Error
		}
		return nil
	} else if len(rules) > 1 {
		// multiple rule in same name, delete all and create new
		if err := db.Where("rule_name = ?", i.RuleName).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error; err != nil {
			return err
		}
		return db.Create(i).Error
	} else {
		return db.Create(i).Error
	}
}

func DeleteRuleByRuleName(name string) error {
	db := consts.GetGormProfileDatabase()
	return db.Where("rule_name = ?", name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

func DeleteBuildInRule() error {
	db := consts.GetGormProfileDatabase()
	return db.Where("is_build_in_rule = ?", true).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

func DeleteRuleByLibName(name string) error {
	if name == "" {
		return nil
	}
	db := consts.GetGormProfileDatabase()
	return db.Where("included_name = ?", name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

func DeleteRuleByTitle(name string) error {
	db := consts.GetGormProfileDatabase()
	return db.Where("title = ? or title_zh = ?", name, name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

func CreateRuleByContent(ruleFileName string, content string, buildIn bool, tags ...string) (*schema.SyntaxFlowRule, error) {
	languageRaw, _, _ := strings.Cut(ruleFileName, "-")
	language, err := ssaconfig.ValidateLanguage(languageRaw)
	if err != nil {
		log.Error(err)
	}
	ruleType, err := CheckSyntaxFlowRuleType(ruleFileName)
	if err != nil {
		log.Error(err)
	}
	rule, err := CheckSyntaxFlowRuleContent(content)
	if err != nil {
		return nil, err
	}

	cweList := make([]string, 0)

	// 从tags中提取CWE
	for _, tag := range tags {
		if strings.HasPrefix(tag, "CWE-") {
			cweList = append(cweList, tag)
		}
	}

	// 如果没有从tags中找到CWE，尝试从CVE获取
	if len(cweList) == 0 && rule.CVE != "" {
		cwes, err := getCWEsByCVE(rule.CVE)
		if err == nil {
			cweList = append(cweList, cwes...)
		}
	}

	rule.CWE = append(rule.CWE, cweList...)
	// 去重CWE列表
	rule.CWE = lo.Uniq(rule.CWE)

	rule.Type = ruleType
	rule.RuleName = ruleFileName
	rule.Language = language
	rule.Tag = strings.Join(tags, "|")
	rule.IsBuildInRule = buildIn
	version, err := GetVersion(rule.RuleId)
	if err == nil {
		rule.Version = version
	}
	if buildIn {
		// build in rule, use rule.title if exist
		if rule.TitleZh != "" {
			rule.RuleName = rule.TitleZh
		} else if rule.Title != "" {
			rule.RuleName = rule.Title
		}
	}
	err = MigrateSyntaxFlow(rule.CalcHash(), rule)
	if err != nil {
		return nil, utils.Wrap(err, "migrate syntax flow rule error")
	}
	addGroupsForRule(consts.GetGormProfileDatabase(), rule, true)
	return rule, nil
}

func ImportRuleWithoutValid(ruleName string, content string, buildin bool, tags ...string) (*schema.SyntaxFlowRule, error) {
	rule, err := CreateRuleByContent(ruleName, content, buildin, tags...)
	if err != nil {
		return nil, utils.Errorf("create build in rule failed: %s", err)
	}
	return rule, nil
}

func ImportValidRule(system fi.FileSystem, ruleName string, content string) error {
	languageRaw, _, _ := strings.Cut(ruleName, "-")
	language, err := ssaconfig.ValidateLanguage(languageRaw)
	if err != nil {
		log.Error(err)
	}
	ruleType, err := CheckSyntaxFlowRuleType(ruleName)
	if err != nil {
		log.Error(err)
	}

	rule, err := CheckSyntaxFlowRuleContent(content)
	if err != nil {
		return err
	}
	rule.Language = language
	rule.Type = ruleType

	err = LoadFileSystem(rule, system)
	if err != nil {
		return utils.Wrap(err, "load file system error")
	}

	if valid != nil {
		err = valid(rule)
		if err != nil {
			return utils.Wrap(err, "valid rule error")
		}
	}

	err = MigrateSyntaxFlow(rule.CalcHash(), rule)
	if err != nil {
		return utils.Wrap(err, "create or update syntax flow rule error")
	}
	return nil
}

func CheckSyntaxFlowRuleType(ruleName string) (schema.SyntaxFlowRuleType, error) {
	switch path.Ext(ruleName) {
	case ".sf", ".syntaxflow":
		return schema.SFR_RULE_TYPE_SF, nil
	default:
		return "", utils.Errorf("invalid rule type: %v is not supported yet, treat it as syntaxflow(.sf, .syntaxflow)", ruleName)
	}
}

func CheckSyntaxFlowRuleContent(content string) (*schema.SyntaxFlowRule, error) {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(content)
	if err != nil {
		return &schema.SyntaxFlowRule{}, err
	}
	rule := frame.GetRule()
	return rule, nil
}

var (
	valid        func(rule *schema.SyntaxFlowRule) error
	registerOnce = new(sync.Once)
)

func RegisterValid(f func(rule *schema.SyntaxFlowRule) error) {
	registerOnce.Do(func() {
		valid = f
	})
}

func GetLibrary(libname string) (*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	var rule schema.SyntaxFlowRule
	if err := db.Where("(title = ?) or (included_name = ?)", libname, libname).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func GetRule(ruleName string) (*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	var rule schema.SyntaxFlowRule
	if err := db.Where("(rule_name = ?) and (allow_included = false)", ruleName).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func GetRulePure(ruleName string) (*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	var rule schema.SyntaxFlowRule
	if err := db.Where("rule_name = ?", ruleName).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}
func GetRuleByLanguage(language string) ([]*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	var rule []*schema.SyntaxFlowRule
	result := db.Where("language = ?", language).Where("allow_included = false").Find(&rule)
	if result.Error != nil {
		return nil, result.Error
	}
	return rule, nil
}

func GetRules(ruleNameGlob string) ([]*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Where("(rule_name like ?) and (allow_included = false)", "%"+fmt.Sprint(ruleNameGlob)+"%")

	var rules []*schema.SyntaxFlowRule
	for r := range YieldSyntaxFlowRules(db, context.Background()) {
		rules = append(rules, r)
	}
	if len(rules) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return rules, nil
}

func YieldBuildInSyntaxFlowRules(db *gorm.DB, ctx context.Context) chan *schema.SyntaxFlowRule {
	db = db.Model(&schema.SyntaxFlowRule{}).Where("is_build_in_rule")
	return YieldSyntaxFlowRules(db, ctx)
}

func YieldSyntaxFlowRules(db *gorm.DB, ctx context.Context) chan *schema.SyntaxFlowRule {
	return bizhelper.YieldModel[*schema.SyntaxFlowRule](ctx, db, bizhelper.WithYieldModel_IndexField("syntax_flow_rules.id"))
}

func YieldSyntaxFlowRulesWithoutLib(db *gorm.DB, ctx context.Context) chan *schema.SyntaxFlowRule {
	db = db.Where("allow_included = ?", false)
	return YieldSyntaxFlowRules(db, ctx)
}

func QueryRuleByName(db *gorm.DB, ruleName string) (*schema.SyntaxFlowRule, error) {
	var rule schema.SyntaxFlowRule
	if err := db.Preload("Groups").Where("rule_name = ?", ruleName).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func QueryRulesByName(db *gorm.DB, ruleNames []string) ([]*schema.SyntaxFlowRule, error) {
	var rules []*schema.SyntaxFlowRule
	if err := db.Preload("Groups").Where("rule_name IN (?)", ruleNames).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func QueryRulesById(db *gorm.DB, ruleIds []string) ([]*schema.SyntaxFlowRule, error) {
	var rules []*schema.SyntaxFlowRule
	if err := db.Preload("Groups").Where("rule_id IN (?)", ruleIds).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func QueryRuleByLanguage(db *gorm.DB, language ssaconfig.Language) ([]*schema.SyntaxFlowRule, error) {
	var rules []*schema.SyntaxFlowRule
	if err := db.Where("language = ?", language).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func UpdateRule(db *gorm.DB, rule *schema.SyntaxFlowRule) error {
	if rule == nil {
		return utils.Errorf("update syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return utils.Errorf("update syntaxFlow rule failed: rule name is empty")
	}
	db = db.Model(&schema.SyntaxFlowRule{})
	if err := db.Where("rule_name = ?", rule.RuleName).Update(rule).Error; err != nil {
		return utils.Errorf("update syntaxFlow rule failed: %s", err)
	}
	return nil
}

func QueryGroupByRuleIds(db *gorm.DB, RuleIds []string) ([]*schema.SyntaxFlowGroup, error) {
	rules, err := QueryRulesById(db, RuleIds)
	if err != nil {
		return nil, err
	}
	var groups []*schema.SyntaxFlowGroup
	for _, rule := range rules {
		groups = append(groups, rule.Groups...)
	}
	return groups, nil
}

func createRuleEx(rule *schema.SyntaxFlowRule, needDefaultGroup bool, groups ...string) (*schema.SyntaxFlowRule, error) {
	if rule == nil {
		return nil, utils.Errorf("create syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return nil, utils.Errorf("create syntaxFlow rule failed: rule name is empty")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowRule{})
	// 只是创建规则而不带着组去创建，后续再添加组。
	// 因为多对多的表直接创建会导致和该组相关的规则都被更新。
	backUp := lo.Map(rule.Groups, func(group *schema.SyntaxFlowGroup, _ int) string {
		return group.GroupName
	})
	groups = append(groups, backUp...)
	rule.Groups = nil
	if err := db.Create(&rule).Error; err != nil {
		return nil, utils.Errorf("create syntaxFlow rule failed: %s", err)
	}
	addGroupsForRule(db, rule, needDefaultGroup, groups...)
	return rule, nil
}

func CreateRule(rule *schema.SyntaxFlowRule, groups ...string) (*schema.SyntaxFlowRule, error) {
	return createRuleEx(rule, false, groups...)
}

func CreateRuleWithDefaultGroup(rule *schema.SyntaxFlowRule, groups ...string) (*schema.SyntaxFlowRule, error) {
	return createRuleEx(rule, true, groups...)
}

func CreateOrUpdateRuleWithGroup(rule *schema.SyntaxFlowRule, groups ...string) (*schema.SyntaxFlowRule, error) {
	if rule == nil {
		return nil, utils.Errorf("create syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return nil, utils.Errorf("create syntaxFlow rule failed: rule name is empty")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowRule{})
	// 只是创建规则而不带着组去创建，后续再添加组。
	// 因为多对多的表直接创建会导致和该组相关的规则都被更新。
	backUp := lo.Map(rule.Groups, func(group *schema.SyntaxFlowGroup, _ int) string {
		return group.GroupName
	})
	groups = append(groups, backUp...)
	rule.Groups = nil
	if err := CreateOrUpdateSyntaxFlowRule(db, rule.RuleName, &rule); err != nil {
		return nil, utils.Errorf("create syntaxFlow rule failed: %s", err)
	}
	CreateOrUpdateGroupsForRule(db, rule, groups...)
	return rule, nil
}

func CreateOrUpdateSyntaxFlowRule(db *gorm.DB, RuleName string, i interface{}) error {
	db = db.Model(&schema.SyntaxFlowRule{})
	db = db.Where("rule_name = ?", RuleName).Assign(i).FirstOrCreate(&schema.SyntaxFlowRule{})
	if db.Error != nil {
		return utils.Errorf("create/update SyntaxFlowRule failed: %s", db.Error)
	}

	return nil
}

func DeleteSyntaxFlowRuleByRuleNameOrRuleId(name, ruleId string) error {
	db := consts.GetGormProfileDatabase()
	return db.Where("rule_name = ? or rule_id = ?", name, ruleId).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

// getCWEsByCVE 通过CVE字符串查询相关的CWE列表
func getCWEsByCVE(cveStr string) ([]string, error) {
	if cveStr == "" {
		return nil, nil
	}

	// 获取CVE数据库连接
	db := consts.GetGormCVEDatabase()

	if db == nil {
		return nil, utils.Errorf("CVE database not available")
	}

	// 查询CVE详情
	cve, err := cveresources.GetCVE(db, cveStr)
	if err != nil {
		return nil, utils.Wrapf(err, "get CVE %s failed", cveStr)
	}

	if cve == nil || cve.CWE == "" {
		return nil, utils.Errorf("CVE %s has no associated CWE information", cveStr)
	}

	// 解析CWE字符串，支持多种分隔符
	cwes := utils.PrettifyListFromStringSplitEx(cve.CWE, "|", ",")
	if len(cwes) == 0 {
		return nil, utils.Errorf("CVE %s has no associated CWE information", cveStr)
	}

	// 清理和标准化CWE格式
	var result []string
	for _, cwe := range cwes {
		cwe = strings.TrimSpace(cwe)
		if cwe == "" {
			continue
		}

		// 确保CWE格式正确（以CWE-开头）
		if !strings.HasPrefix(strings.ToUpper(cwe), "CWE-") {
			// 如果只是数字，添加CWE-前缀
			if strings.TrimSpace(cwe) != "" {
				cwe = "CWE-" + cwe
			}
		}

		result = append(result, cwe)
	}
	return result, nil
}
