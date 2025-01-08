package yakgrpc

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/samber/lo"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Rules []*MITMReplaceRule

func isMatchTimeout(err error) bool {
	return strings.Contains(err.Error(), "match timeout")
}

func NewRuleGroup(r ...*ypb.MITMContentReplacer) Rules {
	var ret Rules
	for _, i := range r {
		ret = append(ret, &MITMReplaceRule{
			MITMContentReplacer: i,
		})
	}
	return ret
}

func (a Rules) MITMContentReplacers() []*ypb.MITMContentReplacer {
	var ret []*ypb.MITMContentReplacer
	for _, i := range a {
		ret = append(ret, i.MITMContentReplacer)
	}
	return ret
}

func (a Rules) Len() int { // 重写 Len() 方法
	return len(a)
}

func (a Rules) Swap(i, j int) { // 重写 Swap() 方法
	a[i], a[j] = a[j], a[i]
}

func (a Rules) Less(i, j int) bool { // 重写 Less() 方法， 从大到小排序
	return a[i].Index < a[j].Index
}

func FixPacket(packet []byte, isReq bool) (fixed []byte, use bool) {
	if !isReq {
		fixed, _, err := lowhttp.FixHTTPResponse(packet)
		if err != nil {
			return nil, false
		}
		packet = fixed
	}
	// 修复混杂的换行符，统一为\r\n
	packet = bytes.Replace(packet, []byte("\n"), []byte("\r\n"), -1)
	packet = bytes.Replace(packet, []byte("\r\r\n"), []byte("\r\n"), -1)

	return packet, true
}

type MITMReplaceRule struct {
	*ypb.MITMContentReplacer
	cache *regexp2.Regexp
}

func (r *MITMReplaceRule) compile() (*regexp2.Regexp, error) {
	if r.cache != nil {
		return r.cache, nil
	}

	rule, _, re, err := utils.Regexp2Compile(r.Rule)
	if err != nil {
		log.Debugf("regexp2 compile %v failed: %s", rule, err)
		_, _, re, err = utils.Regexp2Compile(regexp2.Escape(r.Rule))
		if err != nil {
			return nil, err
		} else {
			r.cache = re
			return re, nil
		}
	}
	r.cache = re
	return re, nil
}

func (m *MITMReplaceRule) matchRawSimple(rawPacket []byte) (bool, error) {
	r, err := m.compile()
	if err != nil {
		return false, err
	}
	return r.MatchString(string(rawPacket))
}

func (m *MITMReplaceRule) matchByPacketInfo(info *yakit.PacketInfo) ([]*yakit.MatchResult, error) {
	r, err := m.compile()
	if err != nil {
		return nil, err
	}
	if info.IsRequest && !m.EnableForRequest {
		return nil, nil // match nothing
	}
	if !info.IsRequest && !m.EnableForResponse {
		return nil, nil // match nothing
	}
	var items []*yakit.MatchInfo
	var enableURI, enableHeader, enableBody, enableEntire bool
	enableURI = m.EnableForURI
	enableHeader = m.EnableForHeader
	enableBody = m.EnableForBody
	if info.IsRequest {
		if m.EnableForHeader {
			enableURI = false
		}
	} else {
		enableURI = false
	}
	if enableHeader && enableBody {
		enableEntire = true
		enableHeader = false
		enableBody = false
	}
	if enableURI {
		items = append(items, &yakit.MatchInfo{
			[]byte(info.RequestURI), len(info.Method) + 1,
		})
	}
	if enableHeader {
		items = append(items, &yakit.MatchInfo{
			[]byte(info.HeaderRaw), 0,
		})
	}
	if enableBody {
		items = append(items, &yakit.MatchInfo{info.BodyRaw, len(info.HeaderRaw)})
	}
	if enableEntire {
		items = append(items, &yakit.MatchInfo{info.Raw, 0})
	}
	var res []*yakit.MatchResult
	for _, item := range items {
		match, err := r.FindStringMatch(string(item.Raw))
		if err != nil {
			return nil, err
		}
		if match == nil {
			continue
		}
		var ret string
		for ; err == nil && match != nil; match, err = r.FindNextMatch(match) {
			if match.GroupCount() > 1 {
				extractGroup := match.GroupByNumber(1)
				if extractGroup != nil {
					ret = extractGroup.String()
				}
			} else {
				ret = match.String()
			}
			if ret == "" {
				continue
			}
			res = append(res, &yakit.MatchResult{
				Match:          match,
				IsMatchRequest: info.IsRequest,
				MatchInfo:      item,
			})
		}
	}
	return res, nil
}

