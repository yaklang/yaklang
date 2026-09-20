package bruteutils_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func vencryptTLSConfig(t *testing.T) *tls.Config {
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

func startMockVeNCrypt(t *testing.T, subtype uint32, password string, hangAfterTLS bool) string {
	t.Helper()
	tlsCfg := vencryptTLSConfig(t)
	return startRawRFB(t, func(c net.Conn) {
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
		var st [4]byte
		binary.BigEndian.PutUint32(st[:], subtype)
		_, _ = c.Write(st[:])
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
		case 257, 260: // TLSNone, X509None
			var ok [4]byte
			_, _ = sc.Write(ok[:])
		case 258, 261: // TLSVnc, X509Vnc
			challenge := bytes.Repeat([]byte{0x55}, 16)
			_, _ = sc.Write(challenge)
			resp := make([]byte, 16)
			if _, err := io.ReadFull(sc, resp); err != nil {
				return
			}
			result := uint32(1)
			if bytes.Equal(resp, vncDESChallenge(password, challenge)) {
				result = 0
			}
			var rb [4]byte
			binary.BigEndian.PutUint32(rb[:], result)
			_, _ = sc.Write(rb[:])
			if result != 0 {
				reason := []byte("Authentication failed")
				binary.BigEndian.PutUint32(rb[:], uint32(len(reason)))
				_, _ = sc.Write(rb[:])
				_, _ = sc.Write(reason)
			}
		}
	})
}

func TestVNCHandlerVeNCrypt(t *testing.T) {
	t.Run("tlsvnc-correct", func(t *testing.T) {
		addr := startMockVeNCrypt(t, 258, "TlsPass1!", false)
		res := mockProbe(t, "vnc", addr, "", "TlsPass1!")
		assertProbe(t, "tlsvnc-ok", res, true, false)
		if res.Password != "TlsPass1!" {
			t.Fatalf("TLSVnc success must keep password, got %q", res.Password)
		}
		if res.Username != "" {
			t.Fatalf("VNC password-only username must stay empty, got %q", res.Username)
		}
	})
	t.Run("tlsvnc-wrong", func(t *testing.T) {
		addr := startMockVeNCrypt(t, 258, "TlsPass1!", false)
		res := mockProbe(t, "vnc", addr, "", "WRONG")
		if res.Ok || res.Finished {
			t.Fatalf("TLSVnc wrong: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("tlsnone-unauth", func(t *testing.T) {
		addr := startMockVeNCrypt(t, 257, "", false)
		assertUnauth(t, "tlsnone", mockProbe(t, "vnc", addr, "u", "whatever"))
	})
	t.Run("tlsplain-unsupported", func(t *testing.T) {
		addr := startMockVeNCrypt(t, 259, "x", false)
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("TLSPlain-only must finish, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("cancel-after-tls", func(t *testing.T) {
		addr := startMockVeNCrypt(t, 258, "x", true)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		res := mockProbeContext(t, ctx, "vnc", addr, "", "x")
		if res.Ok || res.Finished {
			t.Fatalf("cancel after TLS must retry, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
}
