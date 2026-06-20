package mail

// SMTP 邮件发送，functional-options 风格（对标 crawler 库）。
// 底层使用 gomail.v2 构建 Message（支持 text/html/附件/自定义头）+ net/smtp 认证。
// 支持 SSL 直连(465) / STARTTLS(587) / 明文(25)，以及 PLAIN(自动) / LOGIN / CRAM-MD5 / 无认证。

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	gomail "gopkg.in/gomail.v2"
)

type tlsMode int

const (
	tlsAuto     tlsMode = iota // 默认：明文连接 + 服务器支持则 STARTTLS
	tlsSSL                     // SSL 直连（端口 465）
	tlsSTARTTLS                // 强制 STARTTLS（端口 587）
	tlsNone                    // 纯明文，不升级
)

type sendConfig struct {
	host       string
	port       int
	username   string
	password   string
	tls        tlsMode
	skipVerify bool
	authMethod string // ""/auto/login/cram-md5/none
	from       string
	to         []string
	cc         []string
	bcc        []string
	subject    string
	text       string
	html       string
	attachments []string
	headers    map[string]string
}

// SendOption 配置邮件发送参数（functional option）。
type SendOption func(*sendConfig)

// Server 设置 SMTP 服务器与端口。
func Server(host string, port int) SendOption {
	return func(c *sendConfig) {
		c.host = host
		c.port = port
	}
}

// Username 设置认证用户名。
func Username(s string) SendOption {
	return func(c *sendConfig) { c.username = s }
}

// Password 设置认证密码 / 授权码。
func Password(s string) SendOption {
	return func(c *sendConfig) { c.password = s }
}

// SSL 使用 SSL 直连（端口 465）。
func SSL() SendOption { return func(c *sendConfig) { c.tls = tlsSSL } }

// STARTTLS 强制使用 STARTTLS（端口 587）。
func STARTTLS() SendOption { return func(c *sendConfig) { c.tls = tlsSTARTTLS } }

// NoTLS 使用纯明文连接，不升级 TLS。
func NoTLS() SendOption { return func(c *sendConfig) { c.tls = tlsNone } }

// SkipVerify 跳过 TLS 证书校验（自签名 / 内网邮件服务器常用）。
func SkipVerify() SendOption { return func(c *sendConfig) { c.skipVerify = true } }

// AuthMethod 设置认证方式：auto（默认）/ login / cram-md5 / none。
func AuthMethod(s string) SendOption {
	return func(c *sendConfig) { c.authMethod = strings.ToLower(s) }
}

// From 设置发件人地址。
func From(s string) SendOption { return func(c *sendConfig) { c.from = s } }

// To 设置收件人（可多次调用累加）。
func To(addrs ...string) SendOption {
	return func(c *sendConfig) { c.to = append(c.to, addrs...) }
}

// Cc 设置抄送（可多次调用累加）。
func Cc(addrs ...string) SendOption {
	return func(c *sendConfig) { c.cc = append(c.cc, addrs...) }
}

// Bcc 设置密送（可多次调用累加）。
func Bcc(addrs ...string) SendOption {
	return func(c *sendConfig) { c.bcc = append(c.bcc, addrs...) }
}

// Subject 设置邮件主题。
func Subject(s string) SendOption { return func(c *sendConfig) { c.subject = s } }

// Text 设置纯文本正文。
func Text(s string) SendOption { return func(c *sendConfig) { c.text = s } }

// HTML 设置 HTML 正文。
func HTML(s string) SendOption { return func(c *sendConfig) { c.html = s } }

// Attach 添加附件（文件路径，可多次调用）。
func Attach(path string) SendOption {
	return func(c *sendConfig) { c.attachments = append(c.attachments, path) }
}

// Header 添加自定义邮件头（可多次调用）。
func Header(key, value string) SendOption {
	return func(c *sendConfig) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[key] = value
	}
}

// loginAuth 实现 SMTP AUTH LOGIN（部分 Exchange / 企业邮箱仅支持 LOGIN）。
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:", "username:", "User name\x00", "Username\x00":
			return []byte(a.username), nil
		case "Password:", "password:", "Password\x00":
			return []byte(a.password), nil
		default:
			// 兜底：第二次 challenge 视为密码
			return []byte(a.password), nil
		}
	}
	return nil, nil
}

// Send 按配置发送邮件。至少需要 Server / From / To。
// 示例：
//
//	mail.Send(
//	    mail.Server("smtp.qq.com", 465), mail.SSL(),
//	    mail.Username("u"), mail.Password("authcode"),
//	    mail.From("a@b.com"), mail.To("c@d.com"),
//	    mail.Subject("标题"), mail.HTML("<b>正文</b>"), mail.Attach("./x.pdf"),
//	)
func Send(opts ...SendOption) error {
	c := &sendConfig{tls: tlsAuto, authMethod: "auto"}
	for _, opt := range opts {
		opt(c)
	}

	if c.host == "" || c.port == 0 {
		return fmt.Errorf("smtp server/host is required (use mail.Server)")
	}
	if c.from == "" {
		c.from = c.username
	}
	if c.from == "" {
		return fmt.Errorf("from address is required (use mail.From or mail.Username)")
	}
	if len(c.to) == 0 {
		return fmt.Errorf("at least one recipient is required (use mail.To)")
	}

	// 构建 Message
	msg := gomail.NewMessage()
	msg.SetHeader("From", c.from)
	msg.SetHeader("To", c.to...)
	if len(c.cc) > 0 {
		msg.SetHeader("Cc", c.cc...)
	}
	if len(c.bcc) > 0 {
		msg.SetHeader("Bcc", c.bcc...)
	}
	msg.SetHeader("Subject", c.subject)

	switch {
	case c.html != "" && c.text != "":
		msg.SetBody("text/html", c.html)
		msg.AddAlternative("text/plain", c.text)
	case c.html != "":
		msg.SetBody("text/html", c.html)
	default:
		msg.SetBody("text/plain", c.text)
	}

	for _, p := range c.attachments {
		msg.Attach(p)
	}
	for k, v := range c.headers {
		msg.SetHeader(k, v)
	}

	// 构建 Dialer
	d := gomail.NewDialer(c.host, c.port, c.username, c.password)
	if c.tls == tlsSSL {
		d.SSL = true
	}
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: c.skipVerify,
		ServerName:         c.host,
	}
	switch c.authMethod {
	case "login":
		d.Auth = &loginAuth{c.username, c.password}
	case "cram-md5":
		d.Auth = smtp.CRAMMD5Auth(c.username, c.password)
	case "none":
		d.Auth = nil
		d.Username = ""
		d.Password = ""
	}

	if err := d.DialAndSend(msg); err != nil {
		return fmt.Errorf("send mail failed: %v", err)
	}
	return nil
}