func (m *MITMReplaceRule) splitPacket(packet []byte) (*yakit.PacketInfo, error) {
	info := &yakit.PacketInfo{Raw: packet}
	headerRaw, bodyRaw := lowhttp.SplitHTTPHeadersAndBodyFromPacketEx(
		packet, func(method string, requestUri string, proto string) error {
			info.RequestURI = requestUri
			info.Method = method
			info.Proto = proto
			return nil
		}, func(line string) {
			key, value := lowhttp.SplitHTTPHeader(line)
			info.Headers = append(info.Headers, [2]string{key, value})
			switch strings.ToLower(key) {
			case "transfer-encoding":
				if utils.IContains(value, "chunked") {
					info.ChunkedHeader = key
				}
			case "content-encoding":
				if value == "gzip" {
					info.GzipHeader = key
				}
			case "cookie":
				info.Cookies = append(info.Cookies, lowhttp.ParseCookie("cookie", value)...)
			}
		})
	if info.ChunkedHeader != "" {
		unchunked, err := codec.HTTPChunkedDecode(bodyRaw)
		if err == nil {
			bodyRaw = unchunked
			headerRaw = string(lowhttp.DeleteHTTPPacketHeader([]byte(headerRaw), info.ChunkedHeader))
		}
	}
	if info.GzipHeader != "" {
		ungzip, err := utils.GzipDeCompress(bodyRaw)
		if err == nil {
			bodyRaw = ungzip
			headerRaw = string(lowhttp.DeleteHTTPPacketHeader([]byte(headerRaw), info.ChunkedHeader))
		}
	}

	info.HeaderRaw = headerRaw
	info.BodyRaw = bodyRaw
	if strings.HasPrefix(info.Proto, "HTTP") {
		info.IsRequest = true
	}
	return info, nil
}

func (m *MITMReplaceRule) MatchPacket(packet []byte, isReq bool) (*yakit.PacketInfo, []*yakit.MatchResult, error) {
	fixed, ok := FixPacket(packet, isReq)
	if ok {
		packet = fixed
	}
	// parse http packet
	packetInfo, err := m.splitPacket(packet)
	if err != nil {
		return packetInfo, nil, err
	}
	packetInfo.IsRequest = isReq
	matched, err := m.matchByPacketInfo(packetInfo)
	return packetInfo, matched, err
}

