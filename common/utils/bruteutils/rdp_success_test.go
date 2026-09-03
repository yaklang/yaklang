package bruteutils

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestRDPBruteSuccessCases 给「爆破成功」看的证据：在可控靶机上把正确密码
// 混进字典，调度器必须打出 Ok=true，且错密码不得误报。
//
// 覆盖：
//   - CredSSP v2（Win7/2008 风格）
//   - CredSSP v6（Win10/2016+ 风格）
//   - Unicode 用户/密码
//   - 流式调度器整段爆破（多用户×多密码，命中即停）
func TestRDPBruteSuccessCases(t *testing.T) {
	type hit struct {
		name string
		addr string
		user string
		pass string
	}
	var hits []hit
	report := func(h hit, ok, finished bool, extra string) {
		t.Helper()
		line := fmt.Sprintf("BRUTE SUCCESS  target=%s user=%q pass=%q ok=%v finished=%v %s",
			h.addr, h.user, h.pass, ok, finished, extra)
		if !ok {
			t.Errorf("EXPECTED SUCCESS but not ok: %s", line)
			return
		}
		hits = append(hits, h)
		t.Log(line)
	}

	t.Run("credssp-v2-ascii", func(t *testing.T) {
		srv := startNLATestServerVer(t, map[string]string{"administrator": "P@ssw0rd!"}, 2)
		res := rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: srv.addr(), Username: "administrator", Password: "P@ssw0rd!",
			Context: context.Background(),
		})
		report(hit{"credssp-v2", srv.addr(), "administrator", "P@ssw0rd!"}, res.Ok, res.Finished, "nla=v2")
		bad := rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: srv.addr(), Username: "administrator", Password: "wrong",
			Context: context.Background(),
		})
		if bad.Ok {
			t.Fatal("v2 wrong password must not succeed")
		}
	})

	t.Run("credssp-v6-ascii", func(t *testing.T) {
		srv := startNLATestServerVer(t, map[string]string{"rdpuser": "RdpPass123!"}, 6)
		res := rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: srv.addr(), Username: "rdpuser", Password: "RdpPass123!",
			Context: context.Background(),
		})
		report(hit{"credssp-v6", srv.addr(), "rdpuser", "RdpPass123!"}, res.Ok, res.Finished, "nla=v6")
	})

	t.Run("credssp-v6-unicode", func(t *testing.T) {
		srv := startNLATestServerVer(t, map[string]string{"unicode用户": "密码🔐123"}, 6)
		res := rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: srv.addr(), Username: "unicode用户", Password: "密码🔐123",
			Context: context.Background(),
		})
		report(hit{"unicode", srv.addr(), "unicode用户", "密码🔐123"}, res.Ok, res.Finished, "nla=v6 unicode")
	})

	t.Run("scheduler-finds-password-in-dict", func(t *testing.T) {
		srv := startNLATestServerVer(t, map[string]string{"admin": "CorrectHorse!"}, 6)
		util, err := NewMultiTargetBruteUtilEx(
			WithBruteCallback(rdpAuth.BrutePass),
			WithOkToStop(true),
			WithTargetsConcurrent(1),
			WithTargetTasksConcurrent(1),
			WithFinishingThreshold(1),
		)
		if err != nil {
			t.Fatal(err)
		}

		users := []string{"guest", "admin", "administrator"}
		passes := []string{"123456", "admin", "admin123", "CorrectHorse!", "password", "P@ssw0rd"}
		var found *BruteItemResult
		start := time.Now()
		err = util.StreamBruteContext(context.Background(), "rdp",
			[]string{srv.addr()}, users, passes,
			func(res *BruteItemResult) {
				t.Logf("attempt user=%q ok=%v finished=%v", res.Username, res.Ok, res.Finished)
				if res.Ok {
					cp := *res
					found = &cp
				}
			})
		if err != nil {
			t.Fatal(err)
		}
		if found == nil {
			t.Fatal("scheduler did not report a hit")
		}
		if found.Username != "admin" || found.Password != "CorrectHorse!" {
			t.Fatalf("hit wrong creds user=%q pass=%q", found.Username, found.Password)
		}
		report(hit{"scheduler-dict", srv.addr(), found.Username, found.Password},
			true, found.Finished, fmt.Sprintf("dict hunt in %s", time.Since(start).Round(time.Millisecond)))
	})

	t.Run("xrdp-docker-correct-creds", func(t *testing.T) {
		if os.Getenv("YAK_BRUTE_REAL") != "1" {
			t.Skip("set YAK_BRUTE_REAL=1 and start yak-rdp-nla on :33389")
		}
		addr := "127.0.0.1:33389"
		if !rdpHostReachable(addr) {
			t.Skipf("%s not reachable", addr)
		}
		res := rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: addr, Username: "rdpuser", Password: "RdpPass123!",
			Context: context.Background(),
		})
		report(hit{"xrdp-24.04", addr, "rdpuser", "RdpPass123!"}, res.Ok, res.Finished, "xrdp SSL/session")
		uni := rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: addr, Username: "unicode用户", Password: "密码🔐123",
			Context: context.Background(),
		})
		report(hit{"xrdp-unicode", addr, "unicode用户", "密码🔐123"}, uni.Ok, uni.Finished, "xrdp unicode")
	})

	t.Cleanup(func() {
		t.Log("==== 爆破成功清单 ====")
		if len(hits) == 0 {
			t.Error("no success hits recorded")
			return
		}
		for _, h := range hits {
			t.Logf("  HIT  %-24s  %s / %s", h.name, h.user, strings.Repeat("*", len(h.pass)))
			t.Logf("       user=%q pass=%q @ %s", h.user, h.pass, h.addr)
		}
		t.Logf("total brute SUCCESS cases: %d", len(hits))
	})
}
