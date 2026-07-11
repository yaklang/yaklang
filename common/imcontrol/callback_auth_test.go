package imcontrol

import (
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
)

func TestCallbackAuth_SignVerify(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	token := a.Sign(CallbackSignInput{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	if token == "" {
		t.Fatal("sign returned empty token")
	}
	if !strings.HasPrefix(token, "cb.v1.") {
		t.Errorf("token prefix = %q", token[:10])
	}
	result := a.Verify(token, CallbackVerifyExpected{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	if !result.OK {
		t.Errorf("verify failed: %s", result.Reason)
	}
}

func TestCallbackAuth_Expired(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	// 用一个已过去的时间签发（通过替换 now）
	a.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	token := a.Sign(CallbackSignInput{
		RunID: "r1", ChatID: "oc1", Action: "stop",
		TTL: time.Minute,
	})
	// 验签用真实 now（token 已过期）
	a.now = time.Now
	result := a.Verify(token, CallbackVerifyExpected{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	if result.OK {
		t.Error("expired token should fail")
	}
	if result.Reason != "expired" {
		t.Errorf("reason = %q, want expired", result.Reason)
	}
}

func TestCallbackAuth_NonceReplay(t *testing.T) {
	for _, action := range []string{"stop", "review_decision"} {
		t.Run(action, func(t *testing.T) {
			a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
			token := a.Sign(CallbackSignInput{
				RunID: "r1", ChatID: "oc1", Action: action,
			})
			expected := CallbackVerifyExpected{
				RunID: "r1", ChatID: "oc1", Action: action,
			}
			r1 := a.Verify(token, expected)
			if !r1.OK {
				t.Fatalf("first verify failed: %s", r1.Reason)
			}
			r2 := a.Verify(token, expected)
			if r2.OK {
				t.Fatalf("replay should fail for one-shot action %s", action)
			}
			if r2.Reason != "nonce-replay" {
				t.Fatalf("reason = %q, want nonce-replay", r2.Reason)
			}
		})
	}
}

// TestCallbackAuth_NonOneShotActionReusable 验证非一次性动作（new/resume/update_reply_mode）
// 允许同一 token 多次验签通过（最终卡片按钮可多次点击）。
func TestCallbackAuth_NonOneShotActionReusable(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	token := a.Sign(CallbackSignInput{
		ChatID: "oc1", Action: "update_reply_mode",
	})
	expected := CallbackVerifyExpected{
		ChatID: "oc1", Action: "update_reply_mode",
	}
	r1 := a.Verify(token, expected)
	if !r1.OK {
		t.Fatalf("first verify failed: %s", r1.Reason)
	}
	// 同 token 二次验签应通过（非 one-shot 动作不消费 nonce）
	r2 := a.Verify(token, expected)
	if !r2.OK {
		t.Errorf("non-one-shot action should be reusable, got: %s", r2.Reason)
	}
}

func TestCallbackAuth_ContextMismatch(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	token := a.Sign(CallbackSignInput{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	// 用不同 chat 验签
	result := a.Verify(token, CallbackVerifyExpected{
		RunID: "r1", ChatID: "oc_other", Action: "stop",
	})
	if result.OK {
		t.Error("mismatched chat should fail")
	}
	if result.Reason != "context-mismatch" {
		t.Errorf("reason = %q, want context-mismatch", result.Reason)
	}
}

func TestCallbackAuth_BadSignature(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	token := a.Sign(CallbackSignInput{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	// 篡改 token 末尾的签名部分：cb.v1.<payload>.<sig> → 替换 sig
	prefix := "cb.v1."
	rest := token[len(prefix):]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		t.Fatal("token should have payload.sig after prefix")
	}
	tampered := prefix + parts[0] + ".AAAAAA"
	result := a.Verify(tampered, CallbackVerifyExpected{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	if result.OK {
		t.Error("tampered signature should fail")
	}
	if result.Reason != "bad-signature" && result.Reason != "malformed" {
		t.Errorf("reason = %q, want bad-signature or malformed", result.Reason)
	}
}

func TestCallbackAuth_Malformed(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	result := a.Verify("not-a-token", CallbackVerifyExpected{})
	if result.OK {
		t.Error("malformed token should fail")
	}
	if result.Reason != "malformed" {
		t.Errorf("reason = %q, want malformed", result.Reason)
	}
}

// TestFeishuRunPresenter_ButtonHasToken 验证 SignToken 注入后卡片按钮 value 含 token。
func TestFeishuRunPresenter_ButtonHasToken(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	var sentCard *notify.Message
	deps := PresenterDeps{
		SendCard: func(msg *notify.Message, c *notify.SendConfig) (string, error) {
			sentCard = msg
			return "om-1", nil
		},
		SignToken: func(input CallbackSignInput) string {
			return a.Sign(input)
		},
	}
	p := newFeishuRunPresenter(deps)
	sess := &imSession{platform: "feishu", chatID: "oc1", senderID: "ou_op"}
	rc := &RunContext{Session: sess, RunID: "r1"}

	p.OnRunStart(rc)
	if sentCard == nil || sentCard.Card == nil {
		t.Fatal("card not sent")
	}
	if len(sentCard.Card.Buttons) != 1 {
		t.Fatalf("expected 1 button, got %d", len(sentCard.Card.Buttons))
	}
	btn := sentCard.Card.Buttons[0]
	if btn.Value["action"] != "stop" {
		t.Errorf("action = %v", btn.Value["action"])
	}
	token, ok := btn.Value["token"].(string)
	if !ok || token == "" {
		t.Fatal("button value should contain non-empty token")
	}
	// token 可验签
	result := a.Verify(token, CallbackVerifyExpected{
		RunID: "r1", ChatID: "oc1", Action: "stop",
	})
	if !result.OK {
		t.Errorf("button token verify failed: %s", result.Reason)
	}
}

// TestFeishuRunPresenter_FinalButtonsHasToken 验证终态卡片按钮也含 token。
func TestFeishuRunPresenter_FinalButtonsHasToken(t *testing.T) {
	a := NewCallbackAuth([]byte("test-secret-key-32-bytes-long!!!"))
	var lastPatch *notify.Message
	deps := PresenterDeps{
		SendCard: func(msg *notify.Message, c *notify.SendConfig) (string, error) {
			return "om-1", nil
		},
		PatchCard: func(messageID string, msg *notify.Message, c *notify.SendConfig) error {
			lastPatch = msg
			return nil
		},
		SignToken: func(input CallbackSignInput) string {
			return a.Sign(input)
		},
	}
	p := newFeishuRunPresenter(deps)
	sess := &imSession{platform: "feishu", chatID: "oc1", senderID: "ou_op"}
	rc := &RunContext{Session: sess, RunID: "r1"}

	p.OnRunStart(rc)
	p.OnRunResult(rc, RunEvent{Type: RunEventResult, Text: `{"result":"done"}`})

	if lastPatch == nil || lastPatch.Card == nil {
		t.Fatal("final patch not sent")
	}
	// 终态卡片保留回答级操作（新对话/会话面板/配置），配置细项放在 /config 卡片中。
	if len(lastPatch.Card.Buttons) != 3 {
		t.Fatalf("expected 3 final buttons, got %d", len(lastPatch.Card.Buttons))
	}
	wantActions := map[string]bool{"new": false, "session_info": false, "config": false}
	for _, btn := range lastPatch.Card.Buttons {
		action, _ := btn.Value["action"].(string)
		if _, ok := wantActions[action]; !ok {
			t.Errorf("unexpected final button action %q", action)
		} else {
			wantActions[action] = true
		}
		token, ok := btn.Value["token"].(string)
		if !ok || token == "" {
			t.Errorf("button %v missing token", btn.Value["action"])
		}
		if action == "session_info" && btn.Value["session_id"] != sess.persistentSessionId {
			t.Errorf("session_info session_id = %v, want %s", btn.Value["session_id"], sess.persistentSessionId)
		}
		if action == "session_info" && btn.Text != "📌 会话面板" {
			t.Errorf("session_info button text = %q", btn.Text)
		}
	}
	for action, seen := range wantActions {
		if !seen {
			t.Errorf("missing final button action %q", action)
		}
	}
}
