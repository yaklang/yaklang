package browser

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestExtensionBridgeListenRetry(t *testing.T) {
	for _, cause := range []error{syscall.EADDRINUSE, syscall.EACCES, syscall.EPERM} {
		err := &net.OpError{Op: "listen", Net: "tcp", Err: fmt.Errorf("bind: %w", cause)}
		require.True(t, retryableExtensionBridgeListenError(err))
	}
	for _, code := range []syscall.Errno{10013, 10048} {
		require.Equal(t, runtime.GOOS == "windows", retryableExtensionBridgeListenError(fmt.Errorf("bind: %w", code)))
	}
	require.False(t, retryableExtensionBridgeListenError(syscall.EMFILE))
	require.False(t, retryableExtensionBridgeListenError(errors.New("invalid endpoint")))
}

func TestExtensionBridgeDiscoveryPortWindows(t *testing.T) {
	ports := extensionBridgeCandidatePorts(64333)
	require.Len(t, ports, 64)
	require.Equal(t, 64333, ports[0])
	for _, port := range []int{64348, 14333, 14348, 24333, 34333} {
		require.Contains(t, ports, port)
	}
	seen := make(map[int]bool)
	for _, port := range ports {
		require.False(t, seen[port])
		seen[port] = true
	}
	require.Len(t, extensionBridgeCandidatePorts(65535), 65)
	require.Equal(t, []int{0}, extensionBridgeCandidatePorts(0))
}

func TestExtensionBridgeDiscoveryEndpoint(t *testing.T) {
	manager, err := NewExtensionBridgeManager(NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(0))
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	snapshot := manager.Snapshot()
	endpoint := strings.TrimSuffix(snapshot.URL, extensionBridgePath) + extensionBridgeDiscoveryPath
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": {"chrome-extension://discovery-test"}})
	require.NoError(t, err)
	defer conn.Close()
	var discovered extensionBridgeDiscoveryEnvelope
	require.NoError(t, conn.ReadJSON(&discovered))
	require.Equal(t, "engine", discovered.Type)
	require.Equal(t, managedExtensionBridgeProtocolVersion, discovered.ProtocolVersion)
	require.Equal(t, snapshot.EngineIdentityID, discovered.EngineIdentityID)
	require.Equal(t, snapshot.EngineInstanceID, discovered.EngineInstanceID)
	require.Equal(t, snapshot.URL, discovered.Endpoint)

	_, rejected, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": {"https://example.test"}})
	require.Error(t, err)
	require.NotNil(t, rejected)
	defer rejected.Body.Close()
	require.Equal(t, http.StatusForbidden, rejected.StatusCode)
}

func TestExtensionBridgeDiscoverySkipsOccupiedPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	manager, err := NewExtensionBridgeManager(NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(port))
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.NotEqual(t, fmt.Sprintf("ws://127.0.0.1:%d/extension", port), manager.Snapshot().URL)
}
