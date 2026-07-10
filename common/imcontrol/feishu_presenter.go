package imcontrol

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
)

// feishuRunPresenter 用一张 managed card 渲染一个 turn 的运行输出。
//
// 生命周期：OnRunStart 发卡片（带「停止」按钮）→ OnRunDelta 节流 patch 卡片
// 更新进度 → OnRunResult patch 为最终结果态（去掉停止按钮）→ OnRunError patch
// 为错误态。节流放本层（~500ms 一次，合并最新内容），底层 PatchCard 无状态。
type feishuRunPresenter struct {
	deps PresenterDeps

	// run 状态
	mu           sync.Mutex
	cardMsgID    string
	textFallback bool
	startedAt    time.Time
	titleModel   string
	body         strings.Builder // 当前卡片正文（累积的段文本）
	finished     []string        // 已完成的段文本（按顺序）

	// 节流
	patchInterval time.Duration
	lastPatch     time.Time
	pendingTimer  *time.Timer
	dirty         bool
}

func newFeishuRunPresenter(deps PresenterDeps) *feishuRunPresenter {
	return &feishuRunPresenter{
		deps:          deps,
		patchInterval: 500 * time.Millisecond,
	}
}

// OnRunStart 发 managed card 占位，存 message_id 供后续 patch。
func (p *feishuRunPresenter) OnRunStart(ctx *RunContext) {
	// P1-3: 完整 reset turn-local 状态，避免跨 turn 串扰
	p.mu.Lock()
	if p.pendingTimer != nil {
		p.pendingTimer.Stop()
		p.pendingTimer = nil
	}
	p.body.Reset()
	p.finished = nil
	p.dirty = false
	p.cardMsgID = "" // 清旧卡片 ID，本轮重新发
	p.textFallback = false
	p.startedAt = now()
	p.titleModel = displayModelName(ctx.Session.currentModel)
	p.mu.Unlock()

	stopValue := map[string]any{"action": "stop", "run_id": ctx.RunID}
	addCardSessionContext(stopValue, ctx)
	if p.deps.SignToken != nil {
		stopValue["token"] = p.deps.SignToken(CallbackSignInput{
			RunID: ctx.RunID, ChatID: ctx.Session.chatID,
			Action: "stop",
		})
	}
	card := cardMessage(p.buildRunCard(ctx, "_正在组织回答..._", true, []notify.CardButton{{Text: "⏹ 终止", Style: "danger", Value: stopValue}}))
	msgID, err := p.deps.SendCard(card, p.deps.Config)
	if err != nil {
		log.Warnf("feishu presenter: send managed card failed: %v", err)
		p.mu.Lock()
		p.textFallback = true
		p.mu.Unlock()
		if p.deps.Send != nil {
			_ = p.deps.Send(notify.PlatformType(ctx.Session.platform),
				ctx.Session.chatID, ctx.Session.lastMessageID,
				"🤖 正在执行…（卡片发送失败，将以文本回复）")
		}
		return
	}
	p.mu.Lock()
	p.cardMsgID = msgID
	p.textFallback = false
	p.mu.Unlock()
	ctx.CardMsgID = msgID
}

func (p *feishuRunPresenter) OnRunDelta(ctx *RunContext, ev RunEvent) {
	// 思考过程不进卡片正文（太啰嗦），只累积最终回复段
	if ev.IsReason || ev.WriterID == "" {
		return
	}
	p.mu.Lock()
	if ev.Delta != "" {
		p.body.WriteString(ev.Delta)
	}
	p.mu.Unlock()
	p.schedulePatch(ctx)
}

func (p *feishuRunPresenter) OnRunSegmentFinished(ctx *RunContext, ev RunEvent) {
	if ev.IsReason {
		return // 思考段不进卡片
	}
	// 段完成：把 body 累积的内容定稿追加到 finished，重置 body（下一段重新累积）
	p.mu.Lock()
	text := cleanIMText(p.body.String())
	p.body.Reset()
	if text != "" {
		p.finished = append(p.finished, text)
	}
	p.mu.Unlock()
	ctx.segmentsOutputted++
	p.patchNow(ctx) // 段完成立即 patch
}

