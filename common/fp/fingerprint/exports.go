package fingerprint

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/schema"
)

func GetAllFingerprint() chan *schema.GeneralRule {
	db := consts.GetGormProfileDatabase()
	var allFingerprint []*schema.GeneralRule
	db.Model(&schema.GeneralRule{}).Find(&allFingerprint)
	ch := make(chan *schema.GeneralRule, len(allFingerprint))
	for _, fp := range allFingerprint {
		ch <- fp
	}
	close(ch)
	return ch
}

func MatchRspByRule(rsp []byte, rule any) bool {
	switch rule := rule.(type) {
	case *schema.GeneralRule:
		rules, _ := parsers.ParseExpRule(rule)
		matcher := NewMatcher()
		info := matcher.Match(context.Background(), rsp, rules)
		return len(info) > 0
	case string:
		fp := &schema.GeneralRule{
			MatchExpression: rule,
			CPE:             &schema.CPE{Product: uuid.New().String()},
		}
		return MatchRspByRule(rsp, fp)
	}
	return false
}

func MatchRsp(rsp []byte) []string {
	db := consts.GetGormProfileDatabase()
	var rules []*schema.GeneralRule
	db.Model(&schema.GeneralRule{}).Find(&rules)
	matched := []string{}
	for _, rule := range rules {
		if MatchRspByRule(rsp, rule) {
			matched = append(matched, rule.CPE.Product)
		}
	}
	return matched
}

var Exports = map[string]any{
	"MatchRspByRule":    MatchRspByRule,
	"MatchRsp":          MatchRsp,
	"GetAllFingerprint": GetAllFingerprint,
}
