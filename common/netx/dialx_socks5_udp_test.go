package netx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestSocks5UDPAssociateLocalRelay(t *testing.T) {
	for _, tc := range []struct {
		name       string
		scheme     string
		localDNS   bool
		useAuth    bool
		wantATYP   byte
		wantTarget string
	}{
		{name: "legacy socks remote DNS", scheme: "socks", wantATYP: 3, wantTarget: "private.test"},
		{name: "socks5h remote DNS", scheme: "socks5h", wantATYP: 3, wantTarget: "private.test"},
		{name: "explicit local DNS", scheme: "socks5", localDNS: true, wantATYP: 1, wantTarget: "127.0.0.1"},
		{name: "authenticated socks5h", scheme: "socks5h", useAuth: true, wantATYP: 3, wantTarget: "private.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer backend.Close()
			_ = backend.SetDeadline(time.Now().Add(3 * time.Second))
			backendResult := make(chan error, 1)
			go func() {
				var payload [256]byte
				n, source, err := backend.ReadFromUDP(payload[:])
				if err == nil {
					_, err = backend.WriteToUDP(append([]byte("echo:"), payload[:n]...), source)
				}
				backendResult <- err
			}()
			relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer relay.Close()
			_ = relay.SetDeadline(time.Now().Add(3 * time.Second))
			relayResult := make(chan error, 1)
			go func() {
				relayResult <- serveSocks5UDPRelay(relay, backend.LocalAddr().(*net.UDPAddr), tc.wantATYP, tc.wantTarget)
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
					err = serveSocks5UDPControl(conn, relay.LocalAddr().(*net.UDPAddr).Port, tc.useAuth)
				}
				serverResult <- err
			}()
			proxy := tc.scheme + "://" + listener.Addr().String()
			if tc.useAuth {
				proxy = tc.scheme + "://user:pass@" + listener.Addr().String()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := DialSocks5UDPContext(ctx, net.JoinHostPort("private.test", strconv.Itoa(backend.LocalAddr().(*net.UDPAddr).Port)), proxy,
				DialX_WithResolveBeforeProxy(tc.localDNS),
				DialX_WithDNSOptions(WithTemporaryHosts(map[string]string{"private.test": "127.0.0.1"})),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write([]byte("hello")); err != nil {
				t.Fatal(err)
			}
			var response [64]byte
			if _, err := conn.Read(response[:]); err == nil {
				t.Fatal("fragmented SOCKS5 UDP packet was accepted")
			}
			n, err := conn.Read(response[:])
			if err != nil || string(response[:n]) != "echo:hello" {
				t.Fatalf("UDP response=%q err=%v", response[:n], err)
			}
			_ = conn.Close()
			if err := <-relayResult; err != nil {
				t.Fatal(err)
			}
			if err := <-backendResult; err != nil {
				t.Fatal(err)
			}
			if err := <-serverResult; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func serveSocks5UDPControl(conn net.Conn, relayPort int, auth bool) error {
	var greeting [4]byte
	if _, err := io.ReadFull(conn, greeting[:2]); err != nil {
		return err
	}
	if greeting[0] != 5 || greeting[1] == 0 || greeting[1] > 2 {
		return fmt.Errorf("bad SOCKS5 greeting %v", greeting[:2])
	}
	if _, err := io.ReadFull(conn, greeting[:greeting[1]]); err != nil {
		return err
	}
	method := byte(0)
	if auth {
		method = 2
	}
	if !bytes.Contains(greeting[:greeting[1]], []byte{method}) {
		return fmt.Errorf("missing method %d", method)
	}
	if _, err := conn.Write([]byte{5, method}); err != nil {
		return err
	}
	if auth {
		var head [2]byte
		if _, err := io.ReadFull(conn, head[:]); err != nil {
			return err
		}
		username := make([]byte, head[1])
		if _, err := io.ReadFull(conn, username); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, head[:1]); err != nil {
			return err
		}
		password := make([]byte, head[0])
		if _, err := io.ReadFull(conn, password); err != nil {
			return err
		}
		if string(username) != "user" || string(password) != "pass" {
			return fmt.Errorf("wrong credentials %q:%q", username, password)
		}
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			return err
		}
	}
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return err
	}
	if greeting != [4]byte{5, 3, 0, 1} {
		return fmt.Errorf("bad UDP ASSOCIATE request %v", greeting)
	}
	if _, err := io.CopyN(io.Discard, conn, 6); err != nil {
		return err
	}
	reply := []byte{5, 0, 0, 1, 0, 0, 0, 0, byte(relayPort >> 8), byte(relayPort)}
	if _, err := conn.Write(reply); err != nil {
		return err
	}
	var closed [1]byte
	_, err := conn.Read(closed[:])
	if err != io.EOF {
		return fmt.Errorf("UDP control did not close cleanly: %v", err)
	}
	return nil
}

func serveSocks5UDPRelay(relay *net.UDPConn, backend *net.UDPAddr, wantATYP byte, wantTarget string) error {
	var packet [1024]byte
	n, client, err := relay.ReadFromUDP(packet[:])
	if err != nil {
		return err
	}
	if n < 4 || !bytes.Equal(packet[:3], []byte{0, 0, 0}) || packet[3] != wantATYP {
		return fmt.Errorf("bad UDP request header %v", packet[:n])
	}
	reader := bytes.NewReader(packet[4:n])
	endpoint, err := readSocks5Address(reader, packet[3])
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host != wantTarget || port != strconv.Itoa(backend.Port) {
		return fmt.Errorf("bad UDP target %q: %v", endpoint, err)
	}
	if _, err := relay.WriteToUDP(packet[n-reader.Len():n], backend); err != nil {
		return err
	}
	n, source, err := relay.ReadFromUDP(packet[:])
	if err != nil {
		return err
	}
	if !source.IP.Equal(backend.IP) || source.Port != backend.Port {
		return fmt.Errorf("unexpected backend %v", source)
	}
	address, err := socks5Address(backend.String())
	if err != nil {
		return err
	}
	response := append(append([]byte{0, 0, 1}, address...), packet[:n]...)
	if _, err := relay.WriteToUDP(response, client); err != nil {
		return err
	}
	response[2] = 0
	_, err = relay.WriteToUDP(response, client)
	return err
}

func TestSocks5UDPReplyValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply []byte
	}{
		{name: "short header", reply: []byte{5, 0, 0}},
		{name: "wrong version", reply: []byte{4, 0, 0, 1, 127, 0, 0, 1, 0, 1}},
		{name: "wrong reserved", reply: []byte{5, 0, 1, 1, 127, 0, 0, 1, 0, 1}},
		{name: "rejected", reply: []byte{5, 7, 0, 1, 127, 0, 0, 1, 0, 1}},
		{name: "unknown address type", reply: []byte{5, 0, 0, 9}},
		{name: "empty domain", reply: []byte{5, 0, 0, 3, 0, 0, 1}},
		{name: "short IPv4", reply: []byte{5, 0, 0, 1, 127, 0}},
		{name: "short IPv6", reply: []byte{5, 0, 0, 4, 0, 0}},
		{name: "short domain", reply: []byte{5, 0, 0, 3, 3, 'a'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readSocks5Reply(bytes.NewReader(tc.reply)); err == nil {
				t.Fatal("malformed UDP ASSOCIATE reply was accepted")
			}
		})
	}
}

