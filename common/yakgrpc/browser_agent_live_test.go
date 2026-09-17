//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/browsertools"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/consts"
)

type liveAgentBridge struct {
	*browser.ExtensionBridgeManager
}

func (b liveAgentBridge) Available() bool { return b.Snapshot().Running }
func (b liveAgentBridge) Connections() []browser.ExtensionBridgeConnection {
	return b.Snapshot().Connections
}
func (b liveAgentBridge) CapabilityCatalog(id string) (*browser.ExtensionBridgeCapabilityCatalog, bool) {
	for _, connection := range b.Connections() {
		if connection.DeviceID == id {
			return connection.CapabilityCatalog, connection.CapabilityCatalog != nil
		}
	}
	return nil, false
}

// Opt-in companion to the extension's verify-agent-gateway.mjs. Uses the actual
// signed Go bridge and Agent tool callbacks; no mock transport or crypto output.
func TestBrowserAgentLiveGateway(t *testing.T) {
	if os.Getenv("YAK_BROWSER_AGENT_E2E") != "1" {
		t.Skip("requires Chromium and the local crypto lab")
	}
	require.NoError(t, consts.SetGormProjectDatabase(filepath.Join(t.TempDir(), "agent-http.db")))
	manager, err := browser.NewExtensionBridgeManager(browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(0))
	defer manager.Close()
	manager.OpenPairingWindow(2 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, pending := range manager.Snapshot().Pending {
					_, _ = manager.ApprovePairing(pending.ID, "Agent integration Chromium", "")
				}
			}
		}
	}()
	bridge := liveAgentBridge{manager}
	tools, err := browsertools.BuildDynamicCapabilityTools(bridge)
	require.NoError(t, err)
	prepare, err := buildBrowserTransformPrepareTool(bridge)
	require.NoError(t, err)
	httpTest, err := buildBrowserHTTPTestTool(bridge)
	require.NoError(t, err)
	tools = append(tools, prepare, httpTest)
	secret := make([]byte, 24)
	_, err = rand.Read(secret)
	require.NoError(t, err)
	token := hex.EncodeToString(secret)
	done := make(chan struct{})
	var finish sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-Test-Token") != token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/finish" {
			finish.Do(func() { close(done) })
			return
		}
		var call struct {
			Tool   string              `json:"tool"`
			Params aitool.InvokeParams `json:"params"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&call); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for _, tool := range tools {
			if tool.Name != call.Tool {
				continue
			}
			result, err := tool.Callback(r.Context(), call.Params, nil, io.Discard, io.Discard)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		http.Error(w, "unknown Agent tool", 404)
	}))
	defer server.Close()
	connection, err := json.Marshal(map[string]string{"endpoint": server.URL, "bridge": manager.Snapshot().URL, "token": token})
	require.NoError(t, err)
	fmt.Printf("YAK_AGENT_E2E=%s\n", connection)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("browser Agent integration did not finish")
	}
}
