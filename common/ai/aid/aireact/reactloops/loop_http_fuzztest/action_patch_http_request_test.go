package loop_http_fuzztest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func newHTTPFuzztestLoopForPatchTest(t *testing.T) *reactloops.ReActLoop {
	t.Helper()
	invoker := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create http_fuzztest loop: %v", err)
	}
	return loop
}

func prepareHTTPFuzztestPatchLoop(t *testing.T, rawRequest string, isHTTPS bool) *reactloops.ReActLoop {
	t.Helper()
	loop := newHTTPFuzztestLoopForPatchTest(t)
	fuzzReq, err := newLoopFuzzRequest(context.Background(), loop.GetInvoker(), []byte(rawRequest), isHTTPS)
	if err != nil {
		t.Fatalf("new fuzz request: %v", err)
	}
	storeLoopFuzzRequestState(loop, fuzzReq, []byte(rawRequest), isHTTPS)
	return loop
}

func executeHTTPPatchAction(t *testing.T, loop *reactloops.ReActLoop, params map[string]any) *reactloops.LoopActionHandlerOperator {
	t.Helper()
	handler, err := loop.GetActionHandler("patch_http_request")
	if err != nil {
		t.Fatalf("get patch_http_request action: %v", err)
	}

	actionMap := map[string]any{
		"@action": "patch_http_request",
	}
	for key, value := range params {
		actionMap[key] = value
	}
	rawAction, err := json.Marshal(actionMap)
	if err != nil {
		t.Fatalf("marshal action: %v", err)
	}
	action, err := aicommon.ExtractAction(string(rawAction), "patch_http_request")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := handler.ActionVerifier(loop, action); err != nil {
		t.Fatalf("verify action: %v", err)
	}

	task := aicommon.NewStatefulTaskBase("patch-http-request-test", "patch current request", context.Background(), loop.GetEmitter())
	operator := reactloops.NewActionHandlerOperator(task)
	handler.ActionHandler(loop, action, operator)
	if _, err := operator.IsTerminated(); err != nil {
		t.Fatalf("patch action failed: %v", err)
	}
	return operator
}

func TestPatchHTTPRequestAction_AddHeaderAppliesToCurrentRequest(t *testing.T) {
	loop := prepareHTTPFuzztestPatchLoop(t, "GET /orders?id=1 HTTP/1.1\r\nHost: example.com\r\n\r\n", false)

	operator := executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "header",
		"operation":   "add",
		"field_name":  "X-Test-Probe",
		"field_value": "yak",
		"reason":      "补一个测试头观察服务端差异。",
	})

	currentRequest := loop.Get("current_request")
	if !strings.Contains(currentRequest, "X-Test-Probe: yak") {
		t.Fatalf("expected current request to contain patched header, got:\n%s", currentRequest)
	}
	if loop.Get("previous_request") == "" {
		t.Fatal("expected previous_request to be preserved after patch apply")
	}
	if !strings.Contains(loop.Get("request_change_summary"), "X-Test-Probe: yak") {
		t.Fatalf("expected request change summary to mention patched header, got:\n%s", loop.Get("request_change_summary"))
	}
	if !strings.Contains(operator.GetFeedback().String(), "HTTP 数据包补丁已应用") {
		t.Fatalf("expected feedback to mention applied patch, got:\n%s", operator.GetFeedback().String())
	}
}

func TestPatchHTTPRequestAction_RepairAddsBaselineHeadersAndContentType(t *testing.T) {
	raw := "POST /login HTTP/1.1\r\nHost: example.com\r\n\r\nusername=admin&password=test"
	loop := prepareHTTPFuzztestPatchLoop(t, raw, false)

	operator := executeHTTPPatchAction(t, loop, map[string]any{
		"location":       "request",
		"operation":      "repair",
		"repair_profile": "browser_like",
		"reason":         "把当前包修成更像真实浏览器请求，便于后续继续做安全验证。",
	})

	currentRequest := loop.Get("current_request")
	for _, needle := range []string{
		"User-Agent:",
		"Accept: */*",
		"Accept-Language: en-US,en;q=0.9",
		"Connection: close",
		"Content-Type: application/x-www-form-urlencoded",
	} {
		if !strings.Contains(currentRequest, needle) {
			t.Fatalf("expected repaired request to contain %q, got:\n%s", needle, currentRequest)
		}
	}
	if !strings.Contains(operator.GetFeedback().String(), "repair browser_like") {
		t.Fatalf("expected repair feedback to mention repair profile, got:\n%s", operator.GetFeedback().String())
	}
}

