package netx

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// tlsHandshakeFailureFixture reads a ClientHello and returns a deterministic
// server handshake. Verify the exact failure first, so IsTLSService cannot
// pass just because a different accepted failure branch happened to fire.
func tlsHandshakeFailureFixture(t *testing.T, response []byte, wantError string) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
					t.Error(err)
					return
				}
				var header [5]byte
				if _, err := io.ReadFull(conn, header[:]); err != nil {
					t.Error(err)
					return
				}
				if _, err := io.CopyN(io.Discard, conn, int64(binary.BigEndian.Uint16(header[3:]))); err != nil {
					t.Error(err)
					return
				}
				if _, err := conn.Write(response); err != nil {
					t.Error(err)
					return
				}
				// Consume the client's fatal alert, rather than closing with
				// unread client bytes and risking a TCP reset.
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	t.Cleanup(func() {
		lis.Close()
		wg.Wait()
	})
	conn, err := net.DialTimeout("tcp", lis.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionSSL30, MaxVersion: tls.VersionTLS13})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = client.HandshakeContext(ctx)
	client.Close()
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("fixture handshake error = %v, want %q", err, wantError)
	}
	return lis.Addr().String()
}

func sm2CertificateHandshake(t *testing.T) []byte {
	t.Helper()
	config := utils.GetDefaultOnlyGMTLSConfig(5)
	if config == nil || len(config.Certificates) == 0 || len(config.Certificates[0].Certificate) == 0 {
		t.Fatal("SM2 certificate missing from GM TLS fixture")
	}
	cert := config.Certificates[0].Certificate[0]
	uint24 := func(n int) []byte { return []byte{byte(n >> 16), byte(n >> 8), byte(n)} }
	handshake := func(kind byte, body []byte) []byte {
		return append(append([]byte{kind}, uint24(len(body))...), body...)
	}
	// TLS 1.2 ServerHello, ECDHE_RSA_AES_128_GCM_SHA256. Parsing the
	// following SM2 certificate must reach the unsupported-curve branch,
	// before key exchange. No private key or forged signature is needed.
	hello := append([]byte{3, 3}, make([]byte, 32)...)
	hello = append(hello, 0, 0xc0, 0x2f, 0, 0, 0)
	chain := append(uint24(len(cert)), cert...)
	certificate := append(uint24(len(chain)), chain...)
	payload := append(handshake(2, hello), handshake(11, certificate)...)
	payload = append(payload, handshake(14, nil)...)
	return append([]byte{22, 3, 3, byte(len(payload) >> 8), byte(len(payload))}, payload...)
}

func TestIsTLSService(t *testing.T) {
	mockGMHost, mockGMPort := utils.DebugMockOnlyGMHTTP(context.Background(), nil)

	addr := utils.HostPort(mockGMHost, mockGMPort)
	// Preserve the two historical external-service failure modes locally:
	// fatal protocol_version alert, and an SM2 certificate rejected by Go's
	// standard TLS client. The original positive IsTLSService assertions stay.
	protocolVersionAddr := tlsHandshakeFailureFixture(t, []byte{21, 3, 3, 0, 2, 2, 70}, "protocol version not supported")
	sm2Addr := tlsHandshakeFailureFixture(t, sm2CertificateHandshake(t), "unsupported elliptic curve")
	type args struct {
		addr    string
		proxies []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "mysql",
			args: args{
				addr: "192.168.3.113:3306",
			},
			want: false,
		},
		{
			name: "http",
			args: args{
				addr: "47.120.44.219:8087",
			},
			want: false,
		},
		{
			name: "only gmtls handshake failure",
			args: args{
				addr: addr,
			},
			want: true,
		},
		{
			name: "tls",
			args: args{
				addr: "114.251.196.39:443",
			},
			want: true,
		},
		{
			name: "only gmtls tls: protocol version not supported",
			args: args{
				addr: protocolVersionAddr,
			},
			want: true,
		},
		{
			name: "sm2 unsupported elliptic curve",
			args: args{
				addr: sm2Addr,
			},
			want: true,
		},
		{
			name: "test2 ",
			args: args{
				addr: "113.96.111.24:443",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTLSService(tt.args.addr, tt.args.proxies...); got != tt.want {
				t.Errorf("IsTLSService() = %v, want %v", got, tt.want)
			}
		})
	}
}
