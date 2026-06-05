package aireact

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// TestExecuteToolCallInternal_WaitsForMCPStubBeforeInvoke verifies the require_tool path
// blocks on MCPPendingStub until loadMCPServers replaces the stub with a live tool.
func TestExecuteToolCallInternal_WaitsForMCPStubBeforeInvoke(t *testing.T) {
	toolName := "mcp_wait_srv_echo"
	stub := aitool.NewWithoutCallback(toolName, aitool.WithMCPPendingStub(true))
	live := aitool.NewWithoutCallback(
		toolName,
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			_, _ = stdout.Write([]byte("live-ok"))
			return "live-ok", nil
		}),
	)

	mgr := buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{stub}
	}, buildinaitools.WithExtendTools([]*aitool.Tool{stub}, true))

	react, err := NewTestReAct(
		aicommon.WithContext(context.Background()),
		aicommon.WithDisallowMCPServers(false),
		aicommon.WithAiToolManager(mgr),
	)
	require.NoError(t, err)

	go func() {
		time.Sleep(120 * time.Millisecond)
		mgr.OverrideToolByName(live)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _, err := react.executeToolCallInternal(ctx, toolName, aitool.InvokeParams{"x": "1"}, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	switch data := result.Data.(type) {
	case string:
		assert.Equal(t, "live-ok", data)
	case *aitool.ToolExecutionResult:
		assert.Equal(t, "live-ok", data.Result)
	default:
		t.Fatalf("unexpected result data type: %T", result.Data)
	}
}
