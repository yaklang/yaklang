package yaklib

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

func TestMitmEMudhraOption(t *testing.T) {
	option := MitmExports["emudhra"].(func(bool) MitmConfigOpt)
	server, err := NewMITMServer(option(true), mitmConfigUseDefault(false))
	require.NoError(t, err)
	block, rest := pem.Decode(server.GetCaCert())
	require.NotNil(t, block)
	ca, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "EM OV TLS CA - G3C", ca.Subject.CommonName)
	require.NotEmpty(t, rest)

	_, err = NewMITMServer(option(true), mitmConfigCertAndKey([]byte("custom CA"), []byte("custom key")))
	require.ErrorContains(t, err, "cannot be used together")

	server, err = NewMITMServer(option(true), option(false))
	require.NoError(t, err)
	block, _ = pem.Decode(server.GetCaCert())
	require.NotNil(t, block)
	ca, err = x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.NotEqual(t, "EM OV TLS CA - G3C", ca.Subject.CommonName)
}

func TestMitmEMudhraOptionCustomGMCA(t *testing.T) {
	ca, key, err := tlsutils.GenerateGMSelfSignedCertKey("Yak Custom GM CA")
	require.NoError(t, err)
	server, err := NewMITMServer(mitmConfigGMCertAndKey(ca, key), mitmConfigEMudhra(true))
	require.NoError(t, err)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer upstream.Close()
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
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 10*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	target := upstream.Listener.Addr().String()
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	roots := gmx509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(ca))
	client := gmtls.Client(conn, &gmtls.Config{
		GMSupport: &gmtls.GMSupport{}, RootCAs: roots, ServerName: "127.0.0.1",
	})
	require.NoError(t, client.Handshake())
	require.Equal(t, "Yak Custom GM CA", client.ConnectionState().PeerCertificates[0].Issuer.CommonName)
}
