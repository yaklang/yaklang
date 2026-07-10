package imcontrol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	reviewPolicyManual = "manual"
	reviewPolicyAI     = "ai"
	reviewPolicyYOLO   = "yolo"

	defaultAIReviewRiskControlScore = 0.5
	hotPatchTypeAgreePolicy         = "AgreePolicy"
)

func parseReviewPolicy(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "manual", "man", "human", "手动", "人工", "手动确认":
		return reviewPolicyManual, true
	case "ai", "auto-ai", "ai-auto", "智能", "ai判断", "ai 判断", "协同ai", "协同 ai":
		return reviewPolicyAI, true
	case "yolo", "auto", "all", "全自动", "自动", "托管yolo", "托管 yolo":
		return reviewPolicyYOLO, true
	}
	return "", false
}

func normalizeReviewPolicy(raw string) string {
	if policy, ok := parseReviewPolicy(raw); ok {
		return policy
	}
	return reviewPolicyYOLO
}

func normalizeAIReviewRiskControlScore(score float64) float64 {
	if score <= 0 {
		return defaultAIReviewRiskControlScore
	}
	if score > 1 {
		return 1
	}
	return score
}

func reviewPolicyLabel(policy string) string {
	switch normalizeReviewPolicy(policy) {
	case reviewPolicyManual:
		return "人工"
	case reviewPolicyAI:
		return "协同 AI"
	case reviewPolicyYOLO:
		return "托管 YOLO"
	}
	return policy
}

func reviewPolicyHint(policy string) string {
	switch normalizeReviewPolicy(policy) {
	case reviewPolicyManual:
		return "敏感动作会暂停并等待你在 IM 中确认。"
	case reviewPolicyAI:
		return "AI 参与风险判断，必要时转交人工。"
	case reviewPolicyYOLO:
		return "动作由 Agent 自动执行，仅适合受控环境。"
	}
	return ""
}

func (e *Engine) GetReviewConfig() (policy string, riskScore float64, disallowPrompt bool) {
	return e.GetReviewConfigForPlatform("")
}

func (e *Engine) GetReviewConfigForPlatform(platform string) (policy string, riskScore float64, disallowPrompt bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.configForPlatformLocked(platform)
	return cfg.ReviewPolicy, cfg.AIReviewRiskControlScore, cfg.DisallowRequireForUserPrompt
}

// UpdateReviewPolicy 热更新 AI Agent 执行审批策略。
func (e *Engine) UpdateReviewPolicy(rawPolicy string) {
	e.UpdateReviewPolicyForPlatform("", rawPolicy)
}

func (e *Engine) UpdateReviewPolicyForPlatform(platform, rawPolicy string) {
	policy, ok := parseReviewPolicy(rawPolicy)
	if !ok {
		return
	}

	e.mu.Lock()
	platform = normalizePlatformConfigKey(platform)
	if platform != "" {
		cfg := e.configForPlatformLocked(platform)
		cfg.Platform = platform
		cfg.ReviewPolicy = policy
		e.platformConfigs[platform] = cfg
	} else {
		e.reviewPolicy = policy
	}
	sessions := make([]*imSession, 0, len(e.sessions))
	for _, sess := range e.sessions {
		if sess == nil {
			continue
		}
		if platform != "" && normalizePlatformConfigKey(sess.platform) != platform {
			continue
		}
		sess.reviewPolicy = policy
		sessions = append(sessions, sess)
	}
	e.mu.Unlock()

	for _, sess := range sessions {
		if err := e.hotpatchSessionReviewPolicy(sess); err != nil {
			log.Warnf("im engine: hotpatch review policy for %s failed: %v", sess.sessionKey, err)
		}
	}
	if platform != "" {
		log.Infof("im engine: platform review policy updated platform=%s policy=%s", platform, policy)
	} else {
		log.Infof("im engine: review policy updated to %s", policy)
	}
}

