package httptpl

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func NewMatcherFromGRPCModel(m *ypb.HTTPResponseMatcher) *YakMatcher {
	return &YakMatcher{
		MatcherType:         m.GetMatcherType(),
		ExprType:            m.GetExprType(),
		Scope:               m.GetScope(),
		Condition:           m.GetCondition(),
		Group:               m.GetGroup(),
		GroupEncoding:       m.GetGroupEncoding(),
		Negative:            m.GetNegative(),
		SubMatcherCondition: m.GetSubMatcherCondition(),
		SubMatchers:         funk.Map(m.GetSubMatchers(), NewMatcherFromGRPCModel).([]*YakMatcher),
	}
}

type YakMatcher struct {
	// status
	// content_length
	// binary
	// word
	// regexp
	// expr
	Id          int // first request means 1 second request means 2
	MatcherType string
	/*
		nuclei-dsl
			all_headers
			status_code
			content_length
			body
			raw
	*/
	ExprType string

	// status
	// header
	// body
	// raw
	// interactsh_protocol
	Scope string

	// or
	// and
	Condition string

	Group         []string
	GroupEncoding string

	Negative bool

	// or / and
	SubMatcherCondition string
	SubMatchers         []*YakMatcher

	// record poc name / script name or some verbose
	TemplateName string
}

var matcherResponseCache = utils.NewTTLCache[string](1 * time.Minute)

func cacheHash(rsp []byte, location string) string {
	return utils.CalcSha1(rsp, location)
}

func (y *YakMatcher) ExecuteRawResponse(rsp []byte, vars map[string]interface{}, suf ...string) (bool, error) {
	return y.Execute(&RespForMatch{RawPacket: rsp}, vars, suf...)
}

func (y *YakMatcher) ExecuteRaw(rsp []byte, vars map[string]interface{}, suf ...string) (bool, error) {
	return y.ExecuteRawWithConfig(nil, rsp, vars, suf...)
}

func (y *YakMatcher) ExecuteRawWithConfig(config *Config, rsp []byte, vars map[string]interface{}, suf ...string) (bool, error) {
	if len(y.SubMatchers) > 0 {
		if strings.TrimSpace(strings.ToLower(y.SubMatcherCondition)) == "or" {
			for _, matcher := range y.SubMatchers {
				if b, _ := matcher.ExecuteRawWithConfig(config, rsp, vars, suf...); b {
					return true, nil
				}
			}
			return false, nil
		} else {
			for _, matcher := range y.SubMatchers {
				if b, _ := matcher.ExecuteRawWithConfig(config, rsp, vars, suf...); !b {
					return false, nil
				}
			}
			return true, nil
		}
	}

	if y.Negative {
		res, err := y.executeRaw(y.TemplateName, config, rsp, 0, vars, suf...)
		if err != nil {
			return false, err
		}
		return !res, err
	}
	return y.executeRaw(y.TemplateName, config, rsp, 0, vars, suf...)
}

type RespForMatch struct {
	RawPacket []byte
	Duration  float64
}

func (y *YakMatcher) Execute(rsp *RespForMatch, vars map[string]interface{}, suf ...string) (bool, error) {
	return y.ExecuteWithConfig(nil, rsp, vars, suf...)
}

func (y *YakMatcher) ExecuteWithConfig(config *Config, rsp *RespForMatch, vars map[string]interface{}, suf ...string) (bool, error) {
	if len(y.SubMatchers) > 0 {
		if strings.TrimSpace(strings.ToLower(y.SubMatcherCondition)) == "or" {
			for _, matcher := range y.SubMatchers {
				if b, _ := matcher.ExecuteWithConfig(config, rsp, vars, suf...); b {
					return true, nil
				}
			}
			return false, nil
		} else {
			for _, matcher := range y.SubMatchers {
				if b, _ := matcher.ExecuteWithConfig(config, rsp, vars, suf...); !b {
					return false, nil
				}
			}
			return true, nil
		}
	}

	if y.Negative {
		res, err := y.execute(config, rsp, vars, suf...)
		if err != nil {
			return false, err
		}
		return !res, err
	}
	return y.execute(config, rsp, vars, suf...)
}

