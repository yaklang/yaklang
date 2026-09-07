package mongodb

import (
	"context"
	"encoding/binary"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// ---- BSON 编解码 ----

func TestBSONRoundTrip(t *testing.T) {
	doc := D{
		S("hello", int32(1)),
		S("name", "value"),
		S("flag", true),
		S("payload", []byte{1, 2, 3}),
		S("count", int32(42)),
		S("sub", D{S("inner", "x")}),
	}
	enc := EncodeD(doc)
	dec, err := DecodeD(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := dec.GetInt32("hello"); v != 1 {
		t.Fatal("hello mismatch")
	}
	if v, _ := dec.GetString("name"); v != "value" {
		t.Fatal("name mismatch")
	}
	if !dec.GetBool("flag") {
		t.Fatal("flag mismatch")
	}
	if b, _ := dec.GetBinary("payload"); string(b) != "\x01\x02\x03" {
		t.Fatal("payload mismatch")
	}
	sub, _ := dec.GetDoc("sub")
	if v, _ := sub.GetString("inner"); v != "x" {
		t.Fatal("sub mismatch")
	}
}

func TestBSONUnicodeRoundTrip(t *testing.T) {
	doc := D{S("用户", "密码🔐"), S("cmd", int32(1))}
	dec, err := DecodeD(EncodeD(doc))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := dec.GetString("用户"); v != "密码🔐" {
		t.Fatalf("unicode mismatch: %q", v)
	}
}

func TestBSONMalformed(t *testing.T) {
	cases := [][]byte{
		{},                          // 空
		{5, 0, 0, 0, 0},             // 空文档（合法但无元素）—— 下一行才是非法
		{0},                         // 长度截断
		{0xff, 0xff, 0xff, 0xff, 0}, // 巨大长度
		{6, 0, 0, 0, 0x02},          // 元素截断
	}
	for i, c := range cases {
		_, err := DecodeD(c)
		if err == nil && i != 1 {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

// ---- 认证路径 ----

func TestMongoSCRAMSHA256Success(t *testing.T) {
	sim := newMongoSim(t)
	sim.mechanism = "SCRAM-SHA-256"
	sim.validUser, sim.validPass = "root", "Mongo-Pass256!"
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "root", "Mongo-Pass256!"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoSCRAMSHA256WrongPassword(t *testing.T) {
	sim := newMongoSim(t)
	sim.mechanism = "SCRAM-SHA-256"
	sim.validUser, sim.validPass = "root", "right"
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "root", "wrong"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoSCRAMSHA1Success(t *testing.T) {
	sim := newMongoSim(t)
	sim.mechanism = "SCRAM-SHA-1"
	sim.noSaslMechs = true // 老服务器 hello 不带机制列表 → 客户端默认 SHA-1
	sim.validUser, sim.validPass = "mongoadmin", "Mongo-Pass1!"
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "mongoadmin", "Mongo-Pass1!"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoSCRAMSHA1WrongPassword(t *testing.T) {
	sim := newMongoSim(t)
	sim.mechanism = "SCRAM-SHA-1"
	sim.noSaslMechs = true
	sim.validUser, sim.validPass = "mongoadmin", "right"
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "mongoadmin", "wrong"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoUnknownUser(t *testing.T) {
	sim := newMongoSim(t)
	sim.validUser, sim.validPass = "realuser", "pw"
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "ghost-user", "pw"); res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoUnicodeCredentials(t *testing.T) {
	sim := newMongoSim(t)
	sim.mechanism = "SCRAM-SHA-256"
	sim.validUser, sim.validPass = "用户名✓", "密码🔐パスワード"
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "用户名✓", "密码🔐パスワード"); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoEmptyPassword(t *testing.T) {
	sim := newMongoSim(t)
	sim.validUser, sim.validPass = "nopass", ""
	sim.start()
	defer sim.stop()

	if res := mongoProbeOnce(t, sim.addr, "nopass", ""); res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoUnauthenticatedServerPing(t *testing.T) {
	sim := newMongoSim(t)
	sim.authRequired = false
	sim.start()
	defer sim.stop()

	// 空凭证：SCRAM 对空用户名失败 → AuthFailed（与 mongo-driver Ping 无凭证一致）
	res := mongoProbeOnce(t, sim.addr, "", "")
	if res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth-failed for empty creds, got %v", res.Outcome)
	}

	// UnAuthProber：ping 成功
	target, _ := core.ParseTarget(sim.addr)
	up := UnauthProber{}
	ping := up.Probe(context.Background(), target, core.Credential{}, core.Options{Timeout: 3 * time.Second})
	if ping.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("ping on unauthenticated server: want success got %v (%s)", ping.Outcome, ping.ErrDetail)
	}
}

// ---- 错误分类与异常 ----

func TestMongoMechanismMismatch(t *testing.T) {
	sim := newMongoSim(t)
	sim.mechanism = "SCRAM-SHA-256"
	sim.validUser, sim.validPass = "u", "p"
	// 客户端选择 SHA-256（机制列表声明支持），但服务端声明只支持别的机制 → 机制错误
	sim.scriptErr = &mongoError{Code: codeMechanismUnavailable, Name: "MechanismUnavailable", Message: "Unsupported mechanism 'SCRAM-SHA-256' on database 'admin'"}
	sim.start()
	defer sim.stop()

	res := mongoProbeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeProtocolMismatch {
		t.Fatalf("want protocol-mismatch, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoTooManyClients(t *testing.T) {
	sim := newMongoSim(t)
	sim.scriptErr = &mongoError{Code: 0, Message: "too many clients are already connected"}
	sim.start()
	defer sim.stop()

	res := mongoProbeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeRateLimited {
		t.Fatalf("want rate-limited, got %v (%s)", res.Outcome, res.ErrDetail)
	}
	if res.RetryAfter <= 0 {
		t.Fatal("expected RetryAfter")
	}
}

func TestMongoConnectionRefused(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if res := mongoProbeOnce(t, addr, "u", "p"); res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
}

func TestMongoNotMongoBanner(t *testing.T) {
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
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\n\r\nhello"))
				time.Sleep(300 * time.Millisecond)
			}(conn)
		}
	}()
	res := mongoProbeOnce(t, l.Addr().String(), "u", "p")
	if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
		t.Fatalf("banner mismatch must not classify as auth: %v", res.Outcome)
	}
}

func TestMongoGarbageResponse(t *testing.T) {
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
				// 合法 OP_MSG 头但 body 是垃圾
				hdr := make([]byte, 21)
				binary.LittleEndian.PutUint32(hdr[0:4], 21)
				binary.LittleEndian.PutUint32(hdr[12:16], opMsg)
				_, _ = c.Write(append(hdr, []byte("\xff\xff\xff\xffgarbage")...))
				time.Sleep(300 * time.Millisecond)
			}(conn)
		}
	}()
	res := mongoProbeOnce(t, l.Addr().String(), "u", "p")
	if res.Outcome != core.OutcomeProtocolMismatch && res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want mismatch/unavailable, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestMongoServerCloseAfterHello(t *testing.T) {
	sim := newMongoSim(t)
	sim.closeAfterHello = true
	sim.start()
	defer sim.stop()

	res := mongoProbeOnce(t, sim.addr, "u", "p")
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want unavailable, got %v", res.Outcome)
	}
}

func TestMongoReadTimeout(t *testing.T) {
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

func TestMongoContextCancel(t *testing.T) {
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
					// 读到请求但不响应（黑洞）
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

func TestMongoNoCredentialLeak(t *testing.T) {
	sentinel := "SENTINEL-mongo-绝密-pass"
	sim := newMongoSim(t)
	sim.validUser, sim.validPass = "u", "other"
	sim.start()
	defer sim.stop()

	res := mongoProbeOnce(t, sim.addr, "u", sentinel)
	if strings.Contains(res.String(), sentinel) {
		t.Fatalf("leak: %s", res.String())
	}
}

func TestMongoNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		sim := newMongoSim(t)
		sim.validUser, sim.validPass = "u", "p"
		sim.start()
		if res := mongoProbeOnce(t, sim.addr, "u", "p"); res.Outcome != core.OutcomeAuthSuccess {
			t.Fatalf("iter %d: %v (%s)", i, res.Outcome, res.ErrDetail)
		}
		sim.stop()
	}
	time.Sleep(200 * time.Millisecond)
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, n)
	}
}
