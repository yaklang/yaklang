package vncprobe

import (
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
			err := Probe(context.Background(), dialFunc(func(ctx context.Context, _, _ string) (net.Conn, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > MaxTimeout {
					t.Fatalf("unbounded deadline: %v", deadline)
				}
				if timeout > 0 && timeout < MaxTimeout && time.Until(deadline) > timeout+20*time.Millisecond {
					t.Fatal("short timeout was enlarged")
				}
				return nil, sentinel
			}), Options{Address: "127.0.0.1:5900", Timeout: timeout})
			if !errors.Is(err, sentinel) {
				t.Fatal(err)
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
	err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}), Options{Address: "127.0.0.1:5900", Password: "x", Timeout: 80 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("probe exceeded its shared budget")
	}
}

func TestProbeEmptyAddress(t *testing.T) {
	err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("must not dial")
		return nil, nil
	}), Options{})
	if !errors.Is(err, ErrProtocolMismatch) {
		t.Fatalf("got %v", err)
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
		errCh <- Probe(ctx, nil, Options{Address: ln.Addr().String(), Password: "x", Timeout: 5 * time.Second})
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
	err = Probe(context.Background(), nil, Options{Address: ln.Addr().String(), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
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
	if err := Probe(context.Background(), nil, Options{Address: ln.Addr().String(), Timeout: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
}
