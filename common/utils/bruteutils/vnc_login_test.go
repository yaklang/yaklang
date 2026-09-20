package bruteutils_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

// Flexible in-process RFB mock used by the shipped VNC brute handler tests.
type mockCap struct {
	code        int32
	vendor, sig string
}

type mockRFBConfig struct {
	version       string // 12-byte ProtocolVersion; default RFB 3.8
	secTypes      []byte // 3.7/3.8 list; ignored for 3.3
	sec33         uint32 // 3.3 server-picked type
	password      string
	tightAuth     []uint32 // Tight nested auth codes (default VNC-Auth)
	tightTunnels  []mockCap
	tightAuthCaps []mockCap
	nTunnels      int
	secResult     uint32 // forced SecurityResult; 0 means compute from password
	failReason    string
	hangAfter     string // version|types|challenge
	dropAfter     string
	banner        []byte // raw banner override (non-RFB / truncated)
}

func startMockRFB(t *testing.T, cfg mockRFBConfig) string {
	t.Helper()
	ln := mockListen(t)
	if cfg.version == "" {
		cfg.version = "RFB 003.008\n"
	}
	if cfg.secTypes == nil && cfg.version != "RFB 003.003\n" {
		cfg.secTypes = []byte{2}
	}
	if cfg.sec33 == 0 {
		cfg.sec33 = 2
	}
	if cfg.tightAuth == nil {
		cfg.tightAuth = []uint32{2}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveMockRFB(conn, cfg)
		}
	}()
	return ln.Addr().String()
}

func serveMockRFB(c net.Conn, cfg mockRFBConfig) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if len(cfg.banner) > 0 {
		_, _ = c.Write(cfg.banner)
		if cfg.dropAfter == "banner" {
			return
		}
		_, _ = io.Copy(io.Discard, c)
		return
	}
	_, _ = c.Write([]byte(cfg.version))
	if cfg.hangAfter == "version" {
		_, _ = io.Copy(io.Discard, c)
		return
	}
	ver := make([]byte, 12)
	if _, err := readFullN(c, ver); err != nil {
		return
	}
	if cfg.dropAfter == "version" {
		return
	}

	is33 := cfg.version == "RFB 003.003\n"
	var chosen uint8
	if is33 {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], cfg.sec33)
		_, _ = c.Write(raw[:])
		chosen = uint8(cfg.sec33)
	} else {
		buf := append([]byte{byte(len(cfg.secTypes))}, cfg.secTypes...)
		_, _ = c.Write(buf)
		if cfg.hangAfter == "types" {
			_, _ = io.Copy(io.Discard, c)
			return
		}
		sel := make([]byte, 1)
		if _, err := readFullN(c, sel); err != nil {
			return
		}
		chosen = sel[0]
	}

	switch chosen {
	case 1:
		if !is33 && cfg.version != "RFB 003.007\n" {
			var ok [4]byte
			_, _ = c.Write(ok[:])
		}
	case 2:
		serveVNCAuth(c, cfg)
	case 16:
		serveTight(c, cfg)
	}
}

func serveVNCAuth(c net.Conn, cfg mockRFBConfig) {
	if cfg.hangAfter == "challenge" {
		_, _ = io.Copy(io.Discard, c)
		return
	}
	challenge := bytes.Repeat([]byte{0x11}, 16)
	_, _ = c.Write(challenge)
	if cfg.dropAfter == "challenge" {
		return
	}
	resp := make([]byte, 16)
	if _, err := readFullN(c, resp); err != nil {
		return
	}
	var result uint32 = 1
	if cfg.secResult != 0 {
		result = cfg.secResult
	} else if bytes.Equal(resp, vncDESChallenge(cfg.password, challenge)) {
		result = 0
	}
	var rb [4]byte
	binary.BigEndian.PutUint32(rb[:], result)
	_, _ = c.Write(rb[:])
	if result != 0 && cfg.version != "RFB 003.003\n" && cfg.version != "RFB 003.007\n" {
		reason := []byte(cfg.failReason)
		if len(reason) == 0 {
			reason = []byte("Authentication failed")
		}
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(reason)))
		_, _ = c.Write(n[:])
		_, _ = c.Write(reason)
	}
}

