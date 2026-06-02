package netx

import (
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

func TestClassifyTLSError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantKind  string
		retryable bool
	}{
		{
			name:      "not tls",
			err:       io.ErrUnexpectedEOF,
			wantKind:  tlsErrorKindHandshake,
			retryable: true,
		},
		{
			name:      "plain http",
			err:       errString("tls: first record does not look like a TLS handshake"),
			wantKind:  tlsErrorKindNotTLS,
			retryable: false,
		},
		{
			name:      "client certificate required",
			err:       errString("remote error: tls: certificate required"),
			wantKind:  tlsErrorKindCertificate,
			retryable: false,
		},
		{
			name:      "alpn",
			err:       errString("remote error: tls: no application protocol"),
			wantKind:  tlsErrorKindALPN,
			retryable: true,
		},
		{
			name:      "version",
			err:       errString("remote error: tls: protocol version not supported"),
			wantKind:  tlsErrorKindVersion,
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, retryable, suggestion := classifyTLSError(tt.err)
			require.Equal(t, tt.wantKind, kind)
			require.Equal(t, tt.retryable, retryable)
			require.NotEmpty(t, suggestion)
		})
	}
}

func TestBuildTLSRetryCandidates(t *testing.T) {
	config := &dialXConfig{
		TLSNextProto: []string{"h2", "http/1.1"},
		TraceInfo:    NewDialXTraceInfo(),
	}
	tlsConfig := &gmtls.Config{
		MinVersion: gmtls.VersionTLS10,
		MaxVersion: gmtls.VersionTLS13,
	}

	candidates := buildTLSRetryCandidates("example.com:443", config, tlsConfig, "example.com", nil, tlsErrorKindALPN, "alpn suggestion")
	require.Len(t, candidates, 2)
	require.Equal(t, "alpn-http11", candidates[0].name)
	require.Equal(t, "alpn-disabled", candidates[1].name)

	candidates = buildTLSRetryCandidates("example.com:443", config, tlsConfig, "example.com", nil, tlsErrorKindVersion, "version suggestion")
	require.Len(t, candidates, 1)
	require.Equal(t, "tls12-only", candidates[0].name)

	config.ShouldOverrideSNI = true
	candidates = buildTLSRetryCandidates("example.com:443", config, tlsConfig, "bad.example", nil, tlsErrorKindSNI, "sni suggestion")
	require.Empty(t, candidates)
	require.Contains(t, strings.Join(config.TraceInfo.TLSRetryTips, "\n"), "用户已显式设置 SNI")
}

func TestDialXTLSRetryChromeFallbackSuccess(t *testing.T) {
	addr, closeServer := startTLSRetryTestServer(t, func(n int32, conn net.Conn) {
		if n == 1 {
			_ = conn.Close()
			return
		}
		tlsConn := tlsutils.NewDefaultTLSServer(conn)
		_, _ = tlsConn.Write([]byte("ok"))
		_ = tlsConn.Close()
	})
	defer closeServer()

	trace := NewDialXTraceInfo()
	conn, err := DialX(
		addr,
		DialX_WithTLS(true),
		DialX_WithDialTraceInfo(trace),
		DialX_WithTimeout(2*time.Second),
		DialX_WithTLSTimeout(2*time.Second),
	)
	require.NoError(t, err)
	defer conn.Close()

	buf := make([]byte, 2)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, "ok", string(buf))
	require.GreaterOrEqual(t, trace.TLSRetryCount, 1)
	require.Contains(t, strings.Join(trace.TLSRetryTips, "\n"), "Chrome TLS 指纹重试")
}

func TestDialXTLSRetryNotTLSNoRetry(t *testing.T) {
	addr, closeServer := startTLSRetryTestServer(t, func(_ int32, conn net.Conn) {
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
		_ = conn.Close()
	})
	defer closeServer()

	trace := NewDialXTraceInfo()
	conn, err := DialX(
		addr,
		DialX_WithTLS(true),
		DialX_WithDialTraceInfo(trace),
		DialX_WithTimeout(2*time.Second),
		DialX_WithTLSTimeout(2*time.Second),
	)
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
	require.Zero(t, trace.TLSRetryCount)
	require.Contains(t, strings.Join(trace.TLSRetryTips, "\n"), "疑似非 TLS 服务")
	require.Contains(t, err.Error(), "目标端口疑似不是 TLS 服务")
	require.Contains(t, err.Error(), "原始错误")
	require.Contains(t, err.Error(), "TLS重试: 未进行 TLS 兼容重试")
	require.NotContains(t, err.Error(), "调试配置")
}

func startTLSRetryTestServer(t *testing.T, serve func(int32, net.Conn)) (string, func()) {
	t.Helper()

	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	var count atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serve(count.Add(1), conn)
		}
	}()

	return addr, func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}
