package bruteutils

import (
	"context"
	"os"
	"testing"
	"time"
)

// 真实 Windows NLA 爆破（默认跳过）。
//
//	YAK_BRUTE_RDP_ADDR=127.0.0.1:13389 YAK_BRUTE_RDP_USER=rdpuser YAK_BRUTE_RDP_PASS=RdpPass123! \
//	  go test ./common/utils/bruteutils/ -run TestRDPWindowsLive -v
func TestRDPWindowsLiveBrute(t *testing.T) {
	addr := os.Getenv("YAK_BRUTE_RDP_ADDR")
	if addr == "" {
		t.Skip("set YAK_BRUTE_RDP_ADDR to a Windows RDP you control")
	}
	user := os.Getenv("YAK_BRUTE_RDP_USER")
	if user == "" {
		user = "rdpuser"
	}
	pass := os.Getenv("YAK_BRUTE_RDP_PASS")
	if pass == "" {
		pass = "RdpPass123!"
	}

	hit := func(u, p string) *BruteItemResult {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		start := time.Now()
		res := rdpAuth.BrutePass(&BruteItem{Type: "rdp", Target: addr, Username: u, Password: p, Context: ctx})
		t.Logf("user=%q ok=%v finished=%v elapsed=%s", u, res.Ok, res.Finished, time.Since(start).Round(100*time.Millisecond))
		return res
	}

	ok := hit(user, pass)
	if !ok.Ok || !ok.Finished {
		t.Errorf("correct creds: want ok+finished got ok=%v finished=%v", ok.Ok, ok.Finished)
	}
	bad := hit(user, "WrongPass!")
	if bad.Ok {
		t.Errorf("wrong password must not be ok")
	}
	if bad.Finished {
		t.Errorf("wrong password must not mark target finished")
	}
	unk := hit("no-such-user", "x")
	if unk.Ok {
		t.Errorf("unknown user must not be ok")
	}
}

func TestRDPWindowsLiveDictHunt(t *testing.T) {
	addr := os.Getenv("YAK_BRUTE_RDP_ADDR")
	if addr == "" {
		t.Skip("set YAK_BRUTE_RDP_ADDR")
	}
	user := os.Getenv("YAK_BRUTE_RDP_USER")
	if user == "" {
		user = "rdpuser"
	}
	pass := os.Getenv("YAK_BRUTE_RDP_PASS")
	if pass == "" {
		pass = "RdpPass123!"
	}

	util, err := NewMultiTargetBruteUtilEx(
		WithBruteCallback(rdpAuth.BrutePass),
		WithOkToStop(true),
		WithTargetsConcurrent(1),
		WithTargetTasksConcurrent(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	users := []string{"guest", user, "administrator"}
	passes := []string{"123456", "admin", pass, "password"}
	var found *BruteItemResult
	start := time.Now()
	if err := util.StreamBruteContext(context.Background(), "rdp", []string{addr}, users, passes, func(res *BruteItemResult) {
		t.Logf("try user=%q ok=%v finished=%v", res.Username, res.Ok, res.Finished)
		if res.Ok {
			cp := *res
			found = &cp
		}
	}); err != nil {
		t.Fatal(err)
	}
	if found == nil {
		t.Fatal("scheduler did not hit")
	}
	if found.Username != user || found.Password != pass {
		t.Fatalf("hit wrong creds user=%q pass=%q", found.Username, found.Password)
	}
	t.Logf("DICT HIT user=%q in %s", found.Username, time.Since(start).Round(time.Millisecond))
}