func writeMockCap(c net.Conn, cap mockCap) {
	var buf [16]byte
	binary.BigEndian.PutUint32(buf[0:4], uint32(cap.code))
	copy(buf[4:8], cap.vendor)
	copy(buf[8:16], cap.sig)
	_, _ = c.Write(buf[:])
}

func serveTight(c net.Conn, cfg mockRFBConfig) {
	tunnels := cfg.tightTunnels
	if tunnels == nil && cfg.nTunnels > 0 {
		tunnels = []mockCap{{0, "TGHT", "NOTUNNEL"}}
	}
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(tunnels)))
	_, _ = c.Write(n[:])
	for _, cap := range tunnels {
		writeMockCap(c, cap)
	}
	if len(tunnels) > 0 {
		sel := make([]byte, 4)
		if _, err := readFullN(c, sel); err != nil {
			return
		}
	}
	authCaps := cfg.tightAuthCaps
	if authCaps == nil {
		for _, code := range cfg.tightAuth {
			sig := "NOAUTH__"
			if code == 2 {
				sig = "VNCAUTH_"
			}
			authCaps = append(authCaps, mockCap{int32(code), "STDV", sig})
		}
	}
	binary.BigEndian.PutUint32(n[:], uint32(len(authCaps)))
	_, _ = c.Write(n[:])
	if len(authCaps) == 0 {
		var ok [4]byte
		_, _ = c.Write(ok[:])
		return
	}
	for _, cap := range authCaps {
		writeMockCap(c, cap)
	}
	chosen := make([]byte, 4)
	if _, err := readFullN(c, chosen); err != nil {
		return
	}
	code := binary.BigEndian.Uint32(chosen)
	switch code {
	case 1:
		var ok [4]byte
		_, _ = c.Write(ok[:])
	case 2:
		serveVNCAuth(c, cfg)
	}
}

