package postgres

import (
	"context"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// ---- 三种认证方式 ----

func TestPostgresCleartextSuccessAndFailure(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "cleartext"
	sim.validUser, sim.validPass = "postgres", "PgSecret99"
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "postgres", "PgSecret99"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res := pgProbeOnce(t, sim.addr, "postgres", "wrong"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v", res.Outcome)
	}
	if res := pgProbeOnce(t, sim.addr, "nobody", "x"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed for unknown user, got %v", res.Outcome)
	}
}

func TestPostgresMD5SuccessAndFailure(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "md5"
	sim.validUser, sim.validPass = "postgres", "Md5-密码🔐"
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "postgres", "Md5-密码🔐"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res := pgProbeOnce(t, sim.addr, "postgres", "nope"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v", res.Outcome)
	}
}

func TestPostgresSCRAMSuccessAndFailure(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "scram"
	sim.validUser, sim.validPass = "pguser", "Scram-Passw0rd!"
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "pguser", "Scram-Passw0rd!"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	sim.mu.Lock()
	scramOK := sim.scramOK
	sim.mu.Unlock()
	if !scramOK {
		t.Fatal("server-side SCRAM verification did not pass")
	}

	if res := pgProbeOnce(t, sim.addr, "pguser", "bad-password"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res := pgProbeOnce(t, sim.addr, "otheruser", "Scram-Passw0rd!"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("unknown user: want auth-failed, got %v", res.Outcome)
	}
}

func TestPostgresSCRAMUnicodePassword(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "scram"
	sim.validUser, sim.validPass = "用户", "密码🔐123"
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "用户", "密码🔐123"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("unicode SCRAM: want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

// ---- TLS ----

func TestPostgresTLSUpgrade(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "scram"
	sim.sslMode = "on"
	sim.validUser, sim.validPass = "pg", "tls-pass"
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "pg", "tls-pass"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res := pgProbeOnce(t, sim.addr, "pg", "tls-pass"); res.Transport != core.TransportTLS {
		t.Fatalf("transport=%v want tls", res.Transport)
	}
}

func TestPostgresServerRejectsSSLFallbackPlaintext(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "cleartext"
	sim.sslMode = "reject" // 回 'N'
	sim.validUser, sim.validPass = "pg", "fb"
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "pg", "fb"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success over plaintext, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res := pgProbeOnce(t, sim.addr, "pg", "fb"); res.Transport != core.TransportPlainTCP {
		t.Fatalf("transport=%v want tcp", res.Transport)
	}
}

func TestPostgresRequireSSLStrictPolicy(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "cleartext"
	sim.sslMode = "off"
	sim.validUser, sim.validPass = "pg", "x"
	sim.start()
	defer sim.stop()

	target, _ := core.ParseTarget(sim.addr)
	var p Prober
	res := p.Probe(context.Background(), target, core.Credential{Username: "pg", Password: "x"},
		core.Options{Timeout: 3 * time.Second, TLSPolicy: core.TLSStrict})
	if res.Outcome != core.OutcomeTLSRequired {
		t.Fatalf("want tls-required, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

// ---- 错误分类 ----

func TestPostgresErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		msg    string
		want   core.Outcome
		wantUE bool // UserEliminated
	}{
		{"password-auth-failed", "28P01", "password authentication failed", core.OutcomeAuthFailed, false},
		{"role-not-exist", "28000", `role "ghost" does not exist`, core.OutcomeAuthFailed, true},
		{"pg-hba-reject", "28000", "no pg_hba.conf entry for host \"1.2.3.4\", user \"u\", database \"postgres\"", core.OutcomeTargetUnavailable, false},
		{"too-many-connections", "53300", "too many connections", core.OutcomeRateLimited, false},
		{"cannot-connect-now", "57P03", "the database system is starting up", core.OutcomeRateLimited, false},
		{"conn-exception", "08006", "connection failure", core.OutcomeTargetUnavailable, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim := newPGSim(t)
			sim.authMethod = "trust"
			sim.scriptErr = &pgError{Severity: "FATAL", Code: tc.code, Message: tc.msg}
			sim.start()
			defer sim.stop()

			res := pgProbeOnce(t, sim.addr, "u", "p")
			if res.Outcome != tc.want {
				t.Fatalf("want %v got %v (%s)", tc.want, res.Outcome, res.ErrDetail)
			}
			if res.UserEliminated != tc.wantUE {
				t.Fatalf("UserEliminated=%v want %v", res.UserEliminated, tc.wantUE)
			}
		})
	}
}

// ---- 网络异常 ----

func TestPostgresConnectionRefused(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()

	if res := pgProbeOnce(t, addr, "u", "p"); res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
}

func TestPostgresCloseAfterStartup(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "trust"
	sim.closeAfterStartup = true
	sim.start()
	defer sim.stop()

	res := pgProbeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
}

func TestPostgresNotPostgresBanner(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9.6\r\n\r\n"))
				time.Sleep(500 * time.Millisecond)
			}(conn)
		}
	}()

	res := pgProbeOnce(t, l.Addr().String(), "u", "p")
	if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
		t.Fatalf("banner mismatch must not classify as auth: %v", res.Outcome)
	}
}

