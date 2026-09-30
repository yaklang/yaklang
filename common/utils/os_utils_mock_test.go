package utils

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDebugMockHTTPClosesCompletedResponse(t *testing.T) {
	for _, https := range []bool{false, true} {
		name := "HTTP"
		if https {
			name = "HTTPS"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := strings.Repeat("complete local response", 64)
			var serverTLS, clientTLS *tls.Config
			if https {
				key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err)
				certificate := &x509.Certificate{
					SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
					IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
					KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				}
				der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
				require.NoError(t, err)
				trusted, err := x509.ParseCertificate(der)
				require.NoError(t, err)
				roots := x509.NewCertPool()
				roots.AddCert(trusted)
				serverTLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
				clientTLS = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
			}
			host, port := debugMockHTTPServerWithTLSConfig(ctx, "127.0.0.1:0", https, false, false, false, false, false, func([]byte) []byte {
				// A close-delimited body requires EOF, so a hidden post-write
				// sleep would cause the read deadline to expire.
				return []byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n" + body)
			}, serverTLS)
			var conn net.Conn
			var err error
			if https {
				conn, err = tls.Dial("tcp", HostPort(host, port), clientTLS)
			} else {
				conn, err = net.Dial("tcp", HostPort(host, port))
			}
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(250*time.Millisecond)))
			_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
			require.NoError(t, err)
			response, err := io.ReadAll(conn)
			require.NoError(t, err)
			require.Equal(t, "HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n"+body, string(response))
		})
	}
}
