package fingerprint

import (
	"errors"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
)

var MethodGetterMap = map[string]func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error){}

func RegisterSimpleMatchMethod(name string, getter func(matcher *Matcher, params *rule.MatchMethodParam) (SimpleMatchFun, error)) {
	MethodGetterMap[name] = func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
		match, err := getter(matcher, params)
		if err != nil {
			return nil, err
		}
		return func(data []byte) (*rule.FingerprintInfo, error) {
			ok, err := match(data)
			if err != nil {
				return nil, err
			}
			if ok {
				return params.Info, nil
			}
			return nil, nil
		}, nil
	}
}
func RegisterMatchMethod(name string, getter func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error)) {
	MethodGetterMap[name] = getter
}
func init() {
	RegisterMatchMethod("regexp", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
		pattern := params.RegexpPattern
		if pattern == "" {
			return func(data []byte) (*rule.FingerprintInfo, error) {
				return params.Info, nil
			}, nil
		}
		return func(data []byte) (*rule.FingerprintInfo, error) {
			rePattern, ok := matcher.regexpCache[pattern]
			if !ok {
				var err error
				rePattern, err = regexp.Compile(pattern)
				if err != nil {
					return nil, err
				}
				matcher.regexpCache[pattern] = rePattern
			}
			if rePattern.Match(data) {
				if params.Info == nil {
					params.Info = &rule.FingerprintInfo{}
				}
				setByGroup := func(field *string, group []string, index int) {
					if index != 0 && index < len(group) {
						*field = group[index]
					}
				}
				for _, matchedStringGroup := range rePattern.FindAllStringSubmatch(string(data), 1) {
					setByGroup(&params.Info.CPE.Vendor, matchedStringGroup, params.Keyword.VendorIndex)
					setByGroup(&params.Info.CPE.Product, matchedStringGroup, params.Keyword.ProductIndex)
					setByGroup(&params.Info.CPE.Version, matchedStringGroup, params.Keyword.VersionIndex)
					setByGroup(&params.Info.CPE.Update, matchedStringGroup, params.Keyword.UpdateIndex)
					setByGroup(&params.Info.CPE.Edition, matchedStringGroup, params.Keyword.EditionIndex)
					setByGroup(&params.Info.CPE.Language, matchedStringGroup, params.Keyword.LanguageIndex)
				}
				return params.Info, nil
			}
			return nil, nil
		}, nil
	})
	RegisterSimpleMatchMethod("complex", func(matcher *Matcher, params *rule.MatchMethodParam) (SimpleMatchFun, error) {
		if !utils.StringArrayContains([]string{"and", "or"}, params.Condition) {
			return nil, errors.New("invalid condition")
		}
		var subMethods []MatchFun
		for _, subRule := range params.SubRules {
			m, err := matcher.LoadMethod(subRule.Method, subRule.MatchParam)
			if err != nil {
				return nil, err
			}
			subMethods = append(subMethods, m)
		}
		return func(data []byte) (bool, error) {
			var preOk bool
			for _, f := range subMethods {
				res, err := f(data)
				if err != nil {
					return false, err
				}
				if params.Condition == "or" && res != nil {
					return true, nil
				}
				if params.Condition == "and" && res == nil {
					return false, nil
				}
				params.Info = res
				preOk = res != nil
			}
			return preOk, nil
		}, nil
	})
	RegisterSimpleMatchMethod("http_header", func(matcher *Matcher, params *rule.MatchMethodParam) (SimpleMatchFun, error) {
		headerMatchMethod, err := matcher.LoadMethod(params.HeaderMatchRule.Method, params.HeaderMatchRule.MatchParam)
		if err != nil {
			return nil, err
		}
		return func(data []byte) (bool, error) {
			var vals []string
			lowhttp.SplitHTTPPacket(data, nil, nil, func(line string) string {
				if k, v := lowhttp.SplitHTTPHeader(line); k != "" {
					if strings.Contains(strings.ToLower(k), strings.ToLower(params.HeaderKey)) {
						vals = append(vals, v)
					}
				}
				return line
			})
			for _, val := range vals {
				ok, err := headerMatchMethod([]byte(val))
				if err != nil {
					return false, err
				}
				if ok != nil {
					params.Info = ok
					return true, nil
				}
			}
			return false, nil
		}, nil
	})
	RegisterSimpleMatchMethod("md5", func(matcher *Matcher, params *rule.MatchMethodParam) (SimpleMatchFun, error) {
		return func(data []byte) (bool, error) {
			return params.Md5 == codec.Md5(data), nil
		}, nil
	})
}
