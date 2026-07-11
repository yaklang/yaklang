package imcontrol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestPatchFeishuCardSendCard_CardActionUsesNormalSend(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, &gotBody)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/im/v1/messages":
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_config"}}`))
		case strings.Contains(r.URL.Path, "/reply"):
			t.Fatalf("card action should not use reply API, got path %s", r.URL.Path)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	e := New(Config{ReplyQuote: true})
	cfg := &notify.SendConfig{
		AppID:     "ai",
		AppSecret: "as",
		BaseURL:   srv.URL,
		Timeout:   5 * time.Second,
	}
	out := cardMessage(&notify.Card{Title: "配置", Content: "body"})
	inbound := &notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_chat",
		ReplyContext: "om_clicked_card",
		IsCardAction: true,
	}
	if _, err := e.patchFeishuCard_sendCard("feishu", "oc_chat", out, cfg, inbound); err != nil {
		t.Fatalf("send card from card action: %v", err)
	}
	if gotPath != "/open-apis/im/v1/messages" {
		t.Fatalf("path = %q, want normal send path", gotPath)
	}
	if gotBody["msg_type"] != "interactive" {
		t.Fatalf("msg_type = %v, want interactive", gotBody["msg_type"])
	}
}

func TestBuildConfigCard_StructuredElements(t *testing.T) {
	e := New(Config{ReplyQuote: true, ReplyGranularity: "standard", GroupTrigger: "must_at"})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_chat",
		SenderID: "ou_user",
	}
	card := e.buildConfigCard(msg)
	if card == nil {
		t.Fatal("card is nil")
	}
	if card.Title != "IM 配置" {
		t.Fatalf("title = %q", card.Title)
	}
	if len(card.Buttons) != 0 {
		t.Fatalf("config panel should use inline element buttons, got %d card buttons", len(card.Buttons))
	}
	if len(card.Elements) < 8 {
		t.Fatalf("expected structured elements, got %d", len(card.Elements))
	}
	elementsJSON, _ := json.Marshal(card.Elements)
	raw := string(elementsJSON)
	for _, want := range []string{"会话行为", "引用回复", "回复模式", "群聊触发", "执行审批", "权限", "checker", "select_static", "initial_option", "callback"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("elements missing %q: %s", want, raw)
		}
	}
	if !strings.Contains(raw, `"action":"config"`) {
		t.Fatalf("config button action missing: %s", raw)
	}
	if !strings.Contains(raw, `"sub":"toggle_reply_quote"`) {
		t.Fatalf("quote toggle action missing: %s", raw)
	}
	if !strings.Contains(raw, `"value":"standard"`) || !strings.Contains(raw, `"value":"summary"`) || !strings.Contains(raw, `"value":"detailed"`) {
		t.Fatalf("reply mode actions missing: %s", raw)
	}
	if strings.Count(raw, `"tag":"select_static"`) != 2 {
		t.Fatalf("reply mode and review policy should use two select_static components: %s", raw)
	}
	if strings.Count(raw, `"tag":"checker"`) != 2 {
		t.Fatalf("reply quote and group mention should use checker components: %s", raw)
	}
	if !strings.Contains(raw, `"sub":"toggle_group_mention"`) || !strings.Contains(raw, "需要 @ 提及") {
		t.Fatalf("group trigger actions missing: %s", raw)
	}
	if !strings.Contains(raw, `"sub":"set_review_policy"`) || !strings.Contains(raw, `"value":"manual"`) || !strings.Contains(raw, `"value":"ai"`) || !strings.Contains(raw, `"value":"yolo"`) {
		t.Fatalf("review policy actions missing: %s", raw)
	}
	if !strings.Contains(raw, `"checked":true`) {
		t.Fatalf("current options should be checked: %s", raw)
	}
}

