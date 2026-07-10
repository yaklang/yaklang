package imcontrol

import (
	"testing"

	"github.com/yaklang/yaklang/common/notify"
)

func TestBuildTextRequestUsesFeishuReplyWhenQuoteEnabled(t *testing.T) {
	req, err := buildTextRequest(notify.PlatformFeishu, "oc_chat", "om_msg", "hello", true)
	if err != nil {
		t.Fatalf("buildTextRequest: %v", err)
	}
	if req.Action != notify.ActionMessagesReply {
		t.Fatalf("action = %q", req.Action)
	}
	if req.Target.ReplyTo != "om_msg" {
		t.Fatalf("replyTo = %q", req.Target.ReplyTo)
	}
	if req.Target.Native["receive_id_type"] != "chat_id" {
		t.Fatalf("receive_id_type = %v", req.Target.Native["receive_id_type"])
	}
	if req.Message.Type != notify.MessageText || req.Message.Text != "hello" {
		t.Fatalf("message = %#v", req.Message)
	}
}

func TestBuildTextRequestSendsFeishuNormallyWhenQuoteDisabled(t *testing.T) {
	req, err := buildTextRequest(notify.PlatformFeishu, "oc_chat", "om_msg", "hello", false)
	if err != nil {
		t.Fatalf("buildTextRequest: %v", err)
	}
	if req.Action != notify.ActionMessagesSend {
		t.Fatalf("action = %q", req.Action)
	}
	if req.Target.ID != "oc_chat" {
		t.Fatalf("target id = %q", req.Target.ID)
	}
	if req.Target.ReplyTo != "" {
		t.Fatalf("replyTo = %q", req.Target.ReplyTo)
	}
}

func TestBuildTextRequestAlwaysRepliesDingTalkWithMessageContext(t *testing.T) {
	req, err := buildTextRequest(notify.PlatformDingTalk, "cid", `{"SessionWebhook":"http://127.0.0.1"}`, "hello", false)
	if err != nil {
		t.Fatalf("buildTextRequest: %v", err)
	}
	if req.Action != notify.ActionMessagesReply {
		t.Fatalf("action = %q", req.Action)
	}
	if req.Target.ReplyTo == "" {
		t.Fatal("expected dingtalk reply context")
	}
}

func TestBuildReactionRequest(t *testing.T) {
	req, err := buildReactionRequest(notify.PlatformFeishu, "om_msg", "OK")
	if err != nil {
		t.Fatalf("buildReactionRequest: %v", err)
	}
	if req.Action != notify.ActionReactionsAdd {
		t.Fatalf("action = %q", req.Action)
	}
	if req.Target.ID != "om_msg" {
		t.Fatalf("target id = %q", req.Target.ID)
	}
	if req.Native["emoji_type"] != "OK" {
		t.Fatalf("emoji_type = %v", req.Native["emoji_type"])
	}
}

func TestBuildMessageRequestForCardSend(t *testing.T) {
	card := &notify.Card{Title: "配置", Content: "body"}
	req, err := buildMessageRequest(notify.PlatformFeishu, notify.ActionMessagesSend, "oc_chat", "", "chat_id", cardMessage(card))
	if err != nil {
		t.Fatalf("buildMessageRequest: %v", err)
	}
	if req.Action != notify.ActionMessagesSend {
		t.Fatalf("action = %q", req.Action)
	}
	if req.Target.ID != "oc_chat" || req.Target.Native["receive_id_type"] != "chat_id" {
		t.Fatalf("target = %#v", req.Target)
	}
	if req.Message.Type != notify.MessageCard || req.Message.Card != card {
		t.Fatalf("message = %#v", req.Message)
	}
}

func TestBuildMessageRequestForPatch(t *testing.T) {
	req, err := buildMessageRequest(notify.PlatformFeishu, notify.ActionMessagesPatch, "", "om_card", "", &notify.Message{
		Type:     notify.MessageMarkdown,
		Markdown: "# updated",
	})
	if err != nil {
		t.Fatalf("buildMessageRequest: %v", err)
	}
	if req.Target.ID != "om_card" {
		t.Fatalf("patch target id = %q", req.Target.ID)
	}
	if req.Message.Type != notify.MessageMarkdown || req.Message.Markdown != "# updated" {
		t.Fatalf("message = %#v", req.Message)
	}
}

func TestBuildOnboardingWelcomeRequestUsesCardWhenSupported(t *testing.T) {
	card := &notify.Card{Title: "欢迎", Content: "body"}
	req, err := buildOnboardingWelcomeRequest(notify.PlatformFeishu, "ou_owner", card)
	if err != nil {
		t.Fatalf("buildOnboardingWelcomeRequest: %v", err)
	}
	if req.Action != notify.ActionMessagesSend {
		t.Fatalf("action = %q", req.Action)
	}
	if req.Target.ID != "ou_owner" || req.Target.Native["receive_id_type"] != "open_id" {
		t.Fatalf("target = %#v", req.Target)
	}
	if req.Message.Type != notify.MessageCard || req.Message.Card != card {
		t.Fatalf("message = %#v", req.Message)
	}
}

func TestBuildOnboardingWelcomeRequestFallsBackToText(t *testing.T) {
	req, err := buildOnboardingWelcomeRequest(notify.PlatformDingTalk, "staff_owner", &notify.Card{Title: "欢迎", Content: "body"})
	if err != nil {
		t.Fatalf("buildOnboardingWelcomeRequest: %v", err)
	}
	if req.Message.Type != notify.MessageMarkdown || req.Message.Markdown != "body" {
		t.Fatalf("message = %#v", req.Message)
	}
}
