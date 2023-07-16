package yakgrpc

import (
	"bytes"
	"fmt"
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type mitmContentReplaceRulesSortable []*ypb.MITMContentReplacer

func (a mitmContentReplaceRulesSortable) Len() int { // 重写 Len() 方法
	return len(a)
}
func (a mitmContentReplaceRulesSortable) Swap(i, j int) { // 重写 Swap() 方法
	a[i], a[j] = a[j], a[i]
}
func (a mitmContentReplaceRulesSortable) Less(i, j int) bool { // 重写 Less() 方法， 从大到小排序
	return a[i].Index < a[j].Index
}

func sortContentReplacer(i []*ypb.MITMContentReplacer) []*ypb.MITMContentReplacer {
	sort.Stable(mitmContentReplaceRulesSortable(i))
	return i
}

type mitmReplacer struct {
	// 所有正常启动的规则
	rules []*ypb.MITMContentReplacer
	// 所有规则，包含未启用
	allRules []*ypb.MITMContentReplacer

	// 已经启动的需要劫持修改数据包内容的规则
	_hijackingRules []*ypb.MITMContentReplacer
	// 已经启动的仅需要镜像劫持的规则
	_mirrorRules []*ypb.MITMContentReplacer

	autoSave func(...*ypb.MITMContentReplacer)

	_ruleRegexpCache *sync.Map
}

// getRule 获取不到规则就返回空，通过 sync.Map 缓存规则
func (m *mitmReplacer) getRule(r *ypb.MITMContentReplacer) *regexp2.Regexp {
	raw, ok := m._ruleRegexpCache.Load(r)
	if ok {
		return raw.(*regexp2.Regexp)
	}

	opt := regexp2.ECMAScript | regexp2.Multiline
	var rule string
	if strings.HasPrefix(r.Rule, "(?i)") {
		rule = r.Rule[4:]
		opt |= regexp2.IgnoreCase
	} else if strings.HasPrefix(r.Rule, `(?s)`) {
		rule = r.Rule[4:]
		opt |= regexp2.Singleline
	} else if strings.HasPrefix(r.Rule, `(?si)`) || strings.HasPrefix(r.Rule, `(?si)`) {
		rule = r.Rule[5:]
		opt |= regexp2.Singleline | regexp2.IgnoreCase
	} else {
		rule = r.Rule
	}

	re, err := regexp2.Compile(rule, regexp2.RegexOptions(opt))
	if err != nil {
		log.Infof("regexp2 compile %v failed: %s", rule, err)
		re, err = regexp2.Compile(regexp2.Escape(r.Rule), regexp2.RegexOptions(opt))
		if err != nil {
			log.Infof("regexp2 compile %v failed: %s", regexp2.Escape(r.Rule), err)
			m._ruleRegexpCache.Store(r, nil)
			return nil
		} else {
			m._ruleRegexpCache.Store(r, re)
			return re
		}
	}
	log.Infof("regexp cache store: %v", r.GetVerboseName())
	m._ruleRegexpCache.Store(r, re)
	return re
}

func NewMITMReplacer(initFunc ...func() []*ypb.MITMContentReplacer) *mitmReplacer {
	var rules []*ypb.MITMContentReplacer
	for _, i := range initFunc {
		rules = append(rules, i()...)
	}
	replacer := &mitmReplacer{
		_ruleRegexpCache: new(sync.Map),
	}
	replacer.LoadRules(rules)
	return replacer
}

// LoadRules Load replacer rules, cache regexp and filtered rules
func (m *mitmReplacer) LoadRules(rules []*ypb.MITMContentReplacer) {
	m._ruleRegexpCache = new(sync.Map)
	m._hijackingRules = nil
	m._mirrorRules = nil
	m.allRules = sortContentReplacer(rules)
	enabledRules := funk.Filter(rules, func(i *ypb.MITMContentReplacer) bool {
		if i.Rule == "" {
			return false
		}

		if i.Disabled {
			log.Infof("rule: %v is disabled", i.VerboseName)
			return false
		}

		// 缓存
		raw := m.getRule(i)
		if raw == nil {
			log.Infof("rule: %v is disabled(cannot compiled): %v", i.VerboseName, i.Rule)
			return false
		}
		log.Infof("rule: %v is enabled", raw.String())

		if i.GetNoReplace() {
			// mirror rules
			log.Infof("load mirror rules; %s", i.VerboseName)
			m._mirrorRules = append(m._mirrorRules, i)
		} else {
			log.Infof("load hijacked rules; %s", i.VerboseName)
			m._hijackingRules = append(m._hijackingRules, i)
		}

		return true
	}).([]*ypb.MITMContentReplacer)
	m.rules = sortContentReplacer(enabledRules)
	m._mirrorRules = sortContentReplacer(m._mirrorRules)
	m._hijackingRules = sortContentReplacer(m._hijackingRules)
}

func (m *mitmReplacer) AutoSaveCallback(f func(...*ypb.MITMContentReplacer)) {
	m.autoSave = f
}

func (m *mitmReplacer) SetRules(content ...*ypb.MITMContentReplacer) {
	m.LoadRules(content)
	if m.autoSave != nil {
		m.autoSave(sortContentReplacer(m.allRules)...)
	}
}

func (m *mitmReplacer) ClearRules() {
	m.LoadRules(nil)
	if m.autoSave != nil {
		m.autoSave()
	}
}

// GetRules 获取已经缓存好的规则们
func (m *mitmReplacer) GetRules() []*ypb.MITMContentReplacer {
	return m.allRules
}

func (m *mitmReplacer) GetEnabledRules() []*ypb.MITMContentReplacer {
	return m.rules
}

func (m *mitmReplacer) GetMirrorRules() []*ypb.MITMContentReplacer {
	return m._mirrorRules
}

func (m *mitmReplacer) GetHijackingRules() []*ypb.MITMContentReplacer {
	return m._hijackingRules
}

func stringForSettingColor(s string, extraTag []string, flow *yakit.HTTPFlow) {
	flow.AddTag(extraTag...)

	log.Debugf("set color[%v] for %v", s, flow.Url)
	switch strings.ToLower(s) {
	case "red":
		flow.Red()
	case "green":
		flow.Green()
	case "blue":
		flow.Blue()
	case "yellow":
		flow.Yellow()
	case "orange":
		flow.Orange()
	case "purple":
		flow.Purple()
	case "cyan":
		flow.Cyan()
	case "grey":
		flow.Grey()
	default:
		flow.Red()
	}
}

func (m *mitmReplacer) hookColor(request, response []byte, req *http.Request, flow *yakit.HTTPFlow) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("colorize failed: %v", strconv.Quote(string(request)))
		}
	}()

	if req.Context() != nil {
		if v := req.Context().Value(REQUEST_CONTEXT_KEY_MatchedRules); v != nil {
			if v1, ok := v.(*[]*ypb.MITMContentReplacer); ok && len(*v1) > 0 {
				stringForSettingColor((*v1)[0].Color, (*v1)[0].ExtraTag, flow)
				return
			}
		}
	}
	if m == nil {
		return
	}

	for _, rule := range m.GetMirrorRules() {
		r := m.getRule(rule)
		if rule.EnableForRequest {
			if r != nil {
				if match, err := r.FindStringMatch(string(request)); err == nil && match != nil {
					stringForSettingColor(rule.Color, rule.ExtraTag, flow)
					for {
						if match.GroupCount() > 1 {
							yakit.SaveExtractedDataFromHTTPFlow(
								consts.GetGormProjectDatabase(),
								flow.CalcHash(), rule.VerboseName,
								match.GroupByNumber(1).String(),
								r.String(),
							)
						} else {
							yakit.SaveExtractedDataFromHTTPFlow(
								consts.GetGormProjectDatabase(),
								flow.CalcHash(), rule.VerboseName,
								match.String(),
								r.String(),
							)
						}
						match, err = r.FindNextMatch(match)
						if err != nil || match == nil {
							break
						}
					}
					// return
				}
			}
			//continue
		}

		if rule.EnableForResponse {
			if r != nil {
				if match, err := r.FindStringMatch(string(response)); err == nil && match != nil {
					stringForSettingColor(rule.Color, rule.ExtraTag, flow)
					for {
						if match.GroupCount() > 0 {
							var extractData string
							extractGroup := match.GroupByNumber(1)
							if extractGroup != nil {
								extractData = extractGroup.String()
							}
							if extractData != "" {
								yakit.SaveExtractedDataFromHTTPFlow(
									consts.GetGormProjectDatabase(),
									flow.CalcHash(), rule.VerboseName,
									extractData, r.String(),
								)
							}
						} else {
							if ret := match.String(); ret != "" {
								yakit.SaveExtractedDataFromHTTPFlow(
									consts.GetGormProjectDatabase(),
									flow.CalcHash(), rule.VerboseName,
									ret, r.String(),
								)
							}
						}
						match, err = r.FindNextMatch(match)
						if err != nil || match == nil {
							break
						}
					}
					// return
				}
			}
			//continue
		}

	}
	return
}

