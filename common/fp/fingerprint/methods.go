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

func RegisterMethod(name string, getter func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error)) {
	MethodGetterMap[name] = getter
}
func init() {
	RegisterMethod("regexp", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
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
			return rePattern.Match(data), nil
		}, nil
	})
	RegisterMethod("complex", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
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
				ok, err := f(data)
				if err != nil {
					return false, err
				}
				if params.Condition == "or" && ok {
					return true, nil
				}
				if params.Condition == "and" && !ok {
					return false, nil
				}
				preOk = ok
			}
			return preOk, nil
		}, nil
	})
	RegisterMethod("http_header", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
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
					return true, nil
				}
			}
			return false, nil
		}, nil
	})
	RegisterMethod("md5", func(matcher *Matcher, params *rule.MatchMethodParam) (MatchFun, error) {
		return func(data []byte) (bool, error) {
			return params.Md5 == codec.Md5(data), nil
		}, nil
	})
}
