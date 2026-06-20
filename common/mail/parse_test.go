package mail

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestParse_PlainText 纯文本邮件：基本头解析 + URL 提取
func TestParse_PlainText(t *testing.T) {
	raw := "From: alice@example.com\r\n" +
		"To: bob@example.com\r\n" +
		"Subject: Hello\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Hi, please visit https://example.com/path for details.\r\n"
	m := Parse(raw)
	if m["error"] != nil {
		t.Fatalf("unexpected error: %v", m["error"])
	}
	if m["subject"] != "Hello" {
		t.Errorf("subject = %v, want Hello", m["subject"])
	}
	from, ok := m["from"].(map[string]interface{})
	if !ok || from["address"] != "alice@example.com" {
		t.Errorf("from.address = %v", m["from"])
	}
	if !strings.Contains(asString(m["body_text"]), "visit") {
		t.Errorf("body_text = %v", m["body_text"])
	}
	urls := asStrings(m["urls"])
	if len(urls) != 1 || !strings.HasPrefix(urls[0], "https://example.com") {
		t.Errorf("urls = %v", urls)
	}
}

// TestParse_MultipartHTMLAttachment 综合：multipart + HTML(base64) + 附件 + RFC2047 中文标题 + SPF/DKIM/DMARC
func TestParse_MultipartHTMLAttachment(t *testing.T) {
	html := `<div>请点击链接验证账户 <a href="http://evil.com/login">点击</a></div>`
	htmlB64 := base64.StdEncoding.EncodeToString([]byte(html))
	zipContent := base64.StdEncoding.EncodeToString([]byte("FAKEZIPCONTENT"))
	subjectB64 := base64.StdEncoding.EncodeToString([]byte("【安全提醒】账户验证"))
	fromNameB64 := base64.StdEncoding.EncodeToString([]byte("IT部门"))

	raw := "From: =?UTF-8?B?" + fromNameB64 + "?= <it@company-securlty.com>\r\n" +
		"To: victim@example.com\r\n" +
		"Subject: =?UTF-8?B?" + subjectB64 + "?=\r\n" +
		"Reply-To: attacker@evil.com\r\n" +
		"Authentication-Results: mx.example.com; spf=fail smtp.mailfrom=company-securlty.com; dkim=fail; dmarc=fail\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"BOUND\"\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		htmlB64 + "\r\n" +
		"--BOUND\r\n" +
		"Content-Type: application/zip; name=\"invoice.zip\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"invoice.zip\"\r\n" +
		"\r\n" +
		zipContent + "\r\n" +
		"--BOUND--\r\n"

	m := Parse(raw)
	if m["error"] != nil {
		t.Fatalf("unexpected error: %v", m["error"])
	}
	// RFC 2047 标题解码
	if !strings.Contains(asString(m["subject"]), "账户验证") {
		t.Errorf("subject not decoded: %v", m["subject"])
	}
	from := m["from"].(map[string]interface{})
	if !strings.Contains(asString(from["display"]), "IT") {
		t.Errorf("from.display not decoded: %v", from["display"])
	}
	if from["address"] != "it@company-securlty.com" {
		t.Errorf("from.address = %v", from["address"])
	}
	// Reply-To
	if asString(m["reply_to"]) != "attacker@evil.com" {
		t.Errorf("reply_to = %v", m["reply_to"])
	}
	// 认证结果
	auth := m["auth_results"].(map[string]string)
	if auth["spf"] != "fail" || auth["dkim"] != "fail" || auth["dmarc"] != "fail" {
		t.Errorf("auth_results = %v", auth)
	}
	// HTML 正文解码
	if !strings.Contains(asString(m["body_html"]), "evil.com") {
		t.Errorf("body_html not decoded: %v", m["body_html"])
	}
	// URL 提取（从 HTML href）
	urls := asStrings(m["urls"])
	found := false
	for _, u := range urls {
		if strings.Contains(u, "evil.com/login") {
			found = true
		}
	}
	if !found {
		t.Errorf("evil.com url not extracted: %v", urls)
	}
	// 附件
	atts := m["attachments"].([]map[string]interface{})
	if len(atts) != 1 {
		t.Fatalf("attachments = %v", atts)
	}
	if atts[0]["filename"] != "invoice.zip" || atts[0]["content_type"] != "application/zip" {
		t.Errorf("attachment = %v", atts[0])
	}
	if atts[0]["sha256"] == "" || len(asString(atts[0]["sha256"])) != 64 {
		t.Errorf("attachment sha256 = %v", atts[0]["sha256"])
	}
	// 可疑指标命中
	susps := m["suspicious"].([]map[string]interface{})
	indicators := map[string]bool{}
	for _, s := range susps {
		indicators[asString(s["indicator"])] = true
	}
	if !indicators["auth_spf_issue"] {
		t.Errorf("missing auth_spf_issue in suspicious: %v", indicators)
	}
	if !indicators["reply_to_mismatch"] {
		t.Errorf("missing reply_to_mismatch: %v", indicators)
	}
	if !indicators["dangerous_attachment"] {
		t.Errorf("missing dangerous_attachment: %v", indicators)
	}
}

// TestParse_QuotedPrintable quoted-printable 编码正文
func TestParse_QuotedPrintable(t *testing.T) {
	raw := "From: a@b.com\r\n" +
		"Subject: QP Test\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"=E4=BD=A0=E5=A5=BD world\r\n"
	m := Parse(raw)
	body := asString(m["body_text"])
	if !strings.Contains(body, "你好") || !strings.Contains(body, "world") {
		t.Errorf("QP body not decoded: %v", body)
	}
}

func TestDecodeHeader(t *testing.T) {
	testB64 := base64.StdEncoding.EncodeToString([]byte("测试"))
	cases := map[string]string{
		"=?UTF-8?B?" + testB64 + "?=":    "测试",
		"=?UTF-8?Q?=E4=BD=A0=E5=A5=BD?=": "你好",
		"plain text":                     "plain text",
	}
	for in, want := range cases {
		if got := DecodeHeader(in); got != want {
			t.Errorf("DecodeHeader(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeQP(t *testing.T) {
	if got := DecodeQP("=E4=BD=A0=E5=A5=BD"); !strings.Contains(got, "你好") {
		t.Errorf("DecodeQP = %q", got)
	}
}

func TestExtractURLs(t *testing.T) {
	html := `<a href="https://good.com/a">x</a> visit http://evil.com/b <img src="https://track.com/p">`
	urls := ExtractURLs(html)
	want := map[string]bool{"https://good.com/a": true, "http://evil.com/b": true, "https://track.com/p": true}
	if len(urls) != len(want) {
		t.Fatalf("urls = %v, want %d", urls, len(want))
	}
	for _, u := range urls {
		if !want[u] {
			t.Errorf("unexpected url: %s", u)
		}
	}
}

// TestParse_Empty 空内容
func TestParse_Empty(t *testing.T) {
	m := Parse("")
	if m["error"] == nil {
		t.Errorf("expected error for empty input")
	}
}

// asString 把 interface{} 安全转为 string
func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// asStrings 把 interface{}（[]string）转为 []string
func asStrings(v interface{}) []string {
	if s, ok := v.([]string); ok {
		return s
	}
	return nil
}
