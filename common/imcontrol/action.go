package imcontrol

import (
	"github.com/yaklang/yaklang/common/notify"
)

// IMActionType 标识一个统一动作（命令和卡片按钮共用）。
type IMActionType string

const (
	ActionHelp            IMActionType = "help"
	ActionCommands        IMActionType = "commands"
	ActionNew             IMActionType = "new"
	ActionStop            IMActionType = "stop"
	ActionStatus          IMActionType = "status"
	ActionModel           IMActionType = "model"
	ActionMode            IMActionType = "mode"
	ActionScan            IMActionType = "scan"
	ActionMitm            IMActionType = "mitm"
	ActionRun             IMActionType = "run"
	ActionResume          IMActionType = "resume"
	ActionUpdateReplyMode IMActionType = "update_reply_mode"
	ActionConfig          IMActionType = "config"
	ActionReview          IMActionType = "review"
	ActionReviewDecision  IMActionType = "review_decision"
	ActionReviewConfirm   IMActionType = "review_confirm"
	ActionReviewReject    IMActionType = "review_reject"
	ActionSessionInfo     IMActionType = "session_info"
	ActionUseSession      IMActionType = "use_session"
	// ActionOpenYakit 保留给已发送到群里的旧卡片按钮；新卡片统一使用 session_info。
	ActionOpenYakit IMActionType = "open_yakit"
)

// IMAction 是命令和卡片按钮共用的统一动作模型。
//
// 命令路径（command.go handleCommand）：解析斜杠命令 → 构造 IMAction{Source:"command"}
//
//	→ handleAction。
//
// 卡片路径（engine.go handleMessage 收到 IsCardAction）：解析 button.value.action
//
//	→ 构造 IMAction{Source:"card", RunID} → 同一 handleAction。
//
// 两条路径汇到 handleAction，保证「/stop 命令」和「卡片停止按钮」走同一套逻辑。
type IMAction struct {
	Type   IMActionType
	Args   []string
	Source string // "command" / "card"
	// Msg 触发该动作的原始入站消息（回执/会话定位用）。
	Msg *notify.InboundMessage
	// RunID 卡片按钮触发时携带的 run id；命令路径为空。
	RunID string
}

// handleAction 统一处理命令和卡片按钮产生的动作。
// 所有 cmdXxx 的实际逻辑仍留在 command.go，这里按 Type 分发，
// 保证命令和卡片按钮共用同一实现（不会分叉）。
func (e *Engine) handleAction(act IMAction) {
	if actionRequiresOwner(act.Type) {
		if act.Msg == nil {
			return
		}
		if ok, reason := e.checkOwnerPermission(string(act.Msg.Platform), act.Msg.SenderID); !ok {
			e.replyRecovery(act.Msg, "无权限", "无权限："+reason+"\n\n该操作会修改机器人全局配置，请在 bot 所有者账号下操作。")
			return
		}
	}
	switch act.Type {
	case ActionHelp:
		e.cmdHelp(act.Msg)
	case ActionCommands:
		e.cmdCommands(act.Msg)
	case ActionNew:
		e.cmdNew(act.Msg)
	case ActionStop:
		e.cmdStop(act.Msg)
		// 卡片来源的 stop 额外把 run card 切到「已停止」态
		if act.Source == "card" {
			e.stopRunCard(act.Msg)
		}
	case ActionStatus:
		e.cmdStatus(act.Msg)
	case ActionModel:
		e.cmdModel(act.Msg, act.Args)
	case ActionMode:
		e.cmdMode(act.Msg, act.Args)
	case ActionScan:
		e.cmdScan(act.Msg, act.Args)
	case ActionMitm:
		e.cmdMitm(act.Msg, act.Args)
	case ActionRun:
		e.cmdRun(act.Msg, act.Args)
	case ActionResume:
		e.cmdResume(act.Msg)
	case ActionUpdateReplyMode:
		e.cmdUpdateReplyMode(act.Msg, act.Args)
	case ActionConfig:
		if act.Msg == nil {
			return
		}
		// card 来源的 config 不带 sub 时表示打开配置卡片；带 sub 时表示在配置卡片内修改配置。
		if act.Source == "card" && act.Msg.ActionValue != nil {
			if _, hasSub := act.Msg.ActionValue["sub"]; hasSub {
				e.cmdConfigCard(act.Msg)
				return
			}
		}
		e.cmdConfig(act.Msg, act.Args)
	case ActionReview:
		if act.Msg == nil {
			return
		}
		if act.Source == "card" && act.Msg.ActionValue != nil {
			if _, hasSub := act.Msg.ActionValue["sub"]; hasSub {
				e.cmdReviewCard(act.Msg)
				return
			}
		}
		e.cmdReview(act.Msg, act.Args)
	case ActionReviewDecision:
		e.cmdReviewDecision(act.Msg)
	case ActionReviewConfirm:
		e.cmdReviewCommandDecision(act.Msg, "continue")
	case ActionReviewReject:
		e.cmdReviewCommandDecision(act.Msg, "stop")
	case ActionSessionInfo, ActionOpenYakit:
		e.cmdSessionInfo(act.Msg, act.Args)
	case ActionUseSession:
		e.cmdUseSession(act.Msg)
	default:
		e.cmdUnknownAction(act.Msg, string(act.Type))
	}
}

func actionRequiresOwner(action IMActionType) bool {
	switch action {
	case ActionConfig:
		return true
	}
	return false
}

func knownIMAction(action IMActionType) bool {
	switch action {
	case ActionHelp,
		ActionCommands,
		ActionNew,
		ActionStop,
		ActionStatus,
		ActionModel,
		ActionMode,
		ActionScan,
		ActionMitm,
		ActionRun,
		ActionResume,
		ActionUpdateReplyMode,
		ActionConfig,
		ActionReview,
		ActionReviewDecision,
		ActionReviewConfirm,
		ActionReviewReject,
		ActionSessionInfo,
		ActionUseSession,
		ActionOpenYakit:
		return true
	}
	return false
}

// stopRunCard 把卡片来源的「停止」按钮触发的 run card 切到已停止态。
// 找到对应会话的 presenter，若它是 feishuRunPresenter 则 patch 终态卡片。
func (e *Engine) stopRunCard(msg *notify.InboundMessage) {
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		return
	}
	sess.presenterMu.RLock()
	p := sess.presenter
	sess.presenterMu.RUnlock()
	if fp, ok := p.(*feishuRunPresenter); ok {
		fp.patchCardFinal(&RunContext{Session: sess}, "任务已被用户中断。")
	}
}
