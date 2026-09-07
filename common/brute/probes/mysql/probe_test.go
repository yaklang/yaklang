package mysql

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// ---- 正常认证路径 ----

func TestMySQLNativePasswordSuccess(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.verify = true
	sim.validUser, sim.validPass = "root", "S3cret!"
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "root", "S3cret!")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res.Transport != core.TransportPlainTCP {
		t.Fatalf("transport=%v want tcp", res.Transport)
	}
}

func TestMySQLNativePasswordWrong(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.verify = true
	sim.validUser, sim.validPass = "root", "right"
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "root", "wrong")
	if res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v", res.Outcome)
	}
}

func TestMySQLCachingSHA2FastAuthSuccess(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginCachingSHA2
	sim.verify = true
	sim.validUser, sim.validPass = "admin", "P@ssw0rd-中文"
	// 快路径：先 moredata(0x03) 再 OK
	sim.script = []scriptStep{
		{kind: "moredata", data: cachingFastAuth},
		{kind: "ok"},
	}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "admin", "P@ssw0rd-中文")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	rec := sim.lastRecord()
	if string(rec.scramble) != string(scrambleCachingSHA2(rec.nonce, "P@ssw0rd-中文")) {
		t.Fatalf("caching_sha2 scramble mismatch")
	}
}

func TestMySQLCachingSHA2FullAuthRSAOverPlaintext(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginCachingSHA2
	sim.validUser, sim.validPass = "root", "Fu11Auth-Pass"
	sim.script = []scriptStep{
		{kind: "moredata", data: cachingFullAuth},
	}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "root", "Fu11Auth-Pass")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	// 服务端必须解密出正确密码（证明 RSA 加密正确）。
	if got := sim.lastRecord().decrypted; got != "Fu11Auth-Pass" {
		t.Fatalf("server decrypted %q", got)
	}
}

func TestMySQLCachingSHA2FullAuthWrongPassword(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginCachingSHA2
	sim.validUser, sim.validPass = "root", "right-one"
	sim.script = []scriptStep{
		{kind: "moredata", data: cachingFullAuth},
	}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "root", "wrong-one")
	if res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v", res.Outcome)
	}
}

func TestMySQLAuthSwitchNativeToCaching(t *testing.T) {
	// 服务端 greeting 声明 native，但随后要求切换 caching_sha2（带新 nonce）。
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.validUser, sim.validPass = "root", "switch-pass"
	newNonce := make([]byte, 20)
	for i := range newNonce {
		newNonce[i] = byte(i + 1)
	}
	sim.script = []scriptStep{
		{kind: "switch", plugin: pluginCachingSHA2, nonce: newNonce},
		// 客户端应发送按新 nonce 计算的 caching_sha2 scramble → 校验
	}
	sim.verify = true // switch 步骤后用新 scramble 校验（verifyScramble 使用新 nonce 记录）
	sim.start()
	defer sim.stop()

	// verifyScramble 用记录中的 nonce；switch 后记录的 nonce 已更新
	res := probeOnce(t, sim.addr, "root", "switch-pass")
	// 脚本 switch 后无更多步骤 → 默认 1045 拒绝（因为 verify 模式仅在 script 空时生效）
	if res.Outcome != core.OutcomeAuthFailed {
		t.Logf("outcome=%v detail=%s (switch 流程完成，服务端默认拒绝)", res.Outcome, res.ErrDetail)
	}
	// 关键验证：客户端确实发送了按 caching_sha2 算法 + 新 nonce 的响应
	rec := sim.lastRecord()
	want := scrambleCachingSHA2(newNonce, "switch-pass")
	if string(rec.scramble) != string(want) {
		t.Fatalf("auth-switch response mismatch: got %x want %x", rec.scramble, want)
	}
	if rec.plugin != pluginCachingSHA2 {
		t.Fatalf("recorded plugin=%s", rec.plugin)
	}
}

func TestMySQLTLSThenCleartextFullAuth(t *testing.T) {
	// TLS 信道上的全量认证直接发明文密码。
	sim := newMySQLSim(t)
	sim.plugin = pluginCachingSHA2
	sim.forceSSLCap = true
	sim.tlsServer = true
	sim.validUser, sim.validPass = "root", "tls-fullauth"
	sim.script = []scriptStep{
		{kind: "moredata", data: cachingFullAuth},
	}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "root", "tls-fullauth")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res.Transport != core.TransportTLS {
		t.Fatalf("transport=%v want tls", res.Transport)
	}
	if got := sim.lastRecord().decrypted; got != "tls-fullauth" {
		t.Fatalf("server got cleartext %q", got)
	}
}

