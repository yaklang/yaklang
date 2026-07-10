package imcontrol

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
)

// --- platform capability behavior tests ---

func TestPlatformCapabilities_Feishu(t *testing.T) {
	caps := platformCapabilities(notify.PlatformFeishu)
	// 飞书注册了 Factory，应返回全能力（卡片更新等）
	if !caps.SendCard || !caps.UpdateCard || !caps.CardActions {
		t.Errorf("feishu should support card capabilities, got %+v", caps)
	}
}

func TestPlatformCapabilities_DingTalk(t *testing.T) {
	caps := platformCapabilities(notify.PlatformDingTalk)
	// 钉钉不支持卡片更新/按钮回调
	if caps.UpdateCard || caps.CardActions {
		t.Errorf("dingtalk should not support card update/actions, got %+v", caps)
	}
}

func TestPlatformCapabilities_Unknown(t *testing.T) {
	caps := platformCapabilities(notify.PlatformType("nonexistent"))
	// 未注册平台返回零值
	if caps != (notify.PlatformCapabilities{}) {
		t.Errorf("unknown platform should return zero caps, got %+v", caps)
	}
}

// --- TextRunPresenter 测试 ---

func TestTextRunPresenter_SegmentFlush(t *testing.T) {
	var sentMu sync.Mutex
	var sent []string
	deps := PresenterDeps{
		Send: func(platform notify.PlatformType, chatID, messageID, text string) error {
			sentMu.Lock()
			sent = append(sent, text)
			sentMu.Unlock()
			return nil
		},
	}
	p := newTextRunPresenter(deps, "standard")
	sess := &imSession{platform: "feishu", chatID: "oc1", lastMessageID: "om1"}
	rc := &RunContext{Session: sess}

	// 模拟一个段：两条 delta + segment finished
	p.OnRunDelta(rc, RunEvent{Type: RunEventDelta, WriterID: "w1", Delta: "hello "})
	p.OnRunDelta(rc, RunEvent{Type: RunEventDelta, WriterID: "w1", Delta: "world"})
	p.OnRunSegmentFinished(rc, RunEvent{Type: RunEventSegmentFinished, WriterID: "w1"})

	sentMu.Lock()
	defer sentMu.Unlock()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d: %v", len(sent), sent)
	}
	if sent[0] != "hello world" {
		t.Errorf("sent = %q, want 'hello world'", sent[0])
	}
}

func TestTextRunPresenter_Result(t *testing.T) {
	var sentMu sync.Mutex
	var sent []string
	deps := PresenterDeps{
		Send: func(platform notify.PlatformType, chatID, messageID, text string) error {
			sentMu.Lock()
			sent = append(sent, text)
			sentMu.Unlock()
			return nil
		},
	}
	p := newTextRunPresenter(deps, "standard")
	sess := &imSession{platform: "feishu", chatID: "oc1", lastMessageID: "om1"}
	rc := &RunContext{Session: sess}

	// result 事件（非 after_stream，无 segments）
	p.OnRunResult(rc, RunEvent{Type: RunEventResult, Text: `{"result":"final answer","success":true}`})

	sentMu.Lock()
	defer sentMu.Unlock()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if sent[0] != "final answer" {
		t.Errorf("sent = %q, want 'final answer'", sent[0])
	}
}

func TestTextRunPresenter_Error(t *testing.T) {
	var sentMu sync.Mutex
	var sent []string
	deps := PresenterDeps{
		Send: func(platform notify.PlatformType, chatID, messageID, text string) error {
			sentMu.Lock()
			sent = append(sent, text)
			sentMu.Unlock()
			return nil
		},
	}
	p := newTextRunPresenter(deps, "standard")
	sess := &imSession{platform: "feishu", chatID: "oc1", lastMessageID: "om1"}
	rc := &RunContext{Session: sess}

	p.OnRunError(rc, RunEvent{Type: RunEventError, Text: "tool failed"})

	sentMu.Lock()
	defer sentMu.Unlock()
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "❌ tool failed") {
		t.Errorf("sent = %v", sent)
	}
}

// --- FeishuRunPresenter 节流测试 ---