func TestBuildHelpCard_ControlEntry(t *testing.T) {
	e := New(Config{ReplyQuote: true, ReplyGranularity: "summary", GroupTrigger: "allow_slash"})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_chat",
		SenderID: "ou_user",
		ChatType: "private",
	}
	card := e.buildHelpCard(msg)
	if card == nil {
		t.Fatal("card is nil")
	}
	if card.Title != "Yak Agent 控制台" {
		t.Fatalf("title = %q", card.Title)
	}
	if card.Content != "" {
		t.Fatalf("help card should use structured elements, got content=%q", card.Content)
	}
	elementsJSON, _ := json.Marshal(card.Elements)
	raw := string(elementsJSON)
	for _, want := range []string{"控制台", "主要入口", "会话行为", "button", `"action":"session_info"`, `"action":"config"`, `"action":"review"`, `"action":"status"`, `"action":"commands"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("help card missing %q: %s", want, raw)
		}
	}
}

func TestBuildOnboardingWelcomeCard(t *testing.T) {
	e := New(Config{ReplyQuote: true, ReplyGranularity: "standard", GroupTrigger: "must_at", ReviewPolicy: "manual"})
	e.callbackAuth = NewCallbackAuth(nil)
	card := e.BuildOnboardingWelcomeCard(notify.PlatformFeishu, "ou_owner")
	if card == nil {
		t.Fatal("card is nil")
	}
	if card.Title != "Yak Agent 已连接" {
		t.Fatalf("title = %q", card.Title)
	}
	elementsJSON, _ := json.Marshal(card.Elements)
	raw := string(elementsJSON)
	for _, want := range []string{
		"飞书 已连接",
		"回复：标准 · 审批：人工",
		"群聊默认允许成员使用",
		"直接发送消息即可开始",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("welcome card missing %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, `"token"`) || strings.Contains(raw, `"behaviors"`) {
		t.Fatalf("direct welcome card should not include callback buttons: %s", raw)
	}
}

func TestBuildOnboardingWelcomeCardForMessageSignsRealChat(t *testing.T) {
	auth := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	e := New(Config{ReplyQuote: true})
	e.callbackAuth = auth
	card := e.BuildOnboardingWelcomeCardForMessage(notify.PlatformFeishu, "ou_owner", &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_real_chat",
		SenderID: "ou_owner",
		ChatType: "private",
	})
	if card == nil {
		t.Fatal("card is nil")
	}
	value := onboardingWelcomeButtonValue(t, card, 0)
	if value["action"] != string(ActionSessionInfo) {
		t.Fatalf("button action = %v, want %s", value["action"], ActionSessionInfo)
	}
	token, _ := value["token"].(string)
	if token == "" {
		t.Fatal("button token is empty")
	}
	result := auth.Verify(token, CallbackVerifyExpected{
		ChatID: "oc_real_chat",
		Action: string(ActionSessionInfo),
	})
	if !result.OK {
		t.Fatalf("verify with real chat failed: %s", result.Reason)
	}
	mismatch := auth.Verify(token, CallbackVerifyExpected{
		ChatID: "ou_owner",
		Action: string(ActionSessionInfo),
	})
	if mismatch.OK || mismatch.Reason != "context-mismatch" {
		t.Fatalf("verify with owner id = ok:%v reason:%s, want context-mismatch", mismatch.OK, mismatch.Reason)
	}
}

func onboardingWelcomeButtonValue(t *testing.T, card *notify.Card, index int) map[string]any {
	t.Helper()
	if len(card.Elements) < 4 {
		t.Fatalf("welcome card elements = %d, want action row", len(card.Elements))
	}
	row := card.Elements[3]
	columns, ok := row["columns"].([]map[string]any)
	if !ok || len(columns) <= index {
		t.Fatalf("welcome card columns = %#v", row["columns"])
	}
	elements, ok := columns[index]["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("welcome card column elements = %#v", columns[index]["elements"])
	}
	behaviors, ok := elements[0]["behaviors"].([]map[string]any)
	if !ok || len(behaviors) == 0 {
		t.Fatalf("welcome card button behaviors = %#v", elements[0]["behaviors"])
	}
	value, ok := behaviors[0]["value"].(map[string]any)
	if !ok {
		t.Fatalf("welcome card button value = %#v", behaviors[0]["value"])
	}
	return value
}

func TestBuildStatusCard_StructuredElements(t *testing.T) {
	e := New(Config{ReplyQuote: true, ReplyGranularity: "detailed", GroupTrigger: "must_at"})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_chat",
		SenderID: "ou_user",
		ChatType: "group",
	}
	e.touchSession(imSessionKey(msg), msg)
	card := e.buildStatusCard(msg)
	if card == nil {
		t.Fatal("card is nil")
	}
	if card.Title != "IM 状态" {
		t.Fatalf("title = %q", card.Title)
	}
	elementsJSON, _ := json.Marshal(card.Elements)
	raw := string(elementsJSON)
	for _, want := range []string{"运行概览", "当前会话", "回复配置", "Yakit 历史", "刷新 Yakit 历史元数据", `"action":"session_info"`, `"action":"config"`, `"action":"commands"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("status card missing %q: %s", want, raw)
		}
	}
}