func TestMySQLTLSUpgradeFailureFallsBackToPlaintext(t *testing.T) {
	// 服务端声明 CLIENT_SSL 但实际不支持：客户端必须回退明文并如实记录。
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.forceSSLCap = true
	sim.tlsServer = false // TLS 升级失败
	sim.verify = true
	sim.validUser, sim.validPass = "root", "fb-pass"
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "root", "fb-pass")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success after fallback, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res.Transport != core.TransportPlainTCP {
		t.Fatalf("transport=%v want tcp (recorded fallback)", res.Transport)
	}
	if sim.attempts() != 2 {
		t.Fatalf("expected 2 connection attempts, got %d", sim.attempts())
	}
}

func TestMySQLTLSStrictBlocksPlaintextFallback(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.forceSSLCap = true
	sim.tlsServer = false
	sim.verify = true
	sim.validUser, sim.validPass = "root", "any"
	sim.start()
	defer sim.stop()

	target, _ := core.ParseTarget(sim.addr)
	var prober Prober
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "root", Password: "any"},
		core.Options{Timeout: 3 * time.Second, TLSPolicy: core.TLSStrict})
	if res.Outcome != core.OutcomeTLSRequired {
		t.Fatalf("want tls-required, got %v", res.Outcome)
	}
	// 严格模式下不得发送明文凭证：两次连接尝试都不应产生 handshake response？
	// 服务端在 greeting 后关闭，客户端无从发送。校验未成功：
	if res.Outcome == core.OutcomeAuthSuccess {
		t.Fatal("strict policy must not authenticate over plaintext")
	}
}

func TestMySQLPlaintextAllowedSkipsTLS(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.forceSSLCap = true
	sim.tlsServer = true
	sim.verify = true
	sim.validUser, sim.validPass = "root", "plain"
	sim.start()
	defer sim.stop()

	target, _ := core.ParseTarget(sim.addr)
	var prober Prober
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "root", Password: "plain"},
		core.Options{Timeout: 3 * time.Second, TLSPolicy: core.PlaintextAllowed})
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v", res.Outcome)
	}
	if res.Transport != core.TransportPlainTCP {
		t.Fatalf("transport=%v", res.Transport)
	}
}

func TestMySQLUnknownUser(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.script = []scriptStep{
		{kind: "err", errNum: 1045, errMsg: "Access denied for user 'nosuch'@'localhost'"},
	}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "nosuchuser", "whatever")
	if res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed for unknown user, got %v", res.Outcome)
	}
}

func TestMySQLEmptyPasswordAndEmptyUsername(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.verify = true
	sim.validUser, sim.validPass = "", ""
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "", "")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("empty user/pass should authenticate, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if len(sim.lastRecord().scramble) != 0 {
		t.Fatal("empty password must produce empty scramble")
	}
}

func TestMySQLUnicodeCredentials(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginCachingSHA2
	sim.verify = true
	sim.validUser, sim.validPass = "用户名✓", "密码🔐パスワード"
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "用户名✓", "密码🔐パスワード")
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("unicode creds should authenticate, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

// ---- 服务端错误分类 ----

func TestMySQLErrorClassification(t *testing.T) {
	cases := []struct {
		name    string
		errNum  uint16
		want    core.Outcome
		retryOk bool // 期望 RetryAfter > 0
	}{
		{"access-denied", 1045, core.OutcomeAuthFailed, false},
		{"host-blocked", 1129, core.OutcomeRateLimited, true},
		{"too-many-connections", 1040, core.OutcomeRateLimited, true},
		{"max-user-connections", 1203, core.OutcomeRateLimited, true},
		{"host-not-allowed", 1130, core.OutcomeTargetUnavailable, false},
		{"auth-protocol-mismatch", 1251, core.OutcomeProtocolMismatch, false},
		{"unknown-error-9999", 9999, core.OutcomeUnknown, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim := newMySQLSim(t)
			sim.script = []scriptStep{{kind: "err", errNum: tc.errNum, errMsg: "classified"}}
			sim.start()
			defer sim.stop()

			res := probeOnce(t, sim.addr, "u", "p")
			if res.Outcome != tc.want {
				t.Fatalf("err %d: want %v got %v", tc.errNum, tc.want, res.Outcome)
			}
			if tc.retryOk && res.RetryAfter <= 0 {
				t.Fatalf("err %d: expected RetryAfter set", tc.errNum)
			}
		})
	}
}

func TestMySQLOldAuthSwitchRequest(t *testing.T) {
	// 单字节 0xFE = pre-4.1 认证要求
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.script = []scriptStep{{kind: "raw-old-auth"}}
	// 使用自定义脚本类型
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed (unsupported old auth), got %v", res.Outcome)
	}
}

// ---- 网络异常路径 ----

func TestMySQLConnectionRefused(t *testing.T) {
	// 找一个未监听的端口
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	res := probeOnce(t, addr, "u", "p")
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want target-unavailable, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMySQLServerCloseAfterGreeting(t *testing.T) {
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.script = []scriptStep{{kind: "close"}}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want target-unavailable, got %v", res.Outcome)
	}
}

