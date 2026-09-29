package netx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// The hostname deliberately has no public resolution. Only the local SOCKS
// server knows where private.test lives, so a direct or pre-resolved dial fails.
func TestSocksProxyPrivateDomainThroughLocalServer(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private/" || r.Host != "private.test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "private-response")
	}))
	defer backend.Close()
	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	_, backendPort, err := net.SplitHostPort(backendAddr)
	if err != nil {
		t.Fatal(err)
	}
	for _, scheme := range []string{"socks", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
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
				serverResult <- servePrivateSocksConnect(conn, backendAddr, backendPort)
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := DialContext(ctx, "private.test:"+backendPort, scheme+"://"+listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			if _, err := io.WriteString(conn, "GET /private/ HTTP/1.1\r\nHost: private.test\r\nConnection: close\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			response, err := io.ReadAll(conn)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(response, []byte("private-response")) {
				t.Fatalf("private response missing: %q", response)
			}
			if err := <-serverResult; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func servePrivateSocksConnect(conn net.Conn, backendAddr, backendPort string) error {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:2]); err != nil {
		return err
	}
	if header[0] != 5 || header[1] != 1 {
		return fmt.Errorf("bad greeting: %v", header[:2])
	}
	if _, err := io.ReadFull(conn, header[:1]); err != nil {
		return err
	}
	if header[0] != 0 {
		return fmt.Errorf("expected no-auth method, got %d", header[0])
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return err
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
	name := make([]byte, header[0])
	if _, err := io.ReadFull(conn, name); err != nil {
		return err
	}
	if string(name) != "private.test" {
		return fmt.Errorf("target was resolved on client: %q", name)
	}
	if _, err := io.ReadFull(conn, header[:2]); err != nil {
		return err
	}
	requestedPort, err := strconv.Atoi(backendPort)
	if err != nil {
		return err
	}
	if int(header[0])<<8|int(header[1]) != requestedPort {
		return fmt.Errorf("unexpected port: %v", header[:2])
	}
	upstream, err := net.Dial("tcp", backendAddr)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Time{})
	go func() { _, _ = io.Copy(upstream, conn) }()
	_, err = io.Copy(conn, upstream)
	return err
}

func TestSocks5MalformedHandshakeAndReplies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		greeting     []byte
		connectReply []byte
		target       string
	}{
		{name: "short greeting", greeting: []byte{5}},
		{name: "wrong greeting version", greeting: []byte{4, 0}},
		{name: "unsupported method", greeting: []byte{5, 0xff}},
		{name: "short connect header", greeting: []byte{5, 0}, connectReply: []byte{5, 0, 0}},
		{name: "wrong connect version", greeting: []byte{5, 0}, connectReply: []byte{4, 0, 0, 1}},
		{name: "wrong reserved byte", greeting: []byte{5, 0}, connectReply: []byte{5, 0, 1, 1}},
		{name: "rejected connect", greeting: []byte{5, 0}, connectReply: []byte{5, 5, 0, 1}},
		{name: "unknown bound address type", greeting: []byte{5, 0}, connectReply: []byte{5, 0, 0, 9}},
		{name: "short bound IPv4 address", greeting: []byte{5, 0}, connectReply: []byte{5, 0, 0, 1, 127, 0}},
		{name: "short bound domain address", greeting: []byte{5, 0}, connectReply: []byte{5, 0, 0, 3, 4, 'a'}},
		{name: "short bound IPv6 address", greeting: []byte{5, 0}, connectReply: []byte{5, 0, 0, 4, 0, 0}},
		{name: "domain exceeds 255 bytes", greeting: []byte{5, 0}, target: strings.Repeat("a", 256) + ":443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(time.Second))
			go func() {
				defer server.Close()
				var greeting [3]byte
				if _, err := io.ReadFull(server, greeting[:]); err != nil {
					return
				}
				if _, err := server.Write(tc.greeting); err != nil || len(tc.greeting) < 2 || tc.greeting[0] != 5 || tc.greeting[1] != 0 {
					return
				}
				if tc.target != "" {
					return
				}
				var request [4]byte
				if _, err := io.ReadFull(server, request[:]); err != nil {
					return
				}
				if request[3] != 3 {
					return
				}
				var length [1]byte
				if _, err := io.ReadFull(server, length[:]); err != nil {
					return
				}
				_, _ = io.CopyN(io.Discard, server, int64(length[0])+2)
				_, _ = server.Write(tc.connectReply)
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			cfg := &config{
				Context: ctx,
				Host:    "127.0.0.1:1080",
				ProxyDialer: func(context.Context, string) (net.Conn, error) {
					return client, nil
				},
			}
			target := tc.target
			if target == "" {
				target = "private.test:443"
			}
			conn, err := cfg.dialSocks5(target)
			cancel()
			if conn != nil {
				_ = conn.Close()
			}
			if err == nil {
				t.Fatal("malformed SOCKS5 reply was accepted")
			}
		})
	}
}

