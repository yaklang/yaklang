package vncprobe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func testServerTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vnc-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS10,
	}
}

func writeU32(c net.Conn, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	_, _ = c.Write(b[:])
}

func serveVeNCryptLogin(t *testing.T, subtypes []uint32, password string, hangAfterTLS, omitResult bool, version string) string {
	t.Helper()
	if version == "" {
		version = "RFB 003.008\n"
	}
	needCert := false
	for _, st := range subtypes {
		if st == veX509None || st == veX509Vnc {
			needCert = true
			break
		}
	}
	var tlsCfg *tls.Config
	if needCert {
		tlsCfg = testServerTLSConfig(t)
	}
	return serveOnce(t, func(c net.Conn) {
		serveVeNCryptConn(c, tlsCfg, subtypes, password, hangAfterTLS, omitResult, version)
	})
}

func serveVeNCryptConn(c net.Conn, tlsCfg *tls.Config, subtypes []uint32, password string, hangAfterTLS, omitResult bool, version string) {
	_, _ = c.Write([]byte(version))
	ver := make([]byte, 12)
	if _, err := io.ReadFull(c, ver); err != nil {
		return
	}
	_, _ = c.Write([]byte{1, 19})
	sel := make([]byte, 1)
	if _, err := io.ReadFull(c, sel); err != nil {
		return
	}
	_, _ = c.Write([]byte{0, 2})
	cver := make([]byte, 2)
	if _, err := io.ReadFull(c, cver); err != nil {
		return
	}
	_, _ = c.Write([]byte{0}) // version accepted
	_, _ = c.Write([]byte{byte(len(subtypes))})
	for _, st := range subtypes {
		writeU32(c, st)
	}
	chosenb := make([]byte, 4)
	if _, err := io.ReadFull(c, chosenb); err != nil {
		return
	}
	chosen := binary.BigEndian.Uint32(chosenb)
	_, _ = c.Write([]byte{1})
	if chosen == veTLSNone || chosen == veTLSVnc {
		_, _ = io.Copy(io.Discard, c)
		return
	}
	if tlsCfg == nil {
		return
	}
	sc := tls.Server(c, tlsCfg)
	if err := sc.Handshake(); err != nil {
		return
	}
	if hangAfterTLS {
		_, _ = io.Copy(io.Discard, sc)
		return
	}
	if omitResult {
		_, _ = io.Copy(io.Discard, sc)
		return
	}
	switch chosen {
	case veX509None:
		writeU32(sc, 0)
	case veX509Vnc:
		_, _ = sc.Write(bytesRepeat(0x55, 16))
		resp := make([]byte, 16)
		if _, err := io.ReadFull(sc, resp); err != nil {
			return
		}
		result := uint32(1)
		want, err := vncAuthResponse(password, bytesRepeat(0x55, 16))
		if err == nil && string(resp) == string(want) {
			result = 0
		}
		writeU32(sc, result)
		if result != 0 {
			reason := []byte("Authentication failed")
			writeU32(sc, uint32(len(reason)))
			_, _ = sc.Write(reason)
		}
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestProbeVeNCryptX509Vnc(t *testing.T) {
	addr := serveVeNCryptLogin(t, []uint32{veX509Vnc}, "X509Pass", false, false, "")
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "X509Pass", Timeout: 3 * time.Second})
	if !r.OK() || r.AuthNone || r.Subtype != veX509Vnc || r.OuterType != 19 {
		t.Fatalf("X509Vnc: %+v extra=%q", r, r.Extra)
	}
	if r.TLSVerified {
		t.Fatal("skip-verify must surface verified=false")
	}
	if !strings.Contains(r.Extra, "subtype=261") || !strings.Contains(r.Extra, "verified=false") {
		t.Fatalf("path metadata: %q", r.Extra)
	}
	addr = serveVeNCryptLogin(t, []uint32{veX509Vnc}, "X509Pass", false, false, "")
	r = Probe(context.Background(), nil, Options{Address: addr, Password: "WRONG", Timeout: 3 * time.Second})
	if r.OK() || !errors.Is(r.Err, ErrAuthFailed) {
		t.Fatalf("X509Vnc wrong: %+v", r)
	}
}

func TestProbeVeNCryptX509None(t *testing.T) {
	addr := serveVeNCryptLogin(t, []uint32{veX509None}, "", false, false, "")
	r := Probe(context.Background(), nil, Options{Address: addr, Timeout: 3 * time.Second, UnauthOnly: true})
	if !r.OK() || !r.AuthNone || r.Subtype != veX509None {
		t.Fatalf("X509None unauth: %+v", r)
	}
}

