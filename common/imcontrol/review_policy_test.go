package imcontrol

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type fakeReActStream struct {
	grpc.ClientStream
	sent []*ypb.AIInputEvent
	recv []*ypb.AIOutputEvent
}

func (f *fakeReActStream) Send(event *ypb.AIInputEvent) error {
	f.sent = append(f.sent, event)
	return nil
}

func (f *fakeReActStream) Recv() (*ypb.AIOutputEvent, error) {
	if len(f.recv) > 0 {
		ev := f.recv[0]
		f.recv = f.recv[1:]
		return ev, nil
	}
	return nil, io.EOF
}

type fakeRunPresenter struct {
	interactions int
}

func (f *fakeRunPresenter) OnRunStart(ctx *RunContext)                        {}
func (f *fakeRunPresenter) OnRunDelta(ctx *RunContext, ev RunEvent)           {}
func (f *fakeRunPresenter) OnRunSegmentFinished(ctx *RunContext, ev RunEvent) {}
func (f *fakeRunPresenter) OnRunResult(ctx *RunContext, ev RunEvent)          {}
func (f *fakeRunPresenter) OnRunError(ctx *RunContext, ev RunEvent)           {}
func (f *fakeRunPresenter) Flush(ctx *RunContext)                             {}
func (f *fakeRunPresenter) OnRunInteraction(ctx *RunContext, req *IMInteractiveRequest) {
	f.interactions++
}

func TestReviewPolicyDefaultsAppliedToSessionAndStartParams(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_review",
		SenderID: "ou_review",
		ChatType: "private",
	}
	key := imSessionKey(msg)
	e.touchSession(key, msg)

	e.mu.Lock()
	sess := e.sessions[key]
	e.mu.Unlock()
	if sess == nil {
		t.Fatal("session not created")
	}
	if sess.reviewPolicy != "yolo" {
		t.Fatalf("reviewPolicy = %q, want yolo", sess.reviewPolicy)
	}
	if sess.aiReviewRiskControlScore != 0.5 {
		t.Fatalf("aiReviewRiskControlScore = %v, want 0.5", sess.aiReviewRiskControlScore)
	}
	if sess.disallowRequireForUserPrompt {
		t.Fatal("IM sessions should allow agent user prompts by default")
	}

	params := e.buildStartParams(sess)
	if params.GetReviewPolicy() != "yolo" {
		t.Fatalf("start ReviewPolicy = %q, want yolo", params.GetReviewPolicy())
	}
	if params.GetAIReviewRiskControlScore() != 0.5 {
		t.Fatalf("start AIReviewRiskControlScore = %v, want 0.5", params.GetAIReviewRiskControlScore())
	}
	if params.GetDisallowRequireForUserPrompt() {
		t.Fatal("start params should allow user prompt for IM")
	}
}

func TestReviewCommandRecognitionAndActionWhitelist(t *testing.T) {
	if got := matchPrefix("review"); got != "review" {
		t.Fatalf("matchPrefix(review) = %q, want review", got)
	}
	if got := matchPrefix("rev"); got != "review" {
		t.Fatalf("matchPrefix(rev) = %q, want review", got)
	}
	if !knownIMAction(ActionReview) {
		t.Fatal("review action should be known")
	}
}

func TestShouldSuppressReviewInteractionForYOLO(t *testing.T) {
	yoloSession := &imSession{reviewPolicy: "yolo"}
	manualSession := &imSession{reviewPolicy: "manual"}
	aiSession := &imSession{reviewPolicy: "ai"}

	reviewReq := &IMInteractiveRequest{Type: "tool_use_review_require"}
	promptReq := &IMInteractiveRequest{Type: "require_user_interactive"}

	if !shouldSuppressReviewInteraction(yoloSession, reviewReq) {
		t.Fatal("yolo tool review interaction should be suppressed")
	}
	if shouldSuppressReviewInteraction(manualSession, reviewReq) {
		t.Fatal("manual tool review interaction should still be shown")
	}
	if shouldSuppressReviewInteraction(aiSession, reviewReq) {
		t.Fatal("ai tool review interaction should still be shown")
	}
	if shouldSuppressReviewInteraction(yoloSession, promptReq) {
		t.Fatal("yolo should not suppress non-review user interaction")
	}
}

