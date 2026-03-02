package parsers

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func ParseYamlRule(ruleContent string) ([]*rule.FingerPrintRule, error) {
	rules, err := webfingerprint.ParseWebFingerprintRules([]byte(ruleContent))
	if err != nil {
		return nil, errors.Errorf("parse wappalyzer rules failed: %s", err)
	}
	rs, err := ConvertOldYamlWebRuleToGeneralRule(rules)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

type webRuleConverter struct{}

func (webRuleConverter) convertToSchemaCPE(o *webfingerprint.CPE) *schema.CPE {
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

func (c webRuleConverter) convertRegexpRule(keyword *webfingerprint.KeywordMatcher) *rule.FingerPrintRule {
	r := rule.NewEmptyFingerPrintRule()
	r.Method = "regexp"
	r.MatchParam = &rule.MatchMethodParam{
		RegexpPattern: keyword.Regexp,
		Keyword:       keyword,
		Info:          c.convertToSchemaCPE(&keyword.CPE),
	}
	return r
}

func (webRuleConverter) newComplexRule(rules []*rule.FingerPrintRule, condition string) *rule.FingerPrintRule {
	r := rule.NewEmptyFingerPrintRule()
	r.Method = "complex"
	r.MatchParam = &rule.MatchMethodParam{
		SubRules:  rules,
		Condition: condition,
	}
	r.MatchParam.Info = utils.GetLastElement(rules).MatchParam.Info
	return r
}

func (c webRuleConverter) convertWebRuleMethods(webRule *webfingerprint.WebRule) []*rule.FingerPrintRule {
	var converted []*rule.FingerPrintRule
	for _, method := range webRule.Methods {
		var methodRules []*rule.FingerPrintRule
		for _, keyword := range method.Keywords {
			methodRules = append(methodRules, c.convertRegexpRule(keyword))
		}
		for _, header := range method.HTTPHeaders {
			headerRule := rule.NewEmptyFingerPrintRule()
			headerRule.Method = "http_header"
			headerRule.MatchParam = &rule.MatchMethodParam{
				HeaderKey:       header.HeaderName,
				HeaderMatchRule: c.convertRegexpRule(&header.HeaderValue),
			}
			headerRule.MatchParam.Info = headerRule.MatchParam.HeaderMatchRule.MatchParam.Info
			methodRules = append(methodRules, headerRule)
		}
		for _, md5 := range method.MD5s {
			md5Rule := rule.NewEmptyFingerPrintRule()
			md5Rule.Method = "md5"
			md5Rule.MatchParam = &rule.MatchMethodParam{
				Md5:  md5.MD5,
				Info: c.convertToSchemaCPE(&md5.CPE),
			}
			methodRules = append(methodRules, md5Rule)
		}

		if method.Condition != "" && len(methodRules) > 0 {
			methodRules = []*rule.FingerPrintRule{c.newComplexRule(methodRules, method.Condition)}
		}
		converted = append(converted, methodRules...)
	}
	return converted
}

func (c webRuleConverter) convertWebRuleToOne(webRule *webfingerprint.WebRule) *rule.FingerPrintRule {
	methodRules := c.convertWebRuleMethods(webRule)
	if len(methodRules) == 0 {
		return nil
	}

	var root *rule.FingerPrintRule
	if len(methodRules) == 1 {
		root = methodRules[0]
	} else {
		root = c.newComplexRule(methodRules, "or")
	}

	if webRule.NextStep != nil {
		next := c.convertWebRuleToOne(webRule.NextStep)
		if next != nil {
			root = c.newComplexRule([]*rule.FingerPrintRule{root, next}, "and")
		}
	}
	root.WebPath = webRule.Path
	return root
}

func (c webRuleConverter) convertWebRule(webRule *webfingerprint.WebRule) []*rule.FingerPrintRule {
	// NextStep needs gating (any initial rule) AND (next step rule).
	if webRule.NextStep != nil {
		one := c.convertWebRuleToOne(webRule)
		if one == nil {
			return nil
		}
		return []*rule.FingerPrintRule{one}
	}

	// No NextStep: keep each matcher as its own rule to preserve per-matcher CPE.
	methodRules := c.convertWebRuleMethods(webRule)
	for _, r := range methodRules {
		r.WebPath = webRule.Path
	}
	return methodRules
}

func ConvertOldYamlWebRuleToGeneralRule(rules []*webfingerprint.WebRule) ([]*rule.FingerPrintRule, error) {
	converter := webRuleConverter{}
	var generalRules []*rule.FingerPrintRule
	for _, webRule := range rules {
		generalRules = append(generalRules, converter.convertWebRule(webRule)...)
	}
	return generalRules, nil
}
