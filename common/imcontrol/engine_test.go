package imcontrol

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/notify"
)

func setupIMEngineTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create profile db: %v", err)
	}
	consts.BindProfileDatabase(db, dbPath)
}

func TestMatchPrefix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"help", "help"},
		{"h", "help"},
		{"?", "help"},
		{"new", "new"},
		{"reset", "new"},
		{"ne", "new"}, // 前缀匹配
		{"stop", "stop"},
		{"cancel", "stop"},
		{"session", "session_info"},
		{"sessions", "session_info"},
		{"history", "session_info"},
		{"s", ""}, // 歧义: scan/session/stop/status 都以 s 开头
		{"scan", "scan"},
		{"sc", "scan"},
		{"commands", "commands"},
		{"cmds", "commands"},
		{"model", "model"},
		{"m", ""},   // 歧义: model/mitm/mode
		{"mo", ""},  // model/mode 都以 mo 开头 → 歧义
		{"mod", ""}, // model/mode 都以 mod 开头 → 歧义
		{"mode", "mode"},
		{"mitm", "mitm"},
		{"bogus", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := matchPrefix(c.input)
		if got != c.want {
			t.Errorf("matchPrefix(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSplitCommandArgs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{`/help`, []string{"/help"}},
		{`/scan 192.168.1.1`, []string{"/scan", "192.168.1.1"}},
		{`/scan 192.168.1.0/24`, []string{"/scan", "192.168.1.0/24"}},
		{`/shell ls -la /tmp`, []string{"/shell", "ls", "-la", "/tmp"}},
		{`/model "gpt-4 turbo"`, []string{"/model", "gpt-4 turbo"}},
		{`/run  端口扫描器`, []string{"/run", "端口扫描器"}},
	}
	for _, c := range cases {
		got := splitCommandArgs(c.input)
		if len(got) != len(c.want) {
			t.Errorf("splitCommandArgs(%q) = %v, want %v", c.input, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCommandArgs(%q)[%d] = %q, want %q", c.input, i, got[i], c.want[i])
			}
		}
	}
}

func TestImSessionKey(t *testing.T) {
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_xxx",
		SenderID: "ou_yyy",
	}
	key := imSessionKey(msg)
	want := "feishu:oc_xxx:ou_yyy"
	if key != want {
		t.Errorf("imSessionKey = %q, want %q", key, want)
	}
}

func TestImSessionKeySharesGroupContext(t *testing.T) {
	ownerInGroup := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_shared",
		SenderID: "ou_owner",
		ChatType: "group",
	}
	otherInGroup := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_shared",
		SenderID: "ou_other",
		ChatType: "group",
	}
	ownerPrivate := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "ou_owner",
		SenderID: "ou_owner",
		ChatType: "private",
	}

	if imSessionKey(ownerInGroup) != imSessionKey(otherInGroup) {
		t.Fatalf("same group chat should share context: owner=%q other=%q", imSessionKey(ownerInGroup), imSessionKey(otherInGroup))
	}
	if imSessionKey(ownerInGroup) == imSessionKey(ownerPrivate) {
		t.Fatalf("group and private sessions must be isolated: %q", imSessionKey(ownerInGroup))
	}
}