// MatchAndReplacePacket match and replace package, return matched result and replaced package
func (m *MITMReplaceRule) MatchAndReplacePacket(packet []byte, isReq bool) ([]*yakit.MatchResult, []byte, error) {
	originPacket := packet // backup origin packet
	fixed, ok := FixPacket(packet, isReq)
	if ok {
		packet = fixed
	}

	// parse http packet
	packetInfo, err := m.splitPacket(packet)
	if err != nil {
		return nil, nil, err
	}

	packetInfo.IsRequest = isReq

	matched, err := m.matchByPacketInfo(packetInfo)
	if err != nil {
		return nil, nil, err
	}
	// if not matched, skip replace step
	if len(matched) <= 0 {
		return matched, originPacket, nil
	}

	replaceHeadersByKV := false
	if m.ExtraHeaders != nil || m.ExtraCookies != nil {
		replaceHeadersByKV = true
	}
	re, err := m.compile()
	if err != nil {
		return nil, nil, fmt.Errorf("compile rule failed: %v", err)
	}
	headerRaw := packetInfo.HeaderRaw
	bodyRaw := packetInfo.BodyRaw
	var newPacket []byte
	if replaceHeadersByKV {
		if !packetInfo.IsRequest {
			return nil, nil, errors.New("replace headers by kv only support request")
		}
		var buf bytes.Buffer
		// write first line
		buf.Write([]byte(fmt.Sprintf("%v %v %v", packetInfo.Method, packetInfo.RequestURI, packetInfo.Proto)))
		buf.WriteString(lowhttp.CRLF)
		var extCookies []*http.Cookie // cookie config covert to http.Cookie
		for _, c := range m.ExtraCookies {
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
			extCookies = append(extCookies, tc)
		}
		keyHeader := make(map[string]*ypb.HTTPHeader) // build map for index by key
		for _, v := range m.ExtraHeaders {
			keyHeader[v.Header] = v
		}

		// write headers
		setCookie := false
		for _, c := range packetInfo.Headers {
			key := c[0]
			val := c[1]
			if strings.ToLower(key) == "cookie" {
				key = "Cookie"
				val = lowhttp.MergeCookies(append(packetInfo.Cookies, extCookies...)...)
				setCookie = true
			}

			i, ok := keyHeader[key]
			if ok {
				buf.WriteString(fmt.Sprintf("%v: %v", key, i.Value))
				delete(keyHeader, key) // just replace once
			} else {
				buf.WriteString(fmt.Sprintf("%v: %v", key, val))
			}
			buf.WriteString(lowhttp.CRLF)
		}

		// is origin header not contains extheaders, append it
		for k, v := range keyHeader {
			buf.WriteString(fmt.Sprintf("%v: %v", k, v.Value))
			buf.WriteString(lowhttp.CRLF)
		}
		// is origin header not contains cookie, append it
		if len(extCookies) > 0 && !setCookie {
			buf.WriteString("Cookie: " + lowhttp.MergeCookies(extCookies...))
			buf.WriteString(lowhttp.CRLF)
		}
		buf.WriteString(lowhttp.CRLF)
		headerRaw = buf.String()
	} else {
		if m.EnableForHeader && m.EnableForBody {
			raw, err := re.Replace(string(lowhttp.ReplaceHTTPPacketBody([]byte(packetInfo.HeaderRaw), packetInfo.BodyRaw, false)), m.Result, -1, -1)
			if err != nil {
				return nil, nil, fmt.Errorf("replace packet failed: %v", err)
			}
			newPacket = []byte(raw)
		} else {
			uri := packetInfo.RequestURI
			if isReq && m.EnableForURI {
				uri, err = re.Replace(uri, m.Result, -1, -1)
				if err != nil {
					return nil, nil, fmt.Errorf("replace uri failed: %v", err)
				}
			}
			headerRaw = strings.Replace(headerRaw, packetInfo.RequestURI, uri, 1)
			if m.EnableForHeader {
				headerRaw, err = re.Replace(packetInfo.HeaderRaw, m.Result, -1, -1)
				if err != nil {
					return nil, nil, fmt.Errorf("replace header failed: %v", err)
				}
			}
			if m.EnableForBody {
				body, err := re.Replace(string(bodyRaw), m.Result, -1, -1)
				if err != nil {
					return nil, nil, fmt.Errorf("replace body failed: %v", err)
				}
				bodyRaw = []byte(body)
			}
		}
	}
	if newPacket != nil {
		return matched, newPacket, nil
	} else {
		modifiedPacket := lowhttp.ReplaceHTTPPacketBody([]byte(headerRaw), bodyRaw, false)
		return matched, modifiedPacket, nil
	}
}

func sortContentReplacer(i []*MITMReplaceRule) []*MITMReplaceRule {
	sort.Stable(Rules(i))
	return i
}

type mitmReplacer struct {
	// 所有正常启动的规则
	rules Rules
	// 所有规则，包含未启用
	allRules Rules

	// 已经启动的需要劫持修改数据包内容的规则
	_hijackingRules Rules
	// 已经启动的仅需要镜像劫持的规则
	_mirrorRules Rules

	autoSave func(...*MITMReplaceRule)

	_ruleRegexpCache *sync.Map

	wg *sync.WaitGroup
}

