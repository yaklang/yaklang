package vncprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type dialFunc func(context.Context, string, string) (net.Conn, error)

func (f dialFunc) DialContext(ctx context.Context, n, a string) (net.Conn, error) {
	return f(ctx, n, a)
}

func TestProbeTimeoutBudget(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second, time.Hour, 40 * time.Millisecond} {
		t.Run(timeout.String(), func(t *testing.T) {
			sentinel := errors.New("dial inspected")
			r := Probe(context.Background(), dialFunc(func(ctx context.Context, _, _ string) (net.Conn, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > MaxTimeout {
					t.Fatalf("unbounded deadline: %v", deadline)
				}
				if timeout > 0 && timeout < MaxTimeout && time.Until(deadline) > timeout+20*time.Millisecond {
					t.Fatal("short timeout was enlarged")
				}
				return nil, sentinel
			}), Options{Address: "127.0.0.1:5900", Timeout: timeout})
			if !errors.Is(r.Err, sentinel) {
				t.Fatal(r.Err)
			}
		})
	}
}

func TestProbeStalledHandshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		_, _ = io.Copy(io.Discard, server)
	}()
	start := time.Now()
	r := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}), Options{Address: "127.0.0.1:5900", Password: "x", Timeout: 80 * time.Millisecond})
	if r.OK() {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("probe exceeded its shared budget")
	}
}

func TestProbeEmptyAddress(t *testing.T) {
	r := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("must not dial")
		return nil, nil
	}), Options{})
	if !errors.Is(r.Err, ErrProtocolMismatch) {
		t.Fatalf("got %v", r.Err)
	}
}

func TestProbeCancelClosesConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c
		_, _ = io.Copy(io.Discard, c)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Probe(ctx, nil, Options{Address: ln.Addr().String(), Password: "x", Timeout: 5 * time.Second}).Err
	}()
	var srv net.Conn
	select {
	case srv = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not accept")
	}
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) && err == nil {
			t.Fatalf("want cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("probe ignored cancel")
	}
	_ = srv.SetDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := srv.Read(buf); err == nil {
		t.Fatal("expected closed connection after cancel")
	}
	_ = srv.Close()
}

func TestProbeDoesNotSendClientInit(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	sawExtra := make(chan bool, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			sawExtra <- false
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = c.Write([]byte("RFB 003.008\n"))
		ver := make([]byte, 12)
		if _, err := io.ReadFull(c, ver); err != nil {
			sawExtra <- false
			return
		}
		_, _ = c.Write([]byte{1, 1}) // None
		sel := make([]byte, 1)
		if _, err := io.ReadFull(c, sel); err != nil {
			sawExtra <- false
			return
		}
		var ok [4]byte
		_, _ = c.Write(ok[:])
		_ = c.SetDeadline(time.Now().Add(150 * time.Millisecond))
		buf := make([]byte, 8)
		n, _ := io.ReadFull(c, buf[:1])
		sawExtra <- n > 0
	}()
	r := Probe(context.Background(), nil, Options{Address: ln.Addr().String(), Timeout: 2 * time.Second})
	if !r.OK() {
		t.Fatal(r.Err)
	}
	select {
	case extra := <-sawExtra:
		if extra {
			t.Fatal("login probe sent ClientInit / extra bytes after SecurityResult")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mock hung")
	}
}

func TestProbeOversizedListsDoNotAllocate(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = c.Write([]byte("RFB 003.008\n"))
		ver := make([]byte, 12)
		_, _ = io.ReadFull(c, ver)
		_, _ = c.Write([]byte{255})
		_, _ = c.Write(bytes.Repeat([]byte{19}, 32))
	}()
	start := time.Now()
	r := Probe(context.Background(), nil, Options{Address: ln.Addr().String(), Password: "x", Timeout: 500 * time.Millisecond})
	if r.OK() {
		t.Fatal("oversized type list must not authenticate")
	}
	if time.Since(start) > time.Second {
		t.Fatal("oversized type list must fail fast")
	}
}

func TestProbeHugeFailureReason(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = c.Write([]byte("RFB 003.008\n"))
		ver := make([]byte, 12)
		if _, err := io.ReadFull(c, ver); err != nil {
			return
		}
		_, _ = c.Write([]byte{1, 2})
		sel := make([]byte, 1)
		if _, err := io.ReadFull(c, sel); err != nil {
			return
		}
		_, _ = c.Write(bytes.Repeat([]byte{0x33}, 16))
		resp := make([]byte, 16)
		if _, err := io.ReadFull(c, resp); err != nil {
			return
		}
		var fail [4]byte
		binary.BigEndian.PutUint32(fail[:], 1)
		_, _ = c.Write(fail[:])
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], 0xffffffff)
		_, _ = c.Write(n[:])
	}()
	start := time.Now()
	r := Probe(context.Background(), nil, Options{Address: ln.Addr().String(), Password: "x", Timeout: time.Second})
	if !errors.Is(r.Err, ErrAuthFailed) {
		t.Fatalf("want auth failed, got %v", r.Err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("huge reason length must not block until the full timeout")
	}
}