func TestReadAgentOutputSuppressesYOLOReviewInteraction(t *testing.T) {
	e := New(Config{})
	stream := &fakeReActStream{
		recv: []*ypb.AIOutputEvent{
			{
				Type:    "tool_use_review_require",
				Content: []byte(`{"id":"interactive-1","tool":"do_http_request"}`),
			},
		},
	}
	presenter := &fakeRunPresenter{}
	sess := &imSession{
		sessionKey:   "dingtalk:chat:user",
		platform:     string(notify.PlatformDingTalk),
		chatID:       "chat",
		senderID:     "user",
		reviewPolicy: "yolo",
		started:      true,
		stream:       stream,
		presenter:    presenter,
		curRunCtx:    &RunContext{RunID: "run-yolo"},
	}
	sess.curRunCtx.Session = sess

	e.readAgentOutput(sess, stream)

	if presenter.interactions != 0 {
		t.Fatalf("yolo review interaction should not be presented, got %d", presenter.interactions)
	}
}

func TestBuildConfigCardIncludesReviewPolicySelect(t *testing.T) {
	e := New(Config{ReplyQuote: true, ReplyGranularity: "standard", GroupTrigger: "must_at", ReviewPolicy: "ai"})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_review",
		SenderID: "ou_review",
	}
	card := e.buildConfigCard(msg)
	if card == nil {
		t.Fatal("card is nil")
	}
	rawBytes, _ := json.Marshal(card.Elements)
	raw := string(rawBytes)
	for _, want := range []string{
		"执行审批",
		"人工",
		"托管 YOLO",
		"协同 AI",
		`"sub":"set_review_policy"`,
		`"value":"manual"`,
		`"value":"ai"`,
		`"value":"yolo"`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("config card missing %q: %s", want, raw)
		}
	}
	if strings.Count(raw, `"tag":"select_static"`) != 2 {
		t.Fatalf("reply mode and review policy should use two select_static components: %s", raw)
	}
}

func TestStatusAndSessionInfoShowReviewPolicy(t *testing.T) {
	e := New(Config{ReviewPolicy: "ai", ReplyGranularity: "summary", GroupTrigger: "must_at"})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_review",
		SenderID: "ou_review",
		ChatType: "group",
	}
	e.touchSession(imSessionKey(msg), msg)

	status := e.buildStatusText(msg)
	if !strings.Contains(status, "执行审批") || !strings.Contains(status, "协同 AI") {
		t.Fatalf("status text should show review policy, got: %s", status)
	}
	infoCard := e.buildSessionInfoCard(msg)
	rawBytes, _ := json.Marshal(infoCard.Elements)
	raw := string(rawBytes)
	if !strings.Contains(raw, "执行审批") || !strings.Contains(raw, "协同 AI") {
		t.Fatalf("session info card should show review policy, got: %s", raw)
	}
}

func TestApplyConfigCardActionSetReviewPolicyHotpatchesRunningSession(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_review",
		SenderID: "ou_review",
		ActionValue: map[string]any{
			"sub":    "set_review_policy",
			"policy": "ai",
		},
	}
	key := imSessionKey(msg)
	e.touchSession(key, msg)

	stream := &fakeReActStream{}
	e.mu.Lock()
	sess := e.sessions[key]
	e.mu.Unlock()
	sess.streamMu.Lock()
	sess.stream = stream
	sess.started = true
	sess.streamMu.Unlock()

	reply, shouldPatch := e.applyConfigCardAction(msg)
	if !shouldPatch {
		t.Fatal("review policy change should patch config card")
	}
	if !strings.Contains(reply, "协同 AI") {
		t.Fatalf("reply = %q, want 协同 AI", reply)
	}
	if sess.reviewPolicy != "ai" {
		t.Fatalf("session reviewPolicy = %q, want ai", sess.reviewPolicy)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("hotpatch send count = %d, want 1", len(stream.sent))
	}
	hotpatch := stream.sent[0]
	if !hotpatch.GetIsConfigHotpatch() || hotpatch.GetHotpatchType() != "AgreePolicy" {
		t.Fatalf("unexpected hotpatch event: %+v", hotpatch)
	}
	if hotpatch.GetParams().GetReviewPolicy() != "ai" {
		t.Fatalf("hotpatch ReviewPolicy = %q, want ai", hotpatch.GetParams().GetReviewPolicy())
	}
}

