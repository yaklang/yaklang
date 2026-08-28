package minimartian

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

// probeFriendlyH2Origin accepts h2 over ALPN and sends its SETTINGS frame, so
// an ALPN+SETTINGS probe concludes the origin speaks h2. It never answers an
// actual request — like a fingerprinting WAF that lets connections in and only
// rejects non-browser clients once they send a real request.
func probeFriendlyH2Origin(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "h2-probe-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { lis.Close() })

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				tc := conn.(*tls.Conn)
				if err := tc.Handshake(); err != nil {
					return
				}
				// Delay the server preface so the probe reliably finishes
				// *after* the real request has recorded its verdict, which is
				// the ordering that exposes the overwrite.
				time.Sleep(300 * time.Millisecond)
				fr := http2.NewFramer(conn, conn)
				_ = fr.WriteSettings()
				for {
					if _, err := fr.ReadFrame(); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return lis.Addr().String()
}

// TestH2ProbeMustNotOverwriteRealRequestVerdict pins the priority between the
// two writers of h2Cache.
//
// The background probe only checks ALPN + SETTINGS, so it reports "h2 works"
// for origins that accept h2 connections and then stall real requests. An
// actual proxied request is the authoritative signal: once it has learned the
// origin's h2 path is unusable, a probe finishing later must not resurrect it,
// or every subsequent request retries the dead path and stalls.
func TestH2ProbeMustNotOverwriteRealRequestVerdict(t *testing.T) {
	origin := probeFriendlyH2Origin(t)

	p := NewProxy()
	p.http2 = true

	// Real traffic arrives for an origin we know nothing about yet, so the
	// background probe starts. This is the only point at which it starts.
	p.detectServerH2Async(origin, "")

	// While that probe is still in flight, the actual proxied request finishes
	// and concludes the origin's h2 path is unusable — a fingerprinting WAF
	// rejects a real h2 request far faster than the probe's own handshake.
	p.h2Cache.Store(origin, false)

	deadline := time.After(20 * time.Second)
	for {
		if _, inflight := p.h2ProbeInflight.Load(origin); !inflight {
			break
		}
		select {
		case <-deadline:
			t.Fatal("probe did not finish in time")
		case <-time.After(20 * time.Millisecond):
		}
	}

	cached, ok := p.h2Cache.Load(origin)
	require.True(t, ok, "cache entry disappeared")
	require.Falsef(t, cached.(bool),
		"the probe overwrote the downgrade verdict for %v: later requests will retry the dead h2 path and stall", origin)
}
