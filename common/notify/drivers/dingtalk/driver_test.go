package dingtalk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
)

func TestDescriptorDoesNotPretendDingTalkHasCardActions(t *testing.T) {
	desc := Descriptor()
	if desc.Capabilities.CardActions {
		t.Fatal("dingtalk descriptor must not declare card actions until callbacks are implemented")
	}
	if desc.Capabilities.SupportsNativeCard("feishu.card.v2") {
		t.Fatal("dingtalk must not claim feishu native card schema")
	}
}

func TestDescriptorDeclaresDingTalkTextAndReceiveActions(t *testing.T) {
	desc := Descriptor()
	if !desc.Capabilities.SendText || !desc.Capabilities.Reactions || !desc.Capabilities.ReceiveEvents || !desc.Capabilities.DownloadResources || !desc.Capabilities.Onboarding {
		t.Fatalf("dingtalk base capabilities incomplete: %#v", desc.Capabilities)
	}
	required := map[string]bool{
		"messages:send":      false,
		"messages:reply":     false,
		"reactions:add":      false,
		"resources:download": false,
		"events:receive":     false,
		"onboarding:start":   false,
	}
	for _, action := range desc.Actions {
		if _, ok := required[string(action)]; ok {
			required[string(action)] = true
		}
	}
	for action, found := range required {
		if !found {
			t.Fatalf("dingtalk descriptor missing action %s", action)
		}
	}
}