func TestApplyConfigCardActionRejectsInvalidReviewPolicy(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		ActionValue: map[string]any{
			"sub":    "set_review_policy",
			"policy": "bad",
		},
	}
	reply, shouldPatch := e.applyConfigCardAction(msg)
	if shouldPatch {
		t.Fatal("invalid review policy should not patch config card")
	}
	if !strings.Contains(reply, "无效执行审批策略") {
		t.Fatalf("reply = %q, want invalid review policy error", reply)
	}
}

func TestFeishuRunPresenterReviewInteractionPatchesManagedCard(t *testing.T) {
	var patched *notify.Message
	signedReviewDecision := false
	p := newFeishuRunPresenter(PresenterDeps{
		SendCard: func(msg *notify.Message, cfg *notify.SendConfig) (string, error) {
			return "om_run", nil
		},
		PatchCard: func(messageID string, msg *notify.Message, cfg *notify.SendConfig) error {
			if messageID != "om_run" {
				t.Fatalf("messageID = %q, want om_run", messageID)
			}
			patched = msg
			return nil
		},
		SignToken: func(input CallbackSignInput) string {
			switch input.Action {
			case "stop":
			case "review_decision":
				signedReviewDecision = true
			default:
				t.Fatalf("unexpected signed action = %q", input.Action)
			}
			return "signed-token"
		},
	})
	ctx := &RunContext{
		Session: &imSession{
			platform:     string(notify.PlatformFeishu),
			sessionKey:   "feishu:oc_review",
			chatID:       "oc_review",
			senderID:     "ou_review",
			chatType:     "group",
			currentModel: "qwen-test",
		},
		RunID: "run-review",
	}
	p.OnRunStart(ctx)
	p.OnRunInteraction(ctx, &IMInteractiveRequest{
		ID:      "interactive-1",
		Type:    "tool_use_review_require",
		Title:   "工具执行确认",
		Content: "AI 请求使用工具，请确认是否继续。",
	})

	if patched == nil || patched.Card == nil {
		t.Fatal("expected review interaction to patch managed card")
	}
	if !signedReviewDecision {
		t.Fatal("review decision buttons should be signed")
	}
	rawBytes, _ := json.Marshal(patched.Card)
	raw := string(rawBytes)
	for _, want := range []string{
		"等待手动确认",
		"AI 请求使用工具",
		"确认继续",
		"停止任务",
		`"action":"review_decision"`,
		`"interactive_id":"interactive-1"`,
		`"suggestion":"continue"`,
		`"suggestion":"stop"`,
		`"session_key":"feishu:oc_review"`,
		`"chat_type":"group"`,
		"signed-token",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("review card missing %q: %s", want, raw)
		}
	}
}