func (p *feishuRunPresenter) OnRunResult(ctx *RunContext, ev RunEvent) {
	if ctx != nil && ctx.Session != nil {
		ctx.Session.clearPendingInteraction()
	}
	rawContent := strings.TrimSpace(ev.Text)
	if rawContent == "" {
		return
	}
	// 解析 result JSON 复刻 text presenter 的 after_stream 判定
	var resInfo struct {
		AfterStream bool   `json:"after_stream"`
		Result      string `json:"result"`
		Success     bool   `json:"success"`
	}
	var resultText string
	if isJSON := strings.HasPrefix(rawContent, "{"); isJSON {
		if err := json.Unmarshal([]byte(rawContent), &resInfo); err == nil {
			if resInfo.AfterStream && ctx.segmentsOutputted > 0 {
				// 已流式输出过，用已累积的段文本作最终正文，不再追加 result
				resultText = "" // 用 p.finished
			} else {
				resultText = cleanIMText(resInfo.Result)
			}
		}
	} else {
		resultText = cleanIMText(rawContent)
	}

	// 组装最终卡片：去掉停止按钮，正文=最终结果
	p.mu.Lock()
	var content string
	if resultText != "" {
		// result 优先覆盖流式段
		content = resultText
	} else if len(p.finished) > 0 {
		content = strings.Join(p.finished, "\n\n---\n\n")
	} else if p.body.Len() > 0 {
		content = p.body.String()
	} else {
		content = "_(已完成)_"
	}
	p.mu.Unlock()

	if p.isTextFallback() {
		p.sendTextFallback(ctx, "✅ 执行完成", content)
		return
	}
	p.patchCardFinal(ctx, content)
}

func (p *feishuRunPresenter) OnRunError(ctx *RunContext, ev RunEvent) {
	if ctx != nil && ctx.Session != nil {
		ctx.Session.clearPendingInteraction()
	}
	errMsg := ev.Text
	if ev.Err != nil {
		errMsg = ev.Err.Error()
	}
	if strings.TrimSpace(errMsg) == "" {
		return
	}
	if p.isTextFallback() {
		p.sendTextFallback(ctx, "❌ 执行出错", "```\n"+errMsg+"\n```")
		return
	}
	p.patchCardFinal(ctx, "```\n"+errMsg+"\n```")
}

func (p *feishuRunPresenter) OnRunInteraction(ctx *RunContext, req *IMInteractiveRequest) {
	if req == nil || ctx == nil || ctx.Session == nil {
		return
	}
	ctx.Session.setPendingInteraction(req)
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = "AI 需要你确认后继续。"
	}
	if req.Title != "" {
		content = "**" + req.Title + "**\n\n" + content
	}
	content = "⏸ **等待手动确认**\n\n" + content
	if p.isTextFallback() {
		p.sendTextFallback(ctx, "⏸ 等待手动确认", content)
		return
	}

	p.mu.Lock()
	msgID := p.cardMsgID
	if p.pendingTimer != nil {
		p.pendingTimer.Stop()
		p.pendingTimer = nil
	}
	p.dirty = false
	p.mu.Unlock()
	if msgID == "" || p.deps.PatchCard == nil {
		return
	}
	card := cardMessage(p.buildRunCard(ctx, content, true, p.reviewButtons(ctx, req)))
	if err := p.deps.PatchCard(msgID, card, p.deps.Config); err != nil {
		log.Warnf("feishu presenter: patch review card failed: %v", err)
	}
}

func (p *feishuRunPresenter) Flush(ctx *RunContext) {
	// 兜底：把未提交的 body 内容 patch 出去
	p.mu.Lock()
	if p.body.Len() > 0 {
		text := p.body.String()
		p.finished = append(p.finished, cleanIMText(text))
		p.body.Reset()
	}
	content := ""
	if len(p.finished) > 0 {
		content = strings.Join(p.finished, "\n\n---\n\n")
	}
	p.mu.Unlock()
	if content != "" {
		if p.isTextFallback() {
			p.sendTextFallback(ctx, "✅ 执行完成", content)
			return
		}
		p.patchCardFinal(ctx, content)
	}
}

func (p *feishuRunPresenter) isTextFallback() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.textFallback
}

func (p *feishuRunPresenter) sendTextFallback(ctx *RunContext, title, content string) {
	if p.deps.Send == nil || ctx == nil || ctx.Session == nil {
		return
	}
	text := strings.TrimSpace(title + "\n\n" + cleanIMText(content))
	if text == "" {
		return
	}
	if err := p.deps.Send(notify.PlatformType(ctx.Session.platform), ctx.Session.chatID, ctx.Session.lastMessageID, text); err != nil {
		log.Warnf("feishu presenter: send text fallback failed: %v", err)
	}
}

// --- 节流 patch ---

