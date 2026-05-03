package aibalance

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 关键词: user_identity_test, 优先级 / IPv6 / 不同 source_kind hash 不同

// fakeConn 提供一个最小可用的 net.Conn 实现，仅 RemoteAddr 被测试使用。
type fakeConn struct {
	addr net.Addr
}

func (f *fakeConn) Read(b []byte) (int, error)  { return 0, nil }
func (f *fakeConn) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeConn) Close() error                { return nil }
func (f *fakeConn) LocalAddr() net.Addr         { return f.addr }
func (f *fakeConn) RemoteAddr() net.Addr        { return f.addr }
func (f *fakeConn) SetDeadline(time.Time) error { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func newFakeTCPConn(ipStr string) *fakeConn {
	return &fakeConn{addr: &net.TCPAddr{IP: net.ParseIP(ipStr), Port: 23456}}
}

func packetWith(headers ...string) []byte {
	body := "POST /v1/chat/completions HTTP/1.1\r\n"
	for _, h := range headers {
		body += h + "\r\n"
	}
	body += "Host: api.example.com\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"
	return []byte(body)
}

func TestExtractUserIdentity_APIKeyPriority(t *testing.T) {
	conn := newFakeTCPConn("1.2.3.4")
	pkt := packetWith("Authorization: Bearer sk-paid-001", "Trace-ID: trace-xx")
	key := &Key{Key: "sk-paid-001"}

	kind, hash := extractUserIdentity(pkt, conn, key, false)
	assert.Equal(t, SourceKindAPIKey, kind, "paid request must hash by api_key")
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 32)
}

func TestExtractUserIdentity_TraceIDOverIP(t *testing.T) {
	conn := newFakeTCPConn("9.9.9.9")
	pkt := packetWith("Trace-ID: trace-aaa")

	kind, hash := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, SourceKindFreeTrace, kind, "free user with Trace-ID must use free_trace bucket")
	assert.Len(t, hash, 32)
}

func TestExtractUserIdentity_IPFallback(t *testing.T) {
	conn := newFakeTCPConn("203.0.113.7")
	pkt := packetWith() // no Trace-ID, no Authorization

	kind, hash := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, SourceKindFreeIP, kind, "no Trace-ID + no api key must fall back to free_ip")
	assert.Len(t, hash, 32)
}

func TestExtractUserIdentity_DifferentSourceKindDifferentHash(t *testing.T) {
	raw := "shared-token"
	hKey := fingerprintIdentity(SourceKindAPIKey, raw)
	hTrace := fingerprintIdentity(SourceKindFreeTrace, raw)
	hIP := fingerprintIdentity(SourceKindFreeIP, raw)
	assert.NotEqual(t, hKey, hTrace, "same raw under different source_kind must produce different fingerprints")
	assert.NotEqual(t, hTrace, hIP)
	assert.NotEqual(t, hKey, hIP)
}

func TestExtractUserIdentity_IPv6Fallback(t *testing.T) {
	conn := &fakeConn{addr: &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 9999}}
	pkt := packetWith()
	kind, hash := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, SourceKindFreeIP, kind)
	assert.Len(t, hash, 32)
}

func TestExtractUserIdentity_FreeModelIgnoresKey(t *testing.T) {
	conn := newFakeTCPConn("4.3.2.1")
	pkt := packetWith("Authorization: Bearer sk-leaked", "Trace-ID: trace-bbb")

	// 即使带了 Authorization，但因为 isFreeModel=true 我们应当走 trace 而不是 api_key。
	kind, hash := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, SourceKindFreeTrace, kind)
	assert.Len(t, hash, 32)
}

func TestExtractUserIdentity_NoKeyButHasBearerOnPaid(t *testing.T) {
	// 极端兜底：key=nil 但 Authorization 头里有 token，我们仍然能按 api_key 桶记录。
	conn := newFakeTCPConn("5.5.5.5")
	pkt := packetWith("Authorization: Bearer sk-orphan-token")
	kind, hash := extractUserIdentity(pkt, conn, nil, false)
	assert.Equal(t, SourceKindAPIKey, kind)
	assert.Len(t, hash, 32)
}

func TestExtractUserIdentity_StableHash(t *testing.T) {
	conn := newFakeTCPConn("7.7.7.7")
	pkt := packetWith("Trace-ID: trace-stable")

	kind1, h1 := extractUserIdentity(pkt, conn, nil, true)
	kind2, h2 := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, kind1, kind2)
	assert.Equal(t, h1, h2, "extractUserIdentity must be deterministic for the same input")
}

func TestParseBearerToken(t *testing.T) {
	assert.Equal(t, "abc", parseBearerToken("Bearer abc"))
	assert.Equal(t, "abc", parseBearerToken("bearer abc"))
	assert.Equal(t, "", parseBearerToken("Basic abc"))
	assert.Equal(t, "", parseBearerToken(""))
	assert.Equal(t, "", parseBearerToken("Bearer "))
}

func TestRemoteIPOf_Nil(t *testing.T) {
	assert.Equal(t, "", remoteIPOf(nil))
	conn := &fakeConn{addr: nil}
	assert.Equal(t, "", remoteIPOf(conn))
}
