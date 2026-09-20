package bruteutils_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// Flexible in-process RFB mock used by the shipped VNC brute handler tests.
type mockRFBConfig struct {
	version   string // 12-byte ProtocolVersion; default RFB 3.8
	secTypes  []byte // 3.7/3.8 list; ignored for 3.3
	sec33     uint32 // 3.3 server-picked type
	password  string
	tightAuth []uint32 // Tight nested auth codes (default VNC-Auth)
	nTunnels  int
	hangAfter string // version|types|challenge
	dropAfter string
	banner    []byte // raw banner override (non-RFB / truncated)
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
	if bytes.Equal(resp, vncDESChallenge(cfg.password, challenge)) {
		result = 0
	}
	var rb [4]byte
	binary.BigEndian.PutUint32(rb[:], result)
	_, _ = c.Write(rb[:])
	if result != 0 && (cfg.version == "RFB 003.008\n" || cfg.version == "") {
		reason := []byte("Authentication failed")
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(reason)))
		_, _ = c.Write(n[:])
		_, _ = c.Write(reason)
	}
}

func serveTight(c net.Conn, cfg mockRFBConfig) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(cfg.nTunnels))
	_, _ = c.Write(n[:])
	for i := 0; i < cfg.nTunnels; i++ {
		var cap [16]byte
		copy(cap[4:8], "TGHT")
		copy(cap[8:], "NOTUNNEL")
		_, _ = c.Write(cap[:])
	}
	if cfg.nTunnels > 0 {
		sel := make([]byte, 4)
		if _, err := readFullN(c, sel); err != nil {
			return
		}
	}
	binary.BigEndian.PutUint32(n[:], uint32(len(cfg.tightAuth)))
	_, _ = c.Write(n[:])
	if len(cfg.tightAuth) == 0 {
		var ok [4]byte
		_, _ = c.Write(ok[:])
		return
	}
	for _, code := range cfg.tightAuth {
		var cap [16]byte
		binary.BigEndian.PutUint32(cap[0:4], code)
		copy(cap[4:8], "STDV")
		if code == 2 {
			copy(cap[8:], "VNCAUTH_")
		} else {
			copy(cap[8:], "NOAUTH__")
		}
		_, _ = c.Write(cap[:])
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
		assertProbe(t, "correct", mockProbe(t, "vnc", addr, "", "VncPass123!"), true, false)
	})
	t.Run("rfb38-wrong", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{password: "VncPass123!"})
		assertProbe(t, "wrong", mockProbe(t, "vnc", addr, "ignored", "WRONG"), false, false)
	})
	t.Run("rfb38-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{1}})
		assertProbe(t, "none-empty", mockProbe(t, "vnc", addr, "", ""), true, false)
		assertProbe(t, "none-any", mockProbe(t, "vnc", addr, "", "whatever"), true, false)
	})
	t.Run("rfb33-vnauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.003\n", password: "p33"})
		assertProbe(t, "33-ok", mockProbe(t, "vnc", addr, "", "p33"), true, false)
		assertProbe(t, "33-bad", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("rfb33-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.003\n", sec33: 1})
		assertProbe(t, "33-none", mockProbe(t, "vnc", addr, "", ""), true, false)
	})
	t.Run("rfb37-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{version: "RFB 003.007\n", secTypes: []byte{1}})
		assertProbe(t, "37-none", mockProbe(t, "vnc", addr, "", "x"), true, false)
	})
	t.Run("prefer-vnauth-over-none", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{1, 2}, password: "secret"})
		assertProbe(t, "uses-type2", mockProbe(t, "vnc", addr, "", "secret"), true, false)
		assertProbe(t, "wrong-not-none", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("tight-vnauth", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{secTypes: []byte{16}, password: "TightPass", nTunnels: 1})
		assertProbe(t, "tight-ok", mockProbe(t, "vnc", addr, "", "TightPass"), true, false)
		assertProbe(t, "tight-bad", mockProbe(t, "vnc", addr, "", "nope"), false, false)
	})
	t.Run("truncated-banner", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{banner: []byte("RFB"), dropAfter: "banner"})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok {
			t.Fatal("truncated banner must not authenticate")
		}
	})
	t.Run("non-rfb", func(t *testing.T) {
		addr := startMockRFB(t, mockRFBConfig{banner: []byte("HTTP/1.1 200 OK\r\n\r\n")})
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok {
			t.Fatal("HTTP banner must not authenticate")
		}
		if !res.Finished {
			t.Fatal("non-RFB should finish the target")
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		res := mockProbe(t, "vnc", "127.0.0.1:1", "", "x")
		if res.Ok {
			t.Fatal("unreachable must not be ok")
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
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		start := time.Now()
		res := mockProbeContext(t, ctx, "vnc", addr, "", "x")
		if res.Ok {
			t.Fatal("cancelled probe must not be ok")
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("cancel did not bound the probe")
		}
	})
}
