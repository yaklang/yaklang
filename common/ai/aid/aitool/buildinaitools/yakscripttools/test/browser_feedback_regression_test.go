package test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestBrowserFeedbackOperationStatus(t *testing.T) {
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
