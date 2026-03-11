package test

import (
	"bytes"
	"context"
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

const httpResponseDiffToolName = "http_response_diff"

func getHTTPResponseDiffTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/http/http_response_diff.yak")
	if err != nil {
		t.Fatalf("failed to read http_response_diff.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(httpResponseDiffToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse http_response_diff.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execHTTPDiffTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestHTTPResponseDiff_IdenticalResponses(t *testing.T) {
	body := "Hello Identical World"
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(body))
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getHTTPResponseDiffTool(t)
	stdout, _ := execHTTPDiffTool(t, tool, aitool.InvokeParams{
		"url1":    baseURL + "/path1",
		"url2":    baseURL + "/path2",
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "IDENTICAL"), "should report identical responses, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestHTTPResponseDiff_DifferentStatusCodes(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(200)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(403)
			w.Write([]byte("Forbidden"))
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getHTTPResponseDiffTool(t)
	stdout, _ := execHTTPDiffTool(t, tool, aitool.InvokeParams{
		"url1":    baseURL + "/ok",
		"url2":    baseURL + "/forbidden",
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "200"), "should show status 200")
	assert.Assert(t, strings.Contains(stdout, "403"), "should show status 403")
	assert.Assert(t, strings.Contains(stdout, "STATUS CODES DIFFER"), "should report status difference, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestHTTPResponseDiff_DifferentBodies(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/a" {
			w.WriteHeader(200)
			w.Write([]byte("Response A content"))
		} else {
			w.WriteHeader(200)
			w.Write([]byte("Response B content"))
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getHTTPResponseDiffTool(t)
	stdout, _ := execHTTPDiffTool(t, tool, aitool.InvokeParams{
		"url1":    baseURL + "/a",
		"url2":    baseURL + "/b",
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "Body Diff"), "should show body diff section, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "Diff Complete"), "should complete diff")
	t.Logf("stdout:\n%s", stdout)
}

func TestHTTPResponseDiff_BodyOnlyMode(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x" {
			w.WriteHeader(200)
			w.Write([]byte("Body X"))
		} else {
			w.WriteHeader(200)
			w.Write([]byte("Body Y"))
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getHTTPResponseDiffTool(t)
	stdout, _ := execHTTPDiffTool(t, tool, aitool.InvokeParams{
		"url1":         baseURL + "/x",
		"url2":         baseURL + "/y",
		"diff-headers": "no",
		"diff-body":    "yes",
		"timeout":      10,
	})

	assert.Assert(t, !strings.Contains(stdout, "Headers Diff"), "should NOT show headers diff when disabled")
	assert.Assert(t, strings.Contains(stdout, "Body Diff") || strings.Contains(stdout, "IDENTICAL"),
		"should show body diff or identical, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestHTTPResponseDiff_MissingParams(t *testing.T) {
	tool := getHTTPResponseDiffTool(t)
	stdout, stderr := execHTTPDiffTool(t, tool, aitool.InvokeParams{
		"url1": "http://example.com",
	})

	combined := stdout + stderr
	assert.Assert(t,
		strings.Contains(strings.ToLower(combined), "error") || strings.Contains(strings.ToLower(combined), "provide"),
		"should report error for missing params, got:\n%s", combined)
	t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
}