func (m *mitmReplacer) haveRules() bool {
	return m.rules != nil
}

func (m *mitmReplacer) haveHijackingRules() bool {
	return m._hijackingRules != nil
}

func (m *mitmReplacer) replaceHTTPHeader(rule *ypb.MITMContentReplacer, headerMerged string, bodyMerged []byte, isRequest bool) (string, bool /*matched*/) {
	r := m.getRule(rule)
	if r == nil {
		return headerMerged, false
	}

	if !(isRequest && (rule.ExtraHeaders != nil || rule.ExtraCookies != nil)) {
		return headerMerged, false
	}

	/* 匹配规则 */
	if ok1, _ := r.MatchString(headerMerged); !ok1 {
		if ok2, _ := r.MatchString(string(bodyMerged)); !ok2 {
			return headerMerged, false
		}
	}

	var buf bytes.Buffer
	var cookies []*http.Cookie
	var keyHeader = make(map[string]*ypb.HTTPHeader)
	for _, v := range rule.ExtraHeaders {
		keyHeader[v.Header] = v
	}
	cookieHeaderPlaceHolder := fmt.Sprintf("HEADER_PLACEHOLDER_%v", utils.RandStringBytes(20))
	lowhttp.SplitHTTPHeadersAndBodyFromPacketEx(
		[]byte(headerMerged), func(method string, requestUri string, proto string) error {
			buf.WriteString(fmt.Sprintf("%v %v %v", method, requestUri, proto))
			buf.WriteString(lowhttp.CRLF)
			return nil
		}, func(line string) {
			key, value := lowhttp.SplitHTTPHeader(line)
			if strings.ToLower(key) == "cookie" {
				if cookies == nil {
					buf.WriteString("Cookie: " + cookieHeaderPlaceHolder)
					buf.WriteString(lowhttp.CRLF)
				}
				cookies = append(cookies, lowhttp.ParseCookie(value)...)
			} else {
				i, ok := keyHeader[key]
				if ok {
					buf.WriteString(fmt.Sprintf("%v: %v", key, i.Value))
					delete(keyHeader, key)
				} else {
					buf.WriteString(fmt.Sprintf("%v: %v", key, value))
				}
				buf.WriteString(lowhttp.CRLF)
			}
		})
	for _, c := range rule.ExtraCookies {
		tc := &http.Cookie{
			Name:     c.Key,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Expires:  time.Unix(c.Expires, 0),
			MaxAge:   int(c.MaxAge),
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		}
		switch c.SameSiteMode {
		case "default":
			tc.SameSite = http.SameSiteDefaultMode
		case "lax":
			tc.SameSite = http.SameSiteLaxMode
		case "strict":
			tc.SameSite = http.SameSiteStrictMode
		case "none":
			tc.SameSite = http.SameSiteNoneMode
		default:
			if c.SameSiteMode != "" {
				log.Errorf("invalid same site mode: %s", c.SameSiteMode)
			}
		}
		cookies = append(cookies, tc)
	}
	for _, c := range keyHeader {
		buf.WriteString(fmt.Sprintf("%v: %v", c.Header, c.Value))
		buf.WriteString(lowhttp.CRLF)
	}
	buf.WriteString(lowhttp.CRLF)
	merged := strings.ReplaceAll(buf.String(), cookieHeaderPlaceHolder, lowhttp.MergeCookies(cookies...))
	return merged, true
}