func (e *Engine) reviewConfigForMessage(msg *notify.InboundMessage) (policy string, riskScore float64, disallowPrompt bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	platform := ""
	if msg != nil {
		platform = string(msg.Platform)
	}
	cfg := e.configForPlatformLocked(platform)
	policy = cfg.ReviewPolicy
	riskScore = cfg.AIReviewRiskControlScore
	disallowPrompt = cfg.DisallowRequireForUserPrompt
	if msg == nil {
		return
	}
	if sess := e.sessions[imSessionKey(msg)]; sess != nil {
		policy = sess.reviewPolicy
		riskScore = sess.aiReviewRiskControlScore
		disallowPrompt = sess.disallowRequireForUserPrompt
	}
	return
}

func (e *Engine) setReviewPolicyForMessage(msg *notify.InboundMessage, rawPolicy string) (replyText string, shouldPatch bool) {
	if msg == nil {
		return "❌ 执行审批设置失败：消息为空", false
	}
	policy, ok := parseReviewPolicy(rawPolicy)
	if !ok {
		return "❌ 无效执行审批策略，可选: manual / ai / yolo", false
	}
	if ok, reason := e.canSetReviewPolicy(msg, policy); !ok {
		return "❌ 无法切换执行审批：" + reason, false
	}

	sessionKey := imSessionKey(msg)
	e.touchSession(sessionKey, msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	if sess != nil {
		sess.reviewPolicy = policy
	}
	e.mu.Unlock()

	if err := e.hotpatchSessionReviewPolicy(sess); err != nil {
		log.Warnf("im engine: hotpatch review policy for %s failed: %v", sessionKey, err)
		return fmt.Sprintf("⚠️ 执行审批已切换为 %s，但同步到当前运行中的 Agent 失败：%v\n\n下一轮消息会使用新策略。", reviewPolicyLabel(policy), err), true
	}
	return fmt.Sprintf("✅ 执行审批已切换为 %s", reviewPolicyLabel(policy)), true
}

func (e *Engine) canSetReviewPolicy(msg *notify.InboundMessage, policy string) (bool, string) {
	if policy != reviewPolicyYOLO || msg == nil {
		return true, ""
	}
	bot, err := credential.GetBotConfig(string(msg.Platform))
	if err != nil || bot == nil {
		return true, ""
	}
	if strings.TrimSpace(bot.OwnerID) != "" && msg.SenderID != bot.OwnerID {
		return false, "托管 YOLO 模式仅 bot 所有者可开启"
	}
	return true, ""
}

func (e *Engine) hotpatchSessionReviewPolicy(sess *imSession) error {
	if sess == nil {
		return nil
	}
	policy := sess.reviewPolicy
	riskScore := sess.aiReviewRiskControlScore
	disallowPrompt := sess.disallowRequireForUserPrompt

	sess.streamMu.Lock()
	stream := sess.stream
	started := sess.started
	sess.streamMu.Unlock()
	if !started || stream == nil {
		return nil
	}
	return stream.Send(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     hotPatchTypeAgreePolicy,
		Params: &ypb.AIStartParams{
			ReviewPolicy:                 policy,
			AIReviewRiskControlScore:     riskScore,
			DisallowRequireForUserPrompt: disallowPrompt,
		},
	})
}

func (e *Engine) cmdReviewDecision(msg *notify.InboundMessage) {
	if msg == nil {
		return
	}
	suggestion := actionValueString(msg.ActionValue, "suggestion")
	if suggestion == "" {
		suggestion = "continue"
	}

	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		e.reply(msg, "❌ 确认已失效：当前 AI 会话已结束或引擎已重启，请重新发起任务。")
		return
	}

	switch strings.ToLower(strings.TrimSpace(suggestion)) {
	case "stop", "cancel":
		sess.clearPendingInteraction()
		e.cmdStop(msg)
		e.stopRunCard(msg)
		return
	}

	interactiveID := actionValueString(msg.ActionValue, "interactive_id")
	if interactiveID == "" {
		interactiveID = actionValueString(msg.ActionValue, "id")
	}
	if interactiveID == "" {
		if pendingID, ok := sess.pendingInteraction(); ok {
			interactiveID = pendingID
		}
	}
	if interactiveID == "" {
		e.reply(msg, "❌ 确认失败：缺少 interactive_id")
		return
	}

	if err := e.sendInteractiveResponse(sess, interactiveID, map[string]any{"suggestion": suggestion}); err != nil {
		e.reply(msg, fmt.Sprintf("❌ 确认失败：%v", err))
		return
	}
	sess.clearPendingInteraction()
	if !e.patchReviewDecisionCard(msg, "✅ 已确认，正在继续执行。") {
		e.reply(msg, "✅ 已确认，AI 将继续执行。")
	}
}

