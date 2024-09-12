package fingerprint

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule_resources"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"strings"
)

func LoadCPEFromWebfingerrintCPE(o *webfingerprint.CPE) *schema.CPE {
	return &schema.CPE{
		Part:     o.Part,
		Vendor:   o.Vendor,
		Product:  o.Product,
		Version:  o.Version,
		Update:   o.Update,
		Edition:  o.Edition,
		Language: o.Language,
	}
}

func LoadAllDefaultRules() (rules []*rule.FingerPrintRule) {
	for _, f := range []func() error{
		func() error {
			content, err := rule_resources.FS.ReadFile("exp_rule.txt")
			if err != nil {
				return err
			}
			ruleInfos := funk.Map(strings.Split(string(content), "\n"), func(s string) *schema.GeneralRule {
				splits := strings.Split(s, "\x00")
				return &schema.GeneralRule{MatchExpression: splits[1], CPE: &schema.CPE{Product: splits[0]}}
			})
			rs, err := parsers.ParseExpRule(ruleInfos.([]*schema.GeneralRule)...)
			if err != nil {
				return err
			}
			rules = append(rules, rs...)
			return nil
		},
		func() error {
			db := consts.GetGormProjectDatabase()
			var rs []*schema.GeneralRule
			db = db.Model(&schema.GeneralRule{}).Find(&rs)
			if db.Error != nil {
				return db.Error
			}
			codes, err := parsers.ParseExpRule(rs...)
			if err != nil {
				return err
			}
			rules = append(rules, codes...)
			return nil
		},
		func() error {
			for _, ruleFile := range []string{"custom.yml", "replenish.yml", "fingerprint-rules.yml"} {
				content, err := rule_resources.FS.ReadFile(ruleFile)
				if err != nil {
					return err
				}
				rs, err := parsers.ParseYamlRule(string(content))
				if err != nil {
					return err
				}
				rules = append(rules, rs...)
			}
			return nil
		},
	} {
		err := f()
		if err != nil {
			log.Error(err)
		}
	}
	return
}