func (m *mitmReplacer) replaceHeader(rule *ypb.MITMContentReplacer, headerMerged string, isRequest bool) (string /*matched*/, bool) {
	i := m.getRule(rule)
	if i == nil {
		return headerMerged, false
	}

	matched, _ := i.MatchString(headerMerged)
	if !matched {
		return headerMerged, false
	}

	merged, _ := i.Replace(headerMerged, rule.Result, 0, -1)
	if merged == "" {
		return headerMerged, false
	}
	return merged, true
}

func (m *mitmReplacer) replaceURIInHeader(rule *ypb.MITMContentReplacer, headerMerged string, isRequest bool) (string, bool) {
	i := m.getRule(rule)
	if i == nil {
		return headerMerged, false
	}

	if !isRequest {
		return headerMerged, false
	}

	var headers []string
	var matched bool
	lowhttp.SplitHTTPHeadersAndBodyFromPacketEx([]byte(headerMerged), func(method string, requestUri string, proto string) error {
		requestUriReplaced, err := i.Replace(requestUri, rule.Result, 0, -1)
		if err != nil {
			headers = append(headers, fmt.Sprintf("%v %v %v", method, requestUri, proto))
			return nil
		}
		if requestUriReplaced != requestUri {
			requestUri = requestUriReplaced
			matched = true
		}
		headers = append(headers, fmt.Sprintf("%v %v %v", method, requestUri, proto))
		return nil
	}, func(line string) {
		headers = append(headers, line)
	})
	return strings.Join(headers, "\r\n") + "\r\n", matched
}

func (m *mitmReplacer) replaceBody(rule *ypb.MITMContentReplacer, bodyMerged []byte) ([]byte, bool) {
	if bodyMerged == nil {
		return bodyMerged, false
	}

	i := m.getRule(rule)
	if i == nil {
		return bodyMerged, false
	}

	origin := string(bodyMerged)
	matched, _ := i.MatchString(origin)
	if !matched {
		return bodyMerged, false
	}

	merged, _ := i.Replace(origin, rule.Result, 0, -1)
	if merged == "" {
		return bodyMerged, false
	}
	return []byte(merged), true
}

