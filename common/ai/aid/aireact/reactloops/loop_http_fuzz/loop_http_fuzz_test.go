package loop_http_fuzz_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	loop_http_fuzz "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_http_fuzz"
)

func TestLoadHTTPRequestBuildsSurfaceAndPlan(t *testing.T) {
	loop := newTestLoop(t)
	rawReq := buildRawRequest("POST", "http://example.com/api/login", map[string]string{
		"Content-Type": "application/json",
	}, `{"username":"admin","password":"admin"}`)

	executeLoopAction(t, loop, "load_http_request", map[string]any{
		"http_request": rawReq,
		"is_https":     false,
		"reason":       "测试登录接口弱口令",
	})

	profileJSON := marshalAny(t, loop.GetVariable("request_profile"))
	require.Contains(t, profileJSON, `"business_guess":"login"`)

	inventoryJSON := marshalAny(t, loop.GetVariable("parameter_inventory"))
	require.Contains(t, inventoryJSON, "json:$.username")
	require.Contains(t, inventoryJSON, "json:$.password")

	planJSON := marshalAny(t, loop.GetVariable("test_plan"))
	require.Contains(t, planJSON, "weak_password")
}

func TestMutateAndExecuteBatchCollectsAnomaly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "'") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("SQL syntax error near " + q))
			return
		}
		_, _ = w.Write([]byte("search=" + q))
	}))
	defer srv.Close()

	loop := newTestLoop(t)
	rawReq := buildRawRequest("GET", srv.URL+"/search?q=test", nil, "")
	executeLoopAction(t, loop, "load_http_request", map[string]any{
		"http_request": rawReq,
		"is_https":     false,
		"reason":       "测试搜索参数是否存在 SQL 注入",
	})
	executeLoopAction(t, loop, "mutate_target", map[string]any{
		"target_ref":    "query:q",
		"mutation_mode": "replace",
		"payloads":      []string{"'"},
		"reason":        "准备基础引号探测",
	})
	executeLoopAction(t, loop, "execute_test_batch", map[string]any{
		"scenario":       "sqli",
		"variant_source": "last_mutation",
		"max_requests":   4,
		"reason":         "执行单批探测",
	})

	candidatesJSON := marshalAny(t, loop.GetVariable("anomaly_candidates"))
	require.Contains(t, candidatesJSON, "error_signature_detected")
}

func TestRunWeakPasswordAndCommitFinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Username == "admin" && req.Password == "admin123" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok"})
			w.Header().Set("Location", "/dashboard")
			w.WriteHeader(http.StatusFound)
			_, _ = w.Write([]byte("welcome admin"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid credentials"))
	}))
	defer srv.Close()

	loop := newTestLoop(t)
	rawReq := buildRawRequest("POST", srv.URL+"/api/login", map[string]string{
		"Content-Type": "application/json",
	}, `{"username":"guest","password":"guest"}`)
	executeLoopAction(t, loop, "load_http_request", map[string]any{
		"http_request": rawReq,
		"is_https":     false,
		"reason":       "测试登录接口弱口令",
	})
	executeLoopAction(t, loop, "run_weak_password_test", map[string]any{
		"username_targets": []string{"json:$.username"},
		"password_targets": []string{"json:$.password"},
		"max_pairs":        12,
		"reason":           "枚举基础弱口令",
	})

	candidatesJSON := marshalAny(t, loop.GetVariable("anomaly_candidates"))
	require.Contains(t, candidatesJSON, "auth_state_changed")

	executeLoopAction(t, loop, "commit_finding", map[string]any{
		"category": "weak_password",
		"severity": "high",
		"reason":   "成功凭据触发登录态变化",
	})

	findingsJSON := marshalAny(t, loop.GetVariable("confirmed_findings"))
	require.Contains(t, findingsJSON, `"category":"weak_password"`)
}

func newTestLoop(t *testing.T) *reactloops.ReActLoop {
	t.Helper()
	invoker, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName(loop_http_fuzz.LoopHTTPFuzzName, invoker)
	require.NoError(t, err)
	return loop
}

func marshalAny(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

func executeLoopAction(t *testing.T, loop *reactloops.ReActLoop, actionName string, params map[string]any) {
	t.Helper()
	payload := make(map[string]any, len(params)+1)
	payload["@action"] = actionName
	payload["identifier"] = "test_" + actionName
	for k, v := range params {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	action, err := aicommon.ExtractAction(string(raw), actionName)
	require.NoError(t, err)
	handler, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	if handler.ActionVerifier != nil {
		require.NoError(t, handler.ActionVerifier(loop, action))
	}
	op := reactloops.NewActionHandlerOperator(nil)
	handler.ActionHandler(loop, action, op)
	terminated, actionErr := op.IsTerminated()
	if terminated && actionErr != nil {
		require.NoError(t, actionErr)
	}
}

func buildRawRequest(method, rawURL string, headers map[string]string, body string) string {
	u, _ := url.Parse(rawURL)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", method, path))
	b.WriteString(fmt.Sprintf("Host: %s\r\n", u.Host))
	for k, v := range headers {
		b.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	if body != "" {
		b.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	}
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

func TestLoopFactoryRegistered(t *testing.T) {
	_, ok := reactloops.GetLoopFactory(loop_http_fuzz.LoopHTTPFuzzName)
	require.True(t, ok)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = ctx
}
