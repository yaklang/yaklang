package mustpass

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

func TestTLSInspect_StandardTLS(t *testing.T) {
	// Test standard TLS inspection with DebugMockHTTPS
	host, port := utils.DebugMockHTTPSEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	})

	addr := utils.HostPort(host, port)
	log.Infof("testing TLS inspect on standard TLS server: %s", addr)

	// Use short timeout for faster testing
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := netx.TLSInspectContext(ctx, addr)
	require.NoError(t, err)
	require.NotEmpty(t, results, "should get at least one certificate from standard TLS server")

	for _, r := range results {
		log.Infof("TLS Version: 0x%04x, CipherSuite: 0x%04x, Protocol: %s", r.Version, r.CipherSuite, r.Protocol)
		log.Infof("Description preview: %s", r.Description[:min(200, len(r.Description))])
		require.NotEmpty(t, r.Raw, "certificate raw bytes should not be empty")
		require.NotEmpty(t, r.Description, "certificate description should not be empty")
	}
}

func TestTLSInspect_GMTLS(t *testing.T) {
	// Test GM TLS inspection with DebugMockOnlyGMHTTP
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	host, port := utils.DebugMockOnlyGMHTTP(ctx, func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	})

	addr := utils.HostPort(host, port)
	log.Infof("testing TLS inspect on GM TLS only server: %s", addr)

	// Use short timeout for faster testing
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer inspectCancel()

	results, err := netx.TLSInspectContext(inspectCtx, addr)
	require.NoError(t, err)
	require.NotEmpty(t, results, "should get at least one certificate from GM TLS server")

	foundGMTLS := false
	for _, r := range results {
		log.Infof("TLS Version: 0x%04x, CipherSuite: 0x%04x, Protocol: %s, IsGMTLS: %v", r.Version, r.CipherSuite, r.Protocol, r.IsGMTLS())
		log.Infof("Description preview: %s", r.Description[:min(200, len(r.Description))])
		require.NotEmpty(t, r.Raw, "certificate raw bytes should not be empty")
		require.NotEmpty(t, r.Description, "certificate description should not be empty")
		if r.IsGMTLS() {
			foundGMTLS = true
		}
	}
	require.True(t, foundGMTLS, "should find at least one GM TLS certificate")
}

func TestTLSInspect_ForceProtocols(t *testing.T) {
	// Test protocol forcing with standard TLS
	host, port := utils.DebugMockHTTPSEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	})

	addr := utils.HostPort(host, port)

	t.Run("ForceHTTP1_1", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		results, err := netx.TLSInspectContext(ctx, addr, "http/1.1")
		require.NoError(t, err)
		require.NotEmpty(t, results, "should get certificates when forcing HTTP/1.1")
		for _, r := range results {
			log.Infof("HTTP/1.1 - TLS Version: 0x%04x, Protocol: %s", r.Version, r.Protocol)
		}
	})

	t.Run("ForceHTTP2", func(t *testing.T) {
		// HTTP/2 might not be supported by the mock server, but we should still get certificates
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		results, err := netx.TLSInspectContext(ctx, addr, "h2")
		require.NoError(t, err)
		// Note: might be empty if server doesn't support H2, but should not error
		log.Infof("HTTP/2 inspection returned %d results", len(results))
	})
}

func TestTLSInspect_Deduplication(t *testing.T) {
	// Test that duplicate certificates are properly deduplicated
	host, port := utils.DebugMockHTTPSEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	})

	addr := utils.HostPort(host, port)
	log.Infof("testing TLS inspect deduplication: %s", addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := netx.TLSInspectContext(ctx, addr)
	require.NoError(t, err)

	// Check for duplicates
	seen := make(map[string]bool)
	for _, r := range results {
		key := string(r.Raw)
		require.False(t, seen[key], "found duplicate certificate in results")
		seen[key] = true
	}
	log.Infof("deduplication test passed with %d unique certificates", len(results))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