func TestDriverDoDownloadResource(t *testing.T) {
	imageBytes := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43}
	mediaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image.jpg" {
			t.Fatalf("media path = %q", r.URL.Path)
		}
		if r.URL.RawQuery != "token=signed" {
			t.Fatalf("media query = %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(imageBytes)
	}))
	t.Cleanup(mediaSrv.Close)

	var got map[string]any
	var gotToken string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"mock-token","expireIn":7200}`))
		case "/v1.0/robot/messageFiles/download":
			gotToken = r.Header.Get("x-acs-dingtalk-access-token")
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"downloadUrl":"` + mediaSrv.URL + `/image.jpg?token=signed"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(apiSrv.Close)

	driver, err := Descriptor().New(notify.DriverConfig{
		SendConfig: notify.NewSendConfig(
			notify.WithAppID("robot_x"),
			notify.WithAppSecret("secret"),
			notify.WithBaseURL(apiSrv.URL),
			notify.WithTimeout(5*time.Second),
		),
	})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformDingTalk,
		Action:   notify.ActionResourcesDownload,
		Resource: &notify.ResourceRef{
			ID:   "download-code",
			Type: "image",
		},
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if gotToken != "mock-token" {
		t.Fatalf("access token header = %q", gotToken)
	}
	if got["downloadCode"] != "download-code" || got["robotCode"] != "robot_x" {
		t.Fatalf("download request = %#v", got)
	}
	if resp.Resource == nil || resp.Resource.Path == "" {
		t.Fatalf("resource = %#v", resp.Resource)
	}
	defer os.Remove(resp.Resource.Path)
	if resp.Resource.MimeType != "image/jpeg" {
		t.Fatalf("mime = %q", resp.Resource.MimeType)
	}
	if resp.Resource.Size != int64(len(imageBytes)) {
		t.Fatalf("size = %d", resp.Resource.Size)
	}
	if b, err := os.ReadFile(resp.Resource.Path); err != nil || string(b) != string(imageBytes) {
		t.Fatalf("downloaded file mismatch: %v %x", err, b)
	}
}

func TestDriverDoAddReactionUsesEmotionAPI(t *testing.T) {
	var got map[string]any
	var gotPath string
	srv := mockGateway(t, &got, &gotPath, "dt_msg")

	driver, err := Descriptor().New(notify.DriverConfig{
		SendConfig: notify.NewSendConfig(
			notify.WithAppID("app-key"),
			notify.WithAppSecret("secret"),
			notify.WithBaseURL(srv.URL),
			notify.WithTimeout(5*time.Second),
		),
	})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	rc, _ := json.Marshal(replyContext{
		ConversationID: "cid_x",
		MsgID:          "msg_x",
		RobotCode:      "robot_x",
	})
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformDingTalk,
		Action:   notify.ActionReactionsAdd,
		Target: notify.Target{
			ID: string(rc),
		},
		Native: notify.NativeOptions{
			"emoji_type": "开始任务",
		},
	})
	if err != nil {
		t.Fatalf("Do add reaction: %v", err)
	}
	if resp.Platform != notify.PlatformDingTalk || resp.Action != notify.ActionReactionsAdd {
		t.Fatalf("response = %#v", resp)
	}
	if gotPath != "/v1.0/robot/emotion/reply" {
		t.Fatalf("path = %q", gotPath)
	}
	if got["robotCode"] != "robot_x" || got["openConversationId"] != "cid_x" || got["openMsgId"] != "msg_x" {
		t.Fatalf("emotion target = %#v", got)
	}
	if got["emotionName"] != "开始任务" {
		t.Fatalf("emotionName = %v", got["emotionName"])
	}
}

func TestDriverDoSendGroupMarkdown(t *testing.T) {
	var got map[string]any
	var gotPath string
	srv := mockGateway(t, &got, &gotPath, "dt_msg")

	driver, err := Descriptor().New(notify.DriverConfig{
		SendConfig: notify.NewSendConfig(
			notify.WithAppID("app-key"),
			notify.WithAppSecret("secret"),
			notify.WithBaseURL(srv.URL),
			notify.WithTimeout(5*time.Second),
		),
	})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformDingTalk,
		Action:   notify.ActionMessagesSend,
		Target: notify.Target{
			ID:   "cid_x",
			Kind: notify.TargetChat,
		},
		Message: &notify.Message{
			Type:     notify.MessageMarkdown,
			Markdown: "# title\nbody",
			Card:     &notify.Card{Title: "T"},
		},
	})
	if err != nil {
		t.Fatalf("Do send group markdown: %v", err)
	}
	if resp.MessageID != "dt_msg" {
		t.Fatalf("message id = %q", resp.MessageID)
	}
	if gotPath != "/v1.0/robot/groupMessages/send" {
		t.Fatalf("path = %q", gotPath)
	}
	if got["openConversationId"] != "cid_x" {
		t.Fatalf("openConversationId = %v", got["openConversationId"])
	}
	if got["msgKey"] != "sampleMarkdown" {
		t.Fatalf("msgKey = %v", got["msgKey"])
	}
	param, _ := got["msgParam"].(string)
	if !strings.Contains(param, "# title") || !strings.Contains(param, "body") {
		t.Fatalf("msgParam = %q", param)
	}
}

func TestDriverDoReplyViaSessionWebhook(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/robot/send" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("session") != "s1" {
			t.Fatalf("session query = %q", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode reply body: %v", err)
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	rc, _ := json.Marshal(replyContext{SessionWebhook: srv.URL + "/robot/send?session=s1"})
	driver, err := Descriptor().New(notify.DriverConfig{SendConfig: notify.NewSendConfig()})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformDingTalk,
		Action:   notify.ActionMessagesReply,
		Target: notify.Target{
			ReplyTo: string(rc),
		},
		Message: &notify.Message{
			Type:     notify.MessageMarkdown,
			Markdown: "## hi\nreply body",
			Card:     &notify.Card{Title: "reply-title"},
		},
	})
	if err != nil {
		t.Fatalf("Do reply via session webhook: %v", err)
	}
	if resp.Platform != notify.PlatformDingTalk || resp.Action != notify.ActionMessagesReply {
		t.Fatalf("response = %#v", resp)
	}
	if got["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %v", got["msgtype"])
	}
	md, _ := got["markdown"].(map[string]any)
	if md["title"] != "reply-title" {
		t.Fatalf("title = %v", md["title"])
	}
	if !strings.Contains(md["text"].(string), "reply body") {
		t.Fatalf("markdown text = %v", md["text"])
	}
}

func TestDriverStreamEventsReceive(t *testing.T) {
	driver := &Driver{
		start: func(ctx context.Context, handler func(*notify.InboundMessage)) error {
			handler(&notify.InboundMessage{
				Platform: notify.PlatformDingTalk,
				ID:       "dt_msg",
				Text:     "hello",
			})
			return nil
		},
	}
	var events []notify.Event
	err := driver.Stream(context.Background(), &notify.Request{
		Platform: notify.PlatformDingTalk,
		Action:   notify.ActionEventsReceive,
	}, func(ev notify.Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Type != notify.EventMessage || events[0].Message.ID != "dt_msg" {
		t.Fatalf("event = %#v", events[0])
	}
}

func TestReceiverPreservesCommandSpacesAndMention(t *testing.T) {
	payload := `{
		"conversationId":"cid_group",
		"chatbotUserId":"bot_staff",
		"senderStaffId":"user_staff",
		"senderNick":"Alice",
		"msgId":"msg-1",
		"sessionWebhook":"https://example.invalid/webhook",
		"conversationType":"2",
		"isInAtList":true,
		"text":{"content":" @机器人 /session 5 "}
	}`
	frame, err := json.Marshal(dataFrame{
		Type:  "callback",
		Topic: topicBotMessage,
		Data:  payload,
	})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	var got *notify.InboundMessage
	c := &Client{}
	c.handleStreamMessage(frame, nil, func(msg *notify.InboundMessage) {
		got = msg
	})
	if got == nil {
		t.Fatal("handler was not called")
	}
	if got.Text != "@机器人 /session 5" {
		t.Fatalf("text = %q, want command spaces preserved", got.Text)
	}
	if !got.MentionBot {
		t.Fatal("MentionBot = false, want true")
	}
}

func TestReceiverExtractsRichTextPictureAttachment(t *testing.T) {
	payload := `{
		"conversationId":"cid_private",
		"chatbotUserId":"bot_staff",
		"senderStaffId":"user_staff",
		"senderNick":"Alice",
		"msgId":"msg-rich",
		"sessionWebhook":"https://example.invalid/webhook",
		"conversationType":"1",
		"robotCode":"robot_x",
		"content":{"richText":[
			{"pictureDownloadCode":"picture-code","downloadCode":"download-code","type":"picture"},
			{"text":"\n"},
			{"text":"你能看见这张图片吗，描述一下"}
		]},
		"msgtype":"richText"
	}`
	frame, err := json.Marshal(dataFrame{
		Type:  "callback",
		Topic: topicBotMessage,
		Data:  payload,
	})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	var got *notify.InboundMessage
	c := &Client{}
	c.handleStreamMessage(frame, nil, func(msg *notify.InboundMessage) {
		got = msg
	})
	if got == nil {
		t.Fatal("handler was not called")
	}
	if got.Text != "你能看见这张图片吗，描述一下" {
		t.Fatalf("text = %q", got.Text)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(got.Attachments))
	}
	att := got.Attachments[0]
	if att.Type != notify.MsgImage || att.FileKey != "download-code" || att.FileName != "picture-code" || att.MessageID != "msg-rich" {
		t.Fatalf("attachment = %#v", att)
	}
}

func TestReceiverExtractsPictureAttachment(t *testing.T) {
	payload := `{
		"conversationId":"cid_private",
		"senderStaffId":"user_staff",
		"senderNick":"Alice",
		"msgId":"msg-picture",
		"sessionWebhook":"https://example.invalid/webhook",
		"conversationType":"1",
		"robotCode":"robot_x",
		"content":{"downloadCode":"download-code"},
		"msgtype":"picture"
	}`
	frame, err := json.Marshal(dataFrame{
		Type:  "callback",
		Topic: topicBotMessage,
		Data:  payload,
	})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	var got *notify.InboundMessage
	c := &Client{}
	c.handleStreamMessage(frame, nil, func(msg *notify.InboundMessage) {
		got = msg
	})
	if got == nil {
		t.Fatal("handler was not called")
	}
	if got.Text != "" {
		t.Fatalf("text = %q", got.Text)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(got.Attachments))
	}
	att := got.Attachments[0]
	if att.Type != notify.MsgImage || att.FileKey != "download-code" || att.MessageID != "msg-picture" {
		t.Fatalf("attachment = %#v", att)
	}
}

func TestDriverStreamOnboardingStart(t *testing.T) {
	driver := &Driver{
		onboard: func(timeoutSeconds int, opts map[string]string, handler notify.OnboardingHandler) error {
			if timeoutSeconds != 33 {
				t.Fatalf("timeout = %d", timeoutSeconds)
			}
			if opts["source"] != "desktop" {
				t.Fatalf("opts = %#v", opts)
			}
			return handler(&notify.OnboardingStep{State: "qr", QrURL: "https://dt.example/qr"})
		},
	}
	var events []notify.Event
	err := driver.Stream(context.Background(), &notify.Request{
		Platform: notify.PlatformDingTalk,
		Action:   notify.ActionOnboardingStart,
		Options: notify.Options{
			"timeout_seconds": 33,
			"source":          "desktop",
		},
	}, func(ev notify.Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Stream onboarding: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Type != notify.EventOnboarding || events[0].Onboarding == nil || events[0].Onboarding.QrURL != "https://dt.example/qr" {
		t.Fatalf("event = %#v", events[0])
	}
}
