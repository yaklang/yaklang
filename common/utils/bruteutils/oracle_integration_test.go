package bruteutils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/oracleprobe"
)

func oracleInfo(t *testing.T, r *BruteItemResult) OracleLoginInfo {
	t.Helper()
	var info OracleLoginInfo
	if err := json.Unmarshal(r.ExtraInfo, &info); err != nil {
		t.Fatal(err)
	}
	return info
}

func TestOracleLockedUserThroughStream(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	b, err := NewMultiTargetBruteUtilEx(WithTargetTasksConcurrent(1), WithBruteCallback(func(i *BruteItem) *BruteItemResult {
		mu.Lock()
		calls[i.Username]++
		mu.Unlock()
		return oracleBrutePass(i, func(context.Context, oracleprobe.Options) error {
			if i.Username == "locked" {
				return &oracleprobe.Error{Code: 28000}
			}
			return &oracleprobe.Error{Code: 1017}
		})
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = b.StreamBruteContext(context.Background(), "oracle", []string{"127.0.0.1:1521"}, []string{"locked", "other"}, []string{"p1", "p2", "p3"}, func(r *BruteItemResult) {
		if r.Username == "locked" && !r.UserEliminated {
			t.Error("sink lost user elimination")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls["locked"] != 1 || calls["other"] != 3 {
		t.Fatalf("calls=%v", calls)
	}
}

func TestOracleTransientAfterKnownService(t *testing.T) {
	calls := 0
	r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", Username: "u", Password: "p", OracleConfig: &OracleConfig{Services: []string{"known", "flaky"}}}, func(_ context.Context, o oracleprobe.Options) error {
		calls++
		if o.Service == "known" {
			return &oracleprobe.Error{Code: 1017}
		}
		return &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}
	})
	if r.Ok || r.Finished || calls != 3 {
		t.Fatalf("calls=%d ok=%v finished=%v", calls, r.Ok, r.Finished)
	}
	info := oracleInfo(t, r)
	if info.Status != oracleprobe.AuthRejected || len(info.Attempts) != 3 || info.Attempts[2].Status != oracleprobe.Unavailable {
		t.Fatalf("evidence=%+v", info)
	}
}

func TestOracleRetryDoesNotSkipCorrectCredential(t *testing.T) {
	calls := 0
	r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", Username: "u", Password: "p", OracleConfig: &OracleConfig{Services: []string{"sales"}}}, func(context.Context, oracleprobe.Options) error {
		calls++
		if calls == 1 {
			return io.EOF
		}
		return nil
	})
	if !r.Ok || calls != 2 {
		t.Fatalf("calls=%d result=%v", calls, r)
	}
}

func TestOracleRetrySharesBudget(t *testing.T) {
	calls := 0
	start := time.Now()
	r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", Username: "u", OracleConfig: &OracleConfig{Services: []string{"a", "b"}, Timeout: 30 * time.Millisecond}}, func(context.Context, oracleprobe.Options) error { calls++; return io.EOF })
	if r.Ok || r.Finished || calls != 1 || time.Since(start) > time.Second {
		t.Fatalf("calls=%d elapsed=%v result=%v", calls, time.Since(start), r)
	}
}

func TestOracleErrorEvidence(t *testing.T) {
	for _, tc := range []struct {
		code             int
		status           oracleprobe.Status
		final, eliminate bool
	}{
		{1017, oracleprobe.AuthRejected, false, false},
		{28000, oracleprobe.AccountLocked, false, true},
		{28001, oracleprobe.PasswordExpired, false, true},
		{65162, oracleprobe.PasswordExpired, false, true},
		{28040, oracleprobe.CredentialUnsupported, false, false},
		{28041, oracleprobe.ServerError, false, false},
		{12650, oracleprobe.Unsupported, true, false},
		{12516, oracleprobe.Unavailable, false, false},
		{12514, oracleprobe.ServiceUnknown, true, false},
		{1031, oracleprobe.ServerError, false, false},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			const secret = "oracle-secret-sentinel"
			r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", Username: "u", Password: secret, OracleConfig: &OracleConfig{Services: []string{"sales"}}}, func(context.Context, oracleprobe.Options) error {
				return &oracleprobe.Error{Code: tc.code, Message: secret}
			})
			info := oracleInfo(t, r)
			if r.Ok || r.Finished != tc.final || r.UserEliminated != tc.eliminate || info.Status != tc.status || info.Attempts[0].Code != tc.code {
				t.Fatalf("result=%v info=%+v", r, info)
			}
			if strings.Contains(string(r.ExtraInfo), secret) || strings.Contains(fmt.Sprint(r.ProbeResult), secret) {
				t.Fatal("secret in diagnostics")
			}
		})
	}
}

func TestOracleServiceCacheIsolationAndCoverage(t *testing.T) {
	b, err := NewMultiTargetBruteUtilEx(WithOracleConfig(&OracleConfig{Services: []string{"missing", "sales", "hr"}}), WithBruteCallback(func(i *BruteItem) *BruteItemResult { return i.Result() }))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var mu sync.Mutex
	b.callback = func(i *BruteItem) *BruteItemResult {
		return oracleBrutePass(i, func(_ context.Context, o oracleprobe.Options) error {
			mu.Lock()
			counts[o.Service]++
			mu.Unlock()
			if o.Service == "missing" {
				return &oracleprobe.Error{Code: 12514}
			}
			return &oracleprobe.Error{Code: 1017}
		})
	}
	for round := 1; round <= 2; round++ {
		if err = b.StreamBruteContext(context.Background(), "oracle", []string{"127.0.0.1:1521"}, []string{"u"}, []string{"p1", "p2", "p3"}, nil); err != nil {
			t.Fatal(err)
		}
		if counts["missing"] != round || counts["sales"] != 3*round || counts["hr"] != 3*round {
			t.Fatalf("round=%d counts=%v", round, counts)
		}
	}
}

