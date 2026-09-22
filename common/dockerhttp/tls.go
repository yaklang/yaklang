package dockerhttp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
)

// LoadTLSConfigFromCertPath loads ca.pem, cert.pem, and key.pem from dir,
// matching the official Docker client layout under DOCKER_CERT_PATH.
func LoadTLSConfigFromCertPath(dir string) (*tls.Config, error) {
	if dir == "" {
		return nil, fmt.Errorf("DOCKER_CERT_PATH is empty")
	}
	caPath := filepath.Join(dir, "ca.pem")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read ca.pem: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA certificate in %s", caPath)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load cert/key: %w", err)
	}

	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// WithTLSFromEnv applies DOCKER_TLS_VERIFY + DOCKER_CERT_PATH when set.
// A non-empty DOCKER_CERT_PATH enables TLS; DOCKER_TLS_VERIFY controls verification.
// If set but DOCKER_CERT_PATH is empty, ~/.docker is used (official client default).
func WithTLSFromEnv() Opt {
	return func(c *Client) error {
		if !TLSVerifyFromEnv() && CertPathFromEnv() == "" {
			return nil
		}
		dir := CertPathFromEnv()
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("DOCKER_TLS_VERIFY set but cannot resolve home for default cert path: %w", err)
			}
			dir = filepath.Join(home, ".docker")
		}
		cfg, err := LoadTLSConfigFromCertPath(dir)
		if err != nil {
			return err
		}
		// Match the Docker SDK's explicit environment opt-out of certificate verification.
		cfg.InsecureSkipVerify = !TLSVerifyFromEnv()
		return WithTLSConfig(cfg)(c)
	}
}