// schedulePatch 请求一次节流 patch。距上次 patch 不足 500ms 时延后到满 500ms，
// 期间多次请求合并为一次（dirty 标志）。
func (p *feishuRunPresenter) schedulePatch(ctx *RunContext) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cardMsgID == "" {
		return
	}
	p.dirty = true
	// 已有待触发的 patch，等它合并即可
	if p.pendingTimer != nil {
		return
	}
	elapsed := time.Since(p.lastPatch)
	if elapsed >= p.patchInterval {
		// 立刻 patch
		p.dirty = false
		p.doPatchLocked(ctx)
		return
	}
	// 延后到满 interval
	wait := p.patchInterval - elapsed
	p.pendingTimer = time.AfterFunc(wait, func() {
		p.mu.Lock()
		p.pendingTimer = nil
		if !p.dirty {
			p.mu.Unlock()
			return
		}
		p.dirty = false
		p.doPatchLocked(ctx)
		p.mu.Unlock()
	})
}

// patchNow 段完成时立即 patch（但仍受 lastPatch 约束避免过密）。
func (p *feishuRunPresenter) patchNow(ctx *RunContext) {
	p.schedulePatch(ctx)
}

// doPatchLocked 调用方已持锁。读取当前 body + finished 组装卡片并 patch。
func (p *feishuRunPresenter) doPatchLocked(ctx *RunContext) {
	if p.deps.PatchCard == nil || p.cardMsgID == "" {
		return
	}
	var content string
	if len(p.finished) > 0 {
		content = strings.Join(p.finished, "\n\n---\n\n")
		if p.body.Len() > 0 {
			content += "\n\n---\n\n" + p.body.String()
		}
	} else if p.body.Len() > 0 {
		content = p.body.String()
	} else {
		return // 无内容可 patch
	}
	card := cardMessage(p.buildRunCard(ctx, content, true, p.runButtons(ctx, "stop")))
	if err := p.deps.PatchCard(p.cardMsgID, card, p.deps.Config); err != nil {
		log.Debugf("feishu presenter: patch card failed: %v", err)
	}
	p.lastPatch = time.Now()
}

// patchCardFinal patch 为终态（去掉停止按钮，标题改为完成/出错，加新对话/继续按钮）。
func (p *feishuRunPresenter) patchCardFinal(ctx *RunContext, content string) {
	p.mu.Lock()
	msgID := p.cardMsgID
	// 取消待触发的节流 patch（终态直接覆盖）
	if p.pendingTimer != nil {
		p.pendingTimer.Stop()
		p.pendingTimer = nil
	}
	p.dirty = false
	p.mu.Unlock()
	if msgID == "" || p.deps.PatchCard == nil {
		return
	}
	card := cardMessage(p.buildRunCard(ctx, content, false, p.finalButtons(ctx)))
	if err := p.deps.PatchCard(msgID, card, p.deps.Config); err != nil {
		log.Warnf("feishu presenter: patch final card failed: %v", err)
	}
}

func (p *feishuRunPresenter) patchReviewDecision(ctx *RunContext, content string) bool {
	if ctx == nil || ctx.Session == nil {
		return false
	}
	content = strings.TrimSpace(content)
	if content == "" {
		content = "✅ 已确认，正在继续执行。"
	}
	p.mu.Lock()
	msgID := p.cardMsgID
	if p.pendingTimer != nil {
		p.pendingTimer.Stop()
		p.pendingTimer = nil
	}
	p.dirty = false
	p.mu.Unlock()
	if msgID == "" || p.deps.PatchCard == nil {
		return false
	}
	card := cardMessage(p.buildRunCard(ctx, content, true, p.runButtons(ctx, "stop")))
	if err := p.deps.PatchCard(msgID, card, p.deps.Config); err != nil {
		log.Debugf("feishu presenter: patch review decision failed: %v", err)
		return false
	}
	return true
}

// runButtons 构造运行中卡片的按钮（含停止）。
func (p *feishuRunPresenter) runButtons(ctx *RunContext, _ string) []notify.CardButton {
	stopValue := map[string]any{"action": "stop", "run_id": ctx.RunID}
	addCardSessionContext(stopValue, ctx)
	if p.deps.SignToken != nil {
		stopValue["token"] = p.deps.SignToken(CallbackSignInput{
			RunID: ctx.RunID, ChatID: ctx.Session.chatID,
			Action: "stop",
		})
	}
	return []notify.CardButton{
		{Text: "⏹ 终止", Style: "danger", Value: stopValue},
	}
}