func TestPostgresReadTimeout(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				time.Sleep(10 * time.Second) // 黑洞
			}(conn)
		}
	}()

	start := time.Now()
	target, _ := core.ParseTarget(l.Addr().String())
	var p Prober
	res := p.Probe(context.Background(), target, core.Credential{Username: "u", Password: "p"},
		core.Options{Timeout: 500 * time.Millisecond})
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout not enforced")
	}
}

// ---- Context 取消 ----

func TestPostgresContextCancelDuringAuth(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "trust"
	// 模拟 hang：认证阶段不回包（closeAfterStartup 后保持连接但不写）
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// 读取 startup 后不响应
				buf := make([]byte, 512)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	_ = sim

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	target, _ := core.ParseTarget(l.Addr().String())
	var p Prober
	res := p.Probe(ctx, target, core.Credential{Username: "u", Password: "p"},
		core.Options{Timeout: 20 * time.Second})
	if res.Outcome != core.OutcomeCancelled {
		t.Fatalf("want cancelled, got %v", res.Outcome)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cancel latency too high: %v", time.Since(start))
	}
}

// ---- 泄漏与资源 ----

func TestPostgresNoCredentialLeak(t *testing.T) {
	sentinel := "SENTINEL-pg-pass-绝密"
	sim := newPGSim(t)
	sim.authMethod = "scram"
	sim.validUser, sim.validPass = "u", "other"
	sim.start()
	defer sim.stop()

	res := pgProbeOnce(t, sim.addr, "u", sentinel)
	if strings.Contains(res.String(), sentinel) {
		t.Fatalf("leak in result: %s", res.String())
	}
	if strings.Contains(res.ErrDetail, sentinel) {
		t.Fatalf("leak in detail: %s", res.ErrDetail)
	}
}

func TestPostgresNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		sim := newPGSim(t)
		sim.authMethod = "scram"
		sim.validUser, sim.validPass = "u", "p"
		sim.start()
		if res := pgProbeOnce(t, sim.addr, "u", "p"); res.Outcome != core.OutcomeAuthSuccess {
			t.Fatalf("iter %d: %v (%s)", i, res.Outcome, res.ErrDetail)
		}
		sim.stop()
	}
	time.Sleep(200 * time.Millisecond)
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, n)
	}
}

func TestPostgresEmptyPasswordAndUsername(t *testing.T) {
	sim := newPGSim(t)
	sim.authMethod = "cleartext"
	sim.validUser, sim.validPass = "", ""
	sim.start()
	defer sim.stop()

	if res := pgProbeOnce(t, sim.addr, "", ""); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("empty creds: want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}