func TestSocks5ExplicitLocalResolution(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "private-response")
	}))
	defer backend.Close()
	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	_, port, err := net.SplitHostPort(backendAddr)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		local   bool
		wantATY byte
	}{
		{name: "legacy remote DNS", wantATY: 3},
		{name: "explicit local DNS", local: true, wantATY: 1},
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
				serverResult <- serveResolvedSocksConnect(conn, backendAddr, tc.wantATY)
			}()
			conn, err := DialX("private.test:"+port,
				DialX_WithProxy("socks5h://"+listener.Addr().String()),
				DialX_WithDNSOptions(WithTemporaryHosts(map[string]string{"private.test": "127.0.0.1"})),
				DialX_WithResolveBeforeProxy(tc.local),
				DialX_WithTimeout(3*time.Second),
			)
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, _ = io.WriteString(conn, "GET /private/ HTTP/1.1\r\nHost: private.test\r\nConnection: close\r\n\r\n")
			response, err := io.ReadAll(conn)
			_ = conn.Close()
			if err != nil || !bytes.Contains(response, []byte("private-response")) {
				t.Fatalf("response=%q err=%v", response, err)
			}
			if err := <-serverResult; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func serveResolvedSocksConnect(conn net.Conn, backendAddr string, wantATYP byte) error {
	var greeting [3]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return err
	}
	if greeting != [3]byte{5, 1, 0} {
		return fmt.Errorf("bad greeting: %v", greeting)
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return err
	}
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return err
	}
	if header != [4]byte{5, 1, 0, wantATYP} {
		return fmt.Errorf("bad CONNECT header: %v", header)
	}
	endpoint, err := readSocks5Address(conn, header[3])
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return err
	}
	if wantATYP == 1 && host != "127.0.0.1" || wantATYP == 3 && host != "private.test" {
		return fmt.Errorf("wrong target host %q", host)
	}
	upstream, err := net.Dial("tcp", backendAddr)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Time{})
	go func() { _, _ = io.Copy(upstream, conn) }()
	_, err = io.Copy(conn, upstream)
	return err
}

func TestSocks4LegacyReplyBoundsAndData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		proto  int
		target string
		reply  []byte
		wantOK bool
	}{
		{name: "socks4 short reply", proto: SOCKS4, target: "127.0.0.1:80", reply: []byte{0}},
		{name: "socks4 wrong version", proto: SOCKS4, target: "127.0.0.1:80", reply: []byte{5, 90, 0, 0, 0, 0, 0, 0}},
		{name: "socks4 rejected", proto: SOCKS4, target: "127.0.0.1:80", reply: []byte{0, 91, 0, 0, 0, 0, 0, 0}},
		{name: "socks4 data after reply", proto: SOCKS4, target: "127.0.0.1:80", reply: []byte{0, 90, 0, 0, 0, 0, 0, 0, 'o', 'k'}, wantOK: true},
		{name: "socks4a remote domain", proto: SOCKS4A, target: "private.test:80", reply: []byte{0, 90, 0, 0, 0, 0, 0, 0, 'o', 'k'}, wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			serverResult := make(chan error, 1)
			go func() {
				defer server.Close()
				var request [9]byte
				if _, err := io.ReadFull(server, request[:]); err != nil {
					serverResult <- err
					return
				}
				if request[0] != 4 || request[1] != 1 || request[8] != 0 {
					serverResult <- fmt.Errorf("bad SOCKS4 request: %v", request)
					return
				}
				if tc.proto == SOCKS4A {
					if !bytes.Equal(request[4:8], []byte{0, 0, 0, 1}) {
						serverResult <- fmt.Errorf("bad SOCKS4A sentinel: %v", request)
						return
					}
					var name []byte
					for {
						var b [1]byte
						if _, err := io.ReadFull(server, b[:]); err != nil {
							serverResult <- err
							return
						}
						if b[0] == 0 {
							break
						}
						name = append(name, b[0])
					}
					if string(name) != "private.test" {
						serverResult <- fmt.Errorf("bad SOCKS4A domain %q", name)
						return
					}
				}
				_, err := server.Write(tc.reply)
				serverResult <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			cfg := &config{Context: ctx, Proto: tc.proto, ProxyDialer: func(context.Context, string) (net.Conn, error) { return client, nil }}
			conn, err := cfg.dialSocks4(tc.target)
			if tc.wantOK {
				if err != nil {
					t.Fatal(err)
				}
				var data [2]byte
				if _, err := io.ReadFull(conn, data[:]); err != nil || string(data[:]) != "ok" {
					t.Fatalf("application data=%q err=%v", data, err)
				}
				_ = conn.Close()
			} else if err == nil {
				_ = conn.Close()
				t.Fatal("malformed SOCKS4 reply was accepted")
			}
			if err := <-serverResult; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSocks5MalformedAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     []byte
		authResult []byte
	}{
		{name: "short method", method: []byte{5}},
		{name: "wrong method version", method: []byte{4, 2}},
		{name: "unsupported method", method: []byte{5, 3}},
		{name: "short auth reply", method: []byte{5, 2}, authResult: []byte{1}},
		{name: "wrong auth version", method: []byte{5, 2}, authResult: []byte{2, 0}},
		{name: "auth rejected", method: []byte{5, 2}, authResult: []byte{1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(time.Second))
			go func() {
				defer server.Close()
				var greeting [4]byte
				if _, err := io.ReadFull(server, greeting[:]); err != nil {
					return
				}
				if _, err := server.Write(tc.method); err != nil || len(tc.method) != 2 || tc.method[0] != 5 || tc.method[1] != 2 {
					return
				}
				var header [2]byte
				if _, err := io.ReadFull(server, header[:]); err != nil {
					return
				}
				_, _ = io.CopyN(io.Discard, server, int64(header[1]))
				if _, err := io.ReadFull(server, header[:1]); err != nil {
					return
				}
				_, _ = io.CopyN(io.Discard, server, int64(header[0]))
				_, _ = server.Write(tc.authResult)
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			cfg := &config{Context: ctx, Auth: &auth{Username: "user", Password: "pass"}, ProxyDialer: func(context.Context, string) (net.Conn, error) { return client, nil }}
			conn, err := cfg.dialSocks5("private.test:443")
			if conn != nil {
				_ = conn.Close()
			}
			if err == nil {
				t.Fatal("malformed authentication reply was accepted")
			}
		})
	}
}

func TestSocks5LocalResolutionRespectsDisallowAddress(t *testing.T) {
	conn, err := DialX("private.test:443",
		DialX_WithProxy("socks5://127.0.0.1:1"),
		DialX_WithResolveBeforeProxy(true),
		DialX_WithDNSOptions(WithTemporaryHosts(map[string]string{"private.test": "127.0.0.1"})),
		DialX_WithDisallowAddress("127.0.0.1"),
	)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "disallow address") {
		t.Fatalf("blocked address should fail before proxy dial: %v", err)
	}
}