func (p *feishuRunPresenter) reviewButtons(ctx *RunContext, req *IMInteractiveRequest) []notify.CardButton {
	continueValue := map[string]any{
		"action":         "review_decision",
		"run_id":         ctx.RunID,
		"interactive_id": req.ID,
		"suggestion":     "continue",
	}
	addCardSessionContext(continueValue, ctx)
	stopValue := map[string]any{
		"action":         "review_decision",
		"run_id":         ctx.RunID,
		"interactive_id": req.ID,
		"suggestion":     "stop",
	}
	addCardSessionContext(stopValue, ctx)
	if p.deps.SignToken != nil {
		continueValue["token"] = p.deps.SignToken(CallbackSignInput{
			RunID: ctx.RunID, ChatID: ctx.Session.chatID,
			Action: "review_decision",
		})
		stopValue["token"] = p.deps.SignToken(CallbackSignInput{
			RunID: ctx.RunID, ChatID: ctx.Session.chatID,
			Action: "review_decision",
		})
	}
	return []notify.CardButton{
		{Text: "确认继续", Style: "primary", Value: continueValue},
		{Text: "停止任务", Style: "danger", Value: stopValue},
	}
}

func (p *feishuRunPresenter) buildRunCard(ctx *RunContext, content string, streaming bool, buttons []notify.CardButton) *notify.Card {
	content = strings.TrimSpace(content)
	if content == "" {
		content = "_正在组织回答..._"
	}
	elements := []map[string]any{
		{"tag": "markdown", "content": content},
		{"tag": "hr"},
		cardDisclaimerElement(),
	}
	return &notify.Card{
		Title: p.cardTitle(ctx),
		Config: map[string]any{
			"streaming_mode":   streaming,
			"wide_screen_mode": true,
		},
		Elements: elements,
		Buttons:  buttons,
	}
}

func cardDisclaimerElement() map[string]any {
	return map[string]any{
		"tag":     "markdown",
		"content": "以上内容由 AI 生成，仅供参考",
	}
}

func (p *feishuRunPresenter) cardTitle(ctx *RunContext) string {
	startedAt := p.startedAt
	if startedAt.IsZero() {
		startedAt = now()
	}
	model := p.titleModel
	if model == "" {
		model = p.displayModelLabel(ctx)
	}
	return "AI 响应 " + model + " " + startedAt.Format("2006-01-02 15:04:05")
}

func (p *feishuRunPresenter) displayModelLabel(ctx *RunContext) string {
	if ctx != nil && ctx.Session != nil {
		if model := displayModelName(ctx.Session.currentModel); model != "" {
			return model
		}
	}
	return "默认模型"
}

func displayModelName(raw string) string {
	model := strings.TrimSpace(raw)
	if model == "" {
		return ""
	}
	model = strings.TrimPrefix(model, "memfit-")
	return strings.ReplaceAll(model, "`", "'")
}

// finalButtons 构造终态卡片的按钮。
// 每条回答的常驻按钮只保留回答级操作；回复颗粒度、引用回复等放到 /config 卡片里。
func (p *feishuRunPresenter) finalButtons(ctx *RunContext) []notify.CardButton {
	newValue := map[string]any{"action": "new"}
	sessionInfoValue := map[string]any{
		"action":     "session_info",
		"session_id": ctx.Session.persistentSessionId,
		"chat_title": ctx.Session.chatTitle,
	}
	configValue := map[string]any{"action": "config"}
	addCardSessionContext(newValue, ctx)
	addCardSessionContext(sessionInfoValue, ctx)
	addCardSessionContext(configValue, ctx)
	if p.deps.SignToken != nil {
		newValue["token"] = p.deps.SignToken(CallbackSignInput{
			ChatID: ctx.Session.chatID, Action: "new",
		})
		sessionInfoValue["token"] = p.deps.SignToken(CallbackSignInput{
			ChatID: ctx.Session.chatID, Action: "session_info",
		})
		configValue["token"] = p.deps.SignToken(CallbackSignInput{
			ChatID: ctx.Session.chatID, Action: "config",
		})
	}
	return []notify.CardButton{
		{Text: "💬 新对话", Style: "primary", Value: newValue},
		{Text: "📌 会话面板", Style: "default", Value: sessionInfoValue},
		{Text: "⚙️ 配置", Style: "default", Value: configValue},
	}
}

func addCardSessionContext(value map[string]any, ctx *RunContext) {
	if value == nil || ctx == nil || ctx.Session == nil {
		return
	}
	if ctx.Session.sessionKey != "" {
		value["session_key"] = ctx.Session.sessionKey
	}
	if ctx.Session.chatType != "" {
		value["chat_type"] = ctx.Session.chatType
	}
	if ctx.Session.threadID != "" {
		value["thread_id"] = ctx.Session.threadID
	}
}

func cardMessage(card *notify.Card) *notify.Message {
	return &notify.Message{
		Type: notify.MessageCard,
		Card: card,
	}
}
