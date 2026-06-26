package trafficguard

// validators.go 实现"第三阶段"命中校验(在 minirehs 存在性预筛 + PCRE2 精确提取之后)。
//
// 设计动机(基于真实历史流量取证): 仅靠正则会产生大量"形似但非泄漏"的误报:
//   - Google/Chrome 第一方流量出现在 Google 自有域(content-autofill.googleapis.com 的
//     x-goog-api-key、www.google.com 搜索建议、gstatic 静态资源、Firebase 遥测 ...): 自用, 非泄漏;
//   - data.bilibili.com 等埋点接口在请求里携带的 JWT/会话 ticket: 第一方会话凭证, 非泄漏;
//   - jquery 等 JS 源码里的 password:function(...) / secret:t 之类: 源码标识符, 非凭证。
//
// 校验只做"剔除明显误报", 不追求判全真假; 残留的疑似项交给 Risk 上下文(命中值 + 前后片段)供人工判定。

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// validateCtx 是第三阶段校验的上下文。
type validateCtx struct {
	// host 目标 host(小写、无端口); 为空表示无 host 上下文(跳过厂商自有域抑制)。
	host string
	// direction 命中所在方向: "request" / "response"。
	direction string
}

// validateFinding 返回 true 表示该命中通过校验(保留); false 表示判为误报(丢弃)。
//
// 关键词: trafficguard 误报治理, 厂商自有域抑制, JWT 校验, 口令字段收紧
func validateFinding(r *rule, raw []byte, vc validateCtx) bool {
	// 1) 厂商自有域抑制: 第一方噪声规则(Google key/token、JWT、通用 api-key/凭证字段与鉴权头)
	//    命中厂商自有域时视为自用(浏览器 autofill/搜索建议/OAuth/遥测), 非泄漏, 直接丢弃。
	if isVendorOwnDomain(r, vc.host) {
		return false
	}
	switch r.ID {
	case 19: // JWT
		return validateJWT(raw, vc)
	case 23: // 敏感口令/凭证字段
		return looksLikeRealSecretValue(raw)
	}
	return true
}

// googleOwnedSuffixes 是 Google/Chrome 自有(及其生态)域后缀; 命中这些域上的 key/token/凭证字段
// 视为第一方自用(浏览器 autofill / 搜索建议 / OAuth / 遥测 / 组件更新等), 不报泄漏。
//
// 实测高频噪声来源: content-autofill.googleapis.com (Chrome 自动填充, 携带 x-goog-api-key)、
// www.google.com/complete/search (搜索建议)、fonts.gstatic.com、app-measurement.com (Firebase 遥测)等。
var googleOwnedSuffixes = []string{
	// 核心
	"google.com", "googleapis.com", "gstatic.com", "googleusercontent.com",
	"google.cn", "googleapis.cn", "google.com.hk",
	// 静态资源 / Chrome 组件更新 / 视频
	"gvt1.com", "gvt2.com", "googlevideo.com", "ggpht.com", "withgoogle.com",
	// 统计 / 广告 / 标签
	"google-analytics.com", "googletagmanager.com", "googletagservices.com",
	"googlesyndication.com", "googleadservices.com", "doubleclick.net", "2mdn.net",
	// 人机验证
	"recaptcha.net",
	// Firebase / 崩溃上报(移动端 + Web SDK 遥测)
	"app-measurement.com", "crashlytics.com", "firebaseio.com",
}

