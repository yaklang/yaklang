package dingtalk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/notify"
)

// mockRegistrationServer 构造本地 httptest server 模拟钉钉三个注册端点。
// pollCount 控制 poll 返回 WAITING 几次后转 SUCCESS。
func mockRegistrationServer(t *testing.T, pollCount int32) *httptest.Server {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		switch r.URL.Path {
		case registrationInitPath:
			_, _ = w.Write([]byte(`{"nonce":"test-nonce"}`))
		case registrationBeginPath:
			_, _ = w.Write([]byte(`{"device_code":"dc-1","user_code":"uc-1","verification_uri":"https://login.dingtalk.com","verification_uri_complete":"https://login.dingtalk.com/oauth2/auth?dc=dc-1","expires_in":600,"interval":1}`))
		case registrationPollPath:
			n := atomic.AddInt32(&calls, 1)
			if n <= pollCount {
				_, _ = w.Write([]byte(`{"status":"WAITING"}`))
			} else {
				_, _ = w.Write([]byte(`{"status":"SUCCESS","client_id":"cid-1","client_secret":"csec-1"}`))
			}
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunOnboarding_Success(t *testing.T) {
	srv := mockRegistrationServer(t, 1)
	registrationBase = srv.URL

	var steps []*notify.OnboardingStep
	handler := func(step *notify.OnboardingStep) error {
		steps = append(steps, step)
		return nil
	}
	if err := RunOnboarding(30, nil, handler); err != nil {
		t.Fatalf("RunOnboarding: %v", err)
	}
	// 期望状态序列：qr -> pending(WAITING) -> success
	if len(steps) < 3 {
		t.Fatalf("expected at least 3 steps, got %d: %+v", len(steps), steps)
	}
	if steps[0].State != "qr" {
		t.Fatalf("first state = %q, want qr", steps[0].State)
	}
	if steps[0].QrURL == "" {
		t.Fatalf("qr step missing QrURL")
	}
	if len(steps[0].QrPNG) == 0 {
		t.Fatalf("qr step missing QrPNG")
	}
	// 最后一步应是 success
	last := steps[len(steps)-1]
	if last.State != "success" {
		t.Fatalf("last state = %q, want success", last.State)
	}
	if last.Result == nil {
		t.Fatalf("success step missing Result")
	}
	if last.Result.AppID != "cid-1" || last.Result.AppSecret != "csec-1" {
		t.Fatalf("result = %+v, want AppID=cid-1 AppSecret=csec-1", last.Result)
	}
	if last.Result.Platform != "dingtalk" {
		t.Fatalf("platform = %q, want dingtalk", last.Result.Platform)
	}
}

func TestRunOnboarding_Fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case registrationInitPath:
			_, _ = w.Write([]byte(`{"nonce":"n"}`))
		case registrationBeginPath:
			_, _ = w.Write([]byte(`{"device_code":"dc","verification_uri_complete":"https://x","interval":1,"expires_in":600}`))
		case registrationPollPath:
			_, _ = w.Write([]byte(`{"status":"FAIL","fail_reason":"user denied"}`))
		}
	}))
	t.Cleanup(srv.Close)
	registrationBase = srv.URL

	var last notify.OnboardingStep
	handler := func(step *notify.OnboardingStep) error {
		last = *step
		return nil
	}
	_ = RunOnboarding(30, nil, handler)
	if last.State != "error" {
		t.Fatalf("state = %q, want error", last.State)
	}
	if !strings.Contains(last.Message, "user denied") {
		t.Fatalf("message = %q, want contain 'user denied'", last.Message)
	}
}

func TestRunOnboarding_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case registrationInitPath:
			_, _ = w.Write([]byte(`{"nonce":"n"}`))
		case registrationBeginPath:
			_, _ = w.Write([]byte(`{"device_code":"dc","verification_uri_complete":"https://x","interval":1,"expires_in":600}`))
		case registrationPollPath:
			_, _ = w.Write([]byte(`{"status":"EXPIRED"}`))
		}
	}))
	t.Cleanup(srv.Close)
	registrationBase = srv.URL

	var last notify.OnboardingStep
	handler := func(step *notify.OnboardingStep) error {
		last = *step
		return nil
	}
	_ = RunOnboarding(30, nil, handler)
	if last.State != "expired" {
		t.Fatalf("state = %q, want expired", last.State)
	}
}

func TestRunOnboarding_InitErrcode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":50001,"errmsg":"source invalid"}`))
	}))
	t.Cleanup(srv.Close)
	registrationBase = srv.URL

	var last notify.OnboardingStep
	handler := func(step *notify.OnboardingStep) error {
		last = *step
		return nil
	}
	_ = RunOnboarding(30, nil, handler)
	if last.State != "error" {
		t.Fatalf("state = %q, want error", last.State)
	}
}
