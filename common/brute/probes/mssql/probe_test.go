package mssql

import (
	"context"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// ---- 认证路径 ----

func TestMSSQLPlaintextSuccessAndFailure(t *testing.T) {
	sim := newMssqlSim(t)
	sim.validUser, sim.validPass = "sa", "Sql-Pass123!"
	sim.encrypt = encryptNotSup
	sim.start()
	defer sim.stop()

	if res := mssqlProbeOnce(t, sim.addr, "sa", "Sql-Pass123!"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	// 服务端必须正确解码混淆后的密码
	if got := sim.lastLogin().password; got != "Sql-Pass123!" {
		t.Fatalf("server decoded password %q", got)
	}
	if got := sim.lastLogin().username; got != "sa" {
		t.Fatalf("server decoded username %q", got)
	}

	if res := mssqlProbeOnce(t, sim.addr, "sa", "wrong"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res := mssqlProbeOnce(t, sim.addr, "ghost", "x"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("unknown user: want auth-failed, got %v", res.Outcome)
	}
}

func TestMSSQLUnicodeCredentials(t *testing.T) {
	sim := newMssqlSim(t)
	sim.validUser, sim.validPass = "用户sa", "密码🔐密码"
	sim.start()
	defer sim.stop()

	if res := mssqlProbeOnce(t, sim.addr, "用户sa", "密码🔐密码"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if got := sim.lastLogin().password; got != "密码🔐密码" {
		t.Fatalf("unicode password decoded %q", got)
	}
}

func TestMSSQLEmptyPasswordAndUsername(t *testing.T) {
	sim := newMssqlSim(t)
	sim.validUser, sim.validPass = "", ""
	sim.start()
	defer sim.stop()

	if res := mssqlProbeOnce(t, sim.addr, "", ""); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("empty creds: want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMSSQLTLSRequiredByServer(t *testing.T) {
	// 服务端要求加密（encryptReq）：探针必须走 TDS 内嵌 TLS。
	sim := newMssqlSim(t)
	sim.validUser, sim.validPass = "sa", "Tls-Pass1!"
	sim.encrypt = encryptReq
	sim.start()
	defer sim.stop()

	res := mssqlProbeOnce(t, sim.addr, "sa", "Tls-Pass1!")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res.Transport != core.TransportTLS {
		t.Fatalf("transport=%v want tls", res.Transport)
	}
	// 密码必须经服务端正确解码（TLS 信道 + 混淆）
	if got := sim.lastLogin().password; got != "Tls-Pass1!" {
		t.Fatalf("server decoded %q", got)
	}
}

func TestMSSQLTLSEncryptOn(t *testing.T) {
	sim := newMssqlSim(t)
	sim.validUser, sim.validPass = "sa", "Enc-On!"
	sim.encrypt = encryptOn
	sim.start()
	defer sim.stop()

	if res := mssqlProbeOnce(t, sim.addr, "sa", "Enc-On!"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMSSQLPlaintextPolicyAgainstTLSServer(t *testing.T) {
	// 策略明文 + 服务端要求加密 → TLSRequired，绝不发送明文凭证。
	sim := newMssqlSim(t)
	sim.validUser, sim.validPass = "sa", "x"
	sim.encrypt = encryptReq
	sim.start()
	defer sim.stop()

	target, _ := core.ParseTarget(sim.addr)
	var p Prober
	res := p.Probe(context.Background(), target, core.Credential{Username: "sa", Password: "x"},
		core.Options{Timeout: 3 * time.Second, TLSPolicy: core.PlaintextAllowed})
	if res.Outcome != core.OutcomeTLSRequired {
		t.Fatalf("want tls-required, got %v", res.Outcome)
	}
	// 不得有 LOGIN7 到达服务端
	sim.mu.Lock()
	loginCount := len(sim.logins)
	sim.mu.Unlock()
	if loginCount != 0 {
		t.Fatalf("plaintext credentials must not be sent: %d logins", loginCount)
	}
}

// ---- 错误分类 ----

func TestMSSQLErrorClassification(t *testing.T) {
	cases := []struct {
		name string
		num  int32
		want core.Outcome
	}{
		{"login-failed-18456", 18456, core.OutcomeAuthFailed},
		{"account-disabled-18470", 18470, core.OutcomeAccountLocked},
		{"locked-out-18486", 18486, core.OutcomeAccountLocked},
		{"password-expired-18487", 18487, core.OutcomeMFARequired},
		{"must-change-18488", 18488, core.OutcomeMFARequired},
		{"no-default-db-4064", 4064, core.OutcomeMFARequired},
		{"busy-17187", 17187, core.OutcomeRateLimited},
		{"unknown-9999", 9999, core.OutcomeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim := newMssqlSim(t)
			sim.scriptErr = &serverError{Number: tc.num, State: 1, Class: 14, Message: "classified"}
			sim.start()
			defer sim.stop()

			res := mssqlProbeOnce(t, sim.addr, "u", "p")
			if res.Outcome != tc.want {
				t.Fatalf("want %v got %v (%s)", tc.want, res.Outcome, res.ErrDetail)
			}
		})
	}
}

// ---- 网络异常 ----

func TestMSSQLConnectionRefused(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if res := mssqlProbeOnce(t, addr, "u", "p"); res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
}

func TestMSSQLCloseAfterPrelogin(t *testing.T) {
	sim := newMssqlSim(t)
	sim.closeAfterPrelogin = true
	sim.start()
	defer sim.stop()
	if res := mssqlProbeOnce(t, sim.addr, "u", "p"); res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
}

func TestMSSQLNotTDSBanner(t *testing.T) {
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
				_, _ = c.Write([]byte("220 FTP server ready\r\n"))
				time.Sleep(300 * time.Millisecond)
			}(conn)
		}
	}()
	res := mssqlProbeOnce(t, l.Addr().String(), "u", "p")
	if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
		t.Fatalf("banner mismatch must not classify as auth: %v", res.Outcome)
	}
}

func TestMSSQLPreloginWithoutEncryption(t *testing.T) {
	// 没有 ENCRYPTION 字段的 PRELOGIN 响应 → 协议不匹配
	sim := newMssqlSim(t)
	sim.noEncryptionField = true
	sim.start()
	defer sim.stop()
	res := mssqlProbeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeProtocolMismatch {
		t.Fatalf("want protocol-mismatch, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMSSQLReadTimeout(t *testing.T) {
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
		t.Fatal("timeout not enforced")
	}
}

func TestMSSQLContextCancel(t *testing.T) {
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
				buf := make([]byte, 512)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
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

func TestMSSQLNoCredentialLeak(t *testing.T) {
	sentinel := "SENTINEL-mssql-绝密!"
	sim := newMssqlSim(t)
	sim.scriptErr = &serverError{Number: 18456, State: 1, Class: 14, Message: "Login failed."}
	sim.start()
	defer sim.stop()

	res := mssqlProbeOnce(t, sim.addr, "leak", sentinel)
	if strings.Contains(res.String(), sentinel) {
		t.Fatalf("leak: %s", res.String())
	}
	if strings.Contains(res.ErrDetail, sentinel) {
		t.Fatalf("leak detail: %s", res.ErrDetail)
	}
}

func TestMSSQLNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		sim := newMssqlSim(t)
		sim.validUser, sim.validPass = "sa", "p"
		sim.start()
		if res := mssqlProbeOnce(t, sim.addr, "sa", "p"); res.Outcome != core.OutcomeAuthSuccess {
			t.Fatalf("iter %d: %v (%s)", i, res.Outcome, res.ErrDetail)
		}
		sim.stop()
	}
	time.Sleep(200 * time.Millisecond)
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, n)
	}
}