// vendorFirstPartyNoiseRules 列出"出现在厂商自有域时几乎全是第一方自用流量"的规则:
// 浏览器自带的 autofill / 搜索建议 / OAuth / 遥测请求会大量携带这些"通用凭证特征"
// (如 x-goog-api-key: AIza... 同时命中规则 4 / 23 / 25), 它们是厂商自己的 key/token/会话,
// 并非泄漏。命中这些规则且 host 为厂商自有域时一律抑制, 以彻底消除这类噪声。
//
// 注意: 强特征第三方凭证(AWS AKIA / GitHub ghp_ / Stripe sk_live_ / PEM 私钥 / 数据库连接串等)
// 不在此列 —— 它们即便出现在 Google 域也更可能是真实泄漏(例如把自家密钥误传给第三方), 予以保留。
var vendorFirstPartyNoiseRules = map[int]struct{}{
	4:  {}, // Google API Key (AIza...)
	5:  {}, // Google OAuth Access Token (ya29....)
	19: {}, // JWT (Google id_token 等第一方令牌)
	23: {}, // 敏感口令/凭证字段 (x-goog-api-key / api-key: ...)
	24: {}, // URL/表单 API Key 参数 (access_token / client_secret 等)
	25: {}, // 自定义鉴权请求头 (x-api-key / api-key)
}

// isVendorOwnDomain 判断某条"第一方噪声规则"的命中是否落在厂商自有域(从而视为自用、非泄漏)。
func isVendorOwnDomain(r *rule, host string) bool {
	if host == "" {
		return false
	}
	if _, ok := vendorFirstPartyNoiseRules[r.ID]; !ok {
		return false
	}
	return hostHasSuffix(normalizeHost(host), googleOwnedSuffixes)
}

// normalizeHost 归一化 host: 转小写、去首尾空白、去端口。
func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	return h
}

// hostHasSuffix 判断 host 是否等于某个域后缀, 或为其子域。
func hostHasSuffix(host string, suffixes []string) bool {
	for _, s := range suffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

// validateJWT 校验 JWT 命中:
//   - 请求方向: 视为第一方会话凭证(等同 Authorization 头), 抑制以降噪;
//   - 响应/脚本方向: 要求首段是含 "alg" 字段的真实 JWT header, 否则视为普通 base64(eyJ...)块。
//
// 关键词: JWT 校验, 第一方会话凭证抑制, alg header
func validateJWT(raw []byte, vc validateCtx) bool {
	if vc.direction == "request" {
		return false
	}
	s := string(raw)
	dot := strings.IndexByte(s, '.')
	if dot <= 0 {
		return false
	}
	header := decodeBase64URLSegment(s[:dot])
	if len(header) == 0 {
		return false
	}
	var m map[string]any
	if json.Unmarshal(header, &m) != nil {
		return false
	}
	if _, ok := m["alg"]; !ok {
		return false
	}
	return true
}

// decodeBase64URLSegment 尽力解码一段 base64url(JWT 各段可能带或不带 padding)。
func decodeBase64URLSegment(seg string) []byte {
	seg = strings.TrimRight(seg, "=")
	if b, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(seg); err == nil {
		return b
	}
	return nil
}

// jsReservedOrCommonWords 是 JS 源码里高频出现、绝不会是真实凭证的标识符/字面量,
// 用于剔除 password:function / secret:true 之类的源码型误报。
var jsReservedOrCommonWords = map[string]struct{}{
	"function": {}, "return": {}, "true": {}, "false": {}, "null": {}, "undefined": {},
	"this": {}, "void": {}, "typeof": {}, "instanceof": {}, "new": {}, "delete": {},
	"var": {}, "let": {}, "const": {}, "nan": {}, "infinity": {}, "arguments": {},
	"prototype": {}, "object": {}, "string": {}, "number": {}, "boolean": {}, "array": {},
	"length": {}, "value": {}, "default": {}, "callback": {}, "props": {}, "state": {},
	"window": {}, "document": {}, "console": {}, "require": {}, "module": {}, "exports": {},
	"async": {}, "await": {}, "yield": {}, "class": {}, "super": {}, "static": {},
	"encodeuri": {}, "encodeuricomponent": {}, "decodeuri": {}, "decodeuricomponent": {},
}

// looksLikeRealSecretValue 对"敏感口令/凭证字段"(规则23)命中收紧: 提取键值对中的"值",
// 剔除明显的源码型误报(JS 标识符/保留字、成员表达式 a.b.c、方法调用、路径、掩码 ****、纯小写词等),
// 保留看起来像真实凭证的值(含数字/大小写混合/足够长度的随机串)。
//
// 注意: 不跳过 JS —— JS 硬编码与注释里可能藏真实凭证。这里只剔除"一眼是代码"的值,
// 残留疑似项靠 Risk 上下文交人工判真假。
//
// 关键词: 口令字段收紧, JS 源码误报剔除, 真实凭证启发式
func looksLikeRealSecretValue(raw []byte) bool {
	v := extractFieldValue(string(raw))
	if len(v) < 6 {
		return false
	}
	lower := strings.ToLower(v)
	if _, ok := jsReservedOrCommonWords[lower]; ok {
		return false
	}
	// 以操作符/路径/赋值符起头: 多为代码片段或路径(如 /passApi/...、+encodeURIComponent)。
	switch v[0] {
	case '/', '+', '.', '=', '-', '*', '\\', '$', '@':
		return false
	}
	// 成员表达式: ident.ident(.ident)* (如 x.auth.slice、o.password、e.data.get)。
	if isMemberExpression(v) {
		return false
	}
	// 掩码占位: ****** / xxxxxx。
	if isRepeatedMask(v) {
		return false
	}
	// 纯小写字母 + 连字符/下划线、无数字: 多为单词/slug(如 routes-api-failed), 非随机凭证。
	if isLowercaseWordOrSlug(v) {
		return false
	}
	return true
}

// extractFieldValue 从规则23命中片段中提取"值"部分:
// 先定位 key 与 value 的分隔符(: 或 =), 取其后内容, 去掉首尾引号/空白,
// 再截断到第一个明显的代码结构分隔符(如 ( ) { } , ; 空白), 得到候选值 token。
func extractFieldValue(s string) string {
	sep := strings.IndexAny(s, ":=")
	if sep < 0 || sep+1 >= len(s) {
		return ""
	}
	v := s[sep+1:]
	v = strings.TrimLeft(v, " \t\"'")
	// 截断到代码结构分隔符: 真实凭证不含这些字符, 出现即说明后面是代码。
	end := len(v)
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '(' || c == ')' || c == '{' || c == '}' || c == ',' || c == ';' ||
			c == ' ' || c == '\t' || c == '"' || c == '\'' || c == '`' {
			end = i
			break
		}
	}
	return v[:end]
}

