package sfdb

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io"
	"path"
	"strings"
	"sync"

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

		err = CreateOrUpdateSyntaxFlow(rule.CalcHash(), &rule)
		if err != nil {
			log.Errorf("create or update syntax flow rule error: %s", err)
			continue
		}
	}

	return nil
}

func CreateOrUpdateSyntaxFlow(hash string, i any) error {
	db := consts.GetGormProfileDatabase()
	var rule schema.SyntaxFlowRule
	if err := db.Where("hash = ?", hash).First(&rule).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(i).Error
		}
		return err
	}

	return db.Model(&rule).Updates(i).Error
}

func DeleteRuleByRuleName(name string) error {
	db := consts.GetGormProfileDatabase()
	return db.Where("rule_name = ?", name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

func DeleteRuleByTitle(name string) error {
	db := consts.GetGormProfileDatabase()
	return db.Where("title = ? or title_zh = ?", name, name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error
}

func ImportRuleWithoutValid(ruleName string, content string) error {
	var language consts.Language
	languageRaw, _, _ := strings.Cut(ruleName, "-")
	switch strings.TrimSpace(strings.ToLower(languageRaw)) {
	case "yak", "yaklang":
		language = consts.Yak
	case "java":
		language = consts.JAVA
	case "php":
		language = consts.PHP
	case "js", "es", "javascript", "ecmascript", "nodejs", "node", "node.js":
		language = consts.JS
	}

	var ruleType schema.SyntaxFlowRuleType
	switch path.Ext(ruleName) {
	case ".sf", ".syntaxflow":
		ruleType = schema.SFR_RULE_TYPE_SF
	default:
		log.Errorf("invalid rule type: %v is not supported yet, treat it as syntaxflow(.sf, .syntaxflow)", ruleName)
	}

	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(content)
	if err != nil {
		return err
	}

	rule := &schema.SyntaxFlowRule{
		Language:    string(language),
		Title:       frame.Title,
		RuleName:    ruleName,
		Description: frame.Description,
		Type:        ruleType,
		Content:     content,
		Purpose:     schema.ValidPurpose(frame.Purpose),
	}

	if frame.AllowIncluded != "" {
		rule.AllowIncluded = true
		rule.IncludedName = frame.AllowIncluded
		rule.Title = frame.AllowIncluded
	}
	err = CreateOrUpdateSyntaxFlow(rule.CalcHash(), rule)
	if err != nil {
		return utils.Wrap(err, "create or update syntax flow rule error")
	}
	return nil
}

func ImportValidRule(system fi.FileSystem, ruleName string, content string) error {
	var language consts.Language
	languageRaw, _, _ := strings.Cut(ruleName, "-")
	switch strings.TrimSpace(strings.ToLower(languageRaw)) {
	case "yak", "yaklang":
		language = consts.Yak
	case "java":
		language = consts.JAVA
	case "php":
		language = consts.PHP
	case "js", "es", "javascript", "ecmascript", "nodejs", "node", "node.js":
		language = consts.JS
	}

	var ruleType schema.SyntaxFlowRuleType
	switch path.Ext(ruleName) {
	case ".sf", ".syntaxflow":
		ruleType = schema.SFR_RULE_TYPE_SF
	default:
		log.Errorf("invalid rule type: %v is not supported yet", ruleName)
		return nil
	}

	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(content)
	if err != nil {
		return err
	}

	rule := &schema.SyntaxFlowRule{
		Language:    string(language),
		Title:       frame.Title,
		Description: frame.Description,
		Type:        ruleType,
		Content:     content,
		Purpose:     schema.ValidPurpose(frame.Purpose),
	}

	if frame.AllowIncluded != "" {
		rule.AllowIncluded = true
		rule.IncludedName = frame.AllowIncluded
		rule.Title = frame.AllowIncluded
	}

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

	err = CreateOrUpdateSyntaxFlow(rule.CalcHash(), rule)
	if err != nil {
		return utils.Wrap(err, "create or update syntax flow rule error")
	}
	return nil
}

var valid func(rule *schema.SyntaxFlowRule) error
var registerOnce = new(sync.Once)

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

func YieldSyntaxFlowRules(db *gorm.DB, ctx context.Context) chan *schema.SyntaxFlowRule {
	outC := make(chan *schema.SyntaxFlowRule)
	go func() {
		defer close(outC)

		var page = 1
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