func TestSocks5OversizedCredentialsRejectedBeforeDial(t *testing.T) {
	cfg := &config{
		Auth: &auth{Username: strings.Repeat("u", 256), Password: "pass"},
		ProxyDialer: func(context.Context, string) (net.Conn, error) {
			t.Fatal("proxy dialed with malformed credentials")
			return nil, nil
		},
	}
	if _, err := cfg.dialSocks5("private.test:443"); err == nil {
		t.Fatal("oversized SOCKS5 username accepted")
	}
}

func TestSocks5ResolveBeforeProxyUsingLocalDNS(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "private-response")
	}))
	defer backend.Close()
	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	_, backendPort, err := net.SplitHostPort(backendAddr)
	if err != nil {
		t.Fatal(err)
	}
	dnsServer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dnsServer.Close()
	_ = dnsServer.SetReadDeadline(time.Now().Add(3 * time.Second))
	dnsResult := make(chan error, 1)
	go func() {
		var packet [512]byte
		for {
			n, peer, err := dnsServer.ReadFrom(packet[:])
			if err != nil {
				dnsResult <- err
				return
			}
			var query dns.Msg
			if err := query.Unpack(packet[:n]); err != nil || len(query.Question) != 1 {
				dnsResult <- fmt.Errorf("invalid local DNS question: %v", err)
				return
			}
			response := new(dns.Msg).SetReply(&query)
			if query.Question[0].Qtype == dns.TypeA {
				response.Answer = []dns.RR{&dns.A{
					Hdr: dns.RR_Header{Name: query.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1},
					A:   net.IPv4(127, 0, 0, 1),
				}}
			}
			raw, err := response.Pack()
			if err == nil {
				_, err = dnsServer.WriteTo(raw, peer)
			}
			if query.Question[0].Qtype == dns.TypeA || err != nil {
				dnsResult <- err
				return
			}
		}
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverResult := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
			err = serveResolvedSocksConnect(conn, backendAddr, 1)
		}
		serverResult <- err
	}()
	conn, err := DialX("private.test:"+backendPort,
		DialX_WithProxy("socks5://"+listener.Addr().String()),
		DialX_WithResolveBeforeProxy(true),
		DialX_WithTimeout(3*time.Second),
		DialX_WithDNSOptions(WithDNSServers(dnsServer.LocalAddr().String()), WithDNSDisableSystemResolver(true), WithDNSNoCache(true), WithDNSPreferDoH(false), WithDNSFallbackDoH(false), WithDNSFallbackTCP(false), WithDNSRetryTimes(1)),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.WriteString(conn, "GET /private/ HTTP/1.1\r\nHost: private.test\r\nConnection: close\r\n\r\n")
	response, err := io.ReadAll(conn)
	_ = conn.Close()
	if err != nil || !bytes.Contains(response, []byte("private-response")) {
		t.Fatalf("response=%q err=%v", response, err)
	}
	if err := <-dnsResult; err != nil {
		t.Fatal(err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}
