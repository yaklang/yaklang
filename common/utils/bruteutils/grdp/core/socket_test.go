package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
)

func TestTlsPubKeyRSA(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := mustSelfSign(t, priv, &priv.PublicKey)
	pub := probeTlsPubKey(t, cert)
	want := x509.MarshalPKCS1PublicKey(&priv.PublicKey)
	if !bytes.Equal(pub, want) {
		t.Fatalf("RSA SubjectPublicKey mismatch: got %d want %d", len(pub), len(want))
	}
}

func TestTlsPubKeyECDSA(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := mustSelfSign(t, priv, &priv.PublicKey)
	pub := probeTlsPubKey(t, cert)
	ecdhKey, err := priv.PublicKey.ECDH()
	if err != nil {
		t.Fatal(err)
	}
	want := ecdhKey.Bytes()
	if !bytes.Equal(pub, want) {
		t.Fatalf("ECDSA SubjectPublicKey mismatch: got %d want %d", len(pub), len(want))
	}
}

func mustSelfSign(t *testing.T, key any, pub any) tls.Certificate {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "rdp-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	var keyPEM []byte
	switch k := key.(type) {
	case *rsa.PrivateKey:
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	default:
		t.Fatalf("unsupported key %T", key)
	}
	c, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func probeTlsPubKey(t *testing.T, cert tls.Certificate) []byte {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	errc := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errc <- err
			return
		}
		defer c.Close()
		srv := gmtls.Server(c, &gmtls.Config{
			Certificates: []gmtls.Certificate{{
				Certificate: cert.Certificate,
				PrivateKey:  cert.PrivateKey,
			}},
			MinVersion: gmtls.VersionTLS12,
			MaxVersion: gmtls.VersionTLS12,
		})
		errc <- srv.Handshake()
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	layer := NewSocketLayer(conn)
	if err := layer.StartTLS(); err != nil {
		t.Fatalf("StartTLS: %v", err)
	}
	pub, err := layer.TlsPubKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
	return pub
}