func TestReviewDecisionCardActionUsesSessionKeyForGroupSession(t *testing.T) {
	e := New(Config{})
	originalMsg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_review",
		SenderID: "ou_original",
		ChatType: "group",
	}
	key := imSessionKey(originalMsg)
	if key != "feishu:oc_review" {
		t.Fatalf("group session key = %q, want feishu:oc_review", key)
	}
	e.touchSession(key, originalMsg)

	stream := &fakeReActStream{}
	e.mu.Lock()
	sess := e.sessions[key]
	e.mu.Unlock()
	sess.streamMu.Lock()
	sess.stream = stream
	sess.started = true
	sess.streamMu.Unlock()

	cardActionMsg := &notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       "oc_review",
		SenderID:     "ou_operator",
		IsCardAction: true,
		ActionValue: map[string]any{
			"session_key":    key,
			"interactive_id": "interactive-1",
			"suggestion":     "continue",
		},
	}
	e.cmdReviewDecision(cardActionMsg)

	if len(stream.sent) != 1 {
		t.Fatalf("interactive send count = %d, want 1", len(stream.sent))
	}
	if got := stream.sent[0].GetInteractiveId(); got != "interactive-1" {
		t.Fatalf("InteractiveId = %q, want interactive-1", got)
	}
	wrongKey := "feishu:oc_review:ou_operator"
	e.mu.Lock()
	wrongSess := e.sessions[wrongKey]
	e.mu.Unlock()
	if wrongSess != nil {
		t.Fatalf("card action should not create operator-scoped group session %q", wrongKey)
	}
}

func TestReviewDecisionSendsInteractiveInput(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformFeishu,
		ChatID:   "oc_review",
		SenderID: "ou_review",
		ActionValue: map[string]any{
			"interactive_id": "interactive-1",
			"suggestion":     "continue",
		},
	}
	key := imSessionKey(msg)
	e.touchSession(key, msg)

	stream := &fakeReActStream{}
	e.mu.Lock()
	sess := e.sessions[key]
	e.mu.Unlock()
	sess.streamMu.Lock()
	sess.stream = stream
	sess.started = true
	sess.streamMu.Unlock()

	e.cmdReviewDecision(msg)

	if len(stream.sent) != 1 {
		t.Fatalf("interactive send count = %d, want 1", len(stream.sent))
	}
	got := stream.sent[0]
	if !got.GetIsInteractiveMessage() {
		t.Fatalf("event should be interactive input: %+v", got)
	}
	if got.GetInteractiveId() != "interactive-1" {
		t.Fatalf("InteractiveId = %q, want interactive-1", got.GetInteractiveId())
	}
	if !strings.Contains(got.GetInteractiveJSONInput(), `"suggestion":"continue"`) {
		t.Fatalf("InteractiveJSONInput = %q, want continue suggestion", got.GetInteractiveJSONInput())
	}
}

func TestReviewConfirmCommandUsesPendingInteraction(t *testing.T) {
	e := New(Config{})
	msg := &notify.InboundMessage{
		Platform: notify.PlatformDingTalk,
		ChatID:   "cid_review",
		SenderID: "ou_review",
	}
	key := imSessionKey(msg)
	e.touchSession(key, msg)

	stream := &fakeReActStream{}
	e.mu.Lock()
	sess := e.sessions[key]
	e.mu.Unlock()
	sess.streamMu.Lock()
	sess.stream = stream
	sess.started = true
	sess.streamMu.Unlock()
	sess.setPendingInteraction(&IMInteractiveRequest{
		ID:      "interactive-1",
		Title:   "工具执行确认",
		Content: "AI 请求使用工具：ls",
	})

	if !e.handleCommand(msg, "/yes") {
		t.Fatal("/yes should be handled as a review confirmation command")
	}

	if len(stream.sent) != 1 {
		t.Fatalf("interactive send count = %d, want 1", len(stream.sent))
	}
	got := stream.sent[0]
	if !got.GetIsInteractiveMessage() {
		t.Fatalf("event should be interactive input: %+v", got)
	}
	if got.GetInteractiveId() != "interactive-1" {
		t.Fatalf("InteractiveId = %q, want interactive-1", got.GetInteractiveId())
	}
	if !strings.Contains(got.GetInteractiveJSONInput(), `"suggestion":"continue"`) {
		t.Fatalf("InteractiveJSONInput = %q, want continue suggestion", got.GetInteractiveJSONInput())
	}
	if _, ok := sess.pendingInteraction(); ok {
		t.Fatal("pending interaction should be cleared after /yes succeeds")
	}
}
