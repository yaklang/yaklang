package netx

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestHTTPProxyHandshakeCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan struct{})
			closed := make(chan struct{})
			go func() {
				defer close(closed)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				close(accepted)
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				_, _ = io.Copy(io.Discard, conn)
			}()
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
			}
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				conn, err := DialContext(ctx, "192.0.2.1:80", "http://"+listener.Addr().String())
				if conn != nil {
					conn.Close()
				}
				finished <- err
			}()
			select {
			case <-accepted:
			case <-time.After(time.Second):
				t.Fatal("proxy not contacted")
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("expected cancellation error")
				}
			case <-time.After(time.Second):
				t.Fatal("proxy handshake ignored context")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("proxy socket remained open")
			}
		})
	}
}

func TestHTTPProxyConnectionSurvivesDialCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		_, _ = io.Copy(conn, reader)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := DialContext(ctx, "192.0.2.1:80", "http://"+listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if _, err = io.WriteString(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err = io.ReadFull(conn, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "ping" {
		t.Fatalf("got %q", response)
	}
	conn.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("proxy did not finish")
	}
}