// ---- LOGIN7 构造单测 ----

func TestBuildLogin7Offsets(t *testing.T) {
	payload := buildLogin7("user", "pass", "srv")
	if len(payload) < 94 {
		t.Fatalf("login7 too short: %d", len(payload))
	}
	declaredLen := int(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
	if declaredLen != len(payload) {
		t.Fatalf("declared length %d != actual %d", declaredLen, len(payload))
	}
	rec := parseLogin7Record(payload)
	if rec.username != "user" || rec.password != "pass" {
		t.Fatalf("roundtrip: user=%q pass=%q", rec.username, rec.password)
	}
}

func TestManglePassword(t *testing.T) {
	// 与已知算法对照：UCS2 后半字节交换 + XOR 0xA5
	mangled := manglePassword("A")
	// 'A' = U+0041 → UCS2 LE = 41 00 → mangle: 0x41 → ((0x41<<4)&0xff | 0x41>>4) ^ 0xA5 = (0x10 | 0x04) ^ 0xA5 = 0x14 ^ 0xA5 = 0xB1
	// 0x00 → 0x00 ^ 0xA5 = 0xA5
	if len(mangled) != 2 || mangled[0] != 0xB1 || mangled[1] != 0xA5 {
		t.Fatalf("manglePassword(A) = %x", mangled)
	}
	// 反混淆（顺序相反：先 XOR 再换位）
	unmangled := make([]byte, len(mangled))
	for i, ch := range mangled {
		x := ch ^ 0xA5
		unmangled[i] = (x<<4)&0xff | x>>4
	}
	if ucs2ToString(unmangled) != "A" {
		t.Fatal("unmangle failed")
	}
}
