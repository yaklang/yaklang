package minimartian

import (
	"net"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// fakeAddrConn is a net.Conn whose LocalAddr/RemoteAddr return controlled
// values, so tests can verify which address GetDialDetectionAddr picks.
type fakeAddrConn struct {
	net.Conn
	local  net.Addr
	remote net.Addr
}

func (f *fakeAddrConn) LocalAddr() net.Addr  { return f.local }
func (f *fakeAddrConn) RemoteAddr() net.Addr { return f.remote }

func addrFor(h string, p int) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(h), Port: p}
}

// newCtxWithSession builds a Context backed by a fresh Session so we can set
// session values without a real connection.
func newCtxWithSession(t *testing.T) *Context {
	t.Helper()
	s, err := newSession(nil, nil)
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	ctx, err := withSession(s)
	if err != nil {
		t.Fatalf("withSession: %v", err)
	}
	return ctx
}

// TestGetDialDetectionAddr_ConnectedHost returns the connected host:port when set.
func TestGetDialDetectionAddr_ConnectedHost(t *testing.T) {
	ctx := newCtxWithSession(t)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, "open.imaa.edu.cn")
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, 443)

	// RemoteAddr intentionally wrong to prove it is NOT used here.
	conn := &fakeAddrConn{remote: addrFor("10.16.0.1", 61643), local: addrFor("222.31.233.160", 443)}

	got := GetDialDetectionAddr(ctx, conn)
	want := utils.HostPort("open.imaa.edu.cn", 443)
	if got != want {
		t.Fatalf("GetDialDetectionAddr = %q, want %q", got, want)
	}
}

// TestGetDialDetectionAddr_TransparentHijackNoTarget returns "" (not RemoteAddr)
// when there is no connected host, matching the transparent-hijack (TUN) entry
// where the upstream target is unknown before the TLS handshake. Returning the
// client's RemoteAddr would make the H2 probe dial the client and block the
// handshake (webfuzzer HTTPS over TUN).
func TestGetDialDetectionAddr_TransparentHijackNoTarget(t *testing.T) {
	ctx := newCtxWithSession(t)
	// no ConnectedToHost/Port, no IsListenedConn
	conn := &fakeAddrConn{remote: addrFor("10.16.0.1", 61643), local: addrFor("222.31.233.160", 443)}

	got := GetDialDetectionAddr(ctx, conn)
	if got != "" {
		t.Fatalf("GetDialDetectionAddr = %q, want \"\" (must not dial client RemoteAddr %q)", got, conn.RemoteAddr().String())
	}
}

// TestGetDialDetectionAddr_ListenedNoTarget returns "" for a listened conn
// without a bound target, rather than the (now removed) RemoteAddr fallback.
func TestGetDialDetectionAddr_ListenedNoTarget(t *testing.T) {
	ctx := newCtxWithSession(t)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_IsListenedConn, true)

	conn := &fakeAddrConn{remote: addrFor("10.16.0.1", 61643), local: addrFor("222.31.233.160", 443)}

	got := GetDialDetectionAddr(ctx, conn)
	if got != "" {
		t.Fatalf("GetDialDetectionAddr = %q, want \"\" for listened conn with no target", got)
	}
}

// TestGetDialDetectionAddr_MissingPort returns "" when host is set but port is 0,
// guarding against half-bound state.
func TestGetDialDetectionAddr_MissingPort(t *testing.T) {
	ctx := newCtxWithSession(t)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, "open.imaa.edu.cn")
	// port intentionally 0

	conn := &fakeAddrConn{remote: addrFor("10.16.0.1", 61643), local: addrFor("222.31.233.160", 443)}

	got := GetDialDetectionAddr(ctx, conn)
	if got != "" {
		t.Fatalf("GetDialDetectionAddr = %q, want \"\" when port missing", got)
	}
}