func TestNormalizeGroupMentionedContent(t *testing.T) {
	msg := &notify.InboundMessage{ChatType: "group", MentionBot: true}
	cases := []struct {
		in   string
		want string
	}{
		{"@机器人 /session 5", "/session 5"},
		{`<at user_id="ou_bot">机器人</at> /session`, "/session"},
		{"@机器人", ""},
		{"/session", "/session"},
	}
	for _, c := range cases {
		if got := normalizeGroupMentionedContent(msg, c.in); got != c.want {
			t.Fatalf("normalizeGroupMentionedContent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInboundDedupeMessageID(t *testing.T) {
	feishuMsg := &notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ReplyContext: " om_feishu_1 ",
	}
	if got := inboundDedupeMessageID(feishuMsg); got != "om_feishu_1" {
		t.Fatalf("feishu dedupe message id = %q, want om_feishu_1", got)
	}

	dingMsg := &notify.InboundMessage{
		Platform: notify.PlatformDingTalk,
		ReplyContext: map[string]any{
			"SessionWebhook": "https://example.invalid/session",
			"MsgID":          "dt_msg_1",
		},
	}
	if got := inboundDedupeMessageID(dingMsg); got != "dt_msg_1" {
		t.Fatalf("dingtalk dedupe message id = %q, want dt_msg_1", got)
	}

	unknownMsg := &notify.InboundMessage{
		Platform:     notify.PlatformType("other"),
		ReplyContext: "not-a-stable-message-id",
	}
	if got := inboundDedupeMessageID(unknownMsg); got != "" {
		t.Fatalf("unknown platform dedupe message id = %q, want empty", got)
	}
}

func TestEngineShouldSkipDuplicateInbound(t *testing.T) {
	setupIMEngineTestDB(t)
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_chat",
		SenderID:     "ou_sender",
		ReplyContext: "om_1",
	}

	if e.shouldSkipDuplicateInbound(msg, inboundDedupeMessageID(msg)) {
		t.Fatalf("first inbound should not be treated as duplicate")
	}
	if !e.shouldSkipDuplicateInbound(msg, inboundDedupeMessageID(msg)) {
		t.Fatalf("same platform/chat/message_id should be treated as duplicate")
	}

	otherMsg := *msg
	otherMsg.ReplyContext = "om_2"
	if e.shouldSkipDuplicateInbound(&otherMsg, inboundDedupeMessageID(&otherMsg)) {
		t.Fatalf("different message_id should not be treated as duplicate")
	}

	cardAction := *msg
	cardAction.IsCardAction = true
	if e.shouldSkipDuplicateInbound(&cardAction, inboundDedupeMessageID(&cardAction)) {
		t.Fatalf("card actions should not be deduped by normal inbound dedupe")
	}
}

func TestEngineShouldSkipOldInboundByEventAge(t *testing.T) {
	oldNow := now
	now = func() time.Time { return time.Date(2026, 7, 10, 12, 0, 0, 0, time.Local) }
	defer func() { now = oldNow }()

	e := New(Config{})
	e.startedAt = now().Add(-time.Hour)
	msg := &notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_chat",
		SenderID:     "ou_sender",
		ReplyContext: "om_old",
		EventTime:    now().Add(-10 * time.Minute),
	}

	if !e.shouldSkipStaleInbound(msg) {
		t.Fatalf("old inbound should be ignored by event age")
	}
}

func TestEngineDuplicateInboundPersistsAcrossRestart(t *testing.T) {
	setupIMEngineTestDB(t)

	msg := &notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_chat",
		SenderID:     "ou_sender",
		ReplyContext: "om_restart",
	}

	first := New(Config{})
	if first.shouldSkipDuplicateInbound(msg, inboundDedupeMessageID(msg)) {
		t.Fatalf("first inbound should not be treated as duplicate")
	}

	restarted := New(Config{})
	if !restarted.shouldSkipDuplicateInbound(msg, inboundDedupeMessageID(msg)) {
		t.Fatalf("same inbound should be deduped after engine restart")
	}
}

func TestEngineHandleCommandRecognition(t *testing.T) {
	// 不启动真实 Engine，只测试命令识别逻辑。
	// 构造一个不调用 Start 的 Engine，handleCommand 应能识别已知命令。
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformType("test"),
		ChatID:   "oc_test",
		SenderID: "ou_test",
		Text:     "",
	}

	// 已知命令应返回 true（只测纯本地处理的命令，不测会触发 dispatchToAgent 的 /scan 等）
	knownCmds := []string{"/help", "/new", "/stop", "/status", "/commands"}
	for _, raw := range knownCmds {
		// handleCommand 会调用 e.reply（需要 bot 配置），但我们只验证它返回 true。
		// 由于 e.reply 在无 bot 配置时会 log error 但不 panic，这里仍能跑通。
		got := e.handleCommand(msg, raw)
		if !got {
			t.Errorf("handleCommand(%q) = false, want true (should be recognized)", raw)
		}
	}

	// 未知命令应被本地兜底处理，避免误投递给 AI agent。
	got := e.handleCommand(msg, "/totally-bogus-command")
	if !got {
		t.Errorf("handleCommand(/totally-bogus-command) = false, want true")
	}
}