func TestOracleCacheBoundsAndExpiry(t *testing.T) {
	c := &oracleServiceCache{entries: make(map[oracleServiceKey]oracleServiceObservation)}
	for n := 0; n < oracleCacheLimit+5; n++ {
		c.remember(oracleServiceKey{target: fmt.Sprint(n), service: "s"}, true)
	}
	if len(c.entries) != oracleCacheLimit {
		t.Fatal(len(c.entries))
	}
	k := oracleServiceKey{target: "expired", service: "s"}
	c.entries[k] = oracleServiceObservation{unknown: true, expires: time.Now().Add(-time.Second)}
	if _, ok := c.observation(k); ok {
		t.Fatal("expired observation reused")
	}
	if _, ok := c.entries[k]; ok {
		t.Fatal("expired observation retained")
	}
}

func TestOracleServiceOrderingWithConcurrentDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(withOracleServiceCache(context.Background()))
	cache := ctx.Value(oracleCacheKey{}).(*oracleServiceCache)
	services := []string{"sales", "hr", "inventory"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			for _, service := range services {
				key := oracleServiceKey{target: "127.0.0.1", service: service}
				cache.remember(key, false)
				cache.mu.Lock()
				delete(cache.entries, key)
				cache.mu.Unlock()
			}
		}
	}()
	defer func() { cancel(); <-done }()
	for n := 0; n < 300; n++ {
		counts := make(map[string]int)
		oracleBrutePass(&BruteItem{Context: ctx, Target: "127.0.0.1", Username: "u", OracleConfig: &OracleConfig{Services: services}}, func(_ context.Context, o oracleprobe.Options) error {
			counts[o.Service]++
			return &oracleprobe.Error{Code: 1017}
		})
		for _, service := range services {
			if counts[service] != 1 {
				t.Fatalf("discovery/expiry omitted or duplicated a service: %v", counts)
			}
		}
	}
}

func TestOracleConfigReachesStream(t *testing.T) {
	mode := false
	config := &OracleConfig{Services: []string{"CUSTOM"}, SID: true, SysDBA: &mode, TLS: &tls.Config{ServerName: "db.example"}, Encryption: "required", Timeout: time.Second}
	calls := 0
	b, err := NewMultiTargetBruteUtilEx(WithOracleConfig(config), WithBruteCallback(func(i *BruteItem) *BruteItemResult {
		return oracleBrutePass(i, func(ctx context.Context, o oracleprobe.Options) error {
			calls++
			if o.Service != "CUSTOM" || !o.SID || o.SysDBA || o.TLS == nil || o.TLS.InsecureSkipVerify || o.TLS.ServerName != "db.example" || o.Encryption != oracleprobe.EncryptionRequired {
				t.Fatalf("configuration not forwarded")
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > time.Second {
				t.Fatal("timeout not forwarded")
			}
			return nil
		})
	}))
	if err != nil {
		t.Fatal(err)
	}
	config.Services[0] = "MUTATED"
	config.TLS.ServerName = "mutated"
	mode = true
	if err = b.StreamBruteContext(context.Background(), "oracle", []string{"127.0.0.1:1521"}, []string{"sys"}, []string{"p"}, func(r *BruteItemResult) {
		info := oracleInfo(t, r)
		if !r.Ok || info.SysDBA || !info.SID || info.Attempts[0].Service != "CUSTOM" {
			t.Fatal("lost evidence")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestOracleInvalidOptionsBeforeNetwork(t *testing.T) {
	for _, cfg := range []*OracleConfig{{Services: []string{"x)(injected)"}}, {Encryption: "requiredd"}, {SID: true}, {Timeout: -time.Second}} {
		called := false
		r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", OracleConfig: cfg}, func(context.Context, oracleprobe.Options) error { called = true; return nil })
		if called || r.Ok || !r.Finished {
			t.Fatal("invalid config reached network")
		}
	}
}

func TestStreamPreservesProbeFields(t *testing.T) {
	i := &BruteItem{Type: "oracle", Target: "x", Username: "u"}
	legacy := i.Result()
	legacy.UserEliminated = true
	legacy.OnlyNeedPassword = true
	legacy.ProbeResult = &core.Result{Outcome: core.OutcomeUnknown, Err: core.ErrIO, RetryAfter: time.Second, Transport: core.TransportTLS}
	r := coreResultFromLegacy(context.Background(), i, legacy)
	if !r.UserEliminated || !r.OnlyNeedPassword || r.RetryAfter != time.Second || r.Transport != core.TransportTLS || r.Err != core.ErrIO || r.Outcome != core.OutcomeUnknown {
		t.Fatalf("fields dropped: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if coreResultFromLegacy(ctx, i, legacy).Outcome != core.OutcomeCancelled {
		t.Fatal("cancel not preserved")
	}
}

func TestOracleUnsupportedCredentialDoesNotEliminateTarget(t *testing.T) {
	for _, err := range []error{oracleprobe.ErrInvalidCredentials, oracleprobe.ErrUnsupportedVerifier, &oracleprobe.Error{Code: 28040}, &oracleprobe.Error{Code: 28041}, &oracleprobe.Failure{Stage: "auth-challenge", Err: errors.New("invalid padding")}} {
		r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", Username: "u", OracleConfig: &OracleConfig{Services: []string{"s"}}}, func(context.Context, oracleprobe.Options) error { return err })
		if r.Finished || r.UserEliminated || r.Ok {
			t.Fatalf("candidate eliminated target: %v", err)
		}
	}
}

func TestOracleDialerPreservesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := defaultDialer.DialContext(ctx, "tcp", "127.0.0.1:1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost dial cause: %v", err)
	}
}
