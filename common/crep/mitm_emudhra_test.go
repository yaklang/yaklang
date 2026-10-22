package crep

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

var emudhraCertificateChainPEM, emudhraPrivateKeyPEM = tlsutils.GetEMudhraCAAndKey()

func TestEMudhraCertificateChain(t *testing.T) {
	server, err := NewMITMServer(MITM_UseEMudhra(true))
	require.NoError(t, err)
	pair, err := tls.X509KeyPair([]byte(emudhraCertificateChainPEM), []byte(emudhraPrivateKeyPEM))
	require.NoError(t, err)
	require.Equal(t, int64(4), pair.PrivateKey.(*ecdsa.PrivateKey).D.Int64())
	issuer, err := x509.ParseCertificate(pair.Certificate[0])
	require.NoError(t, err)
	require.Equal(t, "EM OV TLS CA - G3C", issuer.Subject.CommonName)
	root, err := x509.ParseCertificate(pair.Certificate[1])
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(root)

	for _, host := range []string{"example.test", "127.0.0.1", "::1"} {
		for _, obsolete := range []bool{false, true} {
			var chain [][]byte
			if obsolete {
				cert, err := server.mitmConfig.ObsoleteTLS(host, false).GetCertificate(&gmtls.ClientHelloInfo{ServerName: host})
				require.NoError(t, err)
				chain = cert.Certificate
			} else {
				cert, err := server.mitmConfig.GetCertificateByHostname(host)
				require.NoError(t, err)
				chain = cert.Certificate
			}
			require.Len(t, chain, 3)
			require.Equal(t, pair.Certificate, chain[1:])
			leaf, err := x509.ParseCertificate(chain[0])
			require.NoError(t, err)
			require.NoError(t, leaf.CheckSignatureFrom(issuer))
			intermediates := x509.NewCertPool()
			intermediates.AddCert(issuer)
			_, err = leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: intermediates})
			require.NoError(t, err)
		}
	}
}

func TestEMudhraDoesNotReplaceDefaultCA(t *testing.T) {
	before, err := NewMITMServer()
	require.NoError(t, err)
	savedCert, err := os.ReadFile(GetDefaultCaFilePath())
	require.NoError(t, err)
	enabled, err := NewMITMServer(MITM_UseEMudhra(true))
	require.NoError(t, err)
	require.NotEqual(t, before.GetCaCert(), enabled.GetCaCert())
	after, err := NewMITMServer(MITM_UseEMudhra(false))
	require.NoError(t, err)
	require.Equal(t, before.GetCaCert(), after.GetCaCert())
	currentCert, err := os.ReadFile(GetDefaultCaFilePath())
	require.NoError(t, err)
	require.Equal(t, savedCert, currentCert)
}

func TestMITMCertificateChainRejectsWrongIssuer(t *testing.T) {
	_, err := NewMITMServer()
	require.NoError(t, err)
	ca, _ := pem.Decode([]byte(emudhraCertificateChainPEM))
	wrongChain := append(pem.EncodeToMemory(ca), defaultCA...)
	_, err = NewMITMServer(MITM_SetCaCertAndPrivKey(wrongChain, []byte(emudhraPrivateKeyPEM), nil, nil))
	require.ErrorContains(t, err, "verify CA chain certificate")
}

func TestEMudhraHTTPSProxy(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "emudhra-test")
	}))
	defer upstream.Close()
	server, err := NewMITMServer(MITM_UseEMudhra(true), MITM_SetDisableSystemProxy(true))
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.ServerListener(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("MITM listener did not stop")
		}
	})
	proxyURL, err := url.Parse("http://" + listener.Addr().String())
	require.NoError(t, err)
	_, rest := pem.Decode([]byte(emudhraCertificateChainPEM))
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(rest))

	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					RootCAs: roots, MinVersion: version, MaxVersion: version,
				},
				DisableKeepAlives: true,
			}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
			resp, err := client.Get(upstream.URL)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "emudhra-test", string(body))
			require.Len(t, resp.TLS.PeerCertificates, 3)
			require.Equal(t, "EM OV TLS CA - G3C", resp.TLS.PeerCertificates[0].Issuer.CommonName)
			require.NotEmpty(t, resp.TLS.VerifiedChains)
		})
	}
}
