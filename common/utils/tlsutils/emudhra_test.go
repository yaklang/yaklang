package tlsutils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateEMudhraServerCert(t *testing.T) {
	chain, key, err := GenerateEMudhraServerCert(WithCommonName("example.test"), WithOrganization("TLS Test"),
		WithAlternativeDNS("example.test"), WithAlternativeIPStrings("127.0.0.1", "::1"))
	require.NoError(t, err)
	pair, err := tls.X509KeyPair(chain, key)
	require.NoError(t, err)
	require.Len(t, pair.Certificate, 3)
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	require.NoError(t, err)
	issuer, err := x509.ParseCertificate(pair.Certificate[1])
	require.NoError(t, err)
	root, err := x509.ParseCertificate(pair.Certificate[2])
	require.NoError(t, err)
	require.Equal(t, "EM OV TLS CA - G3C", issuer.Subject.CommonName)
	require.False(t, leaf.IsCA)
	require.NoError(t, leaf.CheckSignatureFrom(issuer))
	require.NoError(t, issuer.CheckSignatureFrom(root))
	roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(root)
	intermediates.AddCert(issuer)
	for _, name := range []string{"example.test", "127.0.0.1", "::1"} {
		_, err = leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: roots, Intermediates: intermediates})
		require.NoError(t, err)
	}
	_, caKey := GetEMudhraCAAndKey()
	require.False(t, bytes.Equal(key, caKey), "must return the leaf private key, not the issuer key")
	_, secondKey, err := GenerateEMudhraServerCert(WithCommonName("example.test"), WithOrganization("TLS Test"))
	require.NoError(t, err)
	require.False(t, bytes.Equal(key, secondKey), "each generated leaf needs its own key")
}

func TestGenerateSignedCertReturnsLeafKey(t *testing.T) {
	for _, caType := range []string{"RSA", "ECDSA"} {
		t.Run(caType, func(t *testing.T) {
			opts := []CertOption{WithCommonName("Test CA"), WithOrganization("TLS Test")}
			if caType == "ECDSA" {
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				require.NoError(t, err)
				opts = append(opts, WithPrivateKey(key))
			}
			ca, caKey, err := GenerateCA(opts...)
			require.NoError(t, err)
			for name, generate := range map[string]func([]byte, []byte, ...CertOption) ([]byte, []byte, error){
				"Server": GenerateServerCert, "Client": GenerateClientCert,
			} {
				t.Run(name, func(t *testing.T) {
					cert, key, err := generate(ca, caKey, WithCommonName("example.test"), WithOrganization("TLS Test"))
					require.NoError(t, err)
					_, err = tls.X509KeyPair(cert, key)
					require.NoError(t, err)
					require.False(t, bytes.Equal(key, caKey), "must not export the issuer private key")
				})
			}
		})
	}
}

func TestEMudhraEmbeddedMaterial(t *testing.T) {
	chain, key := GetEMudhraCAAndKey()
	require.Equal(t, "c83e14e82261ebb52ae8c0ed9bb306a3c2bc7543a6079443bab38200f89df585", fmt.Sprintf("%x", sha256.Sum256(chain)))
	require.Equal(t, "5ca823f143cf31ac1d43f78c1d65d355f316243b0b7ac524d220df414194ef54", fmt.Sprintf("%x", sha256.Sum256(key)))
	originalChain, originalKey := bytes.Clone(chain), bytes.Clone(key)
	chain[0], key[0] = 0, 0
	nextChain, nextKey := GetEMudhraCAAndKey()
	require.Equal(t, originalChain, nextChain, "callers must not mutate the cached CA chain")
	require.Equal(t, originalKey, nextKey, "callers must not mutate the cached signing key")
}