func TestVNCLoginProbeHandler(t *testing.T) {
	t.Run("rfb38-correct", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "VncPass123!"})
		res := mockProbe(t, "vnc", addr, "", "VncPass123!")
		assertProbe(t, "correct", res, true, false)
		if res.Password != "VncPass123!" {
			t.Fatalf("password success must keep the input password, got %q", res.Password)
		}
		if res.AccountLocked {
			t.Fatal("password success must not be locked")
		}
	})
	t.Run("rfb38-wrong", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "VncPass123!"})
		res := mockProbe(t, "vnc", addr, "ignored", "WRONG")
		assertProbe(t, "wrong", res, false, false)
		if res.AccountLocked {
			t.Fatal("wrong password is not a lockout")
		}
	})
	t.Run("rfb38-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{1}})
		assertUnauth(t, "none-empty", mockProbe(t, "vnc", addr, "", ""))
		assertUnauth(t, "none-any", mockProbe(t, "vnc", addr, "u", "whatever"))
	})
	t.Run("rfb33-vnauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.003\n", password: "p33"})
		assertProbe(t, "33-ok", mockProbe(t, "vnc", addr, "", "p33"), true, false)
		assertProbe(t, "33-bad", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("rfb33-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.003\n", sec33: 1})
		assertUnauth(t, "33-none", mockProbe(t, "vnc", addr, "u", "p"))
	})
	t.Run("rfb37-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.007\n", secTypes: []byte{1}})
		assertUnauth(t, "37-none", mockProbe(t, "vnc", addr, "", "x"))
	})
	t.Run("none-and-vncauth-reports-unauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{1, 2}, password: "secret"})
		res := mockProbe(t, "vnc", addr, "admin", "secret")
		assertUnauth(t, "mixed-unauth", res)
		wrong := mockProbe(t, "vnc", addr, "", "nope")
		if wrong.Ok {
			t.Fatal("wrong VNC-Auth password must not succeed via None fallback")
		}
		if wrong.Finished {
			t.Fatal("wrong password must not finish the target")
		}
	})
	t.Run("tight-vnauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{16}, password: "TightPass", nTunnels: 1})
		assertProbe(t, "tight-ok", mockProbe(t, "vnc", addr, "", "TightPass"), true, false)
		assertProbe(t, "tight-bad", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("truncated-banner", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{banner: []byte("RFB"), dropAfter: "banner"})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("truncated banner: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("non-rfb", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{banner: []byte("HTTP/1.1 200 OK\r\n\r\n")})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("HTTP banner: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		res := mockProbe(t, "vnc", "127.0.0.1:1", "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("unreachable: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("long-password-truncated", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "toolongpassword"})
		assertProbe(t, "long", mockProbe(t, "vnc", addr, "", "toolongpassword"), true, false)
		assertProbe(t, "first8", mockProbe(t, "vnc", addr, "", "toolongpXXXX"), true, false)
	})
	t.Run("empty-password-vnauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: ""})
		assertProbe(t, "empty-ok", mockProbe(t, "vnc", addr, "", ""), true, false)
		assertProbe(t, "empty-bad", mockProbe(t, "vnc", addr, "", "x"), false, false)
	})
	t.Run("cancel", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{hangAfter: "version", password: "x"})
		for i := 0; i < 20; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			res := mockProbeContext(t, ctx, "vnc", addr, "", "x")
			cancel()
			if res.Ok || res.Finished {
				t.Fatalf("timeout after RFB banner must retry (i=%d), ok=%v finished=%v", i, res.Ok, res.Finished)
			}
		}
	})
	t.Run("partial-rfb-prefix-timeout", func(t *testing.T) {
		addr := startRawRFB(t, func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003."))
			_, _ = io.Copy(io.Discard, c)
		})
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		res := mockProbeContext(t, ctx, "vnc", addr, "", "x")
		if res.Ok || res.Finished {
			t.Fatalf("timeout after partial RFB prefix must retry, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("rfb37-vnauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.007\n", password: "p37"})
		assertProbe(t, "37-ok", mockProbe(t, "vnc", addr, "", "p37"), true, false)
		assertProbe(t, "37-bad", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("rfb3889-treated-as-38", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.889\n", password: "realvnc"})
		assertProbe(t, "889-ok", mockProbe(t, "vnc", addr, "", "realvnc"), true, false)
		assertProbe(t, "889-bad", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("tight-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{16}, tightAuth: []uint32{}})
		assertUnauth(t, "tight-none", mockProbe(t, "vnc", addr, "", "ignored"))
	})
	t.Run("tight-nested-none-code", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{16}, tightAuth: []uint32{1}})
		assertUnauth(t, "tight-auth1", mockProbe(t, "vnc", addr, "", "x"))
	})
	t.Run("prefer-vnauth-over-tight", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{16, 2}, password: "secret"})
		assertProbe(t, "picks-2", mockProbe(t, "vnc", addr, "", "secret"), true, false)
	})
	t.Run("eight-byte-password", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "12345678"})
		assertProbe(t, "exact8", mockProbe(t, "vnc", addr, "", "12345678"), true, false)
	})
	t.Run("special-chars", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: `P@ss!#`})
		assertProbe(t, "ok", mockProbe(t, "vnc", addr, "", `P@ss!#`), true, false)
		assertProbe(t, "bad", mockProbe(t, "vnc", addr, "", `P@ss!`), false, false)
	})
	t.Run("username-ignored", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "onlypass"})
		res := mockProbe(t, "vnc", addr, "administrator", "onlypass")
		assertProbe(t, "any-user", res, true, false)
		if !res.OnlyNeedPassword {
			t.Fatal("VNC must be password-only")
		}
	})
	t.Run("unsupported-vencrypt", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{19}})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("VeNCrypt-only must finish the target, got ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("unsupported-ra2", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{5, 6}})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("RA2-only must finish the target, got ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("drop-after-version", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{dropAfter: "version", password: "x"})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || res.Finished {
			t.Fatalf("EOF after RFB banner must retry, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("drop-after-challenge", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{dropAfter: "challenge", password: "x"})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || res.Finished {
			t.Fatalf("EOF after challenge must retry, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("lockout-result-2", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "secret", secResult: 2})
		res := mockProbe(t, "vnc", addr, "", "secret")
		if res.Ok || res.Finished || !res.AccountLocked {
			t.Fatalf("SecurityResult 2: ok=%v finished=%v locked=%v", res.Ok, res.Finished, res.AccountLocked)
		}
	})
	t.Run("lockout-too-many-reason", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "secret", failReason: "Too many authentication failures"})
		res := mockProbe(t, "vnc", addr, "", "WRONG")
		if res.Ok || !res.AccountLocked {
			t.Fatalf("too-many reason: ok=%v locked=%v", res.Ok, res.AccountLocked)
		}
	})
	t.Run("generic-security-failure-not-lockout", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "secret", failReason: "Security failure"})
		res := mockProbe(t, "vnc", addr, "", "WRONG")
		if res.Ok || res.Finished || res.AccountLocked {
			t.Fatalf("generic Security failure must be auth-fail, ok=%v finished=%v locked=%v", res.Ok, res.Finished, res.AccountLocked)
		}
	})
	t.Run("tight-missing-notunnel", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{
			secTypes:     []byte{16},
			nTunnels:     1,
			tightTunnels: []mockCap{{1, "TGHT", "ENCRYPTT"}},
			password:     "TightPass",
		})
		res := mockProbe(t, "vnc", addr, "", "TightPass")
		if res.Ok || !res.Finished {
			t.Fatalf("missing NOTUNNEL: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("tight-auth-wrong-signature", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{
			secTypes:      []byte{16},
			tightAuthCaps: []mockCap{{2, "XXXX", "VNCAUTH_"}},
			password:      "TightPass",
		})
		res := mockProbe(t, "vnc", addr, "", "TightPass")
		if res.Ok || !res.Finished {
			t.Fatalf("wrong Tight auth signature: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("password-not-leaked", func(t *testing.T) {
		const secret = "LeakMeP@ss!"
		addr := startMockRFB(t, mockRFBConfig{password: "other"})
		res := mockProbe(t, "vnc", addr, "u", secret)
		if strings.Contains(res.String(), secret) || strings.Contains(string(res.ExtraInfo), secret) {
			t.Fatal("password leaked in result")
		}
	})
}