func TestMySQLNotMySQLBanner(t *testing.T) {
	// HTTP banner：不是 MySQL 协议
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
				_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\nServer: nginx\r\n\r\n"))
				time.Sleep(500 * time.Millisecond)
			}(conn)
		}
	}()

	res := probeOnce(t, l.Addr().String(), "u", "p")
	if res.Outcome != core.OutcomeProtocolMismatch && res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want protocol-mismatch or unavailable, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMySQLMalformedGreeting(t *testing.T) {
	for name, payload := range map[string][]byte{
		"protocol-v9":      append([]byte{9, 'o', 'l', 'd', 0}, make([]byte, 16)...),
		"too-short":        {10},
		"garbage":          []byte("just some garbage bytes that are definitely not mysql"),
		"empty-then-close": {},
	} {
		t.Run(name, func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			go func(payload []byte) {
				for {
					conn, err := l.Accept()
					if err != nil {
						return
					}
					go func(c net.Conn, p []byte) {
						defer c.Close()
						if len(p) > 0 {
							// 按 MySQL 分组格式包一层
							hdr := []byte{byte(len(p)), byte(len(p) >> 8), byte(len(p) >> 16), 0}
							_, _ = c.Write(append(hdr, p...))
						}
						time.Sleep(500 * time.Millisecond)
					}(conn, payload)
				}
			}(payload)

			res := probeOnce(t, l.Addr().String(), "u", "p")
			if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
				t.Fatalf("malformed greeting must not classify as auth result: %v", res.Outcome)
			}
		})
	}
}

func TestMySQLReadTimeout(t *testing.T) {
	// 黑洞：建连后不发任何东西
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
				time.Sleep(10 * time.Second) // 超过探测超时
			}(conn)
		}
	}()

	start := time.Now()
	target, _ := core.ParseTarget(l.Addr().String())
	var prober Prober
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "u", Password: "p"},
		core.Options{Timeout: 500 * time.Millisecond})
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want target-unavailable, got %v", res.Outcome)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout not enforced: %v", time.Since(start))
	}
}

// ---- Context 取消 ----

func TestMySQLContextCancelBeforeGreeting(t *testing.T) {
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
			go func(c net.Conn) { // 黑洞
				defer c.Close()
				time.Sleep(10 * time.Second)
			}(conn)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	target, _ := core.ParseTarget(l.Addr().String())
	var prober Prober
	res := prober.Probe(ctx, target, core.Credential{Username: "u", Password: "p"},
		core.Options{Timeout: 8 * time.Second})
	if res.Outcome != core.OutcomeCancelled {
		t.Fatalf("want cancelled, got %v", res.Outcome)
	}
}

func TestMySQLContextCancelDuringAuth(t *testing.T) {
	// 服务端 greeting 后 hang 住
	sim := newMySQLSim(t)
	sim.plugin = pluginNative
	sim.script = []scriptStep{{kind: "delay", d: 3 * time.Second}}
	sim.start()
	defer sim.stop()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	target, _ := core.ParseTarget(sim.addr)
	var prober Prober
	res := prober.Probe(ctx, target, core.Credential{Username: "u", Password: "p"},
		core.Options{Timeout: 25 * time.Second})
	if res.Outcome != core.OutcomeCancelled {
		t.Fatalf("want cancelled, got %v", res.Outcome)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cancel latency too high: %v", time.Since(start))
	}
}

// ---- 泄漏与脱敏 ----

func TestMySQLNoCredentialLeak(t *testing.T) {
	sentinel := "SENTINEL-PASSw0rd-$(echo)"
	sim := newMySQLSim(t)
	sim.plugin = pluginCachingSHA2
	sim.script = []scriptStep{
		{kind: "err", errNum: 1045, errMsg: "Access denied"},
	}
	sim.start()
	defer sim.stop()

	res := probeOnce(t, sim.addr, "leakuser", sentinel)
	if strings.Contains(res.String(), sentinel) {
		t.Fatalf("result leaks sentinel: %s", res.String())
	}
	if strings.Contains(fmt.Sprintf("%v", res.ErrDetail), sentinel) {
		t.Fatalf("err detail leaks sentinel: %s", res.ErrDetail)
	}
}

func TestMySQLNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 10; i++ {
		sim := newMySQLSim(t)
		sim.plugin = pluginCachingSHA2
		sim.verify = true
		sim.validUser, sim.validPass = "u", "p"
		sim.start()
		res := probeOnce(t, sim.addr, "u", "p")
		if res.Outcome != core.OutcomeAuthSuccess {
			t.Fatalf("iter %d: %v", i, res.Outcome)
		}
		sim.stop()
	}
	// 等待网络运行时清理
	time.Sleep(300 * time.Millisecond)
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, n)
	}
}
