package trafficguard

// validators.go 实现"第三阶段"命中校验(在 minirehs 存在性预筛 + PCRE2 精确提取之后)。
//
// 设计动机(基于真实历史流量取证): 仅靠正则会产生大量"形似但非泄漏"的误报:
//   - Google/Chrome 第一方流量出现在 Google 自有域(content-autofill.googleapis.com 的
//     x-goog-api-key、www.google.com 搜索建议、gstatic 静态资源、Firebase 遥测 ...): 自用, 非泄漏;
//   - data.bilibili.com 等埋点接口在请求里携带的 JWT/会话 ticket: 第一方会话凭证, 非泄漏;
//   - jquery 等 JS 源码里的 password:function(...) / secret:t 之类: 源码标识符, 非凭证;
//   - 登录页/表单的本地化文案(i18n): {"password":"设置密码"} / {"pwd":"忘记密码"} /
//     {"password":"请输入密码"} 之类, 值是给人看的 UI 文案而非真实口令 —— 这是用户反馈最强烈的
//     "访问任意登录页就报 password" 误报源头。
//
// 校验只做"剔除明显误报", 不追求判全真假; 残留的疑似项交给 Risk 上下文(命中值 + 前后片段)供人工判定。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode"
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
// data 为命中所在的完整扫描缓冲区, [from,to) 为命中明文在其中的字节偏移。传完整 data 而非仅命中片段,
// 是为了让口令字段校验能回看命中处的上下文(如是否落在 HTML/JS 注释里), 从而区分"登录框文案误报"与
// "被注释掉的默认口令"。
//
// 关键词: trafficguard 误报治理, 厂商自有域抑制, JWT 校验, 口令字段收紧, 登录框文案抑制, 注释默认口令
func validateFinding(r *rule, data []byte, from, to int, vc validateCtx) bool {
	// 1) 厂商自有域抑制: 第一方噪声规则(Google key/token、JWT、通用 api-key/凭证字段与鉴权头)
	//    命中厂商自有域时视为自用(浏览器 autofill/搜索建议/OAuth/遥测), 非泄漏, 直接丢弃。
	if isVendorOwnDomain(r, vc.host) {
		return false
	}
	raw := data[from:to]
	switch r.ID {
	case 19: // JWT
		return validateJWT(raw, vc)
	case 23: // 敏感口令/凭证字段
		return validateSecretField(data, from, to)
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

// validateSecretField 对"敏感口令/凭证字段"(规则23)命中做分层收紧, 在"登录框不能误报"与
// "注释里的默认口令必须报出"之间取得平衡:
//
//	A 层(永远抑制): 一眼是代码的源码型噪声(保留字、运算符开头、成员表达式 a.b.c、掩码 ****),
//	   绝非真实凭证, 无论是否在注释中都丢弃 —— 被注释掉的代码片段也不该当口令报。
//	B 层(常规抑制 / 注释中放行): 登录框/页面的自然语言文案(含 CJK 本地化文案、通用 UI 词、纯小写 slug)。
//	   常规上下文: 视为 i18n/占位文案误报(设置密码/忘记密码/请输入密码/Password/Submit ...), 丢弃;
//	   注释上下文(<!-- ... -->、/* ... */、行首 // 或 #): 可能是被开发者写进注释里的默认/初始口令
//	   (如 <!-- 默认密码 password: admin -->), 必须报出。
//	C 层(保留): 看起来像真实凭证的值(大小写+数字混合、含特殊字符、足够长度的随机串)。
//
// 注意: 不跳过 JS —— JS 硬编码与注释里可能藏真实凭证。残留疑似项靠 Risk 上下文交人工判真假。
//
// 关键词: 口令字段收紧, 登录框文案抑制, 注释默认口令必报, 真实凭证启发式
func validateSecretField(data []byte, from, to int) bool {
	v := extractFieldValue(string(data[from:to]))
	// 注释里写死的默认/初始口令往往很短(admin / root / 123456 ...), 故注释上下文放宽最小长度到正则下限(4);
	// 常规上下文保持 6, 压住短噪声。
	inComment := isInCommentContext(data, from)
	minLen := 6
	if inComment {
		minLen = 4
	}
	if len(v) < minLen {
		return false
	}
	// A 层: 源码型噪声 —— 永远抑制(被注释掉的代码同样不该误报)。
	if isSourceCodeNoiseValue(v) {
		return false
	}
	// B 层: 登录框/页面自然语言文案。常规抑制; 处于注释中则视为默认口令, 必须报出。
	if isLoginFormLabelValue(v) {
		return inComment
	}
	// C 层: 看起来像真实凭证, 保留。
	return true
}

// isSourceCodeNoiseValue 判断口令字段的"值"是否为一眼是代码的源码型噪声(保留字、运算符/路径开头、
// 成员表达式、掩码占位)。这类值绝不会是真实凭证, 即便出现在注释里也只是被注释掉的代码, 一律抑制。
func isSourceCodeNoiseValue(v string) bool {
	if v == "" {
		return true
	}
	if _, ok := jsReservedOrCommonWords[strings.ToLower(v)]; ok {
		return true
	}
	// 以操作符/路径/赋值符起头: 多为代码片段或路径(如 /passApi/...、+encodeURIComponent)。
	switch v[0] {
	case '/', '+', '.', '=', '-', '*', '\\', '$', '@':
		return true
	}
	// 成员表达式: ident.ident(.ident)* (如 x.auth.slice、o.password、e.data.get)。
	if isMemberExpression(v) {
		return true
	}
	// 掩码占位: ****** / xxxxxx。
	if isRepeatedMask(v) {
		return true
	}
	return false
}

// loginFormLabelWords 是登录页/表单里高频出现、其"值"本身就是字段名或动作/占位词的英文 UI 文案。
// 它们作为口令字段的值时是给人看的标签(占位文本/按钮文案/i18n key), 而非真实凭证。
// 真实的默认口令几乎不会恰好等于这些词(若真有, 也只在注释中放行报出)。
var loginFormLabelWords = map[string]struct{}{
	// 字段名本身(占位/i18n key 常直接复用字段名)
	"password": {}, "passwd": {}, "pwd": {}, "passphrase": {}, "pass": {},
	"secret": {}, "token": {}, "apikey": {}, "accesstoken": {},
	"username": {}, "userid": {}, "account": {}, "email": {}, "mobile": {}, "phone": {},
	// 动作 / 按钮 / 状态文案
	"login": {}, "signin": {}, "signup": {}, "logon": {}, "logout": {},
	"register": {}, "submit": {}, "confirm": {}, "cancel": {}, "reset": {},
	"remember": {}, "captcha": {}, "search": {}, "required": {}, "optional": {},
	// 占位短语被空白截断后的首词(如 "Enter your password" -> "Enter")
	"enter": {}, "input": {}, "type": {}, "your": {}, "please": {}, "forgot": {}, "change": {},
}

// isLoginFormLabelValue 判断口令字段的"值"是否为登录框/页面的自然语言文案(而非真实凭证):
//   - 含 CJK(中日韩)字符: 本地化 UI 文案(设置密码/忘记密码/请输入密码 ...), 真实口令几乎不含;
//   - 命中 loginFormLabelWords: 值就是字段名/动作/占位词;
//   - 纯小写字母 + 连字符/下划线、无数字: 多为单词/slug(如 routes-api-failed), 非随机凭证。
func isLoginFormLabelValue(v string) bool {
	if containsCJK(v) {
		return true
	}
	if _, ok := loginFormLabelWords[strings.ToLower(v)]; ok {
		return true
	}
	if isLowercaseWordOrSlug(v) {
		return true
	}
	return false
}

// containsCJK 判断字符串是否含中日韩文字(汉字 / 假名 / 谚文)。登录框本地化文案几乎都含 CJK,
// 而真实口令/凭证极少包含 CJK, 故含 CJK 即视为自然语言文案。
func containsCJK(s string) bool {
	for _, r := range s {
		if r < 0x80 {
			continue
		}
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}

// isInCommentContext 判断命中位置 pos 是否落在源码/页面的注释里(HTML <!-- -->、块注释 /* */、
// 行首 // 或 # 行注释)。开发者写进注释里的默认/初始口令是真实风险, 故注释中的口令字段不抑制。
//
// 为避免压制的稀释开销与误判, 只回看 pos 之前有限窗口; 行注释只认"行首"标记(不扫行内 //, 以免把
// 压缩 JS 里的 http:// / 协议相对地址 //cdn 误判成注释, 反而把本该抑制的文案放行)。
func isInCommentContext(data []byte, pos int) bool {
	if pos <= 0 || pos > len(data) {
		return false
	}
	const window = 4096
	start := pos - window
	if start < 0 {
		start = 0
	}
	pre := data[start:pos]

	// HTML 注释: pos 之前最近的 <!-- 后没有出现 -->, 说明仍在注释体内。
	if i := bytes.LastIndex(pre, []byte("<!--")); i >= 0 && !bytes.Contains(pre[i:], []byte("-->")) {
		return true
	}
	// 块注释: pos 之前最近的 /* 后没有出现 */, 说明仍在块注释内。
	if i := bytes.LastIndex(pre, []byte("/*")); i >= 0 && !bytes.Contains(pre[i:], []byte("*/")) {
		return true
	}
	// 行注释: 命中所在行去掉前导空白后以 // / # / *(块注释续行)开头。
	line := pre
	if nl := bytes.LastIndexByte(pre, '\n'); nl >= 0 {
		line = pre[nl+1:]
	}
	trimmed := bytes.TrimLeft(line, " \t")
	if len(trimmed) > 0 {
		if trimmed[0] == '#' || trimmed[0] == '*' || bytes.HasPrefix(trimmed, []byte("//")) {
			return true
		}
	}
	return false
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