func TestProbeVeNCryptUnsupportedSubtype(t *testing.T) {
	addr := serveVeNCryptLogin(t, []uint32{259}, "", false, false, "")
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
	if !errors.Is(r.Err, ErrNoCompatibleAuth) {
		t.Fatalf("want no compatible auth, got %v", r.Err)
	}
}

func TestProbeVeNCryptCancelAfterTLS(t *testing.T) {
	addr := serveVeNCryptLogin(t, []uint32{veX509Vnc}, "x", true, false, "")
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 120 * time.Millisecond})
	if r.OK() {
		t.Fatal("cancel after TLS must not authenticate")
	}
	if !errors.Is(r.Err, ErrTransient) && !errors.Is(r.Err, context.DeadlineExceeded) && !errors.Is(r.Err, context.Canceled) {
		t.Fatalf("want transient/cancel after TLS, got %v", r.Err)
	}
}

func TestReviewVeNCrypt37NeedsSecurityResult(t *testing.T) {
	for _, version := range []string{"RFB 003.008\n", "RFB 003.007\n"} {
		t.Run(version, func(t *testing.T) {
			addr := serveVeNCryptLogin(t, []uint32{veX509None}, "", false, true, version)
			r := Probe(context.Background(), nil, Options{Address: addr, UnauthOnly: true, Timeout: 150 * time.Millisecond})
			if r.OK() {
				t.Fatalf("false unauthenticated success without VeNCrypt SecurityResult: %+v", r)
			}
		})
	}
}

func TestReviewMixedVeNCryptSubtypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		types []uint32
	}{
		{"x509_only", []uint32{veX509Vnc}},
		{"x509_and_anonymous", []uint32{veX509Vnc, veTLSVnc}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr := serveVeNCryptLogin(t, tc.types, "secret", false, false, "")
			r := Probe(context.Background(), nil, Options{Address: addr, Password: "secret", Timeout: time.Second})
			if !r.OK() || r.Subtype != veX509Vnc {
				t.Fatalf("working X509Vnc option was missed: %+v extra=%q", r, r.Extra)
			}
		})
	}
}

func TestReviewAnonymousOnlyUnsupported(t *testing.T) {
	for _, st := range []uint32{veTLSNone, veTLSVnc} {
		t.Run(fmt.Sprintf("%d", st), func(t *testing.T) {
			addr := serveVeNCryptLogin(t, []uint32{st}, "x", false, false, "")
			r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: time.Second})
			if r.OK() {
				t.Fatalf("anonymous subtype %d must not authenticate: %+v", st, r)
			}
			if errors.Is(r.Err, ErrTransient) {
				t.Fatalf("anonymous-only must not be transient: %v", r.Err)
			}
			if !errors.Is(r.Err, ErrUnsupportedTLS) && !errors.Is(r.Err, ErrNoCompatibleAuth) {
				t.Fatalf("want unsupported/no-compatible, got %v", r.Err)
			}
		})
	}
}

func TestReviewVeNCryptDoesNotHideTight(t *testing.T) {
	for _, tc := range []struct {
		name  string
		types []byte
	}{
		{"tight_only", []byte{16}},
		{"tight_and_unsupported_vencrypt", []byte{16, 19}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr := serveMany(t, func(c net.Conn) {
				_, _ = c.Write([]byte("RFB 003.008\n"))
				b := make([]byte, 16)
				if _, err := io.ReadFull(c, b[:12]); err != nil {
					return
				}
				_, _ = c.Write(append([]byte{byte(len(tc.types))}, tc.types...))
				if _, err := io.ReadFull(c, b[:1]); err != nil {
					return
				}
				if b[0] == 19 {
					_, _ = c.Write([]byte{0, 2})
					if _, err := io.ReadFull(c, b[:2]); err != nil {
						return
					}
					_, _ = c.Write([]byte{0, 1})
					writeU32(c, 259)
					_, _ = io.Copy(io.Discard, c)
					return
				}
				writeU32(c, 0)
				writeU32(c, 1)
				writeU32(c, 2)
				_, _ = c.Write([]byte("STDVVNCAUTH_"))
				if _, err := io.ReadFull(c, b[:4]); err != nil {
					return
				}
				_, _ = c.Write(bytesRepeat(0x55, 16))
				if _, err := io.ReadFull(c, b); err != nil {
					return
				}
				expected, _ := vncAuthResponse("secret", bytesRepeat(0x55, 16))
				if string(b) != string(expected) {
					writeU32(c, 1)
					writeU32(c, 0)
					return
				}
				writeU32(c, 0)
			})
			r := Probe(context.Background(), nil, Options{Address: addr, Password: "secret", Timeout: time.Second})
			if !r.OK() {
				t.Fatalf("working Tight option was missed: %+v", r)
			}
		})
	}
}