func (e *Engine) cmdReviewCommandDecision(msg *notify.InboundMessage, suggestion string) {
	if msg == nil {
		return
	}
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		e.reply(msg, "当前没有等待确认的操作。")
		return
	}
	interactiveID, ok := sess.pendingInteraction()
	if !ok {
		e.reply(msg, "当前没有等待确认的操作。")
		return
	}
	actionMsg := *msg
	actionMsg.ActionValue = map[string]any{
		"interactive_id": interactiveID,
		"suggestion":     suggestion,
	}
	e.cmdReviewDecision(&actionMsg)
}

func (e *Engine) sendInteractiveResponse(sess *imSession, interactiveID string, payload map[string]any) error {
	if sess == nil {
		return fmt.Errorf("session is nil")
	}
	interactiveID = strings.TrimSpace(interactiveID)
	if interactiveID == "" {
		return fmt.Errorf("interactive_id is empty")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal interactive payload: %w", err)
	}

	sess.streamMu.Lock()
	stream := sess.stream
	started := sess.started
	sess.streamMu.Unlock()
	if !started || stream == nil {
		return fmt.Errorf("确认已失效：当前 AI 会话已结束或引擎已重启，请重新发起任务")
	}
	return stream.Send(&ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        interactiveID,
		InteractiveJSONInput: string(raw),
	})
}

func (e *Engine) patchReviewDecisionCard(msg *notify.InboundMessage, content string) bool {
	if msg == nil {
		return false
	}
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		return false
	}
	sess.presenterMu.RLock()
	presenter := sess.presenter
	runCtx := sess.curRunCtx
	sess.presenterMu.RUnlock()
	if fp, ok := presenter.(*feishuRunPresenter); ok && runCtx != nil {
		return fp.patchReviewDecision(runCtx, content)
	}
	return false
}

func actionValueString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func isIMInteractiveEventType(eventType string) bool {
	switch eventType {
	case "task_review_require",
		"plan_review_require",
		"tool_use_review_require",
		"exec_aiforge_review_require":
		return true
	}
	return false
}

func shouldSuppressReviewInteraction(sess *imSession, req *IMInteractiveRequest) bool {
	if sess == nil || req == nil {
		return false
	}
	return sess.reviewPolicy == reviewPolicyYOLO && isIMInteractiveEventType(req.Type)
}

func parseIMInteractiveRequest(ev *ypb.AIOutputEvent) *IMInteractiveRequest {
	if ev == nil || !isIMInteractiveEventType(ev.GetType()) {
		return nil
	}
	var payload map[string]any
	_ = json.Unmarshal(ev.GetContent(), &payload)
	id := stringFromPayload(payload, "id")
	if id == "" {
		return nil
	}
	title, content := interactiveTitleAndContent(ev.GetType(), payload)
	return &IMInteractiveRequest{
		ID:      id,
		Type:    ev.GetType(),
		Title:   title,
		Content: content,
	}
}

func interactiveTitleAndContent(eventType string, payload map[string]any) (string, string) {
	switch eventType {
	case "plan_review_require":
		return "计划确认", "AI 已生成执行计划，请确认是否继续。"
	case "task_review_require":
		if summary := stringFromPayload(payload, "short_summary"); summary != "" {
			return "任务确认", "当前任务需要人工确认。\n\n" + summary
		}
		return "任务确认", "当前任务需要人工确认，请确认是否继续。"
	case "tool_use_review_require":
		toolName := stringFromPayload(payload, "tool")
		if toolName == "" {
			toolName = stringFromPayload(payload, "tool_name")
		}
		if toolName != "" {
			return "工具执行确认", "AI 请求使用工具：" + toolName + "\n\n请确认是否继续。"
		}
		return "工具执行确认", "AI 请求使用工具，请确认是否继续。"
	case "exec_aiforge_review_require":
		return "任务执行确认", "AI 请求执行 Forge/任务，请确认是否继续。"
	}
	return "需要确认", "AI 需要你确认后继续。"
}

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