func TestVNCStreamPasswordDedupe(t *testing.T) {
	addr := startMockRFB(t, mockRFBConfig{password: "good-pass"})
	handler, err := bruteutils.GetBruteFuncByType("vnc")
	if err != nil {
		t.Fatal(err)
	}
	var attempts int32
	seen := make(map[string]int)
	util, err := bruteutils.NewMultiTargetBruteUtilEx(
		bruteutils.WithOkToStop(false),
		bruteutils.WithBruteCallback(func(item *bruteutils.BruteItem) *bruteutils.BruteItemResult {
			atomic.AddInt32(&attempts, 1)
			return handler(item)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = util.StreamBruteContext(context.Background(), "vnc", []string{addr},
		[]string{"alice", "bob"}, []string{"wrong-pass", "good-pass"},
		func(res *bruteutils.BruteItemResult) {
			seen[res.Username+"/"+res.Password]++
			if res.Ok && res.Password != "good-pass" {
				t.Errorf("unexpected hit user=%q pass=%q", res.Username, res.Password)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&attempts); n != 2 {
		t.Fatalf("password-only VNC must try each password once, attempts=%d want 2 (seen=%v)", n, seen)
	}
	if seen["alice/good-pass"]+seen["bob/good-pass"] == 0 {
		t.Fatalf("correct password was not found: %v", seen)
	}
}

func TestVNCLoginProbeCrashAndGarbage(t *testing.T) {
	// Malformed peers must not panic, must not report success, and must return fast.
	cases := []struct {
		name  string
		write func(net.Conn)
	}{
		{"empty", func(net.Conn) {}},
		{"one-byte", func(c net.Conn) { _, _ = c.Write([]byte{0}) }},
		{"rfb-no-nl", func(c net.Conn) { _, _ = c.Write([]byte("RFB 003.008")) }},
		{"rfb-crlf", func(c net.Conn) { _, _ = c.Write([]byte("RFB 003.008\r\n")) }},
		{"http", func(c net.Conn) { _, _ = c.Write([]byte("HTTP/1.1 400\r\n\r\n")) }},
		{"ssh", func(c net.Conn) { _, _ = c.Write([]byte("SSH-2.0-OpenSSH\r\n")) }},
		{"nulls", func(c net.Conn) { _, _ = c.Write(make([]byte, 12)) }},
		{"major-2", func(c net.Conn) { _, _ = c.Write([]byte("RFB 002.000\n")) }},
		{"minor-0", func(c net.Conn) { _, _ = c.Write([]byte("RFB 003.000\n")) }},
		{"not-ascii", func(c net.Conn) { _, _ = c.Write(bytes.Repeat([]byte{0xff}, 12)) }},
		{"sec-count-zero", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write([]byte{0})
			var n [4]byte
			binary.BigEndian.PutUint32(n[:], 7)
			_, _ = c.Write(n[:])
			_, _ = c.Write([]byte("no auth"))
		}},
		{"sec-count-255", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write([]byte{255})
			_, _ = c.Write(bytes.Repeat([]byte{19}, 255))
		}},
		{"type0-u32", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.003\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write(make([]byte, 4))
		}},
		{"huge-reason", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write([]byte{1, 2})
			sel := make([]byte, 1)
			_, _ = io.ReadFull(c, sel)
			_, _ = c.Write(bytes.Repeat([]byte{0x22}, 16))
			resp := make([]byte, 16)
			_, _ = io.ReadFull(c, resp)
			var fail [4]byte
			binary.BigEndian.PutUint32(fail[:], 1)
			_, _ = c.Write(fail[:])
			var n [4]byte
			binary.BigEndian.PutUint32(n[:], 0xffffffff)
			_, _ = c.Write(n[:])
		}},
		{"tight-huge-tunnels", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write([]byte{1, 16})
			sel := make([]byte, 1)
			_, _ = io.ReadFull(c, sel)
			var n [4]byte
			binary.BigEndian.PutUint32(n[:], 0x100000)
			_, _ = c.Write(n[:])
		}},
		{"short-challenge", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write([]byte{1, 2})
			sel := make([]byte, 1)
			_, _ = io.ReadFull(c, sel)
			_, _ = c.Write([]byte{1, 2, 3, 4})
		}},
		{"tlsvnc-only", func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			_, _ = io.CopyN(io.Discard, c, 12)
			_, _ = c.Write([]byte{1, 18})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr := startRawRFB(t, tc.write)
			start := time.Now()
			res := mockProbe(t, "vnc", addr, "u", "p")
			if time.Since(start) > 2*time.Second {
				t.Fatal("malformed peer took too long")
			}
			if res.Ok {
				t.Fatal("malformed peer must not authenticate")
			}
			if bytes.Contains(res.ExtraInfo, []byte("brute item failed")) {
				t.Fatalf("handler panicked: %s", res.ExtraInfo)
			}
		})
	}
}