func TestFeishuRunPresenter_Throttle(t *testing.T) {
	var patchMu sync.Mutex
	var patchCount int
	var sendCardCount int

	deps := PresenterDeps{
		SendCard: func(msg *notify.Message, c *notify.SendConfig) (string, error) {
			patchMu.Lock()
			sendCardCount++
			patchMu.Unlock()
			return "om-managed", nil
		},
		PatchCard: func(messageID string, msg *notify.Message, c *notify.SendConfig) error {
			patchMu.Lock()
			patchCount++
			patchMu.Unlock()
			return nil
		},
	}
	p := newFeishuRunPresenter(deps)
	// 缩短节流间隔加速测试
	p.patchInterval = 50 * time.Millisecond

	sess := &imSession{platform: "feishu", chatID: "oc1", lastMessageID: "om1"}
	rc := &RunContext{Session: sess, RunID: "r1"}

	p.OnRunStart(rc)
	patchMu.Lock()
	if sendCardCount != 1 {
		t.Errorf("OnRunStart should send 1 card, got %d", sendCardCount)
	}
	patchMu.Unlock()

	// 连续发 10 个 delta（间隔 < 50ms），节流应合并
	for i := 0; i < 10; i++ {
		p.OnRunDelta(rc, RunEvent{Type: RunEventDelta, WriterID: "w1", Delta: "x"})
		time.Sleep(5 * time.Millisecond)
	}
	// 等节流窗口结束
	time.Sleep(80 * time.Millisecond)

	patchMu.Lock()
	finalPatchCount := patchCount
	patchMu.Unlock()
	// 10 个高频 delta 应被合并为远少于 10 次 patch（节流生效）
	if finalPatchCount >= 10 {
		t.Errorf("throttle should reduce patch count, got %d patches for 10 deltas", finalPatchCount)
	}
	if finalPatchCount == 0 {
		t.Errorf("expected at least 1 patch after deltas, got 0")
	}
}

func TestFeishuRunPresenter_Result(t *testing.T) {
	var patchMu sync.Mutex
	var lastPatchContent string
	var lastPatchTitle string
	var lastPatchElements []map[string]any
	var lastPatchButtons []notify.CardButton
	var lastPatchConfig map[string]any

	deps := PresenterDeps{
		SendCard: func(msg *notify.Message, c *notify.SendConfig) (string, error) {
			return "om-managed", nil
		},
		PatchCard: func(messageID string, msg *notify.Message, c *notify.SendConfig) error {
			patchMu.Lock()
			lastPatchTitle = msg.Card.Title
			lastPatchContent = msg.Card.Content
			lastPatchElements = msg.Card.Elements
			lastPatchButtons = msg.Card.Buttons
			lastPatchConfig = msg.Card.Config
			patchMu.Unlock()
			return nil
		},
	}
	p := newFeishuRunPresenter(deps)
	sess := &imSession{
		platform:            "feishu",
		chatID:              "oc1",
		lastMessageID:       "om1",
		currentModel:        "qwen3.6-plus-no-thinking",
		persistentSessionId: "im-session-1",
	}
	rc := &RunContext{Session: sess, RunID: "r1"}
	oldNow := now
	now = func() time.Time { return time.Date(2026, 7, 3, 15, 51, 10, 0, time.Local) }
	defer func() { now = oldNow }()

	p.OnRunStart(rc)
	p.OnRunResult(rc, RunEvent{Type: RunEventResult, Text: `{"result":"the answer","success":true}`})

	patchMu.Lock()
	defer patchMu.Unlock()
	if lastPatchTitle != "AI 响应 qwen3.6-plus-no-thinking 2026-07-03 15:51:10" {
		t.Errorf("title = %q", lastPatchTitle)
	}
	elementsJSON, _ := json.Marshal(lastPatchElements)
	if !strings.Contains(lastPatchContent+string(elementsJSON), "the answer") {
		t.Errorf("content/elements = %q / %s, want contains 'the answer'", lastPatchContent, elementsJSON)
	}
	if strings.Contains(string(elementsJSON), "select_static") || strings.Contains(string(elementsJSON), "回答评价") {
		t.Errorf("elements = %s, should not contain feedback selector", elementsJSON)
	}
	if strings.Contains(string(elementsJSON), `"tag":"note"`) {
		t.Errorf("elements = %s, Card JSON 2.0 should not contain note", elementsJSON)
	}
	if !strings.Contains(string(elementsJSON), "以上内容由 AI 生成，仅供参考") {
		t.Errorf("elements = %s, want AI generated notice", elementsJSON)
	}
	for _, forbidden := range []string{"状态", "会话", "im-session-1"} {
		if strings.Contains(string(elementsJSON), forbidden) {
			t.Errorf("elements = %s, should not contain footer detail %q", elementsJSON, forbidden)
		}
	}
	if strings.Contains(string(elementsJSON), "qwen3.6-plus-no-thinking") {
		t.Errorf("elements = %s, model name should be in title only", elementsJSON)
	}
	if lastPatchConfig["streaming_mode"] != false {
		t.Errorf("streaming_mode = %v, want false on final card", lastPatchConfig["streaming_mode"])
	}
	actions := map[string]notify.CardButton{}
	for _, btn := range lastPatchButtons {
		action, _ := btn.Value["action"].(string)
		actions[action] = btn
	}
	detailBtn, ok := actions["session_info"]
	if !ok {
		t.Fatalf("final card should include session_info action, got buttons=%v", lastPatchButtons)
	}
	if detailBtn.Value["session_id"] != sess.persistentSessionId {
		t.Fatalf("session_info session_id = %v, want %s", detailBtn.Value["session_id"], sess.persistentSessionId)
	}
	if detailBtn.Text != "📌 会话面板" {
		t.Fatalf("session info button text = %q", detailBtn.Text)
	}
}

