package httptpl

import (
	"strings"
	"time"

	"github.com/gobwas/glob"
	"github.com/samber/lo"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func NewMatcherSliceFromGRPCModel(m []*ypb.HTTPResponseMatcher) []*YakMatcher {
	return lo.Map(m, func(item *ypb.HTTPResponseMatcher, index int) *YakMatcher {
		itemMatcher := NewMatcherFromGRPCModel(item)
		return itemMatcher
	})
}

func NewMatcherFromGRPCModel(m *ypb.HTTPResponseMatcher) *YakMatcher {
	res := &YakMatcher{
		MatcherType:         m.GetMatcherType(),
		ExprType:            m.GetExprType(),
		Scope:               m.GetScope(),
		Condition:           m.GetCondition(),
		Group:               m.GetGroup(),
		GroupEncoding:       m.GetGroupEncoding(),
		Negative:            m.GetNegative(),
		SubMatcherCondition: m.GetSubMatcherCondition(),
		SubMatchers: lo.Map(m.GetSubMatchers(), func(item *ypb.HTTPResponseMatcher, index int) *YakMatcher {
			return NewMatcherFromGRPCModel(item)
		}),
	}
	return res
}

const (
	MATCHER_TYPE_STATUS_CODE = "status_code"
	MATCHER_TYPE_CL          = "content_length"
	MATCHER_TYPE_BIN         = "binary"
	MATCHER_TYPE_WORD        = "word"
	MATCHER_TYPE_REGEXP      = "regexp"
	MATCHER_TYPE_SUFFIX      = "suffix"
	MATCHER_TYPE_EXPR        = "expr"
	MATCHER_TYPE_GLOB        = "glob"
	MATCHER_TYPE_MIME        = "mime"
)

const (
	EXPR_TYPE_NUCLEI_DSL = "nuclei-dsl"
)

const (
	SCOPE_STATUS_CODE         = "status_code"
	SCOPE_HEADER              = "header"
	SCOPE_BODY                = "body"
	SCOPE_RAW                 = "raw"
	SCOPE_INTERACTSH_PROTOCOL = "interactsh_protocol"
	SCOPE_INTERACTSH_REQUEST  = "interactsh_request"
	SCOPE_REQUEST_HEADER      = "request_header"
	SCOPE_REQUEST_BODY        = "request_body"
	SCOPE_REQUEST_RAW         = "request_raw"
	SCOPE_REQUEST_URL         = "request_url"
)

const (
	GROUP_ENCODING_HEX    = "hex"
	GROUP_ENCODING_BASE64 = "base64"
)

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
	RawPacket     []byte
	Duration      float64
	RequestPacket []byte // optional request packet for request_* variables
	IsHttps       bool   // whether the request is HTTPS
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

func (y *YakMatcher) executeRaw(name string, config *Config, packet []byte, duration float64, vars map[string]any, sufs ...string) (bool, error) {
	return y.executeRawWithRequest(name, config, packet, nil, duration, false, vars, sufs...)
}

func (y *YakMatcher) executeRawWithRequest(name string, config *Config, packet []byte, reqPacket []byte, duration float64, isHttps bool, vars map[string]any, sufs ...string) (bool, error) {
	isExpr := false

	interactsh_protocol := utils.MapGetString(vars, "interactsh_protocol")
	interactsh_request := utils.MapGetString(vars, "interactsh_request")

	getMaterial := func() string {
		if isExpr {
			return string(packet)
		}
		var material string
		scope := strings.ToLower(y.Scope)
		scopeHash := cacheHash(packet, scope)

		material, ok := matcherResponseCache.Get(scopeHash)
		if !ok {
			switch scope {
			case SCOPE_STATUS_CODE, "status":
				material = utils.InterfaceToString(lowhttp.ExtractStatusCodeFromResponse(packet))
			case SCOPE_HEADER, "all_headers":
				header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
				material = header
			case SCOPE_BODY:
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
				material = string(body)
			case SCOPE_INTERACTSH_PROTOCOL, "oob_protocol":
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
						checkingInteractsh = func(token string, runtimeID string, timeout ...float64) (string, []byte) { // if not set, use default func try get
							return CheckingDNSLogOOB(token, runtimeID, name, timeout...)
						}
					} else {
						checkingInteractsh = config.OOBRequireCheckingTrigger
					}
					if checkingInteractsh != nil {
						token := utils.MapGetString(vars, "reverse_dnslog_token")
						if token != "" {
							material, _ = checkingInteractsh(strings.ToLower(token), config.RuntimeId, oobTimeout)
						}
					}
				}
			case SCOPE_INTERACTSH_REQUEST:
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
						checkingInteractsh = func(token string, runtimeID string, timeout ...float64) (string, []byte) { // if not set, use default func try get
							return CheckingDNSLogOOB(token, runtimeID, name, timeout...)
						}
					} else {
						checkingInteractsh = config.OOBRequireCheckingTrigger
					}
					if checkingInteractsh != nil {
						token := utils.MapGetString(vars, "reverse_dnslog_token")
						if token != "" {
							_, request := checkingInteractsh(strings.ToLower(token), config.RuntimeId, oobTimeout)
							material = string(request)
						}
					}
				}
			case SCOPE_REQUEST_HEADER:
				if len(reqPacket) > 0 {
					header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(reqPacket)
					material = header
				} else {
					material = ""
				}
			case SCOPE_REQUEST_BODY:
				if len(reqPacket) > 0 {
					_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(reqPacket)
					material = string(body)
				} else {
					material = ""
				}
			case SCOPE_REQUEST_RAW:
				if len(reqPacket) > 0 {
					material = string(reqPacket)
				} else {
					material = ""
				}
			case SCOPE_REQUEST_URL:
				if len(reqPacket) > 0 {
					if reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(reqPacket, isHttps); err == nil {
						material = reqUrl.String()
					} else {
						material = ""
					}
				} else {
					material = ""
				}
			case SCOPE_RAW:
				fallthrough
			default:
				material = string(packet)
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
	case MATCHER_TYPE_STATUS_CODE, "status":
		statusCode := lowhttp.ExtractStatusCodeFromResponse(packet)
		if statusCode == 0 {
			return false, utils.Errorf("extract status code failed: %s", string(packet))
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
	case MATCHER_TYPE_CL, "size", "content-length":
		log.Warnf("content-length is untrusted, you should avoid using content-length!")
		header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
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
	case MATCHER_TYPE_BIN:
		y.GroupEncoding = "hex"
		fallthrough
	case MATCHER_TYPE_WORD, "contains":
		matcherFunc = func(s string, sub string) bool {
			if vars == nil {
				return strings.Contains(s, sub)
			} else {
				if strings.Contains(sub, "{{") && strings.Contains(sub, "}}") {
					result, err := ExecNucleiDSL(sub, vars)
					if err == nil {
						return strings.Contains(s, toString(result))
					}
				}
			}
			return strings.Contains(s, sub)
		}
	case MATCHER_TYPE_SUFFIX:
		matcherFunc = strings.HasSuffix
	case MATCHER_TYPE_MIME:
		matcherFunc = utils.MIMEGlobRuleCheck
	case MATCHER_TYPE_REGEXP, "re", "regex":
		matcherFunc = func(s string, sub string) bool {
			regUtils := regexp_utils.DefaultYakRegexpManager.GetYakRegexp(sub)
			result, err := regUtils.MatchString(s)
			if err != nil {
				log.Errorf("[%v] regexp match failed: %s, origin regex: %v", name, err, sub)
				return false
			}
			return result
		}
	case MATCHER_TYPE_GLOB:
		matcherFunc = func(s string, sub string) bool {
			globRule, err := glob.Compile(sub)
			if err != nil {
				log.Errorf("[%v] glob match failed: %s, origin glob: %v", name, err, sub)
				return false
			}
			return globRule.Match(s)
		}
	case MATCHER_TYPE_EXPR, "dsl", "cel":
		isExpr = true
		switch y.ExprType {
		case EXPR_TYPE_NUCLEI_DSL, "nuclei":
			dslEngine := NewNucleiDSLYakSandbox()
			matcherFunc = func(fullResponse string, sub string) bool {
				loadVars := LoadVarFromRawResponseWithRequest(packet, reqPacket, duration, isHttps, sufs...)
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
		case GROUP_ENCODING_HEX:
			raw, err := codec.DecodeHex(wordRaw)
			if err != nil {
				log.Warnf("decode yak matcher hex failed: %s", err)
				continue
			}
			word = string(raw)
		case GROUP_ENCODING_BASE64:
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
	req := utils.CopyBytes(rspIns.RequestPacket)
	duration := rspIns.Duration
	isHttps := rspIns.IsHttps
	return y.executeRawWithRequest(y.TemplateName, config, rsp, req, duration, isHttps, vars, sufs...)
}
