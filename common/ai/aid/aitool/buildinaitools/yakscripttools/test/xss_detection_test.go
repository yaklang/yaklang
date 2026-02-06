package test

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
)

// loadToolFromEmbedFS reads the yak script directly from embed FS (bypassing DB)
// and converts it to an aitool.Tool ready for testing.
// This ensures we always test the latest compiled version of the script.
func loadToolFromEmbedFS(t *testing.T, scriptPath string, toolName string) *aitool.Tool {
	t.Helper()
	efs := yakscripttools.GetEmbedFS()
	content, err := efs.ReadFile(scriptPath)
	require.NoError(t, err, "read script from embed FS: %s", scriptPath)
	require.NotEmpty(t, content, "script content should not be empty")

	// Verify the XSS detection code is present in the script
	t.Logf("script content length: %d bytes", len(content))

	aiTool := yakscripttools.LoadYakScriptToAiTools(toolName, string(content))
	require.NotNil(t, aiTool, "LoadYakScriptToAiTools should return non-nil for %s", toolName)

	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	require.NotEmpty(t, tools, "ConvertTools should produce at least one tool")

	for _, tool := range tools {
		if tool.Name == toolName {
			return tool
		}
	}
	t.Fatalf("tool %s not found after conversion", toolName)
	return nil
}

// TestXSSDetection_ReflectedXSS tests that the send_http_request_by_url tool
// correctly detects XSS/DOM breakage when a server reflects input without escaping.
func TestXSSDetection_ReflectedXSS(t *testing.T) {
	tool := loadToolFromEmbedFS(t,
		"yakscriptforai/http/send_http_request_by_url.yak",
		"send_http_request_by_url",
	)

	// Mock a vulnerable XSS server: reflects 'name' query param without escaping
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Intentionally NOT escaping -> XSS vulnerability
		w.Write([]byte(fmt.Sprintf("<html><body>Hello %s</body></html>", name)))
	})

	xssPayload := "'><img src=x onerror=alert(1)>"
	targetURL := fmt.Sprintf("http://%s:%d/xss?name=%s", host, port, xssPayload)

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetURL,
	}, nil, w1, w2)

	output := w1.String()
	t.Logf("=== stdout output ===\n%s", output)
	if w2.Len() > 0 {
		t.Logf("=== stderr output ===\n%s", w2.String())
	}

	// Verify XSS detection markers are present
	require.Contains(t, output, "[XSS_DETECTION]",
		"should detect XSS indicators in reflected response")
	require.Contains(t, output, "[XSS_VERDICT]",
		"should output XSS verdict")
	require.True(t,
		strings.Contains(output, "img XSS probe") || strings.Contains(output, "event_handler"),
		"should identify specific XSS indicator type, got output: %s", output)
}

// TestXSSDetection_EventHandler tests detection of event handler injection.
func TestXSSDetection_EventHandler(t *testing.T) {
	tool := loadToolFromEmbedFS(t,
		"yakscriptforai/http/send_http_request_by_url.yak",
		"send_http_request_by_url",
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf("<html><body>Hello %s</body></html>", name)))
	})

	targetURL := fmt.Sprintf("http://%s:%d/xss?name=<svg onload=alert(1)>", host, port)

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetURL,
	}, nil, w1, w2)

	output := w1.String()
	t.Logf("=== stdout output ===\n%s", output)
	if w2.Len() > 0 {
		t.Logf("=== stderr output ===\n%s", w2.String())
	}

	require.Contains(t, output, "[XSS_DETECTION]",
		"should detect SVG onload XSS")
	require.Contains(t, output, "onload=",
		"should identify onload event handler")
}

// TestXSSDetection_ScriptInjection tests detection of script tag injection.
func TestXSSDetection_ScriptInjection(t *testing.T) {
	tool := loadToolFromEmbedFS(t,
		"yakscriptforai/http/send_http_request_by_url.yak",
		"send_http_request_by_url",
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf("<html><body>Hello %s</body></html>", name)))
	})

	targetURL := fmt.Sprintf("http://%s:%d/xss?name=<script>alert(1)</script>", host, port)

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetURL,
	}, nil, w1, w2)

	output := w1.String()
	t.Logf("=== stdout output ===\n%s", output)
	if w2.Len() > 0 {
		t.Logf("=== stderr output ===\n%s", w2.String())
	}

	require.Contains(t, output, "[XSS_DETECTION]",
		"should detect script tag injection")
	require.Contains(t, output, "script tag",
		"should identify script tag indicator")
}

// TestXSSDetection_ParamValueReflection tests that param-value reflection detection works.
func TestXSSDetection_ParamValueReflection(t *testing.T) {
	tool := loadToolFromEmbedFS(t,
		"yakscriptforai/http/send_http_request_by_url.yak",
		"send_http_request_by_url",
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf("<html><body>Hello %s</body></html>", name)))
	})

	xssPayload := "'><img src=x onerror=alert(1)>"
	baseURL := fmt.Sprintf("http://%s:%d/xss?name=test", host, port)

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tool.Callback(context.Background(), aitool.InvokeParams{
		"url":            baseURL,
		"param-position": "query",
		"param-name":     "name",
		"param-value":    xssPayload,
	}, nil, w1, w2)

	output := w1.String()
	t.Logf("=== stdout output ===\n%s", output)
	if w2.Len() > 0 {
		t.Logf("=== stderr output ===\n%s", w2.String())
	}

	require.Contains(t, output, "REFLECTED_PAYLOAD",
		"should detect reflected payload when using param-value")
}

// TestXSSDetection_SafeServer tests that NO XSS is reported when server properly escapes output.
func TestXSSDetection_SafeServer(t *testing.T) {
	tool := loadToolFromEmbedFS(t,
		"yakscriptforai/http/send_http_request_by_url.yak",
		"send_http_request_by_url",
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Properly HTML-encode the user input -> safe
		w.Write([]byte(fmt.Sprintf("<html><body>Hello %s</body></html>", html.EscapeString(name))))
	})

	targetURL := fmt.Sprintf("http://%s:%d/xss?name=<script>alert(1)</script>", host, port)

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetURL,
	}, nil, w1, w2)

	output := w1.String()
	t.Logf("=== stdout output ===\n%s", output)

	// Safe server should NOT trigger XSS detection
	require.NotContains(t, output, "[XSS_DETECTION]",
		"safe server should NOT trigger XSS detection")
	require.NotContains(t, output, "[XSS_VERDICT]",
		"safe server should NOT output XSS verdict")
}

// TestXSSDetection_NonHTMLResponse tests that XSS detection is skipped for non-HTML responses.
func TestXSSDetection_NonHTMLResponse(t *testing.T) {
	tool := loadToolFromEmbedFS(t,
		"yakscriptforai/http/send_http_request_by_url.yak",
		"send_http_request_by_url",
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "<script>alert(1)</script>"}`))
	})

	targetURL := fmt.Sprintf("http://%s:%d/api", host, port)

	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetURL,
	}, nil, w1, w2)

	output := w1.String()
	t.Logf("=== stdout output ===\n%s", output)

	// JSON response should NOT trigger HTML-based XSS detection
	require.NotContains(t, output, "[XSS_DETECTION]",
		"non-HTML response should NOT trigger XSS detection")
}