// --- IMAction 测试 ---

func TestHandleAction_StopNoSession(t *testing.T) {
	e := New(Config{})
	// 无会话时 stop 应回执「无活跃会话」
	msg := &notify.InboundMessage{Platform: notify.PlatformFeishu, ChatID: "oc1", SenderID: "ou1"}
	// reply 走 sendRaw，无 bot 配置会失败但不 panic
	e.handleAction(IMAction{Type: ActionStop, Source: "command", Msg: msg})
	// 不 panic 即通过
}

// TestNextGroupTrigger 验证群聊触发策略循环切换。
func TestNextGroupTrigger(t *testing.T) {
	cases := []struct{ in, want string }{
		{"must_at", "allow_all"},
		{"allow_slash", "allow_all"},
		{"allow_all", "must_at"},
		{"", "allow_all"},
	}
	for _, c := range cases {
		if got := nextGroupTrigger(c.in); got != c.want {
			t.Errorf("nextGroupTrigger(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestShouldTriggerInGroup 验证群聊触发策略过滤。
func TestShouldTriggerInGroup(t *testing.T) {
	cases := []struct {
		name      string
		chatType  string
		strategy  string
		content   string
		mention   bool
		wantAllow bool
	}{
		{"private always allowed", "private", "must_at", "hello", false, true},
		{"group must_at slash blocked without mention", "group", "must_at", "/help", false, false},
		{"group must_at mention allowed", "group", "must_at", "hello", true, true},
		{"group must_at plain blocked", "group", "must_at", "hello", false, false},
		{"group allow_slash slash blocked without mention", "group", "allow_slash", "/help", false, false},
		{"group allow_slash plain blocked", "group", "allow_slash", "hello", false, false},
		{"group allow_all plain allowed", "group", "allow_all", "hello", false, true},
		{"topic treated as group", "topic", "allow_slash", "hello", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := New(Config{GroupTrigger: c.strategy})
			msg := &notify.InboundMessage{
				ChatType:   c.chatType,
				Text:       c.content,
				MentionBot: c.mention,
			}
			got := e.shouldTriggerInGroup(msg, c.content)
			if got != c.wantAllow {
				t.Errorf("shouldTriggerInGroup(strategy=%s, chatType=%s, content=%q, mention=%v) = %v, want %v",
					c.strategy, c.chatType, c.content, c.mention, got, c.wantAllow)
			}
		})
	}
}
