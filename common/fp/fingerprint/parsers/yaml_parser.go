package parsers

import (
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
)

func ParseYamlRule(ruleContent string) ([]*rule.FingerPrintRule, error) {
	rules, err := webfingerprint.LoadDefaultDataSource()
	if err != nil {
		return nil, err
	}
	convertToMap := func(o *webfingerprint.CPE) *rule.FingerprintInfo {
		return &rule.FingerprintInfo{
			CPE: &rule.CPE{
				Part:     o.Part,
				Vendor:   o.Vendor,
				Product:  o.Product,
				Version:  o.Version,
				Update:   o.Update,
				Edition:  o.Edition,
				Language: o.Language,
			},
		}
	}
	convertRegexpRule := func(keyword *webfingerprint.KeywordMatcher) *rule.FingerPrintRule {
		r := rule.NewEmptyFingerPrintRule()
		r.Method = "regexp"
		r.MatchParam = &rule.MatchMethodParam{
			RegexpPattern: keyword.Regexp,
		}
		r.Info = convertToMap(&keyword.CPE)
		return r
	}
	var res []*rule.FingerPrintRule
	for _, webRule := range rules {
		if webRule.Path != "" {

		} else {
			for _, method := range webRule.Methods {
				var methodRules []*rule.FingerPrintRule
				for _, keyword := range method.Keywords {
					methodRules = append(methodRules, convertRegexpRule(keyword))
				}
				for _, header := range method.HTTPHeaders {
					r := rule.NewEmptyFingerPrintRule()
					r.Method = "http_header"
					r.MatchParam = &rule.MatchMethodParam{
						HeaderKey:       header.HeaderName,
						HeaderMatchRule: convertRegexpRule(&header.HeaderValue),
					}
					r.Info = r.MatchParam.HeaderMatchRule.Info
					r.MatchParam.HeaderMatchRule.Info = nil
					methodRules = append(methodRules, r)
				}
				for _, md5 := range method.MD5s {
					r := rule.NewEmptyFingerPrintRule()
					r.Method = "md5"
					r.MatchParam = &rule.MatchMethodParam{
						HeaderKey: md5.MD5,
					}
					r.Info = convertToMap(&md5.CPE)
					methodRules = append(methodRules, r)
				}
				if method.Condition != "" {
					if len(methodRules) != 0 {
						r := rule.NewEmptyFingerPrintRule()
						r.Method = "complex"
						r.MatchParam = &rule.MatchMethodParam{
							SubRules:  methodRules,
							Condition: method.Condition,
						}
						r.Info = methodRules[0].Info
						for _, methodRule := range methodRules {
							methodRule.Info = nil
						}
					}
				} else {
					res = append(res, methodRules...)
				}
			}
		}
	}
	return res, err
}
