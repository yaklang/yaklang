package mail

import "testing"

// TestSendOptions 验证 functional options 正确应用到配置
func TestSendOptions(t *testing.T) {
	c := &sendConfig{tls: tlsAuto, authMethod: "auto"}
	opts := []SendOption{
		Server("smtp.example.com", 465),
		Username("u"), Password("p"),
		SSL(), SkipVerify(), AuthMethod("login"),
		From("a@b.com"),
		To("c@d.com", "e@f.com"), Cc("g@h.com"), Bcc("i@j.com"),
		Subject("test"), HTML("<b>x</b>"), Text("x"),
		Attach("/tmp/a.pdf"),
		Header("X-Test", "1"),
	}
	for _, o := range opts {
		o(c)
	}
	if c.host != "smtp.example.com" || c.port != 465 {
		t.Errorf("server = %s:%d", c.host, c.port)
	}
	if c.username != "u" || c.password != "p" {
		t.Errorf("auth creds wrong")
	}
	if c.tls != tlsSSL {
		t.Errorf("tls = %v, want ssl", c.tls)
	}
	if !c.skipVerify {
		t.Errorf("skipVerify not set")
	}
	if c.authMethod != "login" {
		t.Errorf("authMethod = %v", c.authMethod)
	}
	if c.from != "a@b.com" {
		t.Errorf("from = %v", c.from)
	}
	if len(c.to) != 2 || c.to[0] != "c@d.com" {
		t.Errorf("to = %v", c.to)
	}
	if len(c.cc) != 1 || len(c.bcc) != 1 {
		t.Errorf("cc/bcc wrong")
	}
	if c.subject != "test" || c.html != "<b>x</b>" || c.text != "x" {
		t.Errorf("content fields wrong")
	}
	if len(c.attachments) != 1 || c.attachments[0] != "/tmp/a.pdf" {
		t.Errorf("attachments = %v", c.attachments)
	}
	if c.headers["X-Test"] != "1" {
		t.Errorf("headers = %v", c.headers)
	}
}

// TestSend_MissingServer 缺服务器参数应返回错误（不触发网络）
func TestSend_MissingServer(t *testing.T) {
	if err := Send(From("a@b.com"), To("c@d.com")); err == nil {
		t.Errorf("expected error for missing server")
	}
}

// TestSend_MissingRecipient 缺收件人应返回错误
func TestSend_MissingRecipient(t *testing.T) {
	if err := Send(Server("smtp.example.com", 25), From("a@b.com")); err == nil {
		t.Errorf("expected error for missing recipient")
	}
}
