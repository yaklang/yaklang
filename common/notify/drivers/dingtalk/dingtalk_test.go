package dingtalk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
)

// mockGateway 构造一个本地 httptest server 模拟钉钉 token + 发送 API，
// 并把收到的请求体回传给测试断言。
func mockGateway(t *testing.T, gotBody *map[string]any, gotPath *string, msgID string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, gotBody)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/accessToken"):
			_, _ = w.Write([]byte(`{"accessToken":"mock-token","expireIn":7200}`))
		case strings.HasSuffix(r.URL.Path, "/oToMessages/batchSend"):
			_, _ = w.Write([]byte(`{"processQueryKey":"q","messageId":"` + msgID + `"}`))
		case strings.HasSuffix(r.URL.Path, "/groupMessages/send"):
			_, _ = w.Write([]byte(`{"processQueryKey":"q","messageId":"` + msgID + `"}`))
		case strings.HasSuffix(r.URL.Path, "/emotion/reply"):
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTokenManager_GetAndCache(t *testing.T) {
	var got map[string]any
	var gotPath string
	srv := mockGateway(t, &got, &gotPath, "id1")

	tm := newTokenManager(&notify.SendConfig{AppID: "k", AppSecret: "s", BaseURL: srv.URL, Timeout: 5 * time.Second})
	tok1, err := tm.getToken()
	if err != nil {
		t.Fatalf("getToken: %v", err)
	}
	if tok1 != "mock-token" {
		t.Fatalf("got token %q", tok1)
	}
	if gotPath != "/v1.0/oauth2/accessToken" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	// 第二次应命中缓存（不发请求）
	gotPath = ""
	tok2, err := tm.getToken()
	if err != nil || tok2 != "mock-token" {
		t.Fatalf("cached token: %v %q", err, tok2)
	}
	if gotPath != "" {
		t.Fatalf("expected cache hit, but hit %q", gotPath)
	}
}

func TestTokenManager_MissingCredential(t *testing.T) {
	tm := newTokenManager(&notify.SendConfig{})
	if _, err := tm.getToken(); err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestClient_SendSingleText(t *testing.T) {
	var got map[string]any
	var gotPath string
	srv := mockGateway(t, &got, &gotPath, "msg-123")

	c := New(notify.WithAppID("k"), notify.WithAppSecret("s"), notify.WithBaseURL(srv.URL), notify.WithTimeout(5*time.Second))
	res, err := c.Send(&platformMessage{TargetID: "u1", MsgType: notify.MsgText, Content: "hello"}, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.MessageID != "msg-123" {
		t.Fatalf("message id = %q", res.MessageID)
	}
	if gotPath != "/v1.0/robot/oToMessages/batchSend" {
		t.Fatalf("path = %q", gotPath)
	}
	if got["msgKey"] != "sampleText" {
		t.Fatalf("msgKey = %v", got["msgKey"])
	}
	// msgParam 是 JSON 字符串
	param, _ := got["msgParam"].(string)
	if !strings.Contains(param, "hello") {
		t.Fatalf("msgParam = %q", param)
	}
	ids, _ := got["userIds"].([]any)
	if len(ids) != 1 || ids[0] != "u1" {
		t.Fatalf("userIds = %v", ids)
	}
}

func TestClient_SendGroupMarkdown(t *testing.T) {
	var got map[string]any
	var gotPath string
	srv := mockGateway(t, &got, &gotPath, "grp-1")

	c := New(notify.WithAppID("k"), notify.WithAppSecret("s"), notify.WithBaseURL(srv.URL), notify.WithTimeout(5*time.Second))
	_, err := c.Send(&platformMessage{
		TargetID: "cid",
		IsGroup:  true,
		MsgType:  notify.MsgMarkdown,
		Content:  "# title\nbody",
		Card:     &notify.Card{Title: "T"},
	}, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotPath != "/v1.0/robot/groupMessages/send" {
		t.Fatalf("path = %q", gotPath)
	}
	if got["msgKey"] != "sampleMarkdown" {
		t.Fatalf("msgKey = %v", got["msgKey"])
	}
	if got["openConversationId"] != "cid" {
		t.Fatalf("conv = %v", got["openConversationId"])
	}
}

func TestRobotSign(t *testing.T) {
	ts, sign, err := robotSign("SECtest")
	if err != nil {
		t.Fatalf("robotSign: %v", err)
	}
	if ts <= 0 || sign == "" {
		t.Fatalf("invalid sign: ts=%d sign=%q", ts, sign)
	}
	// 同 secret 同 timestamp 应可复现
	_ = sign
}

func TestHandleStreamMessageUsesPayloadCreateAtBeforeFrameTime(t *testing.T) {
	payload := map[string]any{
		"conversationId":            "cid_1",
		"chatbotUserId":             "robot_1",
		"msgId":                     "msg_1",
		"senderNick":                "g",
		"senderId":                  "user_1",
		"conversationType":          "1",
		"msgtype":                   "text",
		"createAt":                  int64(1783591964916),
		"sessionWebhookExpiredTime": int64(1783597365192),
		"sessionWebhook":            "https://oapi.dingtalk.com/robot/sendBySession?session=x",
		"content":                   map[string]any{"text": "hi"},
		"text":                      map[string]any{"content": "hi"},
	}
	payloadBytes, _ := json.Marshal(payload)
	df := dataFrame{
		Data:      string(payloadBytes),
		MessageID: "frame_1",
		Type:      "CALLBACK",
		Topic:     topicBotMessage,
		Time:      1783613557000,
	}
	df.Headers.Topic = topicBotMessage
	df.Headers.MessageID = "frame_1"
	df.Headers.Time = "1783613557000"
	raw, _ := json.Marshal(df)

	c := New()
	var got *notify.InboundMessage
	c.handleStreamMessage(raw, nil, func(m *notify.InboundMessage) { got = m })

	if got == nil {
		t.Fatal("expected inbound message")
	}
	want := time.UnixMilli(1783591964916)
	if !got.EventTime.Equal(want) {
		t.Fatalf("EventTime = %s, want payload createAt %s", got.EventTime, want)
	}
}

func TestBuildMsgKeyParam_Unsupported(t *testing.T) {
	_, _, err := buildMsgKeyParam(&platformMessage{MsgType: notify.MsgImage})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}