func (m *mitmReplacer) WaitTasks() {
	m.wg.Wait()
}

// getRule 获取不到规则就返回空，通过 sync.Map 缓存规则
func (m *mitmReplacer) getRule(r *ypb.MITMContentReplacer) *regexp2.Regexp {
	raw, ok := m._ruleRegexpCache.Load(r)
	if ok {
		return raw.(*regexp2.Regexp)
	}

	rule, _, re, err := utils.Regexp2Compile(r.Rule)
	if err != nil {
		log.Debugf("regexp2 compile %v failed: %s", rule, err)
		_, _, re, err = utils.Regexp2Compile(regexp2.Escape(r.Rule))
		if err != nil {
			log.Debugf("regexp2 compile %v failed: %s", regexp2.Escape(r.Rule), err)
			m._ruleRegexpCache.Store(r, nil)
			return nil
		} else {
			m._ruleRegexpCache.Store(r, re)
			return re
		}
	}
	log.Debugf("regexp cache store: %v", r.GetVerboseName())
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
		wg:               &sync.WaitGroup{},
	}
	replacer.LoadRules(rules)
	return replacer
}

// LoadRules Load replacer rules, cache regexp and filtered rules
func (m *mitmReplacer) LoadRules(ruleContents []*ypb.MITMContentReplacer) {
	var rules []*MITMReplaceRule
	for _, i := range ruleContents {
		rules = append(rules, &MITMReplaceRule{
			i,
			nil,
		})
	}
	m._ruleRegexpCache = new(sync.Map)
	m._hijackingRules = nil
	m._mirrorRules = nil
	m.allRules = sortContentReplacer(rules)
	enabledRules := funk.Filter(rules, func(i *MITMReplaceRule) bool {
		if i.Rule == "" {
			return false
		}

		if i.Disabled {
			log.Debugf("rule: %v is disabled", i.VerboseName)
			return false
		}

		// 缓存
		re, err := i.compile()
		if err != nil {
			log.Debugf("rule: %v(%v) is disabled(cannot compiled): %v", i.VerboseName, i.Rule, err)
			return false
		}
		log.Debugf("rule: %v is enabled", re.String())

		if i.GetNoReplace() {
			// mirror rules
			log.Debugf("load mirror rules; %s", i.VerboseName)
			m._mirrorRules = append(m._mirrorRules, i)
		} else {
			log.Debugf("load hijacked rules; %s", i.VerboseName)
			m._hijackingRules = append(m._hijackingRules, i)
		}

		return true
	}).([]*MITMReplaceRule)
	m.rules = sortContentReplacer(enabledRules)
	m._mirrorRules = sortContentReplacer(m._mirrorRules)
	m._hijackingRules = sortContentReplacer(m._hijackingRules)
}

func (m *mitmReplacer) AutoSaveCallback(f func(...*MITMReplaceRule)) {
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
	return m.allRules.MITMContentReplacers()
}

func (m *mitmReplacer) GetEnabledRules() []*ypb.MITMContentReplacer {
	return m.rules.MITMContentReplacers()
}

func (m *mitmReplacer) GetMirrorRules() []*ypb.MITMContentReplacer {
	return m._mirrorRules.MITMContentReplacers()
}

func (m *mitmReplacer) GetHijackingRules() []*ypb.MITMContentReplacer {
	return m._hijackingRules.MITMContentReplacers()
}

type ColorFlow interface {
	Red()
	Green()
	Blue()
	Yellow()
	Orange()
	Purple()
	Cyan()
	Grey()
}

func stringForSettingColor(s []string, flow ColorFlow) {
	for _, c := range lo.Union(s) {
		switch strings.ToLower(c) {
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
		}
	}
	return
}