func TestSocks5UDPControlCloseEndsAssociation(t *testing.T) {
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
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
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		var greeting [4]byte
		if _, err := io.ReadFull(conn, greeting[:3]); err != nil {
			serverResult <- err
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			serverResult <- err
			return
		}
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			serverResult <- err
			return
		}
		if _, err := io.CopyN(io.Discard, conn, 6); err != nil {
			serverResult <- err
			return
		}
		port := relay.LocalAddr().(*net.UDPAddr).Port
		_, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(port >> 8), byte(port)})
		if err == nil {
			time.Sleep(50 * time.Millisecond)
		}
		serverResult <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := DialSocks5UDPContext(ctx, "private.test:53", "socks5://"+listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var packet [32]byte
	if _, err := conn.Read(packet[:]); err == nil {
		t.Fatal("UDP association stayed readable after control closed")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatalf("control close did not release UDP reader: %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestSocks5UDPRejectMalformedInputBeforeDial(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		proxy  string
		opts   []DialXOption
	}{
		{name: "wrong proxy protocol", target: "private.test:53", proxy: "http://127.0.0.1:1"},
		{name: "missing target port", target: "private.test", proxy: "socks5://127.0.0.1:1"},
		{name: "zero target port", target: "private.test:0", proxy: "socks5://127.0.0.1:1"},
		{name: "oversized target domain", target: string(bytes.Repeat([]byte{'x'}, 256)) + ":53", proxy: "socks5://127.0.0.1:1"},
		{name: "blocked local target", target: "private.test:53", proxy: "socks5://127.0.0.1:1", opts: []DialXOption{
			DialX_WithResolveBeforeProxy(true), DialX_WithDNSOptions(WithTemporaryHosts(map[string]string{"private.test": "127.0.0.1"})), DialX_WithDisallowAddress("127.0.0.1"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := DialSocks5UDPContext(context.Background(), tc.target, tc.proxy, tc.opts...)
			if conn != nil {
				_ = conn.Close()
			}
			if err == nil {
				t.Fatal("malformed or blocked UDP configuration was accepted")
			}
		})
	}
}

func TestSocks5AddressEncodingCompatibility(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		atyp     byte
	}{
		{endpoint: "127.0.0.1:53", atyp: 1},
		{endpoint: "[::1]:53", atyp: 4},
		{endpoint: "private.test:53", atyp: 3},
	} {
		encoded, err := socks5Address(tc.endpoint)
		if err != nil || encoded[0] != tc.atyp {
			t.Fatalf("encode %q: atyp=%d err=%v", tc.endpoint, encoded[0], err)
		}
		decoded, err := readSocks5Address(bytes.NewReader(encoded[1:]), tc.atyp)
		if err != nil || decoded != tc.endpoint {
			t.Fatalf("decode %q: got=%q err=%v", tc.endpoint, decoded, err)
		}
	}
	for _, endpoint := range []string{"", "private.test", "[fe80::1%lo0]:53", "bad\x00name:53", "private.test:65536"} {
		if _, err := socks5Address(endpoint); err == nil {
			t.Fatalf("malformed endpoint %q accepted", endpoint)
		}
	}
}

func TestSocks5UDPMalformedDatagramsAndRecovery(t *testing.T) {
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	relay, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	attacker, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer attacker.Close()
	association := &Socks5UDPConn{udp: client, remote: relay.LocalAddr().(*net.UDPAddr)}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	for _, tc := range []struct {
		name   string
		packet []byte
		source *net.UDPConn
	}{
		{name: "short", packet: []byte{0, 0, 0}, source: relay},
		{name: "reserved", packet: []byte{1, 0, 0, 1, 127, 0, 0, 1, 0, 53}, source: relay},
		{name: "fragment", packet: []byte{0, 0, 1, 1, 127, 0, 0, 1, 0, 53}, source: relay},
		{name: "unknown address type", packet: []byte{0, 0, 0, 9}, source: relay},
		{name: "short address", packet: []byte{0, 0, 0, 1, 127}, source: relay},
		{name: "empty domain", packet: []byte{0, 0, 0, 3, 0, 0, 53}, source: relay},
		{name: "wrong source", packet: []byte{0, 0, 0, 1, 127, 0, 0, 1, 0, 53}, source: attacker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.source.WriteToUDP(tc.packet, client.LocalAddr().(*net.UDPAddr)); err != nil {
				t.Fatal(err)
			}
			var response [16]byte
			if _, err := association.Read(response[:]); err == nil {
				t.Fatal("malformed SOCKS5 UDP packet accepted")
			}
		})
	}
	address, err := socks5Address("127.0.0.1:53")
	if err != nil {
		t.Fatal(err)
	}
	valid := append(append([]byte{0, 0, 0}, address...), 'o', 'k')
	if _, err := relay.WriteToUDP(valid, client.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	var response [16]byte
	n, err := association.Read(response[:])
	if err != nil || string(response[:n]) != "ok" {
		t.Fatalf("valid packet after errors: response=%q err=%v", response[:n], err)
	}
}
