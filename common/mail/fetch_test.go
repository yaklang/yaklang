package mail

import "testing"

// TestFetchOptions 验证 POP3 functional options 与默认值
func TestFetchOptions(t *testing.T) {
	c := defaultFetchConfig()
	if c.port != 110 {
		t.Errorf("default pop3 port = %d, want 110", c.port)
	}
	opts := []FetchOption{
		POP3Server("pop.example.com", 995),
		POP3SSL(), POP3SkipVerify(),
		POP3Username("u"), POP3Password("p"),
		MessageID(3),
	}
	for _, o := range opts {
		o(c)
	}
	if c.host != "pop.example.com" || c.port != 995 {
		t.Errorf("server = %s:%d", c.host, c.port)
	}
	if !c.ssl || !c.skipVerify {
		t.Errorf("ssl/skipVerify not set")
	}
	if c.username != "u" || c.password != "p" {
		t.Errorf("creds wrong")
	}
	if c.messageID != 3 {
		t.Errorf("messageID = %d", c.messageID)
	}
}

// TestFetch_MissingConfig 缺服务器参数应返回错误（不触发网络）
func TestFetch_MissingConfig(t *testing.T) {
	if _, err := Fetch(POP3Username("u"), POP3Password("p"), MessageID(1)); err == nil {
		t.Errorf("expected error for missing pop3 server")
	}
}

// TestFetchList_MissingUsername 缺账号应返回错误
func TestFetchList_MissingUsername(t *testing.T) {
	if _, err := FetchList(POP3Server("pop.example.com", 995), POP3SSL()); err == nil {
		t.Errorf("expected error for missing username")
	}
}
