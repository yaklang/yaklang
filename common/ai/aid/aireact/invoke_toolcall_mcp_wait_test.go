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
	data, ok := result.Data.(string)
	require.True(t, ok, "AI orchestration must expose the canonical bounded string Data")
	assert.Contains(t, data, "COMBINED OUTPUT:\nlive-ok")
	assert.Contains(t, data, "HINT:\nComplete tool output is stored in artifacts:")
}
