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

const ipInfoToolName = "ip_info"

func getIPInfoTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/recon/ip_info.yak")
	if err != nil {
		t.Fatalf("failed to read ip_info.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(ipInfoToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse ip_info.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execIPInfoTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestIPInfo_ToolMetadata(t *testing.T) {
	tool := getIPInfoTool(t)
	assert.Assert(t, tool != nil, "tool should be loaded")
	assert.Assert(t, tool.Name == ipInfoToolName || tool.Name != "", "tool name should be set")
	t.Logf("tool loaded: name=%s, description=%s", tool.Name, tool.Description)
}

func TestIPInfo_SingleIP(t *testing.T) {
	tool := getIPInfoTool(t)
	stdout, _ := execIPInfoTool(t, tool, aitool.InvokeParams{
		"ip": "8.8.8.8",
	})

	assert.Assert(t, strings.Contains(stdout, "8.8.8.8"), "should reference the queried IP")
	assert.Assert(t, strings.Contains(stdout, "Query Complete"), "should complete query")

	if strings.Contains(stdout, "Country:") {
		t.Logf("MMDB GeoIP available - got geolocation data")
	} else if strings.Contains(stdout, "MMDB query failed") || strings.Contains(stdout, "FALLBACK") {
		t.Logf("MMDB not available - graceful degradation confirmed")
	}
	t.Logf("stdout:\n%s", stdout)
}

func TestIPInfo_MultipleIPs(t *testing.T) {
	tool := getIPInfoTool(t)
	stdout, _ := execIPInfoTool(t, tool, aitool.InvokeParams{
		"ip": "1.1.1.1,8.8.8.8,114.114.114.114",
	})

	assert.Assert(t, strings.Contains(stdout, "Querying 3 IP"), "should report 3 IPs")
	assert.Assert(t, strings.Contains(stdout, "1.1.1.1"), "should include first IP")
	assert.Assert(t, strings.Contains(stdout, "8.8.8.8"), "should include second IP")
	assert.Assert(t, strings.Contains(stdout, "114.114.114.114"), "should include third IP")
	assert.Assert(t, strings.Contains(stdout, "[1/3]"), "should show progress 1/3")
	assert.Assert(t, strings.Contains(stdout, "[2/3]"), "should show progress 2/3")
	assert.Assert(t, strings.Contains(stdout, "[3/3]"), "should show progress 3/3")
	assert.Assert(t, strings.Contains(stdout, "Query Complete"), "should complete")
	t.Logf("stdout:\n%s", stdout)
}

func TestIPInfo_PrivateIP(t *testing.T) {
	tool := getIPInfoTool(t)
	stdout, _ := execIPInfoTool(t, tool, aitool.InvokeParams{
		"ip": "192.168.1.1",
	})

	assert.Assert(t, strings.Contains(stdout, "192.168.1.1"), "should show the private IP")
	assert.Assert(t, strings.Contains(stdout, "Query Complete"), "should complete even for private IPs")
	t.Logf("stdout:\n%s", stdout)
}

func TestIPInfo_LocalhostIP(t *testing.T) {
	tool := getIPInfoTool(t)
	stdout, _ := execIPInfoTool(t, tool, aitool.InvokeParams{
		"ip": "127.0.0.1",
	})

	assert.Assert(t, strings.Contains(stdout, "127.0.0.1"), "should show localhost IP")
	assert.Assert(t, strings.Contains(stdout, "Query Complete"), "should complete")
	t.Logf("stdout:\n%s", stdout)
}

func TestIPInfo_EmptyInput(t *testing.T) {
	tool := getIPInfoTool(t)
	stdout, stderr := execIPInfoTool(t, tool, aitool.InvokeParams{
		"ip": "",
	})

	combined := stdout + stderr
	hasError := strings.Contains(strings.ToLower(combined), "error") ||
		strings.Contains(strings.ToLower(combined), "no valid") ||
		strings.Contains(strings.ToLower(combined), "required")
	assert.Assert(t, hasError, "should report error for empty IP, got:\n%s", combined)
	t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
}

func TestIPInfo_ErrorMessageQuality(t *testing.T) {
	tool := getIPInfoTool(t)
	stdout, _ := execIPInfoTool(t, tool, aitool.InvokeParams{
		"ip": "8.8.8.8",
	})

	if strings.Contains(stdout, "MMDB query failed") {
		assert.Assert(t,
			strings.Contains(stdout, "FALLBACK") || strings.Contains(stdout, "AMAP") || strings.Contains(stdout, "HINT"),
			"when MMDB fails, should provide fallback attempt or helpful hints, got:\n%s", stdout)
		t.Logf("MMDB not available - verified fallback and hint messages")
	} else {
		t.Logf("MMDB available - skipping error message quality check (success path)")
	}
	t.Logf("stdout:\n%s", stdout)
}
