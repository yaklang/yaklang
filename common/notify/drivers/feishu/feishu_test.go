package feishu

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
)

func TestIsExpectedLCDisconnect(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "tls closed connection string", err: errors.New("tls: use of closed connection"), want: true},
		{name: "wrapped tls closed connection string", err: fmt.Errorf("close websocket: %w", errors.New("tls: use of closed connection")), want: true},
		{name: "net err closed", err: net.ErrClosed, want: true},
		{name: "real endpoint error", err: errors.New("ws endpoint status 401"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isExpectedLCDisconnect(tc.err); got != tc.want {
				t.Fatalf("isExpectedLCDisconnect(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func mockFeishuGateway(t *testing.T, gotBody *map[string]any, gotPath *string, gotAuth *string, msgID string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		*gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, gotBody)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
		case strings.Contains(r.URL.Path, "/im/v1/messages"):
			// 匹配 send（POST .../messages）、reply、patch（PATCH .../messages/{id}）
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"` + msgID + `"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockFeishuGatewayMethod 记录 HTTP method（用于 PATCH 卡片测试）。
func mockFeishuGatewayMethod(t *testing.T, gotMethod *string, gotBody *map[string]any, gotPath *string, msgID string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotMethod = r.Method
		*gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, gotBody)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
		case strings.Contains(r.URL.Path, "/im/v1/messages"):
			// 匹配 patch（PATCH .../messages/{id}）路径
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"` + msgID + `"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTokenManager_GetAndCache(t *testing.T) {
	var got map[string]any
	var gotPath, gotAuth string
	srv := mockFeishuGateway(t, &got, &gotPath, &gotAuth, "id")

	tm := newTokenManager(&notify.SendConfig{AppID: "ai", AppSecret: "as", BaseURL: srv.URL, Timeout: 5 * time.Second})
	tok, err := tm.getToken()
	if err != nil {
		t.Fatalf("getToken: %v", err)
	}
	if tok != "mock-tt" {
		t.Fatalf("token = %q", tok)
	}
	if gotPath != "/open-apis/auth/v3/tenant_access_token/internal" {
		t.Fatalf("path = %q", gotPath)
	}
	// 缓存
	gotPath = ""
	if tok2, err := tm.getToken(); err != nil || tok2 != "mock-tt" {
		t.Fatalf("cached: %v %q", err, tok2)
	}
	if gotPath != "" {
		t.Fatalf("expected cache hit, got %q", gotPath)
	}
}

func TestClient_SendText(t *testing.T) {
	var got map[string]any
	var gotPath, gotAuth string
	srv := mockFeishuGateway(t, &got, &gotPath, &gotAuth, "msg-7")

	c := New(notify.WithAppID("ai"), notify.WithAppSecret("as"), notify.WithBaseURL(srv.URL), notify.WithTimeout(5*time.Second))
	res, err := c.Send(&platformMessage{TargetID: "ou_x", MsgType: notify.MsgText, Content: "hi"}, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.MessageID != "msg-7" {
		t.Fatalf("id = %q", res.MessageID)
	}
	if !strings.HasPrefix(gotAuth, "Bearer mock-tt") {
		t.Fatalf("auth = %q", gotAuth)
	}
	if got["msg_type"] != "text" {
		t.Fatalf("msg_type = %v", got["msg_type"])
	}
	content, _ := got["content"].(string)
	if !strings.Contains(content, "hi") {
		t.Fatalf("content = %q", content)
	}
}

func TestClient_SendCard(t *testing.T) {
	var got map[string]any
	var gotPath, gotAuth string
	srv := mockFeishuGateway(t, &got, &gotPath, &gotAuth, "c1")
	c := New(notify.WithAppID("ai"), notify.WithAppSecret("as"), notify.WithBaseURL(srv.URL))
	_, err := c.Send(&platformMessage{
		TargetID: "ou_y",
		MsgType:  notify.MsgCard,
		Card:     &notify.Card{Title: "标题", Content: "# body"},
	}, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got["msg_type"] != "interactive" {
		t.Fatalf("msg_type = %v", got["msg_type"])
	}
	content, _ := got["content"].(string)
	if !strings.Contains(content, "标题") || !strings.Contains(content, "body") {
		t.Fatalf("card content = %q", content)
	}
	// Card JSON 2.0 必须含 schema + body
	if !strings.Contains(content, `"schema":"2.0"`) {
		t.Fatalf("card content missing schema 2.0: %q", content)
	}
	if !strings.Contains(content, `"body"`) {
		t.Fatalf("card content missing body: %q", content)
	}
}

func TestBuildContent_Unsupported(t *testing.T) {
	_, _, err := buildFeishuContent(&platformMessage{MsgType: notify.MsgImage})
	if err == nil {
		t.Fatal("expected error")
	}
}

// 帧编解码往返测试：确认飞书 LC Frame（protobuf 编码）自洽。
func TestFrameRoundTrip(t *testing.T) {
	hdr := Headers{}
	hdr.Add(HeaderType, string(MessageTypeEvent))
	hdr.Add(HeaderMessageID, "om_123")
	original := &Frame{
		Method:  int32(FrameTypeData),
		Headers: hdr,
		Payload: []byte(`{"event":{"message":{"message_id":"om_123"}}}`),
	}
	b, err := original.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := &Frame{}
	if err := got.Unmarshal(b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if FrameType(got.Method) != FrameTypeData {
		t.Fatalf("method = %d", got.Method)
	}
	hs := Headers(got.Headers)
	if hs.GetString(HeaderMessageID) != "om_123" {
		t.Fatalf("headers message_id = %q", hs.GetString(HeaderMessageID))
	}
	if !strings.Contains(string(got.Payload), "om_123") {
		t.Fatalf("payload = %s", got.Payload)
	}
}

func TestExtractFeishuContent(t *testing.T) {
	// text 消息
	got, atts := extractFeishuContent(`{"text":"hello"}`, "text", "om1")
	if got != "hello" {
		t.Fatalf("text: got %q", got)
	}
	if len(atts) != 0 {
		t.Fatalf("text: expected 0 attachments, got %d", len(atts))
	}

	// 空内容
	got, _ = extractFeishuContent("", "text", "om1")
	if got != "" {
		t.Fatalf("empty: got %q", got)
	}

	// 非 JSON 回退原文
	got, _ = extractFeishuContent("plain", "", "om1")
	if got != "plain" {
		t.Fatalf("non-json: got %q", got)
	}

	// image 消息
	got, atts = extractFeishuContent(`{"image_key":"img_v3_001"}`, "image", "om_img")
	if got != "" {
		t.Fatalf("image text should be empty, got %q", got)
	}
	if len(atts) != 1 || atts[0].Type != notify.MsgImage || atts[0].FileKey != "img_v3_001" || atts[0].MessageID != "om_img" {
		t.Fatalf("image attachment wrong: %+v", atts)
	}

	// file 消息
	got, atts = extractFeishuContent(`{"file_key":"file_v3_002","file_name":"report.pdf"}`, "file", "om_file")
	if got != "" {
		t.Fatalf("file text should be empty, got %q", got)
	}
	if len(atts) != 1 || atts[0].Type != notify.MsgFile || atts[0].FileKey != "file_v3_002" || atts[0].FileName != "report.pdf" {
		t.Fatalf("file attachment wrong: %+v", atts)
	}

	// post 富文本（含嵌套图片）
	postContent := `{"zh_cn":{"title":"标题","content":[[{"tag":"text","text":"看这张图"},{"tag":"img","image_key":"img_v3_post"}]]}}`
	got, atts = extractFeishuContent(postContent, "post", "om_post")
	if !strings.Contains(got, "看这张图") || !strings.Contains(got, "标题") {
		t.Fatalf("post text wrong: %q", got)
	}
	if len(atts) != 1 || atts[0].FileKey != "img_v3_post" {
		t.Fatalf("post attachment wrong: %+v", atts)
	}

	// 客户端图文混发可能仍以 text message_type 投递，但 content 不只有顶层 text 字段。
	mixedContent := `{"content":[[{"tag":"img","image_key":"img_v3_mixed"},{"tag":"text","text":"你能看见这张图片吗，描述一下"}]]}`
	got, atts = extractFeishuContent(mixedContent, "text", "om_mixed")
	if !strings.Contains(got, "你能看见这张图片吗") {
		t.Fatalf("mixed text wrong: %q", got)
	}
	if len(atts) != 1 || atts[0].Type != notify.MsgImage || atts[0].FileKey != "img_v3_mixed" || atts[0].MessageID != "om_mixed" {
		t.Fatalf("mixed attachment wrong: %+v", atts)
	}

	// 实际客户端图文消息可能以 post 投递，但 content 不是 zh_cn/en_us 包裹，而是顶层 title/content。
	realMixedPost := `{"title":"","content":[[{"tag":"img","image_key":"img_v3_0213e_d783cec5-12bd-4435-ada3-d4910145b06g","width":1015,"height":1015}],[{"tag":"text","text":"看见请描述图片内容","style":[]}]]}`
	got, atts = extractFeishuContent(realMixedPost, "post", "om_real_mixed")
	if !strings.Contains(got, "看见请描述图片内容") {
		t.Fatalf("real mixed post text wrong: %q", got)
	}
	if strings.Contains(got, `"image_key"`) {
		t.Fatalf("real mixed post should not expose raw json as text: %q", got)
	}
	if len(atts) != 1 || atts[0].Type != notify.MsgImage || atts[0].FileKey != "img_v3_0213e_d783cec5-12bd-4435-ada3-d4910145b06g" || atts[0].MessageID != "om_real_mixed" {
		t.Fatalf("real mixed post attachment wrong: %+v", atts)
	}
}

// dispatchEvent 直接接收已反序列化的 Frame，内部对 Payload 做 JSON 解析后回调 handler。
// 这里构造一个最简 Frame（只装 payload），验证 message 事件被正确解析成 InboundMessage，
// 且 chat_type / thread_id / root_id / parent_id 被正确归一化透传。
func TestDispatchEvent_ChatTypeAndThread(t *testing.T) {
	cases := []struct {
		name       string
		payload    string
		wantChat   string // 归一化后的 ChatType: private / group / topic
		wantThr    string
		wantRoot   string
		wantPar    string
		wantChatID string
	}{
		{
			name: "p2p -> private",
			payload: `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{` +
				`"message_id":"om_1","chat_id":"oc_d","chat_type":"p2p",` +
				`"message_type":"text","content":"{\"text\":\"hi\"}"}}}`,
			wantChat:   "private",
			wantChatID: "oc_d",
		},
		{
			name: "group no thread -> group",
			payload: `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{` +
				`"message_id":"om_2","chat_id":"oc_g","chat_type":"group",` +
				`"message_type":"text","content":"{\"text\":\"hi\"}"}}}`,
			wantChat:   "group",
			wantChatID: "oc_g",
		},
		{
			name: "group with thread -> topic",
			payload: `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{` +
				`"message_id":"om_3","chat_id":"oc_g","chat_type":"group",` +
				`"message_type":"text","content":"{\"text\":\"hi\"}",` +
				`"thread_id":"om_thread3"}}}`,
			wantChat:   "topic",
			wantThr:    "om_thread3",
			wantChatID: "oc_g",
		},
		{
			name: "reply carries root/parent",
			payload: `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{` +
				`"message_id":"om_4","chat_id":"oc_g","chat_type":"group",` +
				`"message_type":"text","content":"{\"text\":\"hi\"}",` +
				`"root_id":"om_root4","parent_id":"om_par4"}}}`,
			wantChat:   "group",
			wantRoot:   "om_root4",
			wantPar:    "om_par4",
			wantChatID: "oc_g",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			var got *notify.InboundMessage
			c.dispatchEvent(&Frame{Payload: []byte(tc.payload)}, func(m *notify.InboundMessage) {
				got = m
			})
			if got == nil {
				t.Fatalf("handler not called")
			}
			if got.ChatType != tc.wantChat {
				t.Errorf("ChatType = %q, want %q", got.ChatType, tc.wantChat)
			}
			if got.ChatID != tc.wantChatID {
				t.Errorf("ChatID = %q, want %q", got.ChatID, tc.wantChatID)
			}
			if got.ThreadID != tc.wantThr {
				t.Errorf("ThreadID = %q, want %q", got.ThreadID, tc.wantThr)
			}
			if got.RootID != tc.wantRoot {
				t.Errorf("RootID = %q, want %q", got.RootID, tc.wantRoot)
			}
			if got.ParentID != tc.wantPar {
				t.Errorf("ParentID = %q, want %q", got.ParentID, tc.wantPar)
			}
			if tc.wantRoot != "" && got.ReplyContext != "om_4" {
				t.Errorf("ReplyContext = %v", got.ReplyContext)
			}
		})
	}
}

// TestPatchCard 验证 PatchCard 走 PATCH 方法、URL 含 message_id、body 含 content（card JSON）。
func TestPatchCard(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := mockFeishuGatewayMethod(t, &gotMethod, &gotBody, &gotPath, "om-card1")

	c := New(notify.WithAppID("ai"), notify.WithAppSecret("as"), notify.WithBaseURL(srv.URL))
	_, err := c.PatchCard("om-card1", &platformMessage{
		TargetID: "ou_x",
		MsgType:  notify.MsgCard,
		Card:     &notify.Card{Title: "完成", Content: "result text"},
	}, nil)
	if err != nil {
		t.Fatalf("patch card: %v", err)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/im/v1/messages/om-card1") {
		t.Errorf("path = %q", gotPath)
	}
	content, _ := gotBody["content"].(string)
	if !strings.Contains(content, "result text") {
		t.Errorf("content = %q, want contains result text", content)
	}
	// patch body 不应含 msg_type（与 send/reply 不同）
	if _, has := gotBody["msg_type"]; has {
		t.Errorf("patch body should not contain msg_type")
	}
}

// TestBuildFeishuCard_WithButtons 验证带按钮的卡片渲染成 schema 2.0 behaviors button。
func TestBuildFeishuCard_WithButtons(t *testing.T) {
	msg := &platformMessage{
		MsgType: notify.MsgCard,
		Card: &notify.Card{
			Title:   "执行中",
			Content: "thinking...",
			Buttons: []notify.CardButton{
				{Text: "停止", Style: "danger", Value: map[string]any{"action": "stop", "run_id": "r1"}},
			},
		},
	}
	_, content, err := buildFeishuContent(msg)
	if err != nil {
		t.Fatalf("buildFeishuContent: %v", err)
	}
	// 解析 card JSON 验证结构
	var card map[string]any
	if err := json.Unmarshal([]byte(content), &card); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}
	body, _ := card["body"].(map[string]any)
	elements, _ := body["elements"].([]any)
	if len(elements) < 2 {
		t.Fatalf("expected >=2 elements (markdown + button), got %d", len(elements))
	}
	columnSet, _ := elements[1].(map[string]any)
	if columnSet["tag"] != "column_set" {
		t.Fatalf("button container tag = %v, want column_set", columnSet["tag"])
	}
	columns, _ := columnSet["columns"].([]any)
	if len(columns) != 1 {
		t.Fatalf("expected 1 button column, got %d", len(columns))
	}
	column, _ := columns[0].(map[string]any)
	columnElements, _ := column["elements"].([]any)
	if len(columnElements) != 1 {
		t.Fatalf("expected 1 column element, got %d", len(columnElements))
	}
	btn, _ := columnElements[0].(map[string]any)
	if btn["tag"] != "button" {
		t.Errorf("button tag = %v", btn["tag"])
	}
	if btn["type"] != "danger" {
		t.Errorf("button type = %v", btn["type"])
	}
	behaviors, _ := btn["behaviors"].([]any)
	if len(behaviors) != 1 {
		t.Fatalf("expected 1 behavior, got %d", len(behaviors))
	}
	bh, _ := behaviors[0].(map[string]any)
	if bh["type"] != "callback" {
		t.Errorf("behavior type = %v", bh["type"])
	}
	val, _ := bh["value"].(map[string]any)
	if val["action"] != "stop" {
		t.Errorf("value.action = %v", val["action"])
	}
}

func TestBuildFeishuCard_AdvancedElementsAndConfig(t *testing.T) {
	msg := &platformMessage{
		MsgType: notify.MsgCard,
		Card: &notify.Card{
			Title:  "流式回答",
			Config: map[string]any{"streaming_mode": true},
			Elements: []map[string]any{
				{"tag": "markdown", "content": "answer"},
				{
					"tag":       "column_set",
					"flex_mode": "none",
					"columns": []map[string]any{
						{
							"tag":    "column",
							"width":  "weighted",
							"weight": 1,
							"elements": []map[string]any{
								{"tag": "markdown", "content": "努力回答中..."},
							},
						},
						{
							"tag":   "column",
							"width": "auto",
							"elements": []map[string]any{
								{"tag": "markdown", "content": "`qwen3.6-plus-no-thinking`"},
							},
						},
					},
				},
			},
		},
	}
	_, content, err := buildFeishuContent(msg)
	if err != nil {
		t.Fatalf("buildFeishuContent: %v", err)
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(content), &card); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}
	cfg, _ := card["config"].(map[string]any)
	if cfg["streaming_mode"] != true {
		t.Fatalf("streaming_mode = %v, want true", cfg["streaming_mode"])
	}
	body, _ := card["body"].(map[string]any)
	elements, _ := body["elements"].([]any)
	if len(elements) != 2 {
		t.Fatalf("elements len = %d, want 2", len(elements))
	}
	columnSet, _ := elements[1].(map[string]any)
	if columnSet["tag"] != "column_set" {
		t.Fatalf("second element tag = %v, want column_set", columnSet["tag"])
	}
	if strings.Contains(content, `"tag":"note"`) {
		t.Fatalf("card content should not contain note: %q", content)
	}
}

// TestCapabilities 验证飞书能力声明。
func TestClient_Capabilities(t *testing.T) {
	c := New()
	caps := c.Capabilities()
	if !caps.NativeReply || !caps.Reactions || !caps.SendCard || !caps.UpdateCard || !caps.CardActions {
		t.Errorf("feishu should support all card capabilities, got %+v", caps)
	}
}

func TestBuildFeishuOnboardingQRURL_AddsCardActionCallback(t *testing.T) {
	qrURL, err := buildFeishuOnboardingQRURL("https://example.com/confirm?device_code=d1", map[string]string{"app_id": "cli_1"})
	if err != nil {
		t.Fatalf("build qr url: %v", err)
	}
	parsed, err := url.Parse(qrURL)
	if err != nil {
		t.Fatalf("parse qr url: %v", err)
	}
	if parsed.Query().Get("clientID") != "cli_1" {
		t.Fatalf("clientID = %q, want cli_1", parsed.Query().Get("clientID"))
	}
	addonsRaw := parsed.Query().Get("addons")
	if addonsRaw == "" {
		t.Fatalf("missing addons in qr url: %s", qrURL)
	}
	compressed, err := base64.RawURLEncoding.DecodeString(addonsRaw)
	if err != nil {
		t.Fatalf("decode addons: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip addons: %v", err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read addons: %v", err)
	}
	var addons map[string]any
	if err := json.Unmarshal(body, &addons); err != nil {
		t.Fatalf("unmarshal addons: %v", err)
	}
	callbacks, _ := addons["callbacks"].(map[string]any)
	items, _ := callbacks["items"].([]any)
	if len(items) != 1 || items[0] != "card.action.trigger" {
		t.Fatalf("callbacks.items = %#v, want card.action.trigger", items)
	}
}

func TestDispatchEventMixedContentObject(t *testing.T) {
	c := New()
	payload := `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{` +
		`"message_id":"om_mixed","chat_id":"oc_d","chat_type":"p2p",` +
		`"message_type":"text","content":{"content":[[{"tag":"img","image_key":"img_v3_object"},{"tag":"text","text":"描述一下"}]]}` +
		`}}}`
	var got *notify.InboundMessage
	c.dispatchEvent(&Frame{Payload: []byte(payload)}, func(m *notify.InboundMessage) {
		got = m
	})
	if got == nil {
		t.Fatalf("handler not called")
	}
	if !strings.Contains(got.Text, "描述一下") {
		t.Fatalf("Text = %q, want mixed text", got.Text)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Type != notify.MsgImage || got.Attachments[0].FileKey != "img_v3_object" {
		t.Fatalf("Attachments = %+v, want image attachment", got.Attachments)
	}
}

func TestDispatchEventMixedPostContentString(t *testing.T) {
	c := New()
	content := `{"title":"","content":[[{"tag":"img","image_key":"img_v3_real"},{"tag":"text","text":"描述这张图","style":[]}]]}`
	contentJSON, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	payload := `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{` +
		`"message_id":"om_post_mixed","chat_id":"oc_d","chat_type":"p2p",` +
		`"message_type":"post","content":` + string(contentJSON) +
		`}}}`
	var got *notify.InboundMessage
	c.dispatchEvent(&Frame{Payload: []byte(payload)}, func(m *notify.InboundMessage) {
		got = m
	})
	if got == nil {
		t.Fatalf("handler not called")
	}
	if !strings.Contains(got.Text, "描述这张图") {
		t.Fatalf("Text = %q, want mixed post text", got.Text)
	}
	if strings.Contains(got.Text, `"image_key"`) {
		t.Fatalf("Text should not expose raw json: %q", got.Text)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Type != notify.MsgImage || got.Attachments[0].FileKey != "img_v3_real" {
		t.Fatalf("Attachments = %+v, want image attachment", got.Attachments)
	}
}

func TestFeishuOnboardingResultFromPollExtractsOwnerID(t *testing.T) {
	var poll registrationPollResp
	if err := json.Unmarshal([]byte(`{
		"client_id":"cli_owner",
		"client_secret":"sec_owner",
		"user_info":{
			"open_id":"ou_owner",
			"tenant_brand":"feishu"
		}
	}`), &poll); err != nil {
		t.Fatalf("decode poll response: %v", err)
	}

	result := onboardingResultFromPoll(notify.PlatformFeishu, poll)
	if result.AppID != "cli_owner" || result.AppSecret != "sec_owner" {
		t.Fatalf("credentials = %s/%s", result.AppID, result.AppSecret)
	}
	if result.OwnerID != "ou_owner" {
		t.Fatalf("OwnerID = %q, want ou_owner", result.OwnerID)
	}
}

// TestDispatchCardAction 验证卡片按钮回调事件被解析成 InboundMessage（IsCardAction=true）。
func TestDispatchCardAction(t *testing.T) {
	cases := []struct {
		name       string
		payload    string
		wantAction string
		wantRunID  string
		wantOption string
		wantSub    string
	}{
		{
			name:       "legacy top-level ids",
			wantAction: "stop",
			wantRunID:  "r9",
			payload: `{
				"schema":"2.0",
				"header":{"event_id":"ev1","event_type":"card.action.trigger"},
				"event":{
					"operator":{"open_id":"ou_op1"},
					"action":{"value":{"action":"stop","run_id":"r9"},"tag":"button"},
					"message_id":"om_card9",
					"chat_id":"oc_chat9"
				}
			}`,
		},
		{
			name:       "official context ids",
			wantAction: "stop",
			wantRunID:  "r9",
			payload: `{
				"schema":"2.0",
				"header":{"event_id":"ev1","event_type":"card.action.trigger"},
				"event":{
					"operator":{"open_id":"ou_op1"},
					"context":{"open_message_id":"om_card9","open_chat_id":"oc_chat9"},
					"action":{"value":{"action":"stop","run_id":"r9"},"tag":"button","name":"stop"}
				}
			}`,
		},
		{
			name:       "official nested operator id",
			wantAction: "stop",
			wantRunID:  "r9",
			payload: `{
				"schema":"2.0",
				"header":{"event_id":"ev1","event_type":"card.action.trigger"},
				"event":{
					"operator":{"operator_id":{"open_id":"ou_op1"}},
					"context":{"open_message_id":"om_card9","open_chat_id":"oc_chat9"},
					"action":{"value":{"action":"stop","run_id":"r9"},"tag":"button","name":"stop"}
				}
			}`,
		},
		{
			name:       "select static option",
			wantAction: "config",
			wantOption: "summary",
			wantSub:    "set_granularity",
			payload: `{
				"schema":"2.0",
				"header":{"event_id":"ev1","event_type":"card.action.trigger"},
				"event":{
					"operator":{"open_id":"ou_op1"},
					"context":{"open_message_id":"om_card9","open_chat_id":"oc_chat9"},
					"action":{"value":{"action":"config","sub":"set_granularity"},"tag":"select_static","option":"summary"}
				}
			}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			var got *notify.InboundMessage
			c.dispatchCardAction(&Frame{Payload: []byte(tc.payload)}, func(m *notify.InboundMessage) {
				got = m
			})
			if got == nil {
				t.Fatalf("handler not called")
			}
			if !got.IsCardAction {
				t.Errorf("IsCardAction = false, want true")
			}
			if got.ActionValue["action"] != tc.wantAction {
				t.Errorf("ActionValue.action = %v, want %v", got.ActionValue["action"], tc.wantAction)
			}
			if tc.wantRunID != "" && got.ActionValue["run_id"] != tc.wantRunID {
				t.Errorf("ActionValue.run_id = %v, want %v", got.ActionValue["run_id"], tc.wantRunID)
			}
			if tc.wantOption != "" {
				if got.ActionValue["option"] != "summary" {
					t.Errorf("ActionValue.option = %v, want summary", got.ActionValue["option"])
				}
			}
			if tc.wantSub != "" && got.ActionValue["sub"] != tc.wantSub {
				t.Errorf("ActionValue.sub = %v, want %v", got.ActionValue["sub"], tc.wantSub)
			}
			if got.ChatID != "oc_chat9" {
				t.Errorf("ChatID = %q", got.ChatID)
			}
			if got.SenderID != "ou_op1" {
				t.Errorf("SenderID = %q", got.SenderID)
			}
			if got.ReplyContext != "om_card9" {
				t.Errorf("ReplyContext = %v, want om_card9", got.ReplyContext)
			}
		})
	}
}

func TestDispatchBotMenuEventIgnored(t *testing.T) {
	payload := `{
		"schema":"2.0",
		"header":{
			"event_id":"ev-menu1",
			"event_type":"application.bot.menu_v6",
			"app_id":"cli_x"
		},
		"event":{
			"operator":{
				"operator_id":{
					"open_id":"ou_operator",
					"user_id":"u_operator",
					"union_id":"on_operator"
				}
			},
			"event_key":"model",
			"timestamp":1669364458
		}
	}`

	c := New()
	called := false
	c.dispatchByType("event", &Frame{Payload: []byte(payload)}, func(m *notify.InboundMessage) {
		called = true
	})
	if called {
		t.Fatalf("bot menu events require developer-console configuration and should be ignored by the zero-config onboarding flow")
	}
}

// TestDispatchByType 验证按 LC frame header type 分流到 card / event。
func TestDispatchByType(t *testing.T) {
	cardPayload := `{"header":{"event_type":"card.action.trigger"},"event":{"operator":{"open_id":"ou_o"},"action":{"value":{"action":"new"}},"message_id":"om1","chat_id":"oc1"}}`
	msgPayload := `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{"message_id":"om_m","chat_id":"oc_d","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hi\"}"}}}`

	t.Run("card frame routes to card action", func(t *testing.T) {
		c := New()
		var got *notify.InboundMessage
		f := &Frame{
			Payload: []byte(cardPayload),
		}
		f.Headers = append(f.Headers, Header{Key: HeaderType, Value: string(MessageTypeCard)})
		c.dispatchByType(string(MessageTypeCard), f, func(m *notify.InboundMessage) { got = m })
		if got == nil || !got.IsCardAction {
			t.Fatalf("should route to card action, got=%v", got)
		}
	})

	t.Run("event frame with card payload routes to card action", func(t *testing.T) {
		c := New()
		var got *notify.InboundMessage
		f := &Frame{
			Payload: []byte(cardPayload),
		}
		f.Headers = append(f.Headers, Header{Key: HeaderType, Value: string(MessageTypeEvent)})
		c.dispatchByType(string(MessageTypeEvent), f, func(m *notify.InboundMessage) { got = m })
		if got == nil || !got.IsCardAction {
			t.Fatalf("event frame card payload should route to card action, got=%v", got)
		}
	})

	t.Run("event frame routes to message", func(t *testing.T) {
		c := New()
		var got *notify.InboundMessage
		f := &Frame{Payload: []byte(msgPayload)}
		c.dispatchByType(string(MessageTypeEvent), f, func(m *notify.InboundMessage) { got = m })
		if got == nil || got.IsCardAction {
			t.Fatalf("should route to message, got=%v", got)
		}
		if got.ChatID != "oc_d" {
			t.Errorf("ChatID = %q", got.ChatID)
		}
	})
}

func TestDispatchEventUsesMessageCreateTimeBeforeFrameTimestamp(t *testing.T) {
	payload := `{"event":{"sender":{"sender_id":{"open_id":"ou_a"}},"message":{"message_id":"om_m","chat_id":"oc_d","chat_type":"p2p","message_type":"text","create_time":"1783591964916","content":"{\"text\":\"hi\"}"}}}`
	f := &Frame{Payload: []byte(payload)}
	f.Headers = append(f.Headers, Header{Key: HeaderTimestamp, Value: "1783613557000"})

	c := New()
	var got *notify.InboundMessage
	c.dispatchEvent(f, func(m *notify.InboundMessage) { got = m })

	if got == nil {
		t.Fatal("expected inbound message")
	}
	want := time.UnixMilli(1783591964916)
	if !got.EventTime.Equal(want) {
		t.Fatalf("EventTime = %s, want message create_time %s", got.EventTime, want)
	}
}

func TestNormalizeFeishuChatType(t *testing.T) {
	cases := []struct{ in, thr, want string }{
		{"p2p", "", "private"},
		{"group", "", "group"},
		{"group", "om_t", "topic"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := normalizeFeishuChatType(c.in, c.thr); got != c.want {
			t.Errorf("normalizeFeishuChatType(%q,%q) = %q, want %q", c.in, c.thr, got, c.want)
		}
	}
}

// TestMimeToExt 验证 MIME 到扩展名映射。
func TestMimeToExt(t *testing.T) {
	cases := []struct{ mime, fileKey, want string }{
		{"image/jpeg", "", "jpg"},
		{"image/png", "", "png"},
		{"image/gif", "", "gif"},
		{"image/webp", "", "webp"},
		{"application/pdf", "", "pdf"},
		{"application/octet-stream", "file_v3_test.pdf", "pdf"},
		{"application/octet-stream", "file_v3_noext", "bin"},
		{"text/plain", "", "plain"},
	}
	for _, c := range cases {
		isImage := strings.HasPrefix(c.mime, "image/")
		got := mimeToExt(c.mime, isImage, c.fileKey)
		if got != c.want {
			t.Errorf("mimeToExt(%q, fileKey=%q) = %q, want %q", c.mime, c.fileKey, got, c.want)
		}
	}
}

// TestDownloadResource 验证下载飞书资源到本地文件。
func TestDownloadResource(t *testing.T) {
	// mock 飞书 API 返回 binary 图片
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal") {
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
			return
		}
		if r.URL.Path == "/open-apis/im/v1/messages/om_test/resources/img_v3_001" {
			if got := r.URL.Query().Get("type"); got != "image" {
				t.Errorf("resource type query = %q, want image", got)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}) // PNG header
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)

	c := New(notify.WithAppID("ai"), notify.WithAppSecret("as"), notify.WithBaseURL(srv.URL))
	cfg := &notify.SendConfig{AppID: "ai", AppSecret: "as", BaseURL: srv.URL, Timeout: 5 * time.Second}
	localPath, mimeType, size, err := c.DownloadResource(cfg, "om_test", "img_v3_001", true)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if mimeType != "image/png" {
		t.Errorf("mimeType = %q", mimeType)
	}
	if size != 6 {
		t.Errorf("size = %d, want 6", size)
	}
	if localPath == "" {
		t.Fatal("localPath should not be empty")
	}
	// 验证文件存在
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("downloaded file not found: %v", err)
	}
	// 清理
	_ = os.Remove(localPath)
}
func TestCombineFrame_OutOfRange(t *testing.T) {
	c := New()
	// sum=0 → 丢弃，不 panic
	if r := c.combineFrame("m1", 0, 0, []byte("x")); r != nil {
		t.Errorf("sum=0 should return nil, got %v", r)
	}
	// seq 超出 sum → 丢弃
	if r := c.combineFrame("m2", 2, 5, []byte("x")); r != nil {
		t.Errorf("seq>=sum should return nil, got %v", r)
	}
	// 负 seq → 丢弃
	if r := c.combineFrame("m3", 2, -1, []byte("x")); r != nil {
		t.Errorf("seq<0 should return nil, got %v", r)
	}
	// 正常分包仍能合包
	c.combineFrame("m4", 2, 0, []byte("hello "))
	r := c.combineFrame("m4", 2, 1, []byte("world"))
	if r == nil || string(r) != "hello world" {
		t.Errorf("normal combine failed, got %v", r)
	}
}