func TestVNCLoginProbeConcurrent(t *testing.T) {
	addr := startMockRFB(t, mockRFBConfig{password: "conc-pass"})
	errCh := make(chan string, 16)
	for i := 0; i < 8; i++ {
		go func() {
			ok := mockProbe(t, "vnc", addr, "", "conc-pass")
			bad := mockProbe(t, "vnc", addr, "", "nope")
			if !ok.Ok || ok.Finished {
				errCh <- "correct failed"
				return
			}
			if bad.Ok || bad.Finished {
				errCh <- "wrong succeeded or finished"
				return
			}
			errCh <- ""
		}()
	}
	for i := 0; i < 8; i++ {
		if msg := <-errCh; msg != "" {
			t.Fatal(msg)
		}
	}
}

func assertUnauth(t *testing.T, name string, res *bruteutils.BruteItemResult) {
	t.Helper()
	assertProbe(t, name, res, true, false)
	if res.Username != "" || res.Password != "" {
		t.Errorf("[%s] unauth must clear creds, user=%q pass=%q", name, res.Username, res.Password)
	}
	if res.AccountLocked {
		t.Errorf("[%s] unauth must not be locked", name)
	}
}

func startRawRFB(t *testing.T, write func(net.Conn)) string {
	t.Helper()
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				write(c)
			}(conn)
		}
	}()
	return ln.Addr().String()
}
