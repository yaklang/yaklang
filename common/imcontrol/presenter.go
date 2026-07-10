package imcontrol

import (
	"github.com/yaklang/yaklang/common/notify"
)

// RunEventType 标识一个 turn 运行生命周期内的事件类型。
type RunEventType int

const (
	// RunEventStart 一个 turn 开始（dispatchToAgent 持锁后触发）。
	RunEventStart RunEventType = iota
	// RunEventDelta 流式增量文本（对应 AIOutputEvent stream 事件）。
	RunEventDelta
	// RunEventSegmentFinished 一个 event_writer_id 段流式结束
	// （对应 AIOutputEvent structured + NodeId==stream-finished）。
	// 段可能是思考过程（IsReason=true，detailed 模式才展示）或最终回复。
	RunEventSegmentFinished
	// RunEventResult 整个 turn 的最终结果（对应 AIOutputEvent result 事件）。
	RunEventResult
	// RunEventError 运行期错误（fail_react_task / api_request_failed 等）。
	RunEventError
)

// RunEvent 是 readAgentOutput 翻译给 Presenter 的运行生命周期事件。
// 字段按事件类型取用：Delta 事件填 Delta；SegmentFinished 填 Text（段累积全文）
// + IsReason；Result 填 Text；Error 填 Err。
type RunEvent struct {
	Type     RunEventType
	WriterID string // event_writer_id（Delta/SegmentFinished 用）
	NodeID   string
	Delta    string // Delta 事件的增量文本
	Text     string // SegmentFinished/Result 的完整文本
	IsReason bool   // 是否思考过程
	Err      error  // Error 事件
}

// IMInteractiveRequest 是 agent 发出的人工审批/用户交互请求。
type IMInteractiveRequest struct {
	ID      string
	Type    string
	Title   string
	Content string
}

// RunContext 携带一个 turn 的运行时状态，在同一 turn 的所有 OnRun* 调用间共享。
type RunContext struct {
	Session   *imSession
	RunID     string
	CardMsgID string // managed card 的 message_id（FeishuRunPresenter 用，TextRunPresenter 忽略）
	// Segments 已流式输出过的段计数，用于 OnRunResult 判断是否跳过 after_stream 重复。
	segmentsOutputted int
}

// RunPresenter 渲染一个 turn 的运行输出到 IM 平台。
//
// 生命周期（同一 turn 内串行调用）：
//
//	OnRunStart → (OnRunDelta * → OnRunSegmentFinished) * → OnRunResult | OnRunError
//
// readAgentOutput 负责把 AIOutputEvent 翻译成 RunEvent 喂给 Presenter，
// Presenter 决定怎么渲染（发文本 / 发卡片 / patch 卡片 / reaction）。
type RunPresenter interface {
	// OnRunStart turn 开始。Presenter 可发占位卡片/打字反应。
	OnRunStart(ctx *RunContext)
	// OnRunDelta 流式增量。Presenter 可累积并节流 patch 卡片。
	OnRunDelta(ctx *RunContext, ev RunEvent)
	// OnRunSegmentFinished 一个段流完。Text 为该段累积全文。
	OnRunSegmentFinished(ctx *RunContext, ev RunEvent)
	// OnRunResult 最终结果。
	OnRunResult(ctx *RunContext, ev RunEvent)
	// OnRunError 运行期错误。
	OnRunError(ctx *RunContext, ev RunEvent)
	// OnRunInteraction 人工审批或用户交互请求。
	OnRunInteraction(ctx *RunContext, req *IMInteractiveRequest)
	// Flush 兜底：流异常结束时把所有未提交的内容发出去。
	Flush(ctx *RunContext)
}

// PresenterDeps 注入给 Presenter 的依赖（发送消息/卡片的能力），便于测试替换。
type PresenterDeps struct {
	// Send 发一条文本消息（降级路径用）。
	Send func(platform notify.PlatformType, chatID, messageID, text string) error
	// SendCard 发一张卡片，返回 message_id（FeishuRunPresenter 的 OnRunStart 用）。
	SendCard func(msg *notify.Message, cfg *notify.SendConfig) (string, error)
	// PatchCard 更新已发卡片（FeishuRunPresenter 用）。不支持的实现可留空。
	PatchCard func(messageID string, msg *notify.Message, cfg *notify.SendConfig) error
	// SignToken 签发卡片按钮回调的防伪造 token（HMAC）。nil 时不签名（兼容测试）。
	SignToken func(input CallbackSignInput) string
	// Config 平台发送配置（凭证等）。
	Config *notify.SendConfig
}
