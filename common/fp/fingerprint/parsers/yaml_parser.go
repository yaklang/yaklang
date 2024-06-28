package parsers

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/utils"
)

func ParseYamlRule(ruleContent string) ([][]*rule.OpCode, error) {
	rules, err := webfingerprint.ParseWebFingerprintRules([]byte(ruleContent))
	if err != nil {
		return nil, errors.Errorf("parse wappalyzer rules failed: %s", err)
	}
	rs, err := ConvertOldYamlWebRuleToGeneralRule(rules)
	if err != nil {
		return nil, err
	}
	codes := [][]*rule.OpCode{}
	for _, r := range rs {
		ops := r.ToOpCodes()
		if len(ops) != 0 {
			codes = append(codes, ops)
		}
	}
	return codes, nil
}
func ConvertOldYamlWebRuleToGeneralRule(rules []*webfingerprint.WebRule) ([]*rule.FingerPrintRule, error) {
	convertToMap := func(o *webfingerprint.CPE) *rule.CPE {
		return &rule.CPE{
			Part:     o.Part,
			Vendor:   o.Vendor,
			Product:  o.Product,
			Version:  o.Version,
			Update:   o.Update,
			Edition:  o.Edition,
			Language: o.Language,
		}
	}
	convertRegexpRule := func(keyword *webfingerprint.KeywordMatcher) *rule.FingerPrintRule {
		r := rule.NewEmptyFingerPrintRule()
		r.Method = "regexp"
		r.MatchParam = &rule.MatchMethodParam{
			RegexpPattern: keyword.Regexp,
			Keyword:       keyword,
			Info:          convertToMap(&keyword.CPE),
		}
		return r
	}
	//var res []*rule.FingerPrintRule
	newComplexRule := func(rules []*rule.FingerPrintRule, condition string) *rule.FingerPrintRule {
		r := rule.NewEmptyFingerPrintRule()
		r.Method = "complex"
		r.MatchParam = &rule.MatchMethodParam{
			SubRules:  rules,
			Condition: condition,
		}
		r.MatchParam.Info = utils.GetLastElement(rules).MatchParam.Info
		return r
	}
	var convertRule func(webRule *webfingerprint.WebRule) *rule.FingerPrintRule
	convertRule = func(webRule *webfingerprint.WebRule) *rule.FingerPrintRule {
		var convertedWebRules []*rule.FingerPrintRule
		for _, method := range webRule.Methods {
			var methodRules []*rule.FingerPrintRule
			for _, keyword := range method.Keywords {
				r := convertRegexpRule(keyword)
				methodRules = append(methodRules, r)
			}
			for _, header := range method.HTTPHeaders {
				r := rule.NewEmptyFingerPrintRule()
				r.Method = "http_header"
				r.MatchParam = &rule.MatchMethodParam{
					HeaderKey:       header.HeaderName,
					HeaderMatchRule: convertRegexpRule(&header.HeaderValue),
				}
				r.MatchParam.Info = r.MatchParam.HeaderMatchRule.MatchParam.Info
				methodRules = append(methodRules, r)
			}
			for _, md5 := range method.MD5s {
				r := rule.NewEmptyFingerPrintRule()
				r.Method = "md5"
				r.MatchParam = &rule.MatchMethodParam{
					Md5:  md5.MD5,
					Info: convertToMap(&md5.CPE),
				}
				methodRules = append(methodRules, r)
			}
			if method.Condition != "" {
				if len(methodRules) != 0 {
					r := newComplexRule(methodRules, method.Condition)
					methodRules = []*rule.FingerPrintRule{r}
				}
			}
			convertedWebRules = append(convertedWebRules, methodRules...)
		}
		var generalWebRule *rule.FingerPrintRule
		if len(convertedWebRules) > 1 {
			generalWebRule = newComplexRule(convertedWebRules, "or")
		} else {
			if len(convertedWebRules) == 1 {
				generalWebRule = convertedWebRules[0]
			}
		}
		if webRule.NextStep != nil {
			nextRule := convertRule(webRule.NextStep)
			generalWebRule = newComplexRule([]*rule.FingerPrintRule{generalWebRule, nextRule}, "and")
		}
		generalWebRule.WebPath = webRule.Path
		return generalWebRule
	}
	generalRules := []*rule.FingerPrintRule{}
	for _, webRule := range rules {
		generalRules = append(generalRules, convertRule(webRule))
	}
	return generalRules, nil
}
