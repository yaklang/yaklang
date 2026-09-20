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
	"io"
	"math/big"
	"net"
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

func serveVeNCryptLogin(t *testing.T, subtype uint32, password string, hangAfterTLS bool) string {
	t.Helper()
	tlsCfg := testServerTLSConfig(t)
	return serveOnce(t, func(c net.Conn) {
		_, _ = c.Write([]byte("RFB 003.008\n"))
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
		_, _ = c.Write([]byte{1})
		writeU32(c, subtype)
		chosen := make([]byte, 4)
		if _, err := io.ReadFull(c, chosen); err != nil {
			return
		}
		_, _ = c.Write([]byte{1})
		sc := tls.Server(c, tlsCfg)
		if err := sc.Handshake(); err != nil {
			return
		}
		if hangAfterTLS {
			_, _ = io.Copy(io.Discard, sc)
			return
		}
		switch subtype {
		case veTLSNone, veX509None:
			writeU32(sc, 0)
		case veTLSVnc, veX509Vnc:
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
	})
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestProbeVeNCryptTLSVnc(t *testing.T) {
	addr := serveVeNCryptLogin(t, veTLSVnc, "TlsPass1", false)
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "TlsPass1", Timeout: 3 * time.Second})
	if !r.OK() || r.AuthNone || r.SecurityType != 2 {
		t.Fatalf("TLSVnc correct: %+v", r)
	}
	addr = serveVeNCryptLogin(t, veTLSVnc, "TlsPass1", false)
	r = Probe(context.Background(), nil, Options{Address: addr, Password: "WRONG", Timeout: 3 * time.Second})
	if r.OK() || !errors.Is(r.Err, ErrAuthFailed) {
		t.Fatalf("TLSVnc wrong: %+v", r)
	}
}

func TestProbeVeNCryptTLSNone(t *testing.T) {
	addr := serveVeNCryptLogin(t, veTLSNone, "", false)
	r := Probe(context.Background(), nil, Options{Address: addr, Timeout: 3 * time.Second, UnauthOnly: true})
	if !r.OK() || !r.AuthNone || r.SecurityType != 1 {
		t.Fatalf("TLSNone unauth: %+v", r)
	}
}

func TestProbeVeNCryptUnsupportedSubtype(t *testing.T) {
	addr := serveVeNCryptLogin(t, 259, "", false) // TLSPlain
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 2 * time.Second})
	if !errors.Is(r.Err, ErrNoCompatibleAuth) {
		t.Fatalf("want no compatible auth, got %v", r.Err)
	}
}

func TestProbeVeNCryptCancelAfterTLS(t *testing.T) {
	addr := serveVeNCryptLogin(t, veTLSVnc, "x", true)
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "x", Timeout: 120 * time.Millisecond})
	if r.OK() {
		t.Fatal("cancel after TLS must not authenticate")
	}
	if !errors.Is(r.Err, ErrTransient) && !errors.Is(r.Err, context.DeadlineExceeded) && !errors.Is(r.Err, context.Canceled) {
		t.Fatalf("want transient/cancel after TLS, got %v", r.Err)
	}
}

func TestProbeVeNCryptX509Vnc(t *testing.T) {
	addr := serveVeNCryptLogin(t, veX509Vnc, "X509Pass", false)
	r := Probe(context.Background(), nil, Options{Address: addr, Password: "X509Pass", Timeout: 3 * time.Second})
	if !r.OK() || r.AuthNone {
		t.Fatalf("X509Vnc: %+v", r)
	}
}