func (y *YakMatcher) executeRaw(name string, config *Config, rsp []byte, duration float64, vars map[string]any, sufs ...string) (bool, error) {
	isExpr := false

	var interactsh_protocol = utils.InterfaceToString(vars["interactsh_protocol"])
	var interactsh_request = utils.InterfaceToString(vars["interactsh_request"])

	getMaterial := func() string {
		if isExpr {
			return string(rsp)
		}
		var material string
		scope := strings.ToLower(y.Scope)
		scopeHash := cacheHash(rsp, scope)

		material, ok := matcherResponseCache.Get(scopeHash)
		if !ok {
			switch scope {
			case "status", "status_code":
				material = utils.InterfaceToString(lowhttp.ExtractStatusCodeFromResponse(rsp))
			case "header":
				header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
				material = header
			case "body":
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
				material = string(body)
			case "interactsh_protocol", "oob_protocol":
				if interactsh_protocol != "" {
					material = interactsh_protocol
				} else {
					material = ""
					var oobTimeout float64
					if config == nil || config.OOBTimeout <= 0 {
						oobTimeout = 5
					}
					if config == nil {
						log.Errorf("oob feature need config is nil")
						return ""
					}
					var checkingInteractsh func(string, string, ...float64) (string, []byte)
					if config == nil || config.OOBRequireCheckingTrigger == nil {
						checkingInteractsh = CheckingDNSLogOOB // if not set, use default func try get
					} else {
						checkingInteractsh = config.OOBRequireCheckingTrigger
					}
					if checkingInteractsh != nil {
						token, ok := vars["reverse_dnslog_token"]
						if ok {
							material, _ = checkingInteractsh(strings.ToLower(fmt.Sprint(token)), config.RuntimeId, oobTimeout)
						}
					}
				}
			case "interactsh_request":
				if interactsh_request != "" {
					material = interactsh_request
				} else {
					material = ""
					var oobTimeout float64
					if config == nil || config.OOBTimeout <= 0 {
						oobTimeout = 5
					}
					if config == nil {
						log.Errorf("oob feature need config is nil")
						return ""
					}
					var checkingInteractsh func(string, string, ...float64) (string, []byte)
					if config == nil || config.OOBRequireCheckingTrigger == nil {
						checkingInteractsh = CheckingDNSLogOOB // if not set, use default func try get
					} else {
						checkingInteractsh = config.OOBRequireCheckingTrigger
					}
					if checkingInteractsh != nil {
						token, ok := vars["reverse_dnslog_token"]
						if ok {
							_, request := checkingInteractsh(strings.ToLower(fmt.Sprint(token)), config.RuntimeId, oobTimeout)
							material = string(request)
						}
					}
				}
			case "raw":
				fallthrough
			default:
				material = string(rsp)
			}
		}
		matcherResponseCache.Set(scopeHash, material)
		return material
	}

	matcherFunc := func(s string, sub string) bool {
		return strings.Contains(s, sub)
	}

	condition := strings.TrimSpace(strings.ToLower(y.Condition))
	switch y.MatcherType {
	case "status_code", "status":
		statusCode := lowhttp.ExtractStatusCodeFromResponse(rsp)
		if statusCode == 0 {
			return false, utils.Errorf("extract status code failed: %s", string(rsp))
		}
		ints := utils.ParseStringToInts(strings.Join(y.Group, ","))
		if len(ints) <= 0 {
			return false, nil
		}
		switch condition {
		case "and":
			for _, i := range ints {
				if i != statusCode {
					return false, nil
				}
			}
			return true, nil
		case "or":
			fallthrough
		default:
			for _, i := range ints {
				if i == statusCode {
					return true, nil
				}
			}
			return false, nil
		}
	case "size", "content_length", "content-length":
		log.Warnf("content-length is untrusted, you should avoid using content-length!")
		header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		_ = header
		contentLength := len(body)
		ints := utils.ParseStringToInts(strings.Join(y.Group, ","))
		if len(ints) <= 0 {
			return false, nil
		}
		switch strings.TrimSpace(strings.ToLower(y.Condition)) {
		case "and":
			for _, i := range ints {
				if i != contentLength {
					return false, nil
				}
			}
			return true, nil
		case "or":
			fallthrough
		default:
			for _, i := range ints {
				if i == contentLength {
					return true, nil
				}
			}
			return false, nil
		}
	case "binary":
		y.GroupEncoding = "hex"
		fallthrough
	case "word", "contains":
		matcherFunc = func(s string, sub string) bool {
			if vars == nil {
				return strings.Contains(s, sub)
			} else {
				if strings.Contains(sub, "{{") && strings.Contains(sub, "}}") {
					results, err := ExecNucleiTag(sub, vars)
					if err == nil {
						return strings.Contains(s, results)
					}
				}
			}
			return strings.Contains(s, sub)
		}
	case "regexp", "re", "regex":
		matcherFunc = func(s string, sub string) bool {
			result, err := regexp.MatchString(sub, s)
			if err != nil {
				log.Errorf("[%v] regexp match failed: %s, origin regex: %v", name, err, sub)
				return false
			}
			return result
		}
	case "expr", "dsl", "cel":
		isExpr = true
		switch y.ExprType {
		case "nuclei-dsl", "nuclei":
			dslEngine := NewNucleiDSLYakSandbox()
			matcherFunc = func(fullResponse string, sub string) bool {
				loadVars := LoadVarFromRawResponse(rsp, duration, sufs...)
				// 加载 resp 中的变量
				for k, v := range vars { // 合并若有重名以 vars 为准
					loadVars[k] = v
				}

				result, err := dslEngine.ExecuteAsBool(sub, loadVars)
				if err != nil {
					log.Errorf("[%v] dsl engine execute as bool failed: %s", name, err)
					return false
				}
				return result
			}
		case "xray-cel":
			return false, utils.Errorf("xray-cel is not supported")
		default:
			return false, utils.Errorf("unknown expr type: %s", y.ExprType)
		}
	default:
		return false, utils.Errorf("unknown matcher type: %s", y.MatcherType)
	}

	material := getMaterial()
	var groups []string
	for _, wordRaw := range y.Group {
		word := wordRaw
		switch strings.TrimSpace(strings.ToLower(y.GroupEncoding)) {
		case "hex":
			raw, err := codec.DecodeHex(wordRaw)
			if err != nil {
				log.Warnf("decode yak matcher hex failed: %s", err)
				continue
			}
			word = string(raw)
		case "base64":
			raw, err := codec.DecodeBase64(wordRaw)
			if err != nil {
				log.Warnf("decode yak matcher base64 failed: %s", err)
				continue
			}
			word = string(raw)
		}
		groups = append(groups, word)
	}

	switch condition {
	case "and":
		for _, word := range groups {
			if !matcherFunc(material, word) {
				return false, nil
			}
		}
		return true, nil
	case "or":
		fallthrough
	default:
		for _, word := range groups {
			if matcherFunc(material, word) {
				return true, nil
			}
		}
		return false, nil
	}
}

func (y *YakMatcher) execute(config *Config, rspIns *RespForMatch, vars map[string]interface{}, sufs ...string) (bool, error) {
	rsp := utils.CopyBytes(rspIns.RawPacket)
	duration := rspIns.Duration
	return y.executeRaw(y.TemplateName, config, rsp, duration, vars, sufs...)
}