func (m *mitmReplacer) hookColorWs(rawPacket []byte, flow *schema.WebsocketFlow) {
	if m == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("colorize failed: %v", strconv.Quote(string(rawPacket)))
		}
	}()

	var (
		colorName []string
		tagNames  []string
	)

	defer func() {
		stringForSettingColor(colorName, flow)
		flow.AddTag(tagNames...)
	}()

	for _, rule := range m._mirrorRules {
		if !rule.EnableForRequest && !rule.EnableForResponse {
			continue
		}

		match, err := rule.matchRawSimple(rawPacket)
		if err != nil {
			log.Errorf("match package failed: %v", err)
			continue
		}

		if !match {
			continue
		}

		if rule.Color != "" {
			colorName = append(colorName, rule.Color)
		}
		tagNames = append(tagNames, rule.ExtraTag...) // merge tag name
	}
}

func (m *mitmReplacer) hookColor(request, response []byte, req *http.Request, flow *schema.HTTPFlow) []*schema.ExtractedData {
	if m == nil {
		return nil
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("colorize failed: %v", strconv.Quote(string(request)))
		}
	}()

	var (
		colorName []string
		tagNames  []string
		extracted []*schema.ExtractedData
		err       error
	)

	defer func() {
		stringForSettingColor(colorName, flow)
		flow.AddTag(tagNames...)
	}()

	for _, rule := range m._mirrorRules {
		matchResults := make([]*yakit.MatchResult, 0)
		newMatchResults := make([]*yakit.MatchResult, 0)
		if !rule.EnableForRequest && !rule.EnableForResponse {
			continue
		}

		if rule.EffectiveURL != "" {
			yakRegexp := regexp_utils.DefaultYakRegexpManager.GetYakRegexp(rule.EffectiveURL)
			matchString, err := yakRegexp.MatchString(httpctx.GetRequestURL(req))
			if err == nil && !matchString {
				continue
			}
		}

		if rule.EnableForRequest {
			_, newMatchResults, err = rule.MatchPacket(request, true)
			if err != nil && !isMatchTimeout(err) {
				log.Errorf("match package failed: %v", err)
				continue
			}
			matchResults = append(matchResults, newMatchResults...)
		}
		if rule.EnableForResponse {
			_, newMatchResults, err = rule.MatchPacket(response, false)
			if err != nil && !isMatchTimeout(err) {
				log.Errorf("match package failed: %v", err)
				continue
			}
			matchResults = append(matchResults, newMatchResults...)
		}

		if len(matchResults) <= 0 {
			continue
		}
		if rule.Color != "" {
			colorName = append(colorName, rule.Color)
		}
		tagNames = append(tagNames, rule.ExtraTag...) // merge tag name

		for _, match := range matchResults {
			ret := ""

			if match.GroupCount() > 1 {
				extractGroup := match.GroupByNumber(1)
				if extractGroup != nil {
					ret = extractGroup.String()
				}
			} else {
				ret = match.String()
			}

			if ret == "" {
				continue
			}

			extracted = append(extracted, yakit.ExtractedDataFromHTTPFlow(
				flow.HiddenIndex,
				rule.VerboseName,
				match,
				ret,
				rule.String(),
			))
		}
	}
	// 将替换的规则提前，因为一般来说比较重要
	if ret := httpctx.GetMatchedRule(req); len(ret) > 0 {
		lastRule := ret[len(ret)-1]
		if lastRule.Color != "" {
			colorName = append(colorName, lastRule.Color)
		}
		tagNames = append(lastRule.ExtraTag, tagNames...) // merge tag name
	}
	return extracted
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
	keyHeader := make(map[string]*ypb.HTTPHeader)
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
				cookies = append(cookies, lowhttp.ParseCookie("cookie", value)...)
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

	merged, _ := i.Replace(headerMerged, rule.Result, -1, -1)
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
		requestUriReplaced, err := i.Replace(requestUri, rule.Result, -1, -1)
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

	merged, _ := i.Replace(origin, rule.Result, -1, -1)
	if merged == "" {
		return bodyMerged, false
	}
	return []byte(merged), true
}

