package dockerhttp_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestLoadTLSConfigFromCertPath(t *testing.T) {
	dir := t.TempDir()
	writeTestCerts(t, dir)

	cfg, err := dockerhttp.LoadTLSConfigFromCertPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.RootCAs == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("incomplete tls config: %+v", cfg)
	}
}

func TestFromEnvTLSVerifyRequiresCerts(t *testing.T) {
	dir := t.TempDir()
	writeTestCerts(t, dir)

	t.Setenv(dockerhttp.EnvOverrideHost, "tcp://127.0.0.1:2376")
	t.Setenv(dockerhttp.EnvTLSVerify, "1")
	t.Setenv(dockerhttp.EnvCertPath, dir)
	t.Setenv(dockerhttp.EnvOverrideAPIVersion, "1.44")

	c, err := dockerhttp.New(dockerhttp.FromEnv)
	if err != nil {
		t.Fatalf("New FromEnv TLS: %v", err)
	}
	defer c.Close()
	if c.DaemonHost() != "tcp://127.0.0.1:2376" {
		t.Fatalf("host=%s", c.DaemonHost())
	}
}

func writeTestCerts(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "dockerhttp-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCertPathEnablesTLSWithoutVerification(t *testing.T) {
	dir := t.TempDir()
	writeTestCerts(t, dir)
	t.Setenv(dockerhttp.EnvOverrideHost, "tcp://127.0.0.1:2376")
	t.Setenv(dockerhttp.EnvCertPath, dir)
	t.Setenv(dockerhttp.EnvTLSVerify, "")
	c, err := dockerhttp.New(dockerhttp.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tr := c.HTTPClient().Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("Docker cert-path TLS behavior changed")
	}
}
