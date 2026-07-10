package imcontrol

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
)

// textRunPresenter 保留现有「每段一条文本消息」的渲染行为，用于不支持卡片更新的平台
// （钉钉等）。零行为变更：OnRunDelta 累积，OnRunSegmentFinished 发一条文本，
// OnRunResult 发结果，OnRunError 发 ❌。完全复刻原 readAgentOutput 的 sendRaw 逻辑。
type textRunPresenter struct {
	deps PresenterDeps
	// segments 按 event_writer_id 隔离每个流式段，对齐原 readAgentOutput 的 segments map。
	segments map[string]*streamSegment
	// thoughtBuf 累积思考过程（detailed 模式），stream-finished(is_reason) 时一次性发送。
	thoughtBuf strings.Builder
	// replyGranularity 控制是否展示思考过程。
	replyGranularity string
}

func newTextRunPresenter(deps PresenterDeps, granularity string) *textRunPresenter {
	return &textRunPresenter{
		deps:             deps,
		segments:         map[string]*streamSegment{},
		replyGranularity: granularity,
	}
}

func (p *textRunPresenter) OnRunStart(ctx *RunContext) {} // 文本路径无占位

func (p *textRunPresenter) OnRunDelta(ctx *RunContext, ev RunEvent) {
	if ev.IsReason {
		// 思考过程：仅 detailed 模式累积
		if p.replyGranularity != "detailed" {
			return
		}
		if ev.Delta != "" {
			p.thoughtBuf.WriteString(ev.Delta)
		}
		return
	}
	if ev.WriterID == "" {
		return
	}
	seg := p.getOrCreateSegment(ev.WriterID, ev.NodeID)
	if ev.Delta != "" {
		seg.buf.WriteString(ev.Delta)
	}
}

func (p *textRunPresenter) OnRunSegmentFinished(ctx *RunContext, ev RunEvent) {
	// 思考段：先 flush thought，再 flush 该段（与原逻辑顺序一致）
	if ev.IsReason {
		p.flushThought(ctx)
		return
	}
	p.flushThought(ctx)
	p.flushSegment(ctx, ev.WriterID)
}

func (p *textRunPresenter) OnRunResult(ctx *RunContext, ev RunEvent) {
	if ctx != nil && ctx.Session != nil {
		ctx.Session.clearPendingInteraction()
	}
	// result 事件的 Text 是原始 Content（JSON 字符串），需解析 after_stream/result。
	// readAgentOutput 在调这里之前已把 raw content 透传，这里复刻原解析逻辑。
	rawContent := strings.TrimSpace(ev.Text)
	if rawContent == "" {
		return
	}
	var resInfo struct {
		AfterStream bool   `json:"after_stream"`
		Result      string `json:"result"`
		Success     bool   `json:"success"`
	}
	if json.Unmarshal([]byte(rawContent), &resInfo) == nil {
		if resInfo.AfterStream && ctx.segmentsOutputted > 0 {
			return // 已流式输出过，跳过避免重复
		}
		if strings.TrimSpace(resInfo.Result) != "" {
			p.sendText(ctx, cleanIMText(resInfo.Result))
		}
	} else {
		// 非 JSON 格式的 result，直接发原文
		p.sendText(ctx, cleanIMText(rawContent))
	}
}

func (p *textRunPresenter) OnRunError(ctx *RunContext, ev RunEvent) {
	if ctx != nil && ctx.Session != nil {
		ctx.Session.clearPendingInteraction()
	}
	if ev.Err != nil {
		p.sendText(ctx, "❌ "+ev.Err.Error())
		return
	}
	// fail_* 事件：Text 是 content 原文
	if strings.TrimSpace(ev.Text) != "" {
		p.sendText(ctx, "❌ "+ev.Text)
	}
}

func (p *textRunPresenter) OnRunInteraction(ctx *RunContext, req *IMInteractiveRequest) {
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
	p.sendText(ctx, "## 等待手动确认\n\n"+content+"\n\n**操作**\n- 发送 `/yes` 继续执行\n- 发送 `/no` 拒绝并停止当前任务\n- 发送 `/stop` 中止当前任务")
}

func (p *textRunPresenter) Flush(ctx *RunContext) {
	// 兜底：流结束时把所有未 flush 的段发出去
	for writerID := range p.segments {
		p.flushSegment(ctx, writerID)
	}
	p.flushThought(ctx)
}

// --- 内部：复刻原 readAgentOutput 的 flush 逻辑 ---

func (p *textRunPresenter) getOrCreateSegment(writerID, nodeID string) *streamSegment {
	seg, ok := p.segments[writerID]
	if !ok {
		seg = &streamSegment{nodeID: nodeID}
		p.segments[writerID] = seg
	}
	return seg
}

func (p *textRunPresenter) flushSegment(ctx *RunContext, writerID string) {
	seg, ok := p.segments[writerID]
	if !ok {
		return
	}
	text := cleanIMText(seg.buf.String())
	delete(p.segments, writerID)
	if text == "" {
		return
	}
	p.sendText(ctx, text)
	ctx.segmentsOutputted++
}

func (p *textRunPresenter) flushThought(ctx *RunContext) {
	text := strings.TrimSpace(p.thoughtBuf.String())
	p.thoughtBuf.Reset()
	if text == "" {
		return
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return // 纯 JSON，不发给用户
	}
	if strings.Contains(text, "<|FINAL_ANSWER") || strings.Contains(text, "<|AI_CACHE") {
		return
	}
	p.sendText(ctx, "💭 "+text)
}

func (p *textRunPresenter) sendText(ctx *RunContext, text string) {
	if p.deps.Send == nil {
		log.Warnf("im engine: text presenter Send is nil, drop text")
		return
	}
	if err := p.deps.Send(notify.PlatformType(ctx.Session.platform), ctx.Session.chatID, ctx.Session.lastMessageID, text); err != nil {
		log.Warnf("im engine: text presenter send failed: %v", err)
	}
}