func TestSessionResetForNew(t *testing.T) {
	sess := &imSession{
		sessionKey:          "feishu:oc_x:ou_y",
		persistentSessionId: "feishu:oc_x:ou_y",
	}
	origId := sess.persistentSessionId
	sess.resetForNew()
	if sess.persistentSessionId == origId {
		t.Errorf("resetForNew should change persistentSessionId, got same: %s", origId)
	}
	if sess.started {
		t.Errorf("resetForNew should set started=false")
	}
}

// touchSession 新建会话时应从入站消息填充 IM 元数据字段。
func TestTouchSessionFillsIMMeta(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform:   notify.PlatformFeishu,
		ChatID:     "oc_chat1",
		SenderID:   "ou_sender1",
		SenderName: "张三",
		ChatType:   "group",
		ThreadID:   "om_thread1",
	}
	key := imSessionKey(msg)
	e.touchSession(key, msg)

	e.mu.Lock()
	sess := e.sessions[key]
	e.mu.Unlock()
	if sess == nil {
		t.Fatalf("session not created")
	}
	if sess.chatType != "group" {
		t.Errorf("chatType = %q, want group", sess.chatType)
	}
	if sess.senderName != "张三" {
		t.Errorf("senderName = %q, want 张三", sess.senderName)
	}
	if sess.threadID != "om_thread1" {
		t.Errorf("threadID = %q, want om_thread1", sess.threadID)
	}
	if sess.chatTitle == "" {
		t.Errorf("chatTitle should be generated, got empty")
	}
}

// buildIMSessionTitle 按 chatType 生成不同形式的兜底标题。
func TestBuildIMSessionTitle(t *testing.T) {
	cases := []struct {
		name string
		msg  *notify.InboundMessage
		want string
	}{
		{
			name: "private with senderName",
			msg:  &notify.InboundMessage{Platform: notify.PlatformFeishu, ChatType: "private", SenderName: "张三", SenderID: "ou_abc"},
			want: "私聊会话",
		},
		{
			name: "private no name does not expose id",
			msg:  &notify.InboundMessage{Platform: notify.PlatformFeishu, ChatType: "private", SenderID: "ou_abcdef123456"},
			want: "私聊会话",
		},
		{
			name: "group without group title does not expose sender name",
			msg:  &notify.InboundMessage{Platform: notify.PlatformFeishu, ChatType: "group", SenderID: "ou_s", SenderName: "李四"},
			want: "群聊会话",
		},
		{
			name: "topic without topic title does not expose thread id",
			msg:  &notify.InboundMessage{Platform: notify.PlatformFeishu, ChatType: "topic", ThreadID: "om_threadXYZ12345"},
			want: "话题会话",
		},
		{
			name: "dingtalk private",
			msg:  &notify.InboundMessage{Platform: notify.PlatformDingTalk, ChatType: "private", SenderName: "王五"},
			want: "私聊会话",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildIMSessionTitle(c.msg)
			if got != c.want {
				t.Errorf("buildIMSessionTitle = %q, want %q", got, c.want)
			}
		})
	}
}

func TestShortID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "?"},
		{"ou_abcdef1234567890", "abcdef123456"}, // 去前缀 + 截断到 12
		{"oc_short", "short"},
		{"om_t", "t"},
		{"plainid", "plainid"},
	}
	for _, c := range cases {
		if got := shortID(c.in); got != c.want {
			t.Errorf("shortID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