func TestBuildRecoveryCard_HasControlEntries(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_chat",
		SenderID: "ou_user",
	}
	card := e.buildRecoveryCard(msg, "附件下载失败", "请检查机器人文件权限或稍后重试。")
	if card == nil {
		t.Fatal("card is nil")
	}
	if card.Title != "附件下载失败" {
		t.Fatalf("title = %q", card.Title)
	}
	elementsJSON, _ := json.Marshal(card.Elements)
	raw := string(elementsJSON)
	for _, want := range []string{"请检查机器人文件权限", `"action":"commands"`, `"action":"help"`, `"action":"config"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("recovery card missing %q: %s", want, raw)
		}
	}
}

func TestBuildSessionInfoCard_OwnerPrivateShowsRecentHistory(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppID:     "cli_owner_history",
		AppSecret: "sec_owner_history",
		Enabled:   true,
		OwnerID:   "ou_owner",
	})
	e := New(Config{ReplyQuote: true, ReplyGranularity: "standard", GroupTrigger: "must_at"})
	e.queryAISessionFunc = func(ctx context.Context, req *ypb.QueryAISessionRequest) (*ypb.QueryAISessionResponse, error) {
		if req.GetPagination().GetLimit() != imSessionHistoryLimit {
			t.Fatalf("history limit = %d, want %d", req.GetPagination().GetLimit(), imSessionHistoryLimit)
		}
		return &ypb.QueryAISessionResponse{
			Data: []*ypb.AISession{
				{
					SessionID: "pc-session-1",
					Title:     "PC 创建的渗透测试会话",
					Source:    "ide",
					UpdatedAt: time.Date(2026, 7, 6, 16, 30, 0, 0, time.Local).Unix(),
				},
				{
					SessionID: "im-session-1",
					Title:     "IM 当前会话",
					Source:    "im",
					UpdatedAt: time.Date(2026, 7, 6, 16, 20, 0, 0, time.Local).Unix(),
					IMSourceMeta: &ypb.IMSourceMeta{
						Platform: "feishu",
						ChatType: "private",
					},
				},
			},
		}, nil
	}
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "ou_owner",
		SenderID: "ou_owner",
		ChatType: "private",
		ActionValue: map[string]any{
			"session_id": "im-session-1",
			"chat_title": "Feishu DM - owner",
		},
	}
	card := e.buildSessionInfoCard(msg)
	elementsJSON, _ := json.Marshal(card.Elements)
	raw := string(elementsJSON)
	for _, want := range []string{
		"最近历史",
		"PC 创建的渗透测试会话",
		"07-06 16:30",
		`"action":"use_session"`,
		`"session_id":"pc-session-1"`,
		"IM 当前会话（当前）",
		"07-06 16:20 / 私聊",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("owner session panel missing %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, "SessionID") {
		t.Fatalf("session info card should not expose visible SessionID label: %s", raw)
	}
}

func TestBindAISessionToIMRequiresOwnerPrivate(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformFeishu.String(),
		AppID:     "cli_owner_bind",
		AppSecret: "sec_owner_bind",
		Enabled:   true,
		OwnerID:   "ou_owner",
	})
	e := New(Config{})
	ownerMsg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "ou_owner",
		SenderID: "ou_owner",
		ChatType: "private",
	}
	if err := e.bindAISessionToIM(ownerMsg, "pc-session-1", "PC 历史"); err != nil {
		t.Fatalf("owner bind history session: %v", err)
	}
	e.mu.Lock()
	sess := e.sessions[imSessionKey(ownerMsg)]
	e.mu.Unlock()
	if sess == nil {
		t.Fatal("session not created")
	}
	if sess.persistentSessionId != "pc-session-1" {
		t.Fatalf("persistentSessionId = %q, want pc-session-1", sess.persistentSessionId)
	}
	if sess.yakitSessionTitle != "PC 历史" {
		t.Fatalf("yakitSessionTitle = %q", sess.yakitSessionTitle)
	}

	sharedMsg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "ou_other",
		SenderID: "ou_other",
		ChatType: "private",
	}
	err := e.bindAISessionToIM(sharedMsg, "pc-session-2", "Other")
	if err == nil || !strings.Contains(err.Error(), "bot 所有者") {
		t.Fatalf("shared user bind err = %v, want owner-only error", err)
	}
}

func TestCommandSessionIndexBindsRecentHistoryForTextPlatform(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformDingTalk.String(),
		AppID:     "cli_dingtalk_session_index",
		AppSecret: "sec_dingtalk_session_index",
		Enabled:   true,
		OwnerID:   "ou_owner",
	})
	e := New(Config{})
	e.queryAISessionFunc = func(ctx context.Context, req *ypb.QueryAISessionRequest) (*ypb.QueryAISessionResponse, error) {
		return &ypb.QueryAISessionResponse{
			Data: []*ypb.AISession{
				{SessionID: "current-im-session", Title: "当前 IM 会话", Source: "im"},
				{SessionID: "pc-history-session", Title: "PC 历史会话", Source: "ide"},
			},
		}, nil
	}
	msg := &notify.InboundMessage{
		Platform: notify.PlatformDingTalk,
		ChatID:   "cid_owner",
		SenderID: "ou_owner",
		ChatType: "private",
	}
	e.touchSession(imSessionKey(msg), msg)

	e.handleAction(IMAction{
		Type:   ActionSessionInfo,
		Args:   []string{"2"},
		Source: "command",
		Msg:    msg,
	})

	e.mu.Lock()
	sess := e.sessions[imSessionKey(msg)]
	e.mu.Unlock()
	if sess == nil {
		t.Fatal("session should exist")
	}
	if sess.persistentSessionId != "pc-history-session" {
		t.Fatalf("persistentSessionId = %q, want pc-history-session", sess.persistentSessionId)
	}
	if sess.yakitSessionTitle != "PC 历史会话" {
		t.Fatalf("yakitSessionTitle = %q, want PC 历史会话", sess.yakitSessionTitle)
	}
}

func TestBuildSessionInfoTextUsesMarkdownAndHidesLongSessionIDs(t *testing.T) {
	setupIMAuthTestDB(t, &credential.BotConfig{
		Platform:  notify.PlatformDingTalk.String(),
		AppID:     "cli_dingtalk_session_text",
		AppSecret: "sec_dingtalk_session_text",
		Enabled:   true,
		OwnerID:   "ou_owner",
	})
	e := New(Config{})
	e.queryAISessionFunc = func(ctx context.Context, req *ypb.QueryAISessionRequest) (*ypb.QueryAISessionResponse, error) {
		return &ypb.QueryAISessionResponse{
			Data: []*ypb.AISession{
				{SessionID: "current-im-session", Title: "当前 IM 会话", Source: "im"},
				{SessionID: "pc-history-session", Title: "PC 历史会话", Source: "ide"},
			},
		}, nil
	}
	msg := &notify.InboundMessage{
		Platform: notify.PlatformDingTalk,
		ChatID:   "cid_owner",
		SenderID: "ou_owner",
		ChatType: "private",
		ActionValue: map[string]any{
			"session_id": "current-im-session",
			"chat_title": "钉钉私聊",
		},
	}

	text := e.buildSessionInfoText(msg)
	for _, want := range []string{"## 会话面板", "**当前会话**", "**最近历史**", "PC 历史会话", "`/session 2`"} {
		if !strings.Contains(text, want) {
			t.Fatalf("session text missing %q: %s", want, text)
		}
	}
	for _, notWant := range []string{"current-im-session", "pc-history-session"} {
		if strings.Contains(text, notWant) {
			t.Fatalf("session text should hide raw session id %q: %s", notWant, text)
		}
	}
}