func TestProbeRFB33NoneSkipsSecurityResult(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = c.Write([]byte("RFB 003.003\n"))
		ver := make([]byte, 12)
		if _, err := io.ReadFull(c, ver); err != nil {
			return
		}
		var st [4]byte
		binary.BigEndian.PutUint32(st[:], 1)
		_, _ = c.Write(st[:])
		time.Sleep(200 * time.Millisecond)
	}()
	r := Probe(context.Background(), nil, Options{Address: ln.Addr().String(), Timeout: 2 * time.Second})
	if !r.OK() || !r.AuthNone || r.SecurityType != 1 {
		t.Fatalf("want none success, got %+v", r)
	}
}

func TestProbeTightCapabilities(t *testing.T) {
	handshake := func(c net.Conn) {
		_, _ = c.Write([]byte("RFB 003.008\n"))
		ver := make([]byte, 12)
		_, _ = io.ReadFull(c, ver)
		_, _ = c.Write([]byte{1, 16})
		sel := make([]byte, 1)
		_, _ = io.ReadFull(c, sel)
	}
	writeCap := func(c net.Conn, code int32, vendor, sig string) {
		var cap [16]byte
		binary.BigEndian.PutUint32(cap[0:4], uint32(code))
		copy(cap[4:8], vendor)
		copy(cap[8:16], sig)
		_, _ = c.Write(cap[:])
	}
	u32 := func(c net.Conn, v uint32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		_, _ = c.Write(b[:])
	}

	t.Run("notunnel-and-vncauth", func(t *testing.T) {
		addr := serveOnce(t, func(c net.Conn) {
			handshake(c)
			u32(c, 1)
			writeCap(c, 0, "TGHT", "NOTUNNEL")
			sel := make([]byte, 4)
			if _, err := io.ReadFull(c, sel); err != nil {
				return
			}
			_ = binary.BigEndian.Uint32(sel)
			u32(c, 1)
			writeCap(c, 2, "STDV", "VNCAUTH_")
			auth := make([]byte, 4)
			if _, err := io.ReadFull(c, auth); err != nil {
				return
			}
			_, _ = c.Write(bytes.Repeat([]byte{0x44}, 16))
			resp := make([]byte, 16)
			_, _ = io.ReadFull(c, resp)
			u32(c, 0)
		})
		r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
		if !r.OK() || r.AuthNone || r.SecurityType != 2 {
			t.Fatalf("want vnc-auth success, got %+v", r)
		}
	})
	t.Run("zero-tunnels-sends-none", func(t *testing.T) {
		addr := serveOnce(t, func(c net.Conn) {
			handshake(c)
			u32(c, 0)
			u32(c, 1)
			writeCap(c, 2, "STDV", "VNCAUTH_")
			auth := make([]byte, 4)
			if _, err := io.ReadFull(c, auth); err != nil {
				return
			}
			_ = binary.BigEndian.Uint32(auth)
			_, _ = c.Write(bytes.Repeat([]byte{0x44}, 16))
			resp := make([]byte, 16)
			_, _ = io.ReadFull(c, resp)
			u32(c, 0)
		})
		r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
		if !r.OK() {
			t.Fatal(r.Err)
		}
	})
	t.Run("missing-notunnel", func(t *testing.T) {
		addr := serveOnce(t, func(c net.Conn) {
			handshake(c)
			u32(c, 1)
			writeCap(c, 1, "TGHT", "ENCRYPTT")
			time.Sleep(50 * time.Millisecond)
		})
		r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
		if !errors.Is(r.Err, ErrNoCompatibleAuth) {
			t.Fatalf("want no compatible auth, got %v", r.Err)
		}
	})
	t.Run("auth-code-without-signature", func(t *testing.T) {
		addr := serveOnce(t, func(c net.Conn) {
			handshake(c)
			u32(c, 0)
			u32(c, 1)
			writeCap(c, 2, "XXXX", "VNCAUTH_")
			time.Sleep(50 * time.Millisecond)
		})
		r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
		if !errors.Is(r.Err, ErrNoCompatibleAuth) {
			t.Fatalf("want no compatible auth, got %v", r.Err)
		}
	})
	t.Run("security-result-2-locked", func(t *testing.T) {
		addr := serveOnce(t, func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			ver := make([]byte, 12)
			_, _ = io.ReadFull(c, ver)
			_, _ = c.Write([]byte{1, 2})
			sel := make([]byte, 1)
			_, _ = io.ReadFull(c, sel)
			_, _ = c.Write(bytes.Repeat([]byte{0x33}, 16))
			resp := make([]byte, 16)
			_, _ = io.ReadFull(c, resp)
			u32(c, 2)
		})
		r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
		if !errors.Is(r.Err, ErrLocked) || !r.Locked {
			t.Fatalf("want locked, got %+v", r)
		}
	})
	t.Run("too-many-reason-locked", func(t *testing.T) {
		addr := serveOnce(t, func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			ver := make([]byte, 12)
			_, _ = io.ReadFull(c, ver)
			_, _ = c.Write([]byte{1, 2})
			sel := make([]byte, 1)
			_, _ = io.ReadFull(c, sel)
			_, _ = c.Write(bytes.Repeat([]byte{0x33}, 16))
			resp := make([]byte, 16)
			_, _ = io.ReadFull(c, resp)
			u32(c, 1)
			reason := []byte("Too many authentication failures")
			u32(c, uint32(len(reason)))
			_, _ = c.Write(reason)
		})
		r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
		if !errors.Is(r.Err, ErrLocked) {
			t.Fatalf("want locked from reason, got %v", r.Err)
		}
	})
}

func serveOnce(t *testing.T, fn func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		fn(c)
	}()
	return ln.Addr().String()
}