func TestPatchHTTPRequestAction_ReplaceJSONBodyField(t *testing.T) {
	raw := "POST /profile HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n{\"role\":\"user\"}"
	loop := prepareHTTPFuzztestPatchLoop(t, raw, false)

	executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "body.json",
		"operation":   "replace",
		"field_name":  "role",
		"field_value": "admin",
		"reason":      "将 JSON 字段改成更高权限值，观察是否存在权限边界问题。",
	})

	currentRequest := loop.Get("current_request")
	if !strings.Contains(currentRequest, `"role":"admin"`) {
		t.Fatalf("expected JSON body field to be replaced, got:\n%s", currentRequest)
	}
	if !strings.Contains(currentRequest, "Content-Type: application/json") {
		t.Fatalf("expected json body patch to preserve application/json content type, got:\n%s", currentRequest)
	}
}

func TestPatchHTTPRequestAction_RewritesBasicAuthorizationHeader(t *testing.T) {
	loop := prepareHTTPFuzztestPatchLoop(t, "GET /debug/config HTTP/1.1\r\nHost: api.internal\r\nAuthorization: Basic dGVzdGVyOjEyMzQ1Ng==\r\n\r\n", false)

	executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "auth.basic",
		"operation":   "replace",
		"field_value": `{"username":"superadmin","password":"P@ssw0rd2026"}`,
		"reason":      "将 Basic 认证账号密码改成新的测试凭据。",
	})

	expected := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("superadmin:P@ssw0rd2026"))
	if currentRequest := loop.Get("current_request"); !strings.Contains(currentRequest, expected) {
		t.Fatalf("expected rewritten Authorization header %q, got:\n%s", expected, currentRequest)
	}
}

func TestPatchHTTPRequestAction_TransformsJSONBodyToXML(t *testing.T) {
	raw := "POST /data HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n{\"user\":\"guest\",\"action\":\"ping\",\"tags\":[\"test\",\"dev\"]}"
	loop := prepareHTTPFuzztestPatchLoop(t, raw, false)

	executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "body.format",
		"operation":   "transform",
		"field_name":  "root",
		"field_value": "xml",
		"reason":      "把 JSON Body 转成 XML，验证接口对不同序列化格式的处理。",
	})

	currentRequest := loop.Get("current_request")
	if !strings.Contains(currentRequest, "Content-Type: application/xml") {
		t.Fatalf("expected xml content type, got:\n%s", currentRequest)
	}
	for _, needle := range []string{"<root>", "<user>guest</user>", "<action>ping</action>", "<tags><item>test</item><item>dev</item></tags>"} {
		if !strings.Contains(currentRequest, needle) {
			t.Fatalf("expected XML body to contain %q, got:\n%s", needle, currentRequest)
		}
	}
}

func TestPatchHTTPRequestAction_RewritesBearerJWTClaimsWithoutVerification(t *testing.T) {
	loop := prepareHTTPFuzztestPatchLoop(t, "GET /admin HTTP/1.1\r\nHost: example.com\r\nAuthorization: Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJyb2xlIjoidXNlciIsInVpZCI6MX0\r\n\r\n", false)

	executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "auth.bearer.jwt",
		"operation":   "replace",
		"field_value": `{"role":"admin"}`,
		"reason":      "在无签名校验假设下修改 JWT payload 中的角色字段。",
	})

	currentRequest := loop.Get("current_request")
	if !strings.Contains(currentRequest, "Authorization: Bearer ") {
		t.Fatalf("expected bearer auth header after rewrite, got:\n%s", currentRequest)
	}
	if !strings.Contains(currentRequest, "eyJyb2xlIjoiYWRtaW4iLCJ1aWQiOjF9") {
		t.Fatalf("expected rewritten JWT payload segment, got:\n%s", currentRequest)
	}
}

type patchActionTestInvoker struct {
	*mock.MockInvoker
	artifactDir string
	events      []*schema.AiOutputEvent
	mu          sync.Mutex
}

func newPatchActionTestInvoker(t *testing.T) *patchActionTestInvoker {
	t.Helper()
	invoker := &patchActionTestInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		artifactDir: t.TempDir(),
	}
	if cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig); ok {
		cfg.Emitter = aicommon.NewEmitter("http-fuzztest-patch-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			invoker.mu.Lock()
			defer invoker.mu.Unlock()
			invoker.events = append(invoker.events, e)
			return e, nil
		})
	}
	return invoker
}

func (i *patchActionTestInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return filepath.Join(i.artifactDir, name+ext)
}

