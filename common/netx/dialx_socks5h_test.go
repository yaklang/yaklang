package netx

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestSocks5ProxyDomainAndReply(t *testing.T) {
	for _, tc := range []struct {
		name      string
		scheme    string
		user      string
		password  string
		method    byte
		replyATYP byte
	}{
		{name: "socks5h", scheme: "socks5h", method: 0, replyATYP: 1},
		{name: "socks5 existing behavior", scheme: "socks5", method: 0, replyATYP: 1},
		{name: "socks5h password", scheme: "socks5h", user: "user", password: "pass", method: 2, replyATYP: 3},
		{name: "socks5h optional password", scheme: "socks5h", user: "user", password: "pass", method: 0, replyATYP: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			serverResult := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					serverResult <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				serverResult <- serveSocks5DomainTest(conn, tc.method, tc.replyATYP)
			}()

			proxyURL := &url.URL{Scheme: tc.scheme, Host: listener.Addr().String()}
			if tc.user != "" {
				proxyURL.User = url.UserPassword(tc.user, tc.password)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := DialContext(ctx, "proxy-only.invalid:443", proxyURL.String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			payload := make([]byte, 4)
			if _, err := io.ReadFull(conn, payload); err != nil {
				t.Fatal(err)
			}
			if string(payload) != "pong" {
				t.Fatalf("application data corrupted by SOCKS5 reply: %q", payload)
			}
			if err := <-serverResult; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func serveSocks5DomainTest(conn net.Conn, method, replyATYP byte) error {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:2]); err != nil {
		return err
	}
	if header[0] != 5 || header[1] == 0 {
		return fmt.Errorf("invalid SOCKS5 greeting: %v", header[:2])
	}
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	found := false
	for _, offered := range methods {
		if offered == method {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("method %d not offered: %v", method, methods)
	}
	if _, err := conn.Write([]byte{5}); err != nil {
		return err
	}
	time.Sleep(5 * time.Millisecond) // A SOCKS5 reply may arrive in separate TCP reads.
	if _, err := conn.Write([]byte{method}); err != nil {
		return err
	}
	if method == 2 {
		if _, err := io.ReadFull(conn, header[:2]); err != nil {
			return err
		}
		username := make([]byte, header[1])
		if _, err := io.ReadFull(conn, username); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, header[:1]); err != nil {
			return err
		}
		password := make([]byte, header[0])
		if _, err := io.ReadFull(conn, password); err != nil {
			return err
		}
		if header[1] != 4 || string(username) != "user" || string(password) != "pass" {
			return fmt.Errorf("invalid username/password: %q/%q", username, password)
		}
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			return err
		}
	}
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return err
	}
	if header != [4]byte{5, 1, 0, 3} {
		return fmt.Errorf("expected domain CONNECT, got %v", header)
	}
	if _, err := io.ReadFull(conn, header[:1]); err != nil {
		return err
	}
	domain := make([]byte, header[0])
	if _, err := io.ReadFull(conn, domain); err != nil {
		return err
	}
	if string(domain) != "proxy-only.invalid" {
		return fmt.Errorf("unexpected domain: %q", domain)
	}
	if _, err := io.ReadFull(conn, header[:2]); err != nil {
		return err
	}
	if header[0] != 1 || header[1] != 187 { // 443
		return fmt.Errorf("unexpected port: %v", header[:2])
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return err
	}
	time.Sleep(5 * time.Millisecond)
	reply := []byte{0, replyATYP}
	switch replyATYP {
	case 1:
		reply = append(reply, 127, 0, 0, 1)
	case 3:
		reply = append(reply, 4, 'b', 'i', 'n', 'd')
	case 4:
		reply = append(reply, net.IPv6loopback.To16()...)
	}
	reply = append(reply, 0, 80, 'p', 'o', 'n', 'g')
	_, err := conn.Write(reply)
	return err
}

func TestProxyCheckSocks5hDoesNotConnectToProxyItself(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverResult := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		var greeting [3]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			serverResult <- err
			return
		}
		if greeting != [3]byte{5, 1, 0} {
			serverResult <- fmt.Errorf("unexpected greeting: %v", greeting)
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			serverResult <- err
			return
		}
		var next [1]byte
		_, err = conn.Read(next[:])
		serverResult <- err
	}()
	conn, err := ProxyCheck("socks5h://"+listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := <-serverResult; err != io.EOF {
		t.Fatalf("proxy check sent a CONNECT request or failed to close: %v", err)
	}
}

func TestSocks5hHandshakeCancellation(t *testing.T) {
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
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.Copy(io.Discard, conn) // Deliberately never select a SOCKS5 method.
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		conn, err := DialContext(ctx, "proxy-only.invalid:443", "socks5h://"+listener.Addr().String())
		if conn != nil {
			_ = conn.Close()
		}
		result <- err
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("proxy was not contacted")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS5 handshake ignored context cancellation")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("proxy socket was not closed")
	}
}
