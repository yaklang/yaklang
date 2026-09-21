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
	"strings"
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

func startMockVeNCrypt(t *testing.T, subtypes []uint32, password string, hangAfterTLS bool) string {
	t.Helper()
	needCert := false
	for _, st := range subtypes {
		if st == 260 || st == 261 {
			needCert = true
			break
		}
	}
	var tlsCfg *tls.Config
	if needCert {
		tlsCfg = vencryptTLSConfig(t)
	}
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
		_, _ = c.Write([]byte{0})
		_, _ = c.Write([]byte{byte(len(subtypes))})
		for _, st := range subtypes {
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], st)
			_, _ = c.Write(b[:])
		}
		chosen := make([]byte, 4)
		if _, err := io.ReadFull(c, chosen); err != nil {
			return
		}
		st := binary.BigEndian.Uint32(chosen)
		_, _ = c.Write([]byte{1})
		if st == 257 || st == 258 {
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
		switch st {
		case 260:
			var ok [4]byte
			_, _ = sc.Write(ok[:])
		case 261:
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
	t.Run("x509vnc-correct", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{261}, "TlsPass1!", false)
		res := mockProbe(t, "vnc", addr, "", "TlsPass1!")
		assertProbe(t, "x509vnc-ok", res, true, false)
		if res.Password != "TlsPass1!" {
			t.Fatalf("X509Vnc success must keep password, got %q", res.Password)
		}
		if !strings.Contains(string(res.ExtraInfo), "subtype=261") || !strings.Contains(string(res.ExtraInfo), "verified=false") {
			t.Fatalf("path metadata missing: %q", res.ExtraInfo)
		}
	})
	t.Run("x509vnc-wrong", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{261}, "TlsPass1!", false)
		res := mockProbe(t, "vnc", addr, "", "WRONG")
		if res.Ok || res.Finished {
			t.Fatalf("X509Vnc wrong: ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("mixed-x509-and-anonymous", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{261, 258}, "TlsPass1!", false)
		res := mockProbe(t, "vnc", addr, "", "TlsPass1!")
		assertProbe(t, "mixed-ok", res, true, false)
		if !strings.Contains(string(res.ExtraInfo), "subtype=261") {
			t.Fatalf("must select X509Vnc not TLSVnc: %q", res.ExtraInfo)
		}
	})
	t.Run("x509none-unauth", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{260}, "", false)
		assertUnauth(t, "x509none", mockProbe(t, "vnc", addr, "u", "whatever"))
	})
	t.Run("tlsplain-unsupported", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{259}, "x", false)
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok || !res.Finished {
			t.Fatalf("TLSPlain-only must finish, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("anonymous-tlsvnc-unsupported", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{258}, "x", false)
		res := mockProbe(t, "vnc", addr, "", "x")
		if res.Ok {
			t.Fatal("anonymous TLSVnc must not authenticate")
		}
		if !res.Finished {
			t.Fatal("anonymous-only should finish the target")
		}
	})
	t.Run("cancel-after-tls", func(t *testing.T) {
		addr := startMockVeNCrypt(t, []uint32{261}, "x", true)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		res := mockProbeContext(t, ctx, "vnc", addr, "", "x")
		if res.Ok || res.Finished {
			t.Fatalf("cancel after TLS must retry, ok=%v finished=%v", res.Ok, res.Finished)
		}
	})
	t.Run("tight-not-hidden-by-vencrypt", func(t *testing.T) {
		addr := startRawRFB(t, func(c net.Conn) {
			_, _ = c.Write([]byte("RFB 003.008\n"))
			b := make([]byte, 16)
			if _, err := io.ReadFull(c, b[:12]); err != nil {
				return
			}
			_, _ = c.Write([]byte{2, 16, 19})
			if _, err := io.ReadFull(c, b[:1]); err != nil {
				return
			}
			if b[0] == 19 {
				_, _ = c.Write([]byte{0, 2})
				if _, err := io.ReadFull(c, b[:2]); err != nil {
					return
				}
				_, _ = c.Write([]byte{0, 1})
				var st [4]byte
				binary.BigEndian.PutUint32(st[:], 259)
				_, _ = c.Write(st[:])
				_, _ = io.Copy(io.Discard, c)
				return
			}
			var z [4]byte
			_, _ = c.Write(z[:])
			binary.BigEndian.PutUint32(z[:], 1)
			_, _ = c.Write(z[:])
			binary.BigEndian.PutUint32(z[:], 2)
			_, _ = c.Write(z[:])
			_, _ = c.Write([]byte("STDVVNCAUTH_"))
			if _, err := io.ReadFull(c, b[:4]); err != nil {
				return
			}
			challenge := bytes.Repeat([]byte{0x55}, 16)
			_, _ = c.Write(challenge)
			resp := make([]byte, 16)
			if _, err := io.ReadFull(c, resp); err != nil {
				return
			}
			result := uint32(1)
			if bytes.Equal(resp, vncDESChallenge("secret", challenge)) {
				result = 0
			}
			binary.BigEndian.PutUint32(z[:], result)
			_, _ = c.Write(z[:])
		})
		res := mockProbe(t, "vnc", addr, "", "secret")
		assertProbe(t, "tight-fallback", res, true, false)
	})
}
