package mail

// POP3 收件，functional-options 风格。
// 复用 common/utils/pop3（支持 POP3S 直连 SSL/995 + STARTTLS + UIDL）。
// 取回的原始邮件交给 Parse 统一解析，与本地解析 .eml 完全一致。

import (
	"crypto/tls"
	"fmt"

	"github.com/yaklang/yaklang/common/utils/pop3"
)

type fetchConfig struct {
	host       string
	port       int
	username   string
	password   string
	ssl        bool // POP3S 直连（端口 995）
	starttls   bool // 明文连接后 STARTTLS 升级
	skipVerify bool
	messageID  int
}

func defaultFetchConfig() *fetchConfig {
	return &fetchConfig{port: 110}
}

// FetchOption 配置 POP3 收件参数（functional option）。
type FetchOption func(*fetchConfig)

// POP3Server 设置 POP3 服务器与端口。
func POP3Server(host string, port int) FetchOption {
	return func(c *fetchConfig) { c.host = host; c.port = port }
}

// POP3SSL 使用 POP3S 直连 SSL（端口 995）。
func POP3SSL() FetchOption { return func(c *fetchConfig) { c.ssl = true } }

// POP3StartTLS 明文连接后用 STARTTLS 升级。
func POP3StartTLS() FetchOption { return func(c *fetchConfig) { c.starttls = true } }

// POP3SkipVerify 跳过 TLS 证书校验。
func POP3SkipVerify() FetchOption { return func(c *fetchConfig) { c.skipVerify = true } }

// POP3Username 设置账号。
func POP3Username(s string) FetchOption { return func(c *fetchConfig) { c.username = s } }

// POP3Password 设置密码 / 授权码。
func POP3Password(s string) FetchOption { return func(c *fetchConfig) { c.password = s } }

// MessageID 设置要收取的邮件序号（POP3 邮件 ID 从 1 开始）。
func MessageID(id int) FetchOption { return func(c *fetchConfig) { c.messageID = id } }

// pop3Connect 建立 POP3 连接并完成认证，返回 conn 与清理函数。
func pop3Connect(c *fetchConfig) (*pop3.Conn, func(), error) {
	if c.host == "" || c.port == 0 {
		return nil, nil, fmt.Errorf("pop3 server/host is required (use mail.POP3Server)")
	}
	if c.username == "" {
		return nil, nil, fmt.Errorf("pop3 username is required")
	}

	client := pop3.New(pop3.Opt{
		Host:          c.host,
		Port:          c.port,
		TLSEnabled:    c.ssl,
		TLSSkipVerify: c.skipVerify,
	})
	conn, err := client.NewConn()
	if err != nil {
		return nil, nil, fmt.Errorf("pop3 connect failed: %v", err)
	}
	cleanup := func() { _ = conn.Quit() }

	if c.starttls {
		caps, _ := conn.CAPA()
		if _, ok := caps["STLS"]; ok {
			if err := conn.StartTLS(&tls.Config{
				InsecureSkipVerify: c.skipVerify,
				ServerName:         c.host,
			}); err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("pop3 STARTTLS failed: %v", err)
			}
		}
	}

	if err := conn.Auth(c.username, c.password); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("pop3 auth failed: %v", err)
	}
	return conn, cleanup, nil
}

// FetchList 连接邮箱并列出所有邮件摘要（id / size / uid），不下载正文。
// 示例：
//
//	list, _ = mail.FetchList(mail.POP3Server("pop.qq.com", 995), mail.POP3SSL(),
//	    mail.POP3Username("u"), mail.POP3Password("p"))
func FetchList(opts ...FetchOption) ([]map[string]interface{}, error) {
	c := defaultFetchConfig()
	for _, o := range opts {
		o(c)
	}
	conn, cleanup, err := pop3Connect(c)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	uidMap := map[int]string{}
	if uids, err := conn.Uidl(0); err == nil {
		for _, u := range uids {
			uidMap[u.ID] = u.UID
		}
	}

	list, err := conn.List(0)
	if err != nil {
		return nil, fmt.Errorf("pop3 list failed: %v", err)
	}

	out := make([]map[string]interface{}, 0, len(list))
	for _, m := range list {
		item := map[string]interface{}{
			"id":   m.ID,
			"size": m.Size,
		}
		if uid, ok := uidMap[m.ID]; ok {
			item["uid"] = uid
		}
		out = append(out, item)
	}
	return out, nil
}

// Fetch 取回指定序号的邮件并解析（RetrRaw + Parse），返回与 Parse 一致的结构化结果。
// 示例：
//
//	result, _ = mail.Fetch(mail.POP3Server("pop.qq.com", 995), mail.POP3SSL(),
//	    mail.POP3Username("u"), mail.POP3Password("p"), mail.MessageID(1))
func Fetch(opts ...FetchOption) (map[string]interface{}, error) {
	c := defaultFetchConfig()
	for _, o := range opts {
		o(c)
	}
	if c.messageID <= 0 {
		return nil, fmt.Errorf("message id is required and must > 0 (use mail.MessageID)")
	}
	conn, cleanup, err := pop3Connect(c)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	raw, err := conn.RetrRaw(c.messageID)
	if err != nil {
		return nil, fmt.Errorf("pop3 retr %d failed: %v", c.messageID, err)
	}
	return Parse(raw.String()), nil
}
