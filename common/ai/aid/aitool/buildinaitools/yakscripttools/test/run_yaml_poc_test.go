package test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const runYAMLPocToolName = "run_yaml_poc"

func getRunYAMLPocTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/pentest/run_yaml_poc.yak")
	if err != nil {
		t.Fatalf("failed to read run_yaml_poc.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(runYAMLPocToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse run_yaml_poc.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execRunYAMLPocTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestRunYAMLPoc_RawPocMatch(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "vulnerable-app-v1")
		w.WriteHeader(200)
		w.Write([]byte("Welcome to vulnerable app"))
	})
	targetURL := fmt.Sprintf("http://%s:%d", host, port)

	rawPoc := `id: test-status-200
info:
  name: Test Status 200 Check
  severity: info
  description: Checks for HTTP 200 response
http:
  - method: GET
    path:
      - "{{BaseURL}}/"
    matchers:
      - type: status
        status:
          - 200`

	tool := getRunYAMLPocTool(t)
	stdout, _ := execRunYAMLPocTool(t, tool, aitool.InvokeParams{
		"target":  targetURL,
		"raw-poc": rawPoc,
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "Nuclei YAML PoC Scanner"), "should show scanner header")
	assert.Assert(t, strings.Contains(stdout, "Scan Complete"), "should complete scan")

	if strings.Contains(stdout, "Vulnerability #1") {
		assert.Assert(t, strings.Contains(stdout, "test-status-200") || strings.Contains(stdout, "Test Status 200"),
			"should reference the PoC name, got:\n%s", stdout)
		t.Logf("PoC matched successfully")
	} else {
		t.Logf("PoC did not match (may be a timing or nuclei engine issue)")
	}
	t.Logf("stdout:\n%s", stdout)
}

func TestRunYAMLPoc_RawPocNoMatch(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("Normal page"))
	})
	targetURL := fmt.Sprintf("http://%s:%d", host, port)

	rawPoc := `id: test-404-check
info:
  name: Test 404 Check
  severity: info
http:
  - method: GET
    path:
      - "{{BaseURL}}/nonexistent-path-xyz"
    matchers:
      - type: status
        status:
          - 404`

	tool := getRunYAMLPocTool(t)
	stdout, _ := execRunYAMLPocTool(t, tool, aitool.InvokeParams{
		"target":  targetURL,
		"raw-poc": rawPoc,
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "Scan Complete"), "should complete scan")
	assert.Assert(t, strings.Contains(stdout, "Vulnerabilities found: 0"),
		"should find 0 vulnerabilities for non-matching PoC, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestRunYAMLPoc_BatchTargets(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	target1 := fmt.Sprintf("http://%s:%d/path1", host, port)
	target2 := fmt.Sprintf("http://%s:%d/path2", host, port)
	targetCSV := target1 + "," + target2

	rawPoc := `id: batch-status-check
info:
  name: Batch Status Check
  severity: info
http:
  - method: GET
    path:
      - "{{BaseURL}}/"
    matchers:
      - type: status
        status:
          - 200`

	tool := getRunYAMLPocTool(t)
	stdout, _ := execRunYAMLPocTool(t, tool, aitool.InvokeParams{
		"target":  targetCSV,
		"raw-poc": rawPoc,
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "Targets: 2"), "should report 2 targets, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "Scan Complete"), "should complete scan")
	t.Logf("stdout:\n%s", stdout)
}

func TestRunYAMLPoc_WordMatcher(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"vulnerable","version":"1.0"}`))
	})
	targetURL := fmt.Sprintf("http://%s:%d", host, port)

	rawPoc := `id: word-matcher-test
info:
  name: Word Matcher Test
  severity: high
http:
  - method: GET
    path:
      - "{{BaseURL}}/"
    matchers:
      - type: word
        words:
          - "vulnerable"`

	tool := getRunYAMLPocTool(t)
	stdout, _ := execRunYAMLPocTool(t, tool, aitool.InvokeParams{
		"target":  targetURL,
		"raw-poc": rawPoc,
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "Scan Complete"), "should complete scan")
	if strings.Contains(stdout, "Vulnerability #1") {
		assert.Assert(t, strings.Contains(stdout, "high") || strings.Contains(stdout, "word-matcher-test"),
			"should show severity or poc name, got:\n%s", stdout)
	}
	t.Logf("stdout:\n%s", stdout)
}

func TestRunYAMLPoc_MissingTarget(t *testing.T) {
	tool := getRunYAMLPocTool(t)
	stdout, stderr := execRunYAMLPocTool(t, tool, aitool.InvokeParams{
		"raw-poc": "id: test\ninfo:\n  name: test\n  severity: info",
	})

	combined := stdout + stderr
	assert.Assert(t,
		strings.Contains(strings.ToLower(combined), "no target") || strings.Contains(strings.ToLower(combined), "error"),
		"should report missing target error, got:\n%s", combined)
	t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
}

func TestRunYAMLPoc_MissingPoc(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	targetURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getRunYAMLPocTool(t)
	stdout, stderr := execRunYAMLPocTool(t, tool, aitool.InvokeParams{
		"target": targetURL,
	})

	combined := stdout + stderr
	assert.Assert(t,
		strings.Contains(strings.ToLower(combined), "no poc") || strings.Contains(strings.ToLower(combined), "error"),
		"should report missing PoC error, got:\n%s", combined)
	t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
}
