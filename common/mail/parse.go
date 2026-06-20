package mail

// 邮件（.eml / RFC 5322 + MIME）解析能力，用于钓鱼邮件研判。
// 基于 Go 标准库 net/mail + mime/multipart + mime/quotedprintable + mime.WordDecoder，
// 补齐 yaklang 缺失的 Quoted-Printable 解码、RFC 2047 编码头解码、MIME multipart 解析。
// charset 转换使用 golang.org/x/text/encoding/htmlindex，覆盖邮件常见全部编码。

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/text/encoding/htmlindex"
)

var (
	// urlRe 匹配正文中的 http(s) 链接
	urlRe = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>\)\]]+`)
	// hrefRe 匹配 HTML 中的 href 属性
	hrefRe = regexp.MustCompile(`(?i)href\s*=\s*["']?([^\s"'>]+)`)
	// srcRe 匹配 HTML 中的 src 属性（img 等）
	srcRe = regexp.MustCompile(`(?i)\bsrc\s*=\s*["']?([^\s"'>]+)`)
	// ipv4Re 匹配 Received 头中的源 IPv4
	ipv4Re = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// authResultRe 匹配 Authentication-Results 中的 spf/dkim/dmarc 等结果
	authResultRe = regexp.MustCompile(`(?i)\b(spf|dkim|dmarc|arc|compauth)\s*=\s*([a-zA-Z0-9_.-]+)`)
	// addrRe 从不规范的 From 头中提取邮箱地址
	addrRe = regexp.MustCompile(`<([^>]+)>|([A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,})`)
	// ipv4LeadingRe 判断 URL 主机部分是否为裸 IP
	ipv4LeadingRe = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}`)
)

// DecodeHeader 解码 RFC 2047 编码的邮件头值（如 Subject、显示名）。
// 例如 "=?UTF-8?B?5p2l5L+h?=" -> "测试"。
func DecodeHeader(s string) string {
	if s == "" {
		return s
	}
	dec := new(mime.WordDecoder)
	out, err := dec.DecodeHeader(s)
	if err != nil || out == "" {
		return s
	}
	return out
}

// DecodeQP 解码 quoted-printable 编码的文本。
func DecodeQP(s string) string {
	r := quotedprintable.NewReader(strings.NewReader(s))
	b, err := io.ReadAll(r)
	if err != nil {
		return s
	}
	return string(b)
}

// convertCharset 将给定 charset 的字节流转换为 UTF-8。
// 支持邮件常见编码（utf-8/gbk/gb2312/gb18030/iso-8859-1/shift_jis/big5/windows-1252 等）。
// 转换失败时原样返回（不影响整体解析流程）。
func convertCharset(charset string, data []byte) string {
	charset = strings.TrimSpace(charset)
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") || strings.EqualFold(charset, "ascii") {
		return string(data)
	}
	enc, err := htmlindex.Get(charset)
	if err != nil || enc == nil {
		return string(data)
	}
	decoded, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(decoded)
}

// decodeContent 按 Content-Transfer-Encoding 解码为字节（不做 charset 转换，用于附件）。
func decodeContent(cte string, r io.Reader) []byte {
	cte = strings.ToLower(strings.TrimSpace(cte))
	switch cte {
	case "base64":
		data, err := io.ReadAll(r)
		if err != nil {
			return nil
		}
		cleaned := make([]byte, 0, len(data))
		for _, b := range data {
			if b != '\n' && b != '\r' && b != ' ' && b != '\t' {
				cleaned = append(cleaned, b)
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(string(cleaned))
		if err != nil {
			decoded, err = base64.URLEncoding.DecodeString(string(cleaned))
			if err != nil {
				return data
			}
		}
		return decoded
	case "quoted-printable":
		qr := quotedprintable.NewReader(r)
		data, err := io.ReadAll(qr)
		if err != nil {
			return nil
		}
		return data
	default:
		// 7bit / 8bit / binary / none
		data, err := io.ReadAll(r)
		if err != nil {
			return nil
		}
		return data
	}
}

// decodeBody 按 Content-Transfer-Encoding 与 charset 解码 part 正文为 UTF-8 字符串。
func decodeBody(cte, charset string, r io.Reader) string {
	return convertCharset(charset, decodeContent(cte, r))
}

// ExtractURLs 从文本或 HTML 中提取去重后的 URL 列表。
func ExtractURLs(s string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(u string) {
		u = strings.TrimRight(u, ".,);'")
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	for _, m := range urlRe.FindAllString(s, -1) {
		add(m)
	}
	for _, m := range hrefRe.FindAllStringSubmatch(s, -1) {
		if strings.HasPrefix(strings.ToLower(m[1]), "http") {
			add(m[1])
		}
	}
	for _, m := range srcRe.FindAllStringSubmatch(s, -1) {
		if strings.HasPrefix(strings.ToLower(m[1]), "http") {
			add(m[1])
		}
	}
	return out
}

func extractFirstAddress(s string) string {
	m := addrRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

type parsedEmail struct {
	headers        map[string]string
	fromRaw        string
	fromDisplay    string
	fromAddress    string
	to             []string
	cc             []string
	replyTo        string
	subject        string
	date           string
	messageID      string
	authResults    map[string]string
	authResultsRaw string
	receivedIPs    []string
	bodyText       string
	bodyHTML       string
	attachments    []map[string]interface{}
	urls           []string
}

func newParsedEmail() *parsedEmail {
	return &parsedEmail{
		headers:     map[string]string{},
		authResults: map[string]string{},
		to:          []string{},
		cc:          []string{},
		receivedIPs: []string{},
		attachments: []map[string]interface{}{},
		urls:        []string{},
	}
}

// walkPart 递归遍历 MIME part，收集 text/html 正文与附件。
func (p *parsedEmail) walkPart(part *multipart.Part) {
	ctHeader := part.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ctHeader)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		// 嵌套 multipart：继续递归
		boundary := params["boundary"]
		if boundary != "" {
			mr := multipart.NewReader(part, boundary)
			for {
				sub, err := mr.NextPart()
				if err != nil {
					break
				}
				p.walkPart(sub)
			}
		}
		return
	}

	cte := part.Header.Get("Content-Transfer-Encoding")
	dispHeader := part.Header.Get("Content-Disposition")
	disposition, dParams, dErr := mime.ParseMediaType(dispHeader)
	filename := ""
	if dErr == nil {
		filename = dParams["filename"]
	}
	if filename == "" {
		if name, ok := params["name"]; ok && name != "" {
			filename = name
		}
	}
	filename = DecodeHeader(filename)

	isAttachment := (dErr == nil && strings.EqualFold(disposition, "attachment")) || filename != ""
	if isAttachment {
		content := decodeContent(cte, part)
		ct := mediaType
		if ct == "" {
			ct = "application/octet-stream"
		}
		sum := sha256.Sum256(content)
		p.attachments = append(p.attachments, map[string]interface{}{
			"filename":          filename,
			"content_type":      ct,
			"size":              len(content),
			"transfer_encoding": strings.ToLower(strings.TrimSpace(cte)),
			"sha256":            fmt.Sprintf("%x", sum[:]),
		})
		return
	}

	// 正文：text/plain 或 text/html
	switch {
	case mediaType == "" || strings.HasPrefix(mediaType, "text/plain"):
		if p.bodyText == "" {
			p.bodyText = decodeBody(cte, params["charset"], part)
		}
	case strings.HasPrefix(mediaType, "text/html"):
		if p.bodyHTML == "" {
			p.bodyHTML = decodeBody(cte, params["charset"], part)
		}
	}
}

// parseHeaders 提取并解码常用邮件头。
func (p *parsedEmail) parseHeaders(h mail.Header) {
	p.subject = DecodeHeader(h.Get("Subject"))
	p.date = h.Get("Date")
	p.messageID = h.Get("Message-ID")
	p.replyTo = DecodeHeader(h.Get("Reply-To"))
	p.fromRaw = h.Get("From")

	if addrs, err := h.AddressList("From"); err == nil && len(addrs) > 0 {
		p.fromAddress = addrs[0].Address
		p.fromDisplay = DecodeHeader(addrs[0].Name)
	} else {
		// From 不规范，尝试用正则提取邮箱
		p.fromAddress = extractFirstAddress(p.fromRaw)
		p.fromDisplay = DecodeHeader(p.fromRaw)
	}
	if addrs, err := h.AddressList("To"); err == nil {
		for _, a := range addrs {
			p.to = append(p.to, a.Address)
		}
	}
	if addrs, err := h.AddressList("Cc"); err == nil {
		for _, a := range addrs {
			p.cc = append(p.cc, a.Address)
		}
	}

	// Authentication-Results（SPF/DKIM/DMARC）
	authRaw := h.Get("Authentication-Results")
	if authRaw == "" {
		authRaw = strings.Join(h["X-Authentication-Results"], "; ")
	}
	p.authResultsRaw = authRaw
	if authRaw != "" {
		for _, m := range authResultRe.FindAllStringSubmatch(authRaw, -1) {
			p.authResults[strings.ToLower(m[1])] = strings.ToLower(m[2])
		}
	}

	// Received 头链中的源 IP
	for _, rh := range h["Received"] {
		p.receivedIPs = append(p.receivedIPs, ipv4Re.FindAllString(rh, -1)...)
	}

	// 保留关键原始头供研判
	for _, key := range []string{
		"From", "To", "Cc", "Reply-To", "Subject", "Date", "Message-ID",
		"Return-Path", "Sender", "Authentication-Results", "Received-SPF",
		"DKIM-Signature", "Received", "X-Mailer", "X-Originating-IP",
	} {
		if v := h.Get(key); v != "" {
			p.headers[key] = v
		}
	}
}

func parseMessageCore(msg *mail.Message, p *parsedEmail) {
	p.parseHeaders(msg.Header)

	ctHeader := msg.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ctHeader)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			mr := multipart.NewReader(msg.Body, boundary)
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				p.walkPart(part)
			}
		}
	} else {
		// 非 multipart：整个 body 是单一正文
		cte := msg.Header.Get("Content-Transfer-Encoding")
		charset := ""
		if params != nil {
			charset = params["charset"]
		}
		body := decodeBody(cte, charset, msg.Body)
		if mediaType != "" && strings.HasPrefix(mediaType, "text/html") {
			p.bodyHTML = body
		} else {
			p.bodyText = body
		}
	}

	// 提取 URL（HTML 优先，否则纯文本）
	src := p.bodyHTML
	if src == "" {
		src = p.bodyText
	}
	p.urls = ExtractURLs(src)
}

// suspiciousIndicators 基于解析结果给出初步可疑指标（供 LLM 进一步研判）。
func (p *parsedEmail) suspiciousIndicators() []map[string]interface{} {
	out := []map[string]interface{}{}
	add := func(reason, detail string) {
		out = append(out, map[string]interface{}{
			"indicator": reason,
			"detail":    detail,
		})
	}

	for _, k := range []string{"spf", "dkim", "dmarc"} {
		if v, ok := p.authResults[k]; ok {
			switch v {
			case "fail", "softfail", "neutral", "none", "temperror", "permerror":
				add("auth_"+k+"_issue", fmt.Sprintf("%s=%s", k, v))
			}
		}
	}
	if p.replyTo != "" && p.fromAddress != "" && !strings.EqualFold(p.replyTo, p.fromAddress) {
		add("reply_to_mismatch", fmt.Sprintf("From=%s Reply-To=%s", p.fromAddress, p.replyTo))
	}
	if p.fromDisplay != "" && p.fromAddress != "" {
		if strings.Contains(p.fromDisplay, "@") && !strings.Contains(p.fromDisplay, p.fromAddress) {
			add("display_name_spoofing", fmt.Sprintf("显示名含其它邮箱: %s", p.fromDisplay))
		}
	}
	dangerousExt := []string{".exe", ".scr", ".bat", ".cmd", ".com", ".lnk", ".js", ".jse", ".vbs", ".vba", ".hta", ".msi", ".iso", ".img", ".zip", ".rar", ".7z", ".docm", ".xlsm", ".pptm"}
	for _, att := range p.attachments {
		fn := strings.ToLower(fmt.Sprintf("%v", att["filename"]))
		for _, ext := range dangerousExt {
			if strings.HasSuffix(fn, ext) {
				add("dangerous_attachment", fmt.Sprintf("%s (%s)", att["filename"], att["content_type"]))
				break
			}
		}
	}
	for _, u := range p.urls {
		lower := strings.ToLower(u)
		schemeIdx := strings.Index(lower, "://")
		if schemeIdx < 0 {
			continue
		}
		afterScheme := u[schemeIdx+3:]
		host := afterScheme
		if i := strings.IndexAny(afterScheme, "/?#"); i >= 0 {
			host = afterScheme[:i]
		}
		if ipv4LeadingRe.MatchString(host) {
			add("url_uses_ip", u)
		}
		if strings.Contains(host, "@") {
			add("url_with_credentials", u)
		}
	}
	urgentRe := regexp.MustCompile(`(?i)(urgent|verify|suspend|account|password|密码|账户|账号|验证|紧急|立即|到期|过期|异常|中奖|解锁|安全警告)`)
	if urgentRe.MatchString(p.subject) {
		add("urgent_subject", p.subject)
	}
	return out
}

func (p *parsedEmail) toMap() map[string]interface{} {
	return map[string]interface{}{
		"from": map[string]interface{}{
			"display": p.fromDisplay,
			"address": p.fromAddress,
			"raw":     p.fromRaw,
		},
		"to":               p.to,
		"cc":               p.cc,
		"reply_to":         p.replyTo,
		"subject":          p.subject,
		"date":             p.date,
		"message_id":       p.messageID,
		"auth_results":     p.authResults,
		"auth_results_raw": p.authResultsRaw,
		"received_ips":     p.receivedIPs,
		"body_text":        p.bodyText,
		"body_html":        p.bodyHTML,
		"attachments":      p.attachments,
		"urls":             p.urls,
		"key_headers":      p.headers,
		"suspicious":       p.suspiciousIndicators(),
	}
}

// Parse 解析邮件原始内容（RFC 5322 + MIME），返回结构化研判信息。
// 输入为邮件原始文本。返回 map 包含发件人/收件人/认证结果/正文/附件/URL/可疑指标等。
func Parse(raw string) map[string]interface{} {
	errResult := func(errMsg string) map[string]interface{} {
		return map[string]interface{}{
			"error":   errMsg,
			"raw_size": len(raw),
		}
	}
	if strings.TrimSpace(raw) == "" {
		return errResult("empty email content")
	}

	defer func() {
		_ = recover()
	}()

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return errResult(fmt.Sprintf("mail.ReadMessage failed: %v", err))
	}
	p := newParsedEmail()
	parseMessageCore(msg, p)
	return p.toMap()
}

// ParseFile 读取 .eml 文件并解析。
func ParseFile(path string) (map[string]interface{}, error) {
	if !utils.IsFile(path) {
		return nil, fmt.Errorf("email file does not exist: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(data)), nil
}
