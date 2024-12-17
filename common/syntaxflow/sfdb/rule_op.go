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
	return nil
}

func DeleteRuleByRuleName(name string) error {
	db := consts.GetGormProfileDatabase()
	return db.Where("rule_name = ?", name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
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

func CreateRuleByContent(ruleName string, content string, buildIn bool, tags ...string) (*schema.SyntaxFlowRule, error) {
	languageRaw, _, _ := strings.Cut(ruleName, "-")
	language, err := CheckSyntaxFlowLanguage(languageRaw)
	if err != nil {
		log.Error(err)
	}
	ruleType, err := CheckSyntaxFlowRuleType(ruleName)
	if err != nil {
		log.Error(err)
	}
	rule, err := CheckSyntaxFlowRuleContent(content)
	if err != nil {
		return nil, err
	}
	rule.Type = ruleType
	rule.RuleName = ruleName
	rule.Language = string(language)
	rule.Tag = strings.Join(tags, "|")
	rule.IsBuildInRule = buildIn
	err = MigrateSyntaxFlow(rule.CalcHash(), rule)
	if err != nil {
		return nil, utils.Wrap(err, "migrate syntax flow rule error")
	}
	AddDefaultGroupForRule(consts.GetGormProfileDatabase(), rule)
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
	language, err := CheckSyntaxFlowLanguage(languageRaw)
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
	rule.Language = string(language)
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

func CheckSyntaxFlowLanguage(languageRaw string) (consts.Language, error) {
	switch strings.TrimSpace(strings.ToLower(languageRaw)) {
	case "yak", "yaklang":
		return consts.Yak, nil
	case "java":
		return consts.JAVA, nil
	case "php":
		return consts.PHP, nil
	case "js", "es", "javascript", "ecmascript", "nodejs", "node", "node.js":
		return consts.JS, nil
	case "golang", "go":
		return consts.GO, nil
	case "general":
		return consts.General, nil
	}
	return "", utils.Errorf("invalid language: %v is not supported yet", languageRaw)
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

func GetAllRules() ([]*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Where("allow_included = false")
	outC := make(chan *schema.SyntaxFlowRule)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.SyntaxFlowRule
			if _, b := bizhelper.Paging(db, page, 1000, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				outC <- d
			}

			if len(items) < 1000 {
				return
			}
		}
	}()

	var rules []*schema.SyntaxFlowRule
	for r := range outC {
		rules = append(rules, r)
	}
	if len(rules) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return rules, nil
}

func GetRules(ruleNameGlob string) ([]*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Where("(rule_name like ?) and (allow_included = false)", "%"+fmt.Sprint(ruleNameGlob)+"%")
	outC := make(chan *schema.SyntaxFlowRule)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.SyntaxFlowRule
			if _, b := bizhelper.Paging(db, page, 1000, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				outC <- d
			}

			if len(items) < 1000 {
				return
			}
		}
	}()

	var rules []*schema.SyntaxFlowRule
	for r := range outC {
		rules = append(rules, r)
	}
	if len(rules) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return rules, nil
}

func YieldSyntaxFlowRules(db *gorm.DB, ctx context.Context) chan *schema.SyntaxFlowRule {
	outC := make(chan *schema.SyntaxFlowRule)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.SyntaxFlowRule
			if _, b := bizhelper.Paging(db, page, 1000, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
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

func UpdateRule(rule *schema.SyntaxFlowRule) error {
	if rule == nil {
		return utils.Errorf("update syntaxFlow rule failed: rule is nil")
	}
	if rule.RuleName == "" {
		return utils.Errorf("update syntaxFlow rule failed: rule name is empty")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SyntaxFlowRule{})
	if err := db.Where("rule_name = ?", rule.RuleName).Update(rule).Error; err != nil {
		return utils.Errorf("update syntaxFlow rule failed: %s", err)
	}
	return nil
}

func CreateRule(rule *schema.SyntaxFlowRule, groups ...string) (*schema.SyntaxFlowRule, error) {
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
	rule.Groups = nil
	if err := db.Create(&rule).Error; err != nil {
		return nil, utils.Errorf("create syntaxFlow rule failed: %s", err)
	}
	AddDefaultGroupForRule(db, rule, groups...)
	return rule, nil
}
