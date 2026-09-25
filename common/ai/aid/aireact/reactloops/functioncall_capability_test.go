package reactloops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestResolveFunctionCallModeUsesProbeBeforePrompt(t *testing.T) {
	var probes int
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		probes++
		options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		require.Len(t, options.Tools, 1)
		resp := aicommon.NewAIResponse(c)
		resp.EmitOutputStream(strings.NewReader("plain text, no tool call"))
		resp.Close()
		return resp, nil
	}))
	loop := &ReActLoop{config: cfg, loopName: "probe", functionCallMode: true, functionCallModeRequested: true}
	loop.resolveFunctionCallMode(context.Background())
	require.False(t, loop.functionCallMode)
	loop.functionCallMode = true
	loop.resolveFunctionCallMode(context.Background())
	require.False(t, loop.functionCallMode)
	require.Equal(t, 1, probes, "same config and tier should reuse the result")
}

func TestResolveFunctionCallModeExplicitOverrideSkipsProbe(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := &ReActLoop{config: cfg, functionCallMode: true, functionCallModeRequested: true, functionCallModeExplicit: true}
	loop.resolveFunctionCallMode(context.Background())
	require.True(t, loop.functionCallMode)
}
