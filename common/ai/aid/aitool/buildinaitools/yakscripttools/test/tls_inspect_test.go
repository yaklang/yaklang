package test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const tlsInspectToolName = "tls_inspect"

func getTlsInspectTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/tls/tls_inspect.yak")
	if err != nil {
		t.Fatalf("failed to read tls_inspect.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(tlsInspectToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse tls_inspect.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty, toolCovertHandle may not be registered")
	}
	return tools[0]
}

func execTlsInspectTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

// TestTlsInspect_AIOutputDualChannelEnabled verifies that the script contains
// yakit.AIOutput calls so the framework auto-activates dual-channel mode.
func TestTlsInspect_AIOutputDualChannelEnabled(t *testing.T) {
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/tls/tls_inspect.yak")
	assert.NilError(t, err)
	level := yakscripttools.ParseAIToolEnableAIOutputLog(string(content))
	assert.Equal(t, level, 2, "tls_inspect.yak must contain yakit.AIOutput to enable dual-channel mode")
}

// TestTlsInspect_MissingDomainReturnsClassifiedError verifies that a missing
// domain parameter (required by CLI schema) causes the tool to return an error
// via cli.check(). The framework-level required-param validation fires before
// the script body runs, so the error appears as a tool execution failure.
// The script-level [param error: missing domain] is a secondary guard for
// empty-string values that pass CLI validation.
func TestTlsInspect_MissingDomainReturnsClassifiedError(t *testing.T) {
	tool := getTlsInspectTool(t)
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), aitool.InvokeParams{}, nil, w1, w2)
	// cli.check() with setRequired(true) must produce an error
	assert.Assert(t, err != nil, "missing required domain parameter must return an error")
}

// TestTlsInspect_BugFixLoopVariable verifies the BUG fix: the three probe
// functions (tls.Inspect, tls.InspectForceHttp1_1, tls.InspectForceHttp2) must
// be called individually, not always tls.Inspect. We check that the script
// source does NOT contain the old bug pattern of always calling tls.Inspect
// inside the loop.
func TestTlsInspect_BugFixLoopVariable(t *testing.T) {
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/tls/tls_inspect.yak")
	assert.NilError(t, err)
	source := string(content)
	// The old bug: for inspectCall in [...] { result, err = tls.Inspect(domain) }
	// The fix: the loop variable (probeFn) is actually called, not hardcoded tls.Inspect
	assert.Assert(t, !strings.Contains(source, "result, err = tls.Inspect(domain)"),
		"the old bug pattern 'tls.Inspect(domain)' hardcoded inside loop must be removed")
	assert.Assert(t, strings.Contains(source, "probeFn("),
		"the fix must call the loop variable (probeFn) instead of hardcoding tls.Inspect")
}
