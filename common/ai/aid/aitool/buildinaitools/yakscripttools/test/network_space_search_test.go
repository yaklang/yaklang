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

const networkSpaceSearchToolName = "network_space_search"

func getNetworkSpaceSearchTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/recon/network_space_search.yak")
	if err != nil {
		t.Fatalf("failed to read network_space_search.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(networkSpaceSearchToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse network_space_search.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execNetworkSpaceSearchTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestNetworkSpaceSearch_ToolMetadata(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	assert.Assert(t, tool != nil, "tool should load successfully")
	assert.Assert(t, tool.Name == networkSpaceSearchToolName || tool.Name != "", "tool name should be set")
	t.Logf("tool loaded: name=%s, description length=%d", tool.Name, len(tool.Description))
}

func TestNetworkSpaceSearch_InvalidEngine(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	stdout, stderr := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{
		"engine": "nonexistent_engine",
		"query":  "test query",
	})

	combined := stdout + stderr
	assert.Assert(t,
		strings.Contains(strings.ToLower(combined), "invalid engine") || strings.Contains(strings.ToLower(combined), "error"),
		"should report invalid engine error, got:\n%s", combined)
	t.Logf("stdout:\n%s", stdout)
}

func TestNetworkSpaceSearch_EmptyQuery(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	stdout, stderr := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{
		"engine": "fofa",
		"query":  "",
	})

	combined := stdout + stderr
	hasError := strings.Contains(strings.ToLower(combined), "error") ||
		strings.Contains(strings.ToLower(combined), "empty") ||
		strings.Contains(strings.ToLower(combined), "required")
	assert.Assert(t, hasError, "should report empty query error, got:\n%s", combined)
	t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
}

func TestNetworkSpaceSearch_FofaWithoutAPIKey(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	stdout, _ := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{
		"engine":      "fofa",
		"query":       "title=\"test\"",
		"max-records": 5,
	})

	assert.Assert(t, strings.Contains(stdout, "Network Space Search"), "should show header")

	if strings.Contains(stdout, "Search Complete") && strings.Contains(stdout, "Results found:") {
		t.Logf("FOFA API key is configured - search completed")
	} else if strings.Contains(strings.ToLower(stdout), "error") || strings.Contains(strings.ToLower(stdout), "failed") {
		t.Logf("FOFA API key not configured - error path confirmed")
		if strings.Contains(stdout, "HINT") || strings.Contains(stdout, "configure") {
			t.Logf("Helpful hint message provided")
		}
	}
	t.Logf("stdout:\n%s", stdout)
}

func TestNetworkSpaceSearch_ShodanWithoutAPIKey(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	stdout, _ := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{
		"engine":      "shodan",
		"query":       "apache",
		"max-records": 5,
	})

	assert.Assert(t, strings.Contains(stdout, "Network Space Search"), "should show header")
	assert.Assert(t, strings.Contains(stdout, "shodan"), "should reference shodan engine")

	if strings.Contains(stdout, "Search Complete") {
		t.Logf("Shodan API key is configured - search completed")
	} else if strings.Contains(strings.ToLower(stdout), "error") || strings.Contains(strings.ToLower(stdout), "failed") {
		t.Logf("Shodan API key not configured - error path confirmed")
	}
	t.Logf("stdout:\n%s", stdout)
}

func TestNetworkSpaceSearch_AllValidEngineNames(t *testing.T) {
	engines := []string{"fofa", "shodan", "zoomeye", "hunter", "quake"}
	tool := getNetworkSpaceSearchTool(t)

	for i := 0; i < len(engines); i++ {
		eng := engines[i]
		t.Run(eng, func(t *testing.T) {
			stdout, _ := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{
				"engine":      eng,
				"query":       "test",
				"max-records": 1,
			})

			assert.Assert(t, strings.Contains(stdout, "Network Space Search"),
				"engine %s should show header, got:\n%s", eng, stdout)
			assert.Assert(t, !strings.Contains(stdout, "invalid engine"),
				"engine %s should be accepted as valid, got:\n%s", eng, stdout)
		})
	}
}
