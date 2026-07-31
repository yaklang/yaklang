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

const simpleCrawlerToolName = "simple_crawler"

func getSimpleCrawlerTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/http/simple_crawler.yak")
	if err != nil {
		t.Fatalf("failed to read simple_crawler.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(simpleCrawlerToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse simple_crawler.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty, toolCovertHandle may not be registered")
	}
	return tools[0]
}

func execCrawlerTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("crawler tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

// TestSimpleCrawler_CoverageHint nudges the AI toward broad coverage via
// adjust_todo when the crawl discovers >1 subdomain or pending URLs.
//
// This is the fix for a field coverage gap: the crawler found multiple
// subdomains and many URLs, but the AI fixated on ONE subdomain (desk) and
// skipped the rest (mall, id/portal). Rather than hard-coding an attack-surface
// inventory (over-fit to one shape), the crawler now emits a lightweight hint
// pushing the AI to break the remaining surface into multiple todos, so more
// todos drive better depth and completeness.
func TestSimpleCrawler_CoverageHint(t *testing.T) {
	// port is assigned by DebugMockHTTPHandlerFunc below; capture it for the
	// handler closure via a pointer so the landing page can self-link.
	var port int
	host, p := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Landing page links to several internal paths and two other virtual
		// hostnames on the same loopback server, so the crawler discovers
		// multiple subdomains and pending URLs.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><body>
<a href="/portal">portal</a>
<a href="/home">home</a>
<a href="/api/v1/users">users api</a>
<a href="/static/js/app.js">js</a>
<a href="http://desk.localhost:` + strconv.Itoa(port) + `/desk">desk</a>
<a href="http://mall.localhost:` + strconv.Itoa(port) + `/mall">mall</a>
</body></html>`))
	})
	port = p
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getSimpleCrawlerTool(t)
	stdout, _ := execCrawlerTool(t, tool, aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  20,
		"max-depth": 3,
		"timeout":   5,
	})

	// The coverage hint must be emitted (multiple subdomains were discovered).
	assert.Assert(t, strings.Contains(stdout, "[coverage hint]"),
		"should emit the [coverage hint] when >1 subdomain or pending URLs are found; got:\n%s", stdout)

	// The hint must steer toward adjust_todo / multiple todos (not a fixed inventory).
	assert.Assert(t, strings.Contains(stdout, "adjust_todo"),
		"hint should steer the AI toward adjust_todo; got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "todo"),
		"hint should mention breaking work into todos")

	// The standard crawl output must still be present (summary + requested URLs).
	assert.Assert(t, strings.Contains(stdout, "=== Crawl Summary ==="),
		"standard crawl summary should still be present")
	assert.Assert(t, strings.Contains(stdout, "=== Requested URLs ==="),
		"requested URLs section should still be present")
}
