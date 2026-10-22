package yakgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITMV2_EMudhra(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "emudhra-grpc-test")
	}))
	defer upstream.Close()

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			expected, err := crep.NewMITMServer(crep.MITM_UseEMudhra(enabled))
			require.NoError(t, err)
			signingCA, rest := pem.Decode(expected.GetCaCert())
			require.NotNil(t, signingCA)
			trustAnchor := signingCA
			if enabled {
				trustAnchor, _ = pem.Decode(rest)
				require.NotNil(t, trustAnchor)
			}
			root, err := x509.ParseCertificate(trustAnchor.Bytes)
			require.NoError(t, err)
			signer, err := x509.ParseCertificate(signingCA.Bytes)
			require.NoError(t, err)
			roots := x509.NewCertPool()
			roots.AddCert(root)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			port := uint32(utils.GetRandomAvailableTCPPort())
			var recvMessage func() (string, error)
			{
				stream, err := client.MITMV2(ctx)
				require.NoError(t, err)
				require.NoError(t, stream.Send(&ypb.MITMV2Request{
					Host: "127.0.0.1", Port: port, UseEMudhra: enabled, DisableSystemProxy: true,
					SetAutoForward: true, AutoForwardValue: true, DisableTrafficGuard: true,
				}))
				recvMessage = func() (string, error) {
					msg, err := stream.Recv()
					if err != nil {
						return "", err
					}
					return string(msg.GetMessage().GetMessage()), nil
				}
			}
			for {
				message, err := recvMessage()
				require.NoError(t, err)
				if strings.Contains(message, "starting mitm server") {
					break
				}
			}
			proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
			require.NoError(t, err)
			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: roots},
				DisableKeepAlives: true,
			}
			defer transport.CloseIdleConnections()
			httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			resp, err := httpClient.Get(upstream.URL)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "emudhra-grpc-test", string(body))
			require.Equal(t, signer.Raw, resp.TLS.PeerCertificates[1].Raw)
			require.NotEmpty(t, resp.TLS.VerifiedChains)
			if enabled {
				require.Len(t, resp.TLS.PeerCertificates, 3)
			}
		})
	}
}