func collectHTTPPacketStreamContent(events []*schema.AiOutputEvent) string {
	var out strings.Builder
	for _, event := range events {
		if event.NodeId == "http_flow" && event.IsStream && event.ContentType == aicommon.TypeCodeHTTPRequest && len(event.StreamDelta) > 0 {
			out.Write(event.StreamDelta)
		}
	}
	return out.String()
}

func collectLatestRequestChangeEvent(t *testing.T, events []*schema.AiOutputEvent) loopHTTPFuzzRequestChangeEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != schema.EVENT_TYPE_HTTP_FUZZ_REQUEST_CHANGE || event.NodeId != loopHTTPFuzzRequestChangeEventNode {
			continue
		}
		var payload loopHTTPFuzzRequestChangeEvent
		if err := json.Unmarshal(event.Content, &payload); err != nil {
			t.Fatalf("unmarshal request change event: %v", err)
		}
		return payload
	}
	t.Fatal("expected request change event")
	return loopHTTPFuzzRequestChangeEvent{}
}

func TestPatchHTTPRequestAction_EmitsPatchedPacketToUser(t *testing.T) {
	invoker := newPatchActionTestInvoker(t)
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}

	rawRequest := "GET /orders?id=1 HTTP/1.1\r\nHost: example.com\r\n\r\n"
	fuzzReq, err := newLoopFuzzRequest(context.Background(), invoker, []byte(rawRequest), false)
	if err != nil {
		t.Fatalf("new fuzz request: %v", err)
	}
	storeLoopFuzzRequestState(loop, fuzzReq, []byte(rawRequest), false)

	_ = executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "header",
		"operation":   "add",
		"field_name":  "X-Debug-Case",
		"field_value": "visible",
		"reason":      "修改后应把完整新包展示给用户。",
	})

	if emitter := loop.GetEmitter(); emitter != nil {
		emitter.WaitForStream()
	}

	streamed := collectHTTPPacketStreamContent(invoker.events)
	if !strings.Contains(streamed, "X-Debug-Case: visible") {
		t.Fatalf("expected emitted http packet stream to contain patched header, got:\n%s", streamed)
	}
	var sawHTTPPacketStream bool
	for _, event := range invoker.events {
		if event.NodeId == "http_flow" && event.IsStream && event.ContentType == aicommon.TypeCodeHTTPRequest {
			sawHTTPPacketStream = true
			break
		}
	}
	if !sawHTTPPacketStream {
		t.Fatal("expected patch_http_request to emit code/http-request stream events")
	}
	if !strings.Contains(utils.InterfaceToString(loop.Get("current_request")), "X-Debug-Case: visible") {
		t.Fatalf("expected current_request to keep patched packet, got:\n%s", loop.Get("current_request"))
	}
}

func TestPatchHTTPRequestAction_EmitsVersionedRequestChangeEvent(t *testing.T) {
	invoker := newPatchActionTestInvoker(t)
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}

	rawRequest := "GET /orders?id=1 HTTP/1.1\r\nHost: example.com\r\n\r\n"
	fuzzReq, err := newLoopFuzzRequest(context.Background(), invoker, []byte(rawRequest), false)
	if err != nil {
		t.Fatalf("new fuzz request: %v", err)
	}
	storeLoopFuzzRequestState(loop, fuzzReq, []byte(rawRequest), false)
	loop.Set(loopHTTPFuzzRequestStateKey, loopHTTPFuzzRequestState{
		RawRequest:   rawRequest,
		IsHTTPS:      false,
		Summary:      getCurrentRequestSummary(loop),
		Version:      1,
		SourceAction: "set_http_request",
	})
	loop.Set(loopHTTPFuzzRequestVersionKey, 1)

	executeHTTPPatchAction(t, loop, map[string]any{
		"location":    "header",
		"operation":   "add",
		"field_name":  "X-Versioned",
		"field_value": "2",
		"reason":      "验证补丁事件只广播最新版请求。",
	})

	if emitter := loop.GetEmitter(); emitter != nil {
		emitter.WaitForStream()
	}

	payload := collectLatestRequestChangeEvent(t, invoker.events)
	if payload.Op != loopHTTPFuzzRequestEventOpPatch {
		t.Fatalf("expected patch event op %q, got %q", loopHTTPFuzzRequestEventOpPatch, payload.Op)
	}
	if payload.Request.Version != 2 {
		t.Fatalf("expected request version 2, got %d", payload.Request.Version)
	}
	if payload.SourceAction != "patch_http_request" {
		t.Fatalf("expected source action patch_http_request, got %q", payload.SourceAction)
	}
	if !strings.Contains(payload.Request.Raw, "X-Versioned: 2") {
		t.Fatalf("expected event request raw to contain patched header, got:\n%s", payload.Request.Raw)
	}
}
