package fingerprint

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
)

var MethodGetterMap = map[string]func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error){}

func RegisterMatchMethod(name string, getter func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error)) {
	MethodGetterMap[name] = getter
}
func init() {
	RegisterMatchMethod("regexp", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
		pattern := params.RegexpPattern
		if pattern == "" {
			return func(data []byte) (bool, error) {
				return true, nil
			}, nil
		}
		return func(data []byte) (bool, error) {
			rePattern, ok := matcher.regexpCache[pattern]
			if !ok {
				var err error
				rePattern, err = regexp.Compile(pattern)
				if err != nil {
					return false, err
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
				return true, nil
			}
			return false, nil
		}, nil
	})
	RegisterMatchMethod("complex", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
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
			for i, f := range subMethods {
				res, err := f(data)
				if err != nil {
					return false, err
				}
				if params.Condition == "or" && res {
					return true, nil
				}
				if params.Condition == "and" && !res {
					return false, nil
				}
				params.Info = params.SubRules[i].MatchParam.Info
				preOk = res
			}
			return preOk, nil
		}, nil
	})
	RegisterMatchMethod("http_header", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
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
				if ok {
					params.Info = params.HeaderMatchRule.MatchParam.Info
					return true, nil
				}
			}
			return false, nil
		}, nil
	})
	RegisterMatchMethod("md5", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
		return func(data []byte) (bool, error) {
			return params.Md5 == codec.Md5(data), nil
		}, nil
	})
	RegisterMatchMethod("exp", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
		return func(data []byte) (bool, error) {
			switch params.Op {
			case "=":
				ps := params.Params
				if len(ps) != 2 {
					return false, errors.New("number of params must be 2")
				}
				strParams := []string{}
				for _, p := range ps {
					strParam, ok := p.(string)
					if !ok {
						return false, errors.New("op `=` param type must be string")
					}
					strParams = append(strParams, strParam)
				}
				varName := strParams[0]
				varValue := ""
				matchValue := strParams[1]
				if strings.Contains(matchValue, "/AV732E/setup.exe") {
					print()
				}
				header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
				switch varName {
				case "header":
					varValue = header
				case "title":
					varValue = utils.ExtractTitleFromHTMLTitle(string(data), "")
				case "body":
					varValue = string(body)
				default:
					return false, errors.New("not support var: " + varName)
				}
				return strings.Contains(varValue, matchValue), nil
			default:
				return false, fmt.Errorf("unsupported op: %s", params.Op)
			}
		}, nil
	})
}