// isMemberExpression 判断 v 是否形如 ident.ident(.ident)* (JS 成员访问)。
func isMemberExpression(v string) bool {
	if !strings.Contains(v, ".") {
		return false
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if !isJSIdentifier(p) {
			return false
		}
	}
	return true
}

// isJSIdentifier 判断 p 是否是一个合法的 JS 标识符(首字符字母/_/$, 其余字母数字/_/$)。
func isJSIdentifier(p string) bool {
	if p == "" {
		return false
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$'
		isDigit := c >= '0' && c <= '9'
		if i == 0 {
			if !isAlpha {
				return false
			}
		} else if !isAlpha && !isDigit {
			return false
		}
	}
	return true
}

// isRepeatedMask 判断 v 是否为单一字符重复的掩码(如 ******、xxxxxx)。
func isRepeatedMask(v string) bool {
	if len(v) < 4 {
		return false
	}
	first := v[0]
	if first != '*' && first != 'x' && first != 'X' && first != '.' && first != '-' {
		return false
	}
	for i := 1; i < len(v); i++ {
		if v[i] != first {
			return false
		}
	}
	return true
}

// isLowercaseWordOrSlug 判断 v 是否为"纯小写字母 + 连字符/下划线、且不含数字"的单词/slug
// (如 routes-api-failed、access-denied)。真实凭证通常含数字或大小写混合, 这类视为非凭证。
func isLowercaseWordOrSlug(v string) bool {
	hasLetter := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z':
			hasLetter = true
		case c == '-' || c == '_':
			// allowed separator
		default:
			return false
		}
	}
	return hasLetter
}
