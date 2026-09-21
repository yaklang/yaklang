package bruteutils

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/oracleprobe"
)

func oracleTestCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"oracle.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// Exercise the public scan path with real sockets and the production dialer.
// A listener refusal is enough to verify descriptor/TLS/config/result wiring;
// complete authentication and server-proof checks have independent wire mocks.
func TestOracleStreamWireConfiguration(t *testing.T) {
	cert, roots := oracleTestCertificate(t)
	for _, mode := range []string{"service", "sid", "tcps", "untrusted-tcps"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			type observation struct {
				descriptor string
				err        error
			}
			seen := make(chan observation, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					seen <- observation{err: err}
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if strings.Contains(mode, "tcps") {
					secured := tls.Server(conn, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
					if err = secured.Handshake(); err != nil {
						seen <- observation{err: err}
						return
					}
					conn = secured
				}
				var header [8]byte
				if _, err = io.ReadFull(conn, header[:]); err != nil {
					seen <- observation{err: err}
					return
				}
				n := int(binary.BigEndian.Uint16(header[:]))
				if n < 74 || n > 8192 || header[4] != 1 {
					seen <- observation{err: fmt.Errorf("invalid CONNECT: %x", header)}
					return
				}
				packet := make([]byte, n-8)
				if _, err = io.ReadFull(conn, packet); err != nil {
					seen <- observation{err: err}
					return
				}
				message := []byte("(DESCRIPTION=(ERR=12514))")
				refuse := make([]byte, 12+len(message))
				binary.BigEndian.PutUint16(refuse, uint16(len(refuse)))
				refuse[4] = 4
				binary.BigEndian.PutUint16(refuse[10:], uint16(len(message)))
				copy(refuse[12:], message)
				_, err = conn.Write(refuse)
				seen <- observation{descriptor: string(packet[66:]), err: err}
			}()
			config := &OracleConfig{Services: []string{"CUSTOM"}, SID: mode == "sid", Timeout: time.Second}
			if strings.Contains(mode, "tcps") {
				config.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "oracle.test", RootCAs: roots}
				if mode == "untrusted-tcps" {
					config.TLS.RootCAs = x509.NewCertPool()
				}
			}
			b, err := NewMultiTargetBruteUtilEx(WithOracleConfig(config), WithBruteCallback(oracleAuth.GetBruteHandler()))
			if err != nil {
				t.Fatal(err)
			}
			var result *BruteItemResult
			err = b.StreamBruteContext(context.Background(), "oracle", []string{listener.Addr().String()}, []string{"probe"}, []string{"candidate"}, func(r *BruteItemResult) { result = r })
			if err != nil {
				t.Fatal(err)
			}
			if result == nil {
				t.Fatal("missing scan result")
			}
			info := oracleInfo(t, result)
			if result.Ok || !result.Finished || len(info.Attempts) != 1 {
				t.Fatalf("result=%+v info=%+v", result, info)
			}
			obs := <-seen
			if mode == "untrusted-tcps" {
				if obs.err == nil || obs.descriptor != "" || info.Status != oracleprobe.TLSRejected || info.Attempts[0].Transport != "unknown" || info.RequestedTransport != "tcps" || result.ProbeResult.Transport != core.TransportUnknown {
					t.Fatalf("untrusted TLS sent application data or lost evidence: %+v %+v", obs, info)
				}
				return
			}
			if obs.err != nil {
				t.Fatal(obs.err)
			}
			kind := "SERVICE_NAME"
			if mode == "sid" {
				kind = "SID"
			}
			if !strings.Contains(obs.descriptor, "("+kind+"=CUSTOM)") {
				t.Fatalf("descriptor=%s", obs.descriptor)
			}
			transport, coreTransport := "tcp", core.TransportPlainTCP
			if mode == "tcps" {
				transport, coreTransport = "tcps", core.TransportTLS
			}
			if info.Status != oracleprobe.ServiceUnknown || info.Attempts[0].Code != 12514 || info.Attempts[0].Transport != transport || result.ProbeResult.Transport != coreTransport {
				t.Fatalf("wire evidence lost: %+v %+v", info, result.ProbeResult)
			}
		})
	}
}

func TestOracleProductionDialerStalledTLSBudget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, err = io.Copy(io.Discard, conn)
		done <- err
	}()
	start := time.Now()
	r := oracleAuth.BrutePass(&BruteItem{Target: listener.Addr().String(), Username: "probe", OracleConfig: &OracleConfig{Services: []string{"CUSTOM"}, TLS: &tls.Config{ServerName: "oracle.test"}, Timeout: 50 * time.Millisecond}})
	if r.Ok || r.Finished || time.Since(start) > time.Second || oracleInfo(t, r).Status != oracleprobe.Unavailable {
		t.Fatalf("budget/result: elapsed=%v result=%+v", time.Since(start), r)
	}
	if err := <-done; err != nil {
		t.Fatalf("client did not close its transport: %v", err)
	}
}