func (m *mitmReplacer) hook(isRequest, isResponse bool, origin []byte, args ...any) ([]*ypb.MITMContentReplacer, []byte, bool) {
	var matchedRules []*ypb.MITMContentReplacer
	if m == nil {
		return matchedRules, origin, false
	}
	var rules []*ypb.MITMContentReplacer

	rules = m.GetHijackingRules()
	if len(rules) <= 0 {
		return matchedRules, origin, false
	}

	var originPacket = origin
	_ = originPacket
	if isResponse {
		originDecoded, _, err := lowhttp.FixHTTPResponse(origin)
		if err != nil {
			return matchedRules, origin, false
		}
		origin = originDecoded
	}
	chunked := false
	gzip := false
	var gzipHeader string
	var chunkedHeader string
	headerRaw, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(origin, func(line string) {
		key, value := lowhttp.SplitHTTPHeader(line)
		switch strings.ToLower(key) {
		case "transfer-encoding":
			if value == "chunked" {
				chunkedHeader = key
				chunked = true
			}
		case "content-encoding":
			if value == "gzip" {
				gzipHeader = key
				gzip = true
			}
		}
	})
	if chunked {
		unchunked, err := codec.HTTPChunkedDecode(body)
		if err == nil {
			body = unchunked
			headerRaw = string(lowhttp.DeleteHTTPPacketHeader([]byte(headerRaw), chunkedHeader))
		}
	}
	if gzip {
		ungzip, err := utils.GzipDeCompress(body)
		if err == nil {
			body = ungzip
			headerRaw = string(lowhttp.DeleteHTTPPacketHeader([]byte(headerRaw), gzipHeader))
		}
	}

	var bodyMerged = make([]byte, len(body))
	copy(bodyMerged, body)
	headerMerged := headerRaw
	if len(bodyMerged) <= 0 && headerMerged == "" {
		return matchedRules, origin, false
	}

	// 是否丢包
	dropPacket := false
	extraRepeat := false
	for _, rule := range rules {
		if rule.NoReplace {
			continue
		}
		if !((rule.EnableForRequest && isRequest) || (rule.EnableForResponse && isResponse) || rule.GetEnableForURI()) {
			continue
		}

		// 如果修改了 header，将不会修改其他的了
		// 这个优先级比较高，并且只对请求生效
		var modified bool
		if rule.ExtraHeaders != nil || rule.ExtraCookies != nil {
			if isRequest && rule.EnableForHeader {
				var matched bool
				headerMerged, matched = m.replaceHTTPHeader(rule, headerMerged, bodyMerged, isRequest)
				if matched {
					modified = true
				}
			}
		} else {
			var matched bool
			if rule.GetEnableForURI() && isRequest {
				// 如果是请求，需要判断是否匹配了 uri
				headerMerged, matched = m.replaceURIInHeader(rule, headerMerged, isRequest)
				if matched {
					modified = true
				}
			}

			if rule.EnableForBody {
				bodyMerged, matched = m.replaceBody(rule, bodyMerged)
				if matched {
					modified = true
				}
			}

			if rule.EnableForHeader {
				headerMerged, matched = m.replaceHeader(rule, headerMerged, isRequest)
				if matched {
					modified = true
				}
			}
		}
		if modified {
			if rule.GetDrop() {
				dropPacket = true
			}

			if rule.GetExtraRepeat() && isRequest {
				extraRepeat = true
			}

			matchedRules = append(matchedRules, rule)
		}
	}

	modifiedPacket := lowhttp.ReplaceHTTPPacketBody([]byte(headerMerged), bodyMerged, false)
	if extraRepeat && isRequest {
		var extraArgHttps bool
		if len(args) > 0 {
			extraArgHttps, _ = strconv.ParseBool(utils.InterfaceToString(args[0]))
		}
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("extraRepeat failed: %v", err)
				}
			}()
			log.Info("AUTO(EXTRA)-REPEAT FROM MITM")
			rsp, err := lowhttp.SendHTTPRequestWithRawPacketWithRedirectWithStateWithOptFullEx(
				lowhttp.WithPacket(modifiedPacket), lowhttp.WithHttps(extraArgHttps),
				lowhttp.WithTimeout(15*time.Second), lowhttp.WithRedirectTimes(3),
				lowhttp.WithSaveHTTPFlow(true), lowhttp.WithSource("mitm"),
			)
			if err != nil {
				log.Errorf("extraRepeat failed: %v", err)
			}
			_ = rsp
		}()
		return matchedRules, originPacket, dropPacket
	}

	return matchedRules, modifiedPacket, dropPacket
}
