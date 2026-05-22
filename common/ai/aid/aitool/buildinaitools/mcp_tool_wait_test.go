package buildinaitools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestIsMCPInitializingError(t *testing.T) {
	if !IsMCPInitializingError(utils.Errorf("%s server not ready", MCPToolInitializingErrPrefix)) {
		t.Fatal("expected initializing error to be detected")
	}
	if IsMCPInitializingError(utils.Error("tool not found")) {
		t.Fatal("unexpected initializing detection")
	}
}

func TestWaitForMCPLiveTool_ReplacesStub(t *testing.T) {
	stub := aitool.NewWithoutCallback("mcp_srv_echo", aitool.WithMCPPendingStub(true))
	live := aitool.NewWithoutCallback("mcp_srv_echo")

	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{stub}
	}, WithExtendTools([]*aitool.Tool{stub}, true))

	done := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		mgr.OverrideToolByName(live) // clears MCPPendingStub on replacement
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := WaitForMCPLiveTool(ctx, mgr, "mcp_srv_echo", 3*time.Second, 50*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("WaitForMCPLiveTool failed: %v", err)
	}
	if IsMCPPendingStub(got) {
		t.Fatal("expected live tool after wait")
	}
	<-done
}

func TestMCPToolInitWaitTimeout_Default(t *testing.T) {
	if MCPToolInitWaitTimeout != 10*time.Second {
		t.Fatalf("expected default MCP wait timeout 10s, got %v", MCPToolInitWaitTimeout)
	}
	if MCPToolInitPollInterval != 2*time.Second {
		t.Fatalf("expected default MCP poll interval 2s, got %v", MCPToolInitPollInterval)
	}
}

func TestWaitForMCPLiveTool_TimeoutOnPersistentStub(t *testing.T) {
	stub := aitool.NewWithoutCallback("mcp_srv_stuck", aitool.WithMCPPendingStub(true))
	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{stub}
	}, WithExtendTools([]*aitool.Tool{stub}, true))

	ctx := context.Background()
	_, err := WaitForMCPLiveTool(ctx, mgr, "mcp_srv_stuck", 150*time.Millisecond, 40*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout error while stub persists")
	}
	if !strings.Contains(err.Error(), "still initializing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsMCPInitializingMessage(t *testing.T) {
	if !IsMCPInitializingMessage(MCPToolInitializingErrPrefix + " pending") {
		t.Fatal("expected initializing message")
	}
	if IsMCPInitializingMessage("unrelated error") {
		t.Fatal("unexpected initializing message detection")
	}
}
