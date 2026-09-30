package test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/browser"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestBrowserFeedbackOperationStatusLive(t *testing.T) {
	if os.Getenv("YAK_BROWSER_LIVE_TEST") != "1" {
		t.Skip("set YAK_BROWSER_LIVE_TEST=1 for real Chrome integration")
	}
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/browser/use_browser.yak")
	require.NoError(t, err)
	meta := yakscripttools.LoadYakScriptToAiTools("use_browser", string(content))
	require.NotNil(t, meta)
	tool := yakscripttools.ConvertTools([]*schema.AIYakTool{meta})[0]
	call := func(params aitool.InvokeParams) map[string]any {
		var out, stderr bytes.Buffer
		raw, err := tool.Callback(context.Background(), params, nil, &out, &stderr)
		require.NoError(t, err, stderr.String())
		return utils.InterfaceToGeneralMap(raw)
	}
	result := call(aitool.InvokeParams{"op": "unknown", "session": "feedback-invalid"})
	require.Equal(t, "error", result["operation_status"])
	require.Contains(t, result["error"], "unknown op")
	path, ok := launcher.LookPath()
	if !ok {
		t.Skip("local browser is unavailable; invalid-operation check passed")
	}
	session := "feedback-" + utils.RandStringBytes(12)
	defer call(aitool.InvokeParams{"op": "close", "session": session})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<script>alert("dialog evidence")</script><p>ready</p>`))
	}))
	defer server.Close()
	result = call(aitool.InvokeParams{"op": "open", "url": server.URL, "session": session, "headless": "yes", "exe-path": path, "timeout": 1})
	require.Equal(t, "dialog_open", result["operation_status"])
	require.Equal(t, session, result["session"])
	require.Equal(t, "dialog evidence", utils.InterfaceToGeneralMap(result["dialog"])["message"])
	result = call(aitool.InvokeParams{"op": "handle_dialog", "session": session, "action": "accept"})
	require.Equal(t, "completed", result["operation_status"])
}

// The CI contract is dialog/error/completed status, not Chrome installation or
// startup speed. Keep the live integration above, and deterministically exercise
// the production script's recovery path for alert, confirm, and prompt.
type feedbackFixturePage struct {
	dialog     *browser.JavaScriptDialog
	accepted   bool
	prompt     string
	snapshots  int
	waits      int
	waitBudget []float64
}

func (p *feedbackFixturePage) GetPendingDialog() (*browser.JavaScriptDialog, bool) {
	return p.dialog, p.dialog != nil
}
func (p *feedbackFixturePage) HandleJavaScriptDialog(accept bool, prompt string) error {
	if p.dialog == nil {
		return fmt.Errorf("no pending dialog")
	}
	p.accepted, p.prompt, p.dialog = accept, prompt, nil
	return nil
}
func (p *feedbackFixturePage) Snapshot() (*browser.SnapshotResult, error) {
	if p.dialog != nil {
		return nil, fmt.Errorf("snapshot attempted while dialog blocks page")
	}
	p.snapshots++
	return &browser.SnapshotResult{Text: "plain text", NodeCount: 1, RefMap: browser.NewRefMap()}, nil
}
func (p *feedbackFixturePage) Evaluate(js string) (any, error) { return "fixture recon", nil }
func (p *feedbackFixturePage) WaitFunction(js string, budget ...float64) error {
	p.waits++
	p.waitBudget = budget
	return nil
}

type feedbackFixtureBrowser struct{ page *feedbackFixturePage }

func (b *feedbackFixtureBrowser) Navigate(url string) (*feedbackFixturePage, error) {
	if b.page.dialog != nil {
		return b.page, &browser.DialogBlockingError{Dialog: *b.page.dialog, Err: context.DeadlineExceeded}
	}
	return b.page, nil
}
func (b *feedbackFixtureBrowser) CurrentPage() (*feedbackFixturePage, error) { return b.page, nil }

func TestBrowserFeedbackOperationStatus(t *testing.T) {
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/browser/use_browser.yak")
	require.NoError(t, err)
	meta := yakscripttools.LoadYakScriptToAiTools("use_browser", string(content))
	require.NotNil(t, meta)
	tool := yakscripttools.ConvertTools([]*schema.AIYakTool{meta})[0]
	var out, stderr bytes.Buffer
	raw, err := tool.Callback(context.Background(), aitool.InvokeParams{"op": "unknown", "session": "invalid"}, nil, &out, &stderr)
	require.NoError(t, err)
	invalid := utils.InterfaceToGeneralMap(raw)
	require.Equal(t, "error", invalid["operation_status"])
	require.Contains(t, invalid["error"], "unknown op")
	for _, tc := range []struct {
		kind, action, prompt string
		accept               bool
	}{
		{"alert", "accept", "", true}, {"confirm", "dismiss", "", false}, {"prompt", "accept", "answer", true},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			page := &feedbackFixturePage{dialog: &browser.JavaScriptDialog{Type: tc.kind, Message: "dialog evidence"}}
			instance := &feedbackFixtureBrowser{page: page}
			exports := map[string]any{}
			for key, value := range browser.Exports {
				exports[key] = value
			}
			exports["Open"] = func(opts ...browser.BrowserOption) (*feedbackFixtureBrowser, error) {
				require.Equal(t, "fixture-session", browser.ConfigIDFromOptions(opts...))
				return instance, nil
			}
			exports["Get"] = func(opts ...browser.BrowserOption) (*feedbackFixtureBrowser, error) { return instance, nil }
			call := func(params aitool.InvokeParams) map[string]any {
				params["session"] = "fixture-session"
				raw, _ := executeFixtureScript(t, "yakscriptforai/browser/use_browser.yak", params, map[string]any{"browser": exports, "sleep": func(float64) { panic("fixed sleep in dialog recovery") }})
				return utils.InterfaceToGeneralMap(raw)
			}
			opened := call(aitool.InvokeParams{"op": "open", "url": "http://127.0.0.1/alert", "exe-path": "fixture-chrome", "timeout": 1})
			require.Equal(t, "dialog_open", opened["operation_status"])
			require.Equal(t, "fixture-session", opened["session"])
			require.Equal(t, "dialog evidence", utils.InterfaceToGeneralMap(opened["dialog"])["message"])
			require.Equal(t, "handle_dialog", opened["recovery_operation"])
			require.Equal(t, true, opened["retryable"])
			require.Zero(t, page.snapshots, "dialog recovery must not wait for a page snapshot")
			bad := call(aitool.InvokeParams{"op": "handle_dialog", "action": "invalid"})
			require.Equal(t, "error", bad["operation_status"])
			require.NotNil(t, page.dialog, "invalid action must not accept the dialog")
			handled := call(aitool.InvokeParams{"op": "handle_dialog", "action": tc.action, "value": tc.prompt})
			require.Equal(t, "completed", handled["operation_status"])
			require.Equal(t, tc.accept, page.accepted)
			require.Equal(t, tc.prompt, page.prompt)
			require.Nil(t, page.dialog)
			missing := call(aitool.InvokeParams{"op": "handle_dialog", "action": "accept"})
			require.Equal(t, "error", missing["operation_status"])
			require.Contains(t, missing["error"], "no pending")
		})
	}
}

func TestBrowserOpenPlainTextDoesNotWaitForInteractiveElements(t *testing.T) {
	for _, url := range []string{"http://127.0.0.1/plain", "http://127.0.0.1/#/app"} {
		t.Run(url, func(t *testing.T) {
			page := &feedbackFixturePage{}
			instance := &feedbackFixtureBrowser{page: page}
			exports := map[string]any{}
			for key, value := range browser.Exports {
				exports[key] = value
			}
			exports["Open"] = func(...browser.BrowserOption) (*feedbackFixtureBrowser, error) { return instance, nil }
			result, _ := executeFixtureScript(t, "yakscriptforai/browser/use_browser.yak", aitool.InvokeParams{"op": "open", "url": url, "exe-path": "fixture-chrome"}, map[string]any{"browser": exports, "sleep": func(float64) { panic("fixed rendering sleep") }})
			require.Equal(t, "completed", utils.InterfaceToGeneralMap(result)["operation_status"])
			if strings.Contains(url, "#/") {
				require.Equal(t, 1, page.waits)
				require.Equal(t, []float64{1}, page.waitBudget)
				require.Equal(t, 2, page.snapshots)
			} else {
				require.Zero(t, page.waits)
				require.Equal(t, 1, page.snapshots)
			}
		})
	}
}