func (m *mitmReplacer) hook(isRequest, isResponse bool, url string, origin []byte, args ...any) ([]*ypb.MITMContentReplacer, []byte, bool) {
	var matchedRules Rules
	if m == nil {
		return matchedRules.MITMContentReplacers(), origin, false
	}
	var rules []*MITMReplaceRule

	rules = m._hijackingRules
	if len(rules) <= 0 {
		return matchedRules.MITMContentReplacers(), origin, false
	}

	originPacket := origin
	_ = originPacket
	if isResponse {
		originDecoded, _, err := lowhttp.FixHTTPResponse(origin)
		if err != nil {
			return matchedRules.MITMContentReplacers(), origin, false
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
			if utils.IContains(value, "chunked") {
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

	bodyMerged := make([]byte, len(body))
	copy(bodyMerged, body)
	headerMerged := headerRaw
	if len(bodyMerged) <= 0 && headerMerged == "" {
		return matchedRules.MITMContentReplacers(), origin, false
	}

	// 是否丢包
	dropPacket := false
	extraRepeat := false
	var matchedRule *MITMReplaceRule
	modifiedPacket := origin
	for _, rule := range rules {
		if rule.NoReplace {
			continue
		}
		if !((rule.EnableForRequest && isRequest) || (rule.EnableForResponse && isResponse) || rule.GetEnableForURI()) {
			continue
		}
		if rule.EffectiveURL != "" {
			yakRegexp := regexp_utils.DefaultYakRegexpManager.GetYakRegexp(rule.EffectiveURL)
			matchString, err := yakRegexp.MatchString(url)
			if err == nil && !matchString {
				continue
			}
		}
		matched, packet, err := rule.MatchAndReplacePacket(modifiedPacket, isRequest)
		if err != nil && !isMatchTimeout(err) {
			log.Errorf("match package failed: %v", err)
			continue
		}
		modifiedPacket = packet
		if len(matched) > 0 {
			if rule.GetDrop() {
				dropPacket = true
			}

			if rule.GetExtraRepeat() && isRequest {
				extraRepeat = true
				matchedRule = rule
			}

			matchedRules = append(matchedRules, rule)
		}
	}

	if extraRepeat && isRequest { // 不修改原请求，修改后的请求作为额外发包
		var extraArgHttps bool
		if len(args) > 0 {
			extraArgHttps, _ = strconv.ParseBool(utils.InterfaceToString(args[0]))
		}
		m.wg.Add(1)
		go func() {
			defer func() {
				m.wg.Done()
				if err := recover(); err != nil {
					log.Errorf("extraRepeat failed: %v", err)
				}
			}()
			log.Info("AUTO(EXTRA)-REPEAT FROM MITM")
			//if m.repeatSender != nil {
			//	m.repeatSender(originPacket)
			//}
			_ = extraArgHttps
			opts := []lowhttp.LowhttpOpt{
				lowhttp.WithPacketBytes(modifiedPacket), lowhttp.WithHttps(extraArgHttps),
				lowhttp.WithTimeout(15 * time.Second), lowhttp.WithRedirectTimes(3),
				lowhttp.WithSaveHTTPFlow(true), lowhttp.WithSource("mitm"),
				lowhttp.WithRedirectTimes(0),
			}
			for _, tag := range matchedRule.ExtraTag {
				opts = append(opts, lowhttp.WithAppendHTTPFlowTag("[重发]"+tag))
			}
			if len(matchedRule.ExtraTag) == 0 {
				opts = append(opts, lowhttp.WithAppendHTTPFlowTag("[重发]"))
			}
			if matchedRule.Color != "" {
				opts = append(opts, lowhttp.WithAppendHTTPFlowTag(schema.COLORPREFIX+matchedRule.Color))
			}
			rsp, err := lowhttp.HTTP(opts...)
			if err != nil {
				log.Errorf("extraRepeat failed: %v", err)
			}
			_ = rsp
			_ = matchedRule
		}()
		return matchedRules.MITMContentReplacers(), originPacket, dropPacket
	}

	return matchedRules.MITMContentReplacers(), modifiedPacket, dropPacket
}
