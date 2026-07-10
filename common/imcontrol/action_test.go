package imcontrol

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
)

func setupIMActionTestDB(t *testing.T, baseURL string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create profile db: %v", err)
	}
	consts.BindProfileDatabase(db, dbPath)
	_, err = credential.SaveBotConfig(&credential.BotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppID:     "cli_test",
		AppSecret: "secret_test",
		BaseURL:   baseURL,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("save bot config: %v", err)
	}
	t.Cleanup(func() {
		_ = credential.DeleteBotConfig(notify.PlatformFeishu.String())
	})
}

func newActionReplyServer(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/im/v1/messages":
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &body)
			}
			bodies = append(bodies, body)
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_reply"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"msg":"not found"}`))
		}
	}))
	return srv, &bodies
}

func lastFeishuContent(t *testing.T, bodies []map[string]any) string {
	t.Helper()
	if len(bodies) == 0 {
		t.Fatal("expected a visible action reply, got no send request")
	}
	content, _ := bodies[len(bodies)-1]["content"].(string)
	if content == "" {
		t.Fatalf("last body has no content: %+v", bodies[len(bodies)-1])
	}
	return content
}

func TestHandleCardAction_MissingActionSendsVisibleRecovery(t *testing.T) {
	srv, bodies := newActionReplyServer(t)
	t.Cleanup(srv.Close)
	setupIMActionTestDB(t, srv.URL)

	e := New(Config{ReplyQuote: true})
	e.callbackAuth = nil
	e.handleCardAction(&notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_chat",
		SenderID:     "ou_user",
		IsCardAction: true,
		ActionValue:  map[string]any{"sub": "bad"},
	})

	content := lastFeishuContent(t, *bodies)
	if !strings.Contains(content, "缺少 action") || !strings.Contains(content, "/help") {
		t.Fatalf("missing action recovery content = %s", content)
	}
}

func TestHandleCardAction_UnknownActionSendsVisibleRecovery(t *testing.T) {
	srv, bodies := newActionReplyServer(t)
	t.Cleanup(srv.Close)
	setupIMActionTestDB(t, srv.URL)

	e := New(Config{ReplyQuote: true})
	e.callbackAuth = nil
	e.handleCardAction(&notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_chat",
		SenderID:     "ou_user",
		IsCardAction: true,
		ActionValue:  map[string]any{"action": "missing_action"},
	})

	content := lastFeishuContent(t, *bodies)
	if !strings.Contains(content, "未知操作") || !strings.Contains(content, "missing_action") {
		t.Fatalf("unknown action recovery content = %s", content)
	}
}

func TestKnownIMActionIncludesSessionInfoAlias(t *testing.T) {
	for _, action := range []string{"help", "commands", "new", "stop", "status", "config", "session_info", "use_session", "open_yakit"} {
		if !knownIMAction(IMActionType(action)) {
			t.Fatalf("knownIMAction(%q) = false", action)
		}
	}
	if knownIMAction("missing_action") {
		t.Fatal("unknown action should not be known")
	}
}

func TestApplyConfigCardAction_InvalidValuesReturnVisibleError(t *testing.T) {
	e := New(Config{ReplyQuote: true, ReplyGranularity: "standard", GroupTrigger: "must_at"})
	cases := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{
			name:  "invalid group trigger",
			value: map[string]any{"sub": "set_group_trigger", "trigger": "bad"},
			want:  "无效群聊触发策略",
		},
		{
			name:  "invalid granularity",
			value: map[string]any{"sub": "set_granularity", "mode": "bad"},
			want:  "无效回复模式",
		},
		{
			name:  "unknown sub action",
			value: map[string]any{"sub": "unknown"},
			want:  "未知配置子动作",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := &notify.InboundMessage{ActionValue: c.value}
			reply, _ := e.applyConfigCardAction(msg)
			if !strings.Contains(reply, c.want) {
				t.Fatalf("reply = %q, want contains %q", reply, c.want)
			}
		})
	}
}
