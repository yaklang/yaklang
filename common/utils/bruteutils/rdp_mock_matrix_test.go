package bruteutils

import (
	"context"
	"testing"
	"time"
)

func hitRDP(t *testing.T, addr, user, pass string) *BruteItemResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return rdpAuth.BrutePass(&BruteItem{
		Type: "rdp", Target: addr, Username: user, Password: pass, Context: ctx,
	})
}

func assertOk(t *testing.T, res *BruteItemResult, what string) {
	t.Helper()
	if !res.Ok || !res.Finished {
		t.Fatalf("%s: want ok+finished got ok=%v finished=%v", what, res.Ok, res.Finished)
	}
}

func assertFailContinue(t *testing.T, res *BruteItemResult, what string) {
	t.Helper()
	if res.Ok {
		t.Fatalf("%s: must not be ok", what)
	}
	if res.Finished {
		t.Fatalf("%s: must not finish the target", what)
	}
}

// 真机打通过的路径必须在 mock 里同样能分清 Ok vs 失败。
func TestRDPMockMatrixFromLive(t *testing.T) {
	t.Run("win7-credssp-v2-tls-drop", func(t *testing.T) {
		srv := startNLATestServerWin7(t, map[string]string{"rdpuser": "RdpPass123!"})
		assertOk(t, hitRDP(t, srv.addr(), "rdpuser", "RdpPass123!"), "win7 correct")
		assertFailContinue(t, hitRDP(t, srv.addr(), "rdpuser", "WrongPass!"), "win7 wrong")
		assertFailContinue(t, hitRDP(t, srv.addr(), "no-such-user", "x"), "win7 unknown")
	})

	t.Run("win11-credssp-v6-errorcode", func(t *testing.T) {
		srv := startNLATestServerVer(t, map[string]string{"rdpuser": "RdpPass123!"}, 6)
		assertOk(t, hitRDP(t, srv.addr(), "rdpuser", "RdpPass123!"), "win11 correct")
		assertFailContinue(t, hitRDP(t, srv.addr(), "rdpuser", "WrongPass!"), "win11 wrong")
		assertFailContinue(t, hitRDP(t, srv.addr(), "no-such-user", "x"), "win11 unknown")
	})

	t.Run("credssp-ber-long-form", func(t *testing.T) {
		srv := startNLATestServerBERLong(t, map[string]string{"administrator": "RdpPass123!"})
		assertOk(t, hitRDP(t, srv.addr(), "administrator", "RdpPass123!"), "ber-long correct")
		assertFailContinue(t, hitRDP(t, srv.addr(), "administrator", "WrongPass!"), "ber-long wrong")
		assertFailContinue(t, hitRDP(t, srv.addr(), "no-such-user", "x"), "ber-long unknown")
	})

	t.Run("xp-classic-save-session-info", func(t *testing.T) {
		srv := startClassicRDPServer(t, map[string]string{"administrator": "RdpPass123!"})
		assertOk(t, hitRDP(t, srv.addr(), "Administrator", "RdpPass123!"), "xp correct")
		assertFailContinue(t, hitRDP(t, srv.addr(), "Administrator", "WrongPass!"), "xp wrong")
		assertFailContinue(t, hitRDP(t, srv.addr(), "no-such-user", "x"), "xp unknown")
	})
}
