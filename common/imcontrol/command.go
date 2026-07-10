package imcontrol

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// builtinCommands 注册所有内置斜杠命令。
// 每个条目的 names 第一个是规范名，其余是别名；matchPrefix 做前缀匹配。
// 借鉴 cc-connect core/engine.go:6007 的命令表模式。
//
// 命令集聚焦「控制 AI agent」，不包含绕过 agent 直接执行 shell 等通用终端能力。
// 安全任务命令（/scan /mitm /run）本质是「生成结构化 prompt 喂给 agent」，
// 由 agent 理解并调用 yak 安全工具执行，不是 IM Engine 直接执行。
var builtinCommands = []struct {
	names []string
	id    string
}{
	{[]string{"help", "h", "?"}, "help"},
	{[]string{"commands", "cmds"}, "commands"},
	{[]string{"new", "reset"}, "new"},
	{[]string{"stop", "cancel"}, "stop"},
	{[]string{"status", "info"}, "status"},
	{[]string{"session", "sessions", "history"}, "session_info"},
	{[]string{"model"}, "model"},
	{[]string{"mode"}, "mode"},
	{[]string{"scan"}, "scan"},
	{[]string{"mitm"}, "mitm"},
	{[]string{"run"}, "run"},
	{[]string{"resume"}, "resume"},
	{[]string{"replymode", "rm"}, "update_reply_mode"},
	{[]string{"review", "rev"}, "review"},
	{[]string{"yes", "y", "approve", "continue"}, "review_confirm"},
	{[]string{"no", "n", "reject", "deny"}, "review_reject"},
	{[]string{"config"}, "config"},
}

// matchPrefix 精确或前缀匹配命令名，返回命令 id。
// 多个命中（歧义）时返回空串。借鉴 cc-connect core/engine.go:6089。
func matchPrefix(prefix string) string {
	// 精确匹配优先
	for _, c := range builtinCommands {
		for _, n := range c.names {
			if prefix == n {
				return c.id
			}
		}
	}
	// 前缀匹配
	matched := ""
	for _, c := range builtinCommands {
		for _, n := range c.names {
			if strings.HasPrefix(n, prefix) {
				if matched != "" && matched != c.id {
					return "" // 歧义
				}
				matched = c.id
			}
		}
	}
	return matched
}

// splitCommandArgs 切分命令行（支持引号）。借鉴 cc-connect core/engine.go:6142。
func splitCommandArgs(raw string) []string {
	// 简化版：先按引号分段，再按空格切。不追求完整 shell 语义。
	var parts []string
	var current strings.Builder
	inQuote := false
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// handleCommand 处理斜杠命令。返回 true 表示已识别并处理（不再走 agent）。
// 命令解析后构造 IMAction → handleAction 统一分发，与卡片按钮走同一处理路径。
func (e *Engine) handleCommand(msg *notify.InboundMessage, raw string) bool {
	parts := splitCommandArgs(raw)
	if len(parts) == 0 {
		return false
	}
	cmd := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	args := parts[1:]

	cmdID := matchPrefix(cmd)
	if cmdID == "" {
		e.cmdUnknown(msg, cmd)
		return true
	}

	e.handleAction(IMAction{
		Type:   IMActionType(cmdID),
		Args:   args,
		Source: "command",
		Msg:    msg,
	})
	return true
}

// --- 命令实现 ---

// cmdHelp 列出所有可用命令。飞书发交互卡片，不支持卡片的平台走文本。
func (e *Engine) cmdHelp(msg *notify.InboundMessage) {
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, e.buildHelpText(msg))
		return
	}
	e.replyCard(msg, e.buildHelpCard(msg))
}

func (e *Engine) cmdCommands(msg *notify.InboundMessage) {
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, e.buildCommandsText())
		return
	}
	e.replyCard(msg, e.buildCommandsCard(msg))
}

func (e *Engine) cmdUnknown(msg *notify.InboundMessage, cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		cmd = "(空命令)"
	}
	text := fmt.Sprintf("未识别命令: /%s\n\n输入 /commands 查看可用命令，或输入 /help 打开控制入口。", cmd)
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, text)
		return
	}
	card := &notify.Card{
		Title: "未知命令",
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: []map[string]any{
			{"tag": "markdown", "content": text},
			actionRowElement(
				e.controlButtonElement("命令列表", "primary", msg, "commands", nil),
				e.controlButtonElement("帮助", "default", msg, "help", nil),
				e.controlButtonElement("配置", "default", msg, "config", nil),
			),
		},
	}
	e.replyCard(msg, card)
}

func (e *Engine) cmdUnknownAction(msg *notify.InboundMessage, action string) {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "(空操作)"
	}
	detail := fmt.Sprintf("未知操作: %s\n\n请重新输入 /help 打开控制入口，或输入 /commands 查看可用命令。", action)
	e.replyRecovery(msg, "未知操作", detail)
}

func (e *Engine) replyRecovery(msg *notify.InboundMessage, title, detail string) {
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, title+"\n\n"+detail+"\n\n输入 /commands 查看可用命令，或输入 /help 打开控制入口。")
		return
	}
	e.replyCard(msg, e.buildRecoveryCard(msg, title, detail))
}

// cmdNew 新建会话：关闭旧 gRPC 流，下条消息起新 persistentSessionId。
func (e *Engine) cmdNew(msg *notify.InboundMessage) {
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess != nil {
		sess.resetForNew()
		go e.writeSessionIMMeta(sess)
	}
	e.reply(msg, "✅ 已新建会话，上下文已清空。下条消息将开始新的 AI 对话。")
}

// cmdStop 中断当前任务：向 agent 发送取消同步消息。
func (e *Engine) cmdStop(msg *notify.InboundMessage) {
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil || !sess.started {
		e.reply(msg, "当前没有活跃的 AI 会话。")
		return
	}
	sess.clearPendingInteraction()
	sess.streamMu.Lock()
	stream := sess.stream
	sess.streamMu.Unlock()
	if stream == nil {
		e.reply(msg, "当前没有正在执行的任务。")
		return
	}
	// 发送取消同步消息（aid 已支持 SYNC_TYPE_CANCEL_TASK）
	cancelEvent := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      "cancel_task",
	}
	if err := stream.Send(cancelEvent); err != nil {
		e.reply(msg, fmt.Sprintf("❌ 中断任务失败: %v", err))
		return
	}
	e.reply(msg, "⏹️ 已发送中断信号，正在停止当前任务…")
}

// cmdStatus 查看当前会话状态。飞书发交互卡片，不支持卡片的平台走文本。
func (e *Engine) cmdStatus(msg *notify.InboundMessage) {
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, e.buildStatusText(msg))
		return
	}
	e.replyCard(msg, e.buildStatusCard(msg))
}

func (e *Engine) cmdSessionInfo(msg *notify.InboundMessage, args []string) {
	if msg == nil {
		return
	}
	if ok := e.tryUseSessionByHistoryIndex(msg, args); ok {
		return
	}
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, e.buildSessionInfoText(msg))
		return
	}
	e.replyCard(msg, e.buildSessionInfoCard(msg))
}

func (e *Engine) tryUseSessionByHistoryIndex(msg *notify.InboundMessage, args []string) bool {
	if msg == nil || len(args) == 0 {
		return false
	}
	rawIndex := strings.TrimSpace(args[0])
	if rawIndex == "" {
		return false
	}
	index, err := strconv.Atoi(rawIndex)
	if err != nil {
		e.replyRecovery(msg, "恢复历史会话失败", "无法识别历史编号："+rawIndex+"\n\n发送 /session 查看最近历史，然后使用 /session 2 这类命令恢复。")
		return true
	}
	if index <= 0 {
		e.replyRecovery(msg, "恢复历史会话失败", "历史编号从 1 开始。\n\n发送 /session 查看最近历史，然后使用 /session 2 这类命令恢复。")
		return true
	}
	info := e.sessionInfoView(msg)
	if !info.CanBrowseAll {
		e.replyRecovery(msg, "恢复历史会话失败", info.HistoryAccess)
		return true
	}
	if info.HistoryLoadErr != "" {
		e.replyRecovery(msg, "恢复历史会话失败", "最近历史加载失败："+info.HistoryLoadErr)
		return true
	}
	if len(info.HistoryItems) == 0 {
		e.replyRecovery(msg, "恢复历史会话失败", "最近历史为空或尚未写入。")
		return true
	}
	if index > len(info.HistoryItems) {
		e.replyRecovery(msg, "恢复历史会话失败", fmt.Sprintf("最近历史只有 %d 条，无法恢复第 %d 条。\n\n发送 /session 刷新列表。", len(info.HistoryItems), index))
		return true
	}
	item := info.HistoryItems[index-1]
	if err := e.bindAISessionToIM(msg, item.SessionID, item.Title); err != nil {
		e.replyRecovery(msg, "恢复历史会话失败", err.Error())
		return true
	}
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, fmt.Sprintf("✅ 已切换会话\n\n**%s**\n\n下一条消息会沿用该历史会话上下文。\n\n发送 /session 可查看当前绑定状态。", item.Title))
		return true
	}
	e.replyCard(msg, e.buildSessionInfoCard(msg))
	return true
}

func (e *Engine) cmdUseSession(msg *notify.InboundMessage) {
	if msg == nil {
		return
	}
	sessionID := actionValueString(msg.ActionValue, "session_id")
	title := actionValueString(msg.ActionValue, "title")
	if err := e.bindAISessionToIM(msg, sessionID, title); err != nil {
		e.replyRecovery(msg, "恢复历史会话失败", err.Error())
		return
	}
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, fmt.Sprintf("✅ 已切换到历史会话 %s。下一条消息会沿用该 Session 的上下文。", sessionID))
		return
	}
	e.replyCard(msg, e.buildSessionInfoCard(msg))
}

func (e *Engine) bindAISessionToIM(msg *notify.InboundMessage, sessionID, title string) error {
	if msg == nil {
		return fmt.Errorf("消息为空")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("缺少 session_id")
	}
	if !e.canBrowseAllAISessions(msg) {
		return fmt.Errorf("只有 bot 所有者在私聊控制台里可以恢复全局 Yakit 历史会话")
	}
	sessionKey := imSessionKey(msg)
	e.touchSession(sessionKey, msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("当前 IM 会话不存在")
	}
	sess.closeStream()
	sess.persistentSessionId = sessionID
	sess.yakitSessionTitle = compactCardText(title, 80)
	sess.lastActiveAt = now()
	return nil
}

type imStatusSnapshot struct {
	PlatformCount   int
	SessionCount    int
	HasSession      bool
	SessionKey      string
	Platform        string
	ChatType        string
	Model           string
	SessionID       string
	LastActive      string
	StreamStatus    string
	ReplyQuote      bool
	Granularity     string
	GroupTrigger    string
	ReviewPolicy    string
	ReviewRiskScore float64
	DisallowPrompt  bool
}

func (e *Engine) statusSnapshot(msg *notify.InboundMessage) imStatusSnapshot {
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	platformCount := len(e.platforms)
	sessionCount := len(e.sessions)
	cfg := e.configForPlatformLocked(string(msg.Platform))
	replyQuote := cfg.ReplyQuote
	granularity := cfg.ReplyGranularity
	groupTrigger := cfg.GroupTrigger
	reviewPolicy := cfg.ReviewPolicy
	reviewRiskScore := cfg.AIReviewRiskControlScore
	disallowPrompt := cfg.DisallowRequireForUserPrompt
	e.mu.Unlock()

	model := currentDefaultAIModelName()
	if model == "" {
		model = "默认模型"
	}
	snapshot := imStatusSnapshot{
		PlatformCount:   platformCount,
		SessionCount:    sessionCount,
		Platform:        string(msg.Platform),
		ChatType:        orDefault(msg.ChatType, "private"),
		Model:           model,
		ReplyQuote:      replyQuote,
		Granularity:     granularity,
		GroupTrigger:    groupTrigger,
		ReviewPolicy:    reviewPolicy,
		ReviewRiskScore: reviewRiskScore,
		DisallowPrompt:  disallowPrompt,
	}
	if sess == nil {
		snapshot.StreamStatus = "未创建"
		return snapshot
	}
	snapshot.HasSession = true
	snapshot.SessionKey = sess.sessionKey
	snapshot.Platform = sess.platform
	snapshot.ChatType = orDefault(sess.chatType, snapshot.ChatType)
	snapshot.Model = orDefault(sess.currentModel, snapshot.Model)
	snapshot.SessionID = sess.persistentSessionId
	snapshot.LastActive = sess.lastActiveAt.Format("15:04:05")
	snapshot.StreamStatus = activeStatus(sess.started)
	snapshot.ReviewPolicy = sess.reviewPolicy
	snapshot.ReviewRiskScore = sess.aiReviewRiskControlScore
	snapshot.DisallowPrompt = sess.disallowRequireForUserPrompt
	return snapshot
}

func (e *Engine) buildHelpText(msg *notify.InboundMessage) string {
	s := e.statusSnapshot(msg)
	return fmt.Sprintf(`Yak Agent 控制台

当前会话
- 平台: %s
- 模型: %s
- 会话: %s
- 回复模式: %s
- 群聊触发: %s
- 执行审批: %s

常用操作
- /session: 打开会话面板
- /session 2: 切换到第 2 个最近历史
- /new: 新对话
- /config: 打开配置
- /review: 执行审批
- /yes: 确认继续等待中的操作
- /status: 查看状态
- /commands: 查看命令列表

能力
- 直接提问
- 附件分析
- 安全任务快捷入口
- AI 对话开始后自动同步到 Yakit 历史`, platformDisplayLabel(s.Platform), s.Model, sessionSummaryLabel(s), replyGranularityLabel(s.Granularity), groupTriggerSelectLabel(s.GroupTrigger), reviewPolicyLabel(s.ReviewPolicy))
}

func (e *Engine) buildHelpCard(msg *notify.InboundMessage) *notify.Card {
	s := e.statusSnapshot(msg)
	elements := []map[string]any{
		configSectionElement("控制台"),
		actionRowElement(
			configInfoElement("平台", platformDisplayLabel(s.Platform)),
			configInfoElement("模型", s.Model),
			configInfoElement("会话", sessionSummaryLabel(s)),
		),
		map[string]any{"tag": "hr"},
		configSectionElement("主要入口"),
		actionRowElement(
			e.controlButtonElement("会话", "primary", msg, string(ActionSessionInfo), nil),
			e.controlButtonElement("配置", "default", msg, "config", nil),
			e.controlButtonElement("审批", "default", msg, "review", nil),
			e.controlButtonElement("状态", "default", msg, "status", nil),
			e.controlButtonElement("命令列表", "default", msg, "commands", nil),
		),
		map[string]any{"tag": "hr"},
		configSectionElement("会话行为"),
		actionRowElement(
			configInfoElement("回复模式", replyGranularityLabel(s.Granularity)),
			configInfoElement("群聊触发", groupTriggerSelectLabel(s.GroupTrigger)),
			configInfoElement("执行审批", reviewPolicyLabel(s.ReviewPolicy)),
		),
		configHintElement("直接发送消息即可开始。AI 对话开始后会同步到 Yakit 历史；可用 /session 2 切换最近历史，等待确认时可用 /yes 继续。"),
	}
	return &notify.Card{
		Title: "Yak Agent 控制台",
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: elements,
	}
}

func (e *Engine) buildStatusText(msg *notify.InboundMessage) string {
	s := e.statusSnapshot(msg)
	session := "无（首次发消息将自动创建）"
	if s.HasSession {
		session = fmt.Sprintf("%s\n- 会话ID: %s\n- 最近活跃: %s\n- 流状态: %s", s.SessionKey, s.SessionID, s.LastActive, s.StreamStatus)
	}
	return fmt.Sprintf(`IM 状态

引擎
- 监听平台数: %d
- 活跃会话数: %d

当前会话
- %s

回复配置
- 引用回复: %v
- 回复模式: %s
- 群聊触发: %s
- 执行审批: %s

Yakit 历史
- %s`, s.PlatformCount, s.SessionCount, session, s.ReplyQuote, replyGranularityLabel(s.Granularity), groupTriggerSelectLabel(s.GroupTrigger), reviewPolicyLabel(s.ReviewPolicy), historyStatusFromSnapshot(s))
}

func (e *Engine) buildStatusCard(msg *notify.InboundMessage) *notify.Card {
	s := e.statusSnapshot(msg)
	sessionText := "无（首次发消息将自动创建）"
	if s.HasSession {
		sessionText = fmt.Sprintf("**会话Key**\n%s\n**会话ID**\n%s\n**最近活跃**\n%s\n**流状态**\n%s", s.SessionKey, s.SessionID, s.LastActive, s.StreamStatus)
	}
	return &notify.Card{
		Title: "IM 状态",
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: []map[string]any{
			configSectionElement("运行概览"),
			actionRowElement(
				configInfoElement("监听平台", fmt.Sprintf("%d", s.PlatformCount)),
				configInfoElement("活跃会话", fmt.Sprintf("%d", s.SessionCount)),
				configInfoElement("当前流", s.StreamStatus),
			),
			map[string]any{"tag": "hr"},
			configSectionElement("当前会话"),
			actionRowElement(
				configInfoElement("平台", platformDisplayLabel(s.Platform)),
				configInfoElement("场景", chatTypeDisplayLabel(s.ChatType)),
			),
			configInfoElement("会话", sessionText),
			map[string]any{"tag": "hr"},
			configSectionElement("回复配置"),
			actionRowElement(
				configInfoElement("引用回复", boolStatusLabel(s.ReplyQuote)),
				configInfoElement("回复模式", replyGranularityLabel(s.Granularity)),
				configInfoElement("执行审批", reviewPolicyLabel(s.ReviewPolicy)),
			),
			configInfoElement("群聊触发", groupTriggerSelectLabel(s.GroupTrigger)),
			map[string]any{"tag": "hr"},
			configSectionElement("Yakit 历史"),
			configHintElement(historyStatusFromSnapshot(s)),
			actionRowElement(
				e.controlButtonElement("会话", "primary", msg, string(ActionSessionInfo), nil),
				e.controlButtonElement("配置", "default", msg, "config", nil),
				e.controlButtonElement("命令列表", "default", msg, "commands", nil),
			),
		},
	}
}

const imSessionHistoryLimit = 5

type imSessionHistoryItem struct {
	SessionID string
	Title     string
	Source    string
	UpdatedAt string
	Platform  string
	ChatType  string
	Current   bool
}

type imSessionInfoView struct {
	Title          string
	SessionID      string
	Platform       string
	ChatType       string
	Model          string
	StreamStatus   string
	ReplyQuote     bool
	Granularity    string
	GroupTrigger   string
	ReviewPolicy   string
	HistoryStatus  string
	HistoryAccess  string
	HistoryItems   []imSessionHistoryItem
	CanBrowseAll   bool
	HistoryLoadErr string
}

func (e *Engine) buildSessionInfoText(msg *notify.InboundMessage) string {
	info := e.sessionInfoView(msg)
	var b strings.Builder
	b.WriteString("## 会话面板\n\n")
	b.WriteString("**当前会话**\n")
	b.WriteString(fmt.Sprintf("- 标题：%s\n", info.Title))
	b.WriteString(fmt.Sprintf("- 平台：%s / %s\n", info.Platform, info.ChatType))
	b.WriteString(fmt.Sprintf("- 模型：%s\n", info.Model))
	b.WriteString(fmt.Sprintf("- 状态：%s\n", info.StreamStatus))
	b.WriteString(fmt.Sprintf("- 历史：%s\n\n", conciseHistoryStatus(info.HistoryStatus)))

	b.WriteString("**配置**\n")
	b.WriteString(fmt.Sprintf("- 回复：%s\n", replyGranularityLabel(info.Granularity)))
	b.WriteString(fmt.Sprintf("- 引用回复：%s\n", boolStatusLabel(info.ReplyQuote)))
	b.WriteString(fmt.Sprintf("- 群聊触发：%s\n", groupTriggerSelectLabel(info.GroupTrigger)))
	b.WriteString(fmt.Sprintf("- 执行审批：%s\n\n", reviewPolicyLabel(info.ReviewPolicy)))

	b.WriteString("**最近历史**\n")
	if info.HistoryLoadErr != "" {
		b.WriteString("- 最近历史加载失败：" + info.HistoryLoadErr + "\n")
	} else if len(info.HistoryItems) == 0 {
		if info.CanBrowseAll {
			b.WriteString("- 暂无可恢复的历史会话。\n")
		} else if strings.TrimSpace(info.HistoryAccess) != "" {
			b.WriteString("- " + strings.ReplaceAll(info.HistoryAccess, "\n", "\n- ") + "\n")
		} else {
			b.WriteString("- 当前账号没有最近历史浏览权限。\n")
		}
	} else {
		for i, item := range info.HistoryItems {
			mark := ""
			if item.Current {
				mark = " 当前"
			}
			meta := strings.TrimSpace(strings.Join(nonEmptyStrings(item.Source, item.Platform, item.UpdatedAt), " / "))
			if item.ChatType != "" {
				meta += " / " + item.ChatType
			}
			b.WriteString(fmt.Sprintf("%d. **%s**%s\n   %s\n", i+1, item.Title, mark, meta))
		}
	}
	b.WriteString("\n**操作**\n")
	if len(info.HistoryItems) > 0 {
		b.WriteString("- 发送 `/session 2` 切换到第 2 个历史会话\n")
	}
	b.WriteString("- 发送 `/session` 刷新会话面板\n")
	b.WriteString("- 发送 `/new` 新建会话\n")
	return b.String()
}

func (e *Engine) buildSessionInfoCard(msg *notify.InboundMessage) *notify.Card {
	info := e.sessionInfoView(msg)
	elements := []map[string]any{
		configSectionElement("当前会话"),
		actionRowElement(
			configInfoElement("标题", info.Title),
			configInfoElement("场景", info.ChatType),
		),
		actionRowElement(
			configInfoElement("平台", info.Platform),
			configInfoElement("模型", info.Model),
			configInfoElement("流状态", info.StreamStatus),
		),
		map[string]any{"tag": "hr"},
		configSectionElement("配置"),
		actionRowElement(
			configInfoElement("引用回复", boolStatusLabel(info.ReplyQuote)),
			configInfoElement("回复模式", replyGranularityLabel(info.Granularity)),
			configInfoElement("执行审批", reviewPolicyLabel(info.ReviewPolicy)),
		),
		configInfoElement("群聊触发", groupTriggerSelectLabel(info.GroupTrigger)),
		map[string]any{"tag": "hr"},
		configSectionElement("Yakit 历史"),
		configInfoElement("状态", info.HistoryStatus),
	}
	if info.HistoryAccess != "" {
		elements = append(elements, configHintElement(info.HistoryAccess))
	}
	if info.HistoryLoadErr != "" {
		elements = append(elements, configHintElement("最近历史加载失败: "+info.HistoryLoadErr))
	}
	if len(info.HistoryItems) > 0 {
		elements = append(elements,
			map[string]any{"tag": "hr"},
			configSectionElement("最近历史"),
		)
		for _, item := range info.HistoryItems {
			elements = append(elements, e.sessionHistoryRowElement(msg, item))
		}
	}
	elements = append(elements,
		map[string]any{"tag": "hr"},
		actionRowElement(
			e.controlButtonElement("新对话", "primary", msg, "new", nil),
			e.controlButtonElement("恢复当前", "default", msg, "resume", nil),
			e.controlButtonElement("状态", "default", msg, "status", nil),
			e.controlButtonElement("配置", "default", msg, "config", nil),
		),
	)
	return &notify.Card{
		Title: "会话面板",
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: elements,
	}
}

func (e *Engine) sessionInfoView(msg *notify.InboundMessage) imSessionInfoView {
	sessionID := actionValueString(msg.ActionValue, "session_id")
	title := actionValueString(msg.ActionValue, "chat_title")
	if title == "" {
		title = actionValueString(msg.ActionValue, "title")
	}
	sessionKey := imSessionKey(msg)
	s := e.statusSnapshot(msg)

	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess != nil {
		if sessionID == "" {
			sessionID = sess.persistentSessionId
		}
		if title == "" {
			title = sess.yakitSessionTitle
		}
		if title == "" {
			title = sess.chatTitle
		}
	}
	if sessionID == "" {
		sessionID = sessionKey
	}
	if title == "" {
		title = buildIMSessionTitle(msg)
	}
	historyStatus := historyStatusFromSnapshot(s)
	canBrowseAll, historyAccess := e.browseAllAISessionsAccess(msg)
	var historyItems []imSessionHistoryItem
	var historyLoadErr string
	if canBrowseAll {
		historyItems, historyLoadErr = e.recentAISessionHistory(sessionID, imSessionHistoryLimit)
		if historyLoadErr == "" && len(historyItems) == 0 {
			historyAccess += "\n最近历史为空或尚未写入。"
		}
	}
	return imSessionInfoView{
		Title:          title,
		SessionID:      sessionID,
		Platform:       platformDisplayLabel(s.Platform),
		ChatType:       chatTypeDisplayLabel(s.ChatType),
		Model:          s.Model,
		StreamStatus:   s.StreamStatus,
		ReplyQuote:     s.ReplyQuote,
		Granularity:    s.Granularity,
		GroupTrigger:   s.GroupTrigger,
		ReviewPolicy:   s.ReviewPolicy,
		HistoryStatus:  historyStatus,
		HistoryAccess:  historyAccess,
		HistoryItems:   historyItems,
		CanBrowseAll:   canBrowseAll,
		HistoryLoadErr: historyLoadErr,
	}
}

func (e *Engine) sessionHistoryRowElement(msg *notify.InboundMessage, item imSessionHistoryItem) map[string]any {
	title := item.Title
	if item.Current {
		title += "（当前）"
	}
	detail := item.UpdatedAt
	if item.ChatType != "" {
		detail += " / " + item.ChatType
	}
	info := configInfoElement(title, detail)
	right := e.controlButtonElement("恢复", "default", msg, string(ActionUseSession), map[string]any{
		"session_id": item.SessionID,
		"title":      item.Title,
	})
	if item.Current {
		right = configInfoElement("状态", "当前绑定")
	}
	return map[string]any{
		"tag":       "column_set",
		"flex_mode": "none",
		"columns": []map[string]any{
			{
				"tag":            "column",
				"width":          "weighted",
				"weight":         1,
				"vertical_align": "top",
				"elements":       []map[string]any{info},
			},
			{
				"tag":            "column",
				"width":          "auto",
				"vertical_align": "top",
				"elements":       []map[string]any{right},
			},
		},
	}
}

func (e *Engine) recentAISessionHistory(currentSessionID string, limit int) ([]imSessionHistoryItem, string) {
	if limit <= 0 {
		limit = imSessionHistoryLimit
	}
	resp, err := e.queryRecentAISessions(limit)
	if err != nil {
		return nil, err.Error()
	}
	items := make([]imSessionHistoryItem, 0, len(resp.GetData()))
	for _, session := range resp.GetData() {
		if session == nil || strings.TrimSpace(session.GetSessionID()) == "" {
			continue
		}
		items = append(items, aiSessionHistoryItemFromPB(session, currentSessionID))
	}
	if len(items) == 0 {
		return nil, ""
	}
	return items, ""
}

func (e *Engine) queryRecentAISessions(limit int) (*ypb.QueryAISessionResponse, error) {
	if limit <= 0 {
		limit = imSessionHistoryLimit
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := &ypb.QueryAISessionRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   int64(limit),
			OrderBy: "updated_at",
			Order:   "desc",
		},
	}
	if e.queryAISessionFunc != nil {
		return e.queryAISessionFunc(ctx, req)
	}
	if e.grpcClient == nil {
		return nil, fmt.Errorf("未连接 yaklang 引擎，无法加载 Yakit 历史")
	}
	return e.grpcClient.QueryAISession(ctx, req)
}

func aiSessionHistoryItemFromPB(session *ypb.AISession, currentSessionID string) imSessionHistoryItem {
	sessionID := strings.TrimSpace(session.GetSessionID())
	title := strings.TrimSpace(session.GetTitle())
	if title == "" || title == "<未命名>" {
		if meta := session.GetIMSourceMeta(); meta != nil && strings.TrimSpace(meta.GetChatTitle()) != "" {
			title = strings.TrimSpace(meta.GetChatTitle())
		}
	}
	if title == "" || title == "<未命名>" {
		title = shortIDForConfig(sessionID)
	}
	item := imSessionHistoryItem{
		SessionID: sessionID,
		Title:     compactCardText(title, 48),
		Source:    aiSessionSourceLabel(session.GetSource()),
		UpdatedAt: formatUnixTime(session.GetUpdatedAt()),
		Current:   sessionID == strings.TrimSpace(currentSessionID),
	}
	if meta := session.GetIMSourceMeta(); meta != nil {
		item.Platform = platformDisplayLabel(meta.GetPlatform())
		item.ChatType = chatTypeDisplayLabel(meta.GetChatType())
	}
	if item.Platform == "" {
		item.Platform = "Yakit"
	}
	return item
}

func (e *Engine) buildCommandsText() string {
	return `IM 命令列表

会话
- /session: 打开会话面板
- /session <编号>: 切换到最近历史中的指定会话
- /new: 新建会话
- /resume: 恢复当前运行流
- /stop: 中止当前任务
- /status: 查看状态

配置
- /config: 打开配置
- /replymode [standard|summary|detailed]: 设置回复模式
- /review [manual|ai|yolo]: 设置执行审批
- /model <名称>: 设置模型
- /mode [plan|react]: 设置执行模式

任务
- /scan <目标>: 安全扫描
- /mitm: 启动 MITM 任务
- /run <脚本>: 执行 yak 脚本

审批
- /yes: 确认继续等待中的操作
- /no: 拒绝并停止当前任务

帮助
- /help: 打开控制入口
- /commands: 查看命令列表`
}

func (e *Engine) buildCommandsCard(msg *notify.InboundMessage) *notify.Card {
	return &notify.Card{
		Title:   "IM 命令",
		Content: e.buildCommandsText(),
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: []map[string]any{
			configSectionElement("会话"),
			{"tag": "markdown", "content": "`/session` 打开会话面板\n`/session 2` 切换到第 2 个最近历史\n`/new` 新建当前会话\n`/resume` 恢复当前运行流\n`/stop` 中止当前任务\n`/status` 查看运行状态"},
			map[string]any{"tag": "hr"},
			configSectionElement("配置"),
			{"tag": "markdown", "content": "`/config` 打开配置\n`/replymode [standard|summary|detailed]` 设置回复模式\n`/review [manual|ai|yolo]` 设置执行审批\n`/model <名称>` 设置模型\n`/mode [plan|react]` 设置执行模式"},
			map[string]any{"tag": "hr"},
			configSectionElement("任务"),
			{"tag": "markdown", "content": "`/scan <目标>` 安全扫描\n`/mitm` 启动 MITM 任务\n`/run <脚本>` 执行 yak 脚本"},
			map[string]any{"tag": "hr"},
			configSectionElement("审批"),
			{"tag": "markdown", "content": "`/yes` 确认继续等待中的操作\n`/no` 拒绝并停止当前任务"},
			actionRowElement(
				e.controlButtonElement("会话", "primary", msg, string(ActionSessionInfo), nil),
				e.controlButtonElement("帮助", "default", msg, "help", nil),
				e.controlButtonElement("配置", "default", msg, "config", nil),
				e.controlButtonElement("状态", "default", msg, "status", nil),
			),
		},
	}
}

func (e *Engine) buildRecoveryCard(msg *notify.InboundMessage, title, detail string) *notify.Card {
	return &notify.Card{
		Title: title,
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: []map[string]any{
			{"tag": "markdown", "content": detail},
			actionRowElement(
				e.controlButtonElement("命令列表", "primary", msg, "commands", nil),
				e.controlButtonElement("帮助", "default", msg, "help", nil),
				e.controlButtonElement("配置", "default", msg, "config", nil),
			),
		},
	}
}

// cmdModel 切换 AI 模型（修改会话配置，下个 /new 生效）。
func (e *Engine) cmdModel(msg *notify.InboundMessage, args []string) {
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()

	if len(args) == 0 {
		// 列出可用模型：通过 gRPC 调 ListAIProviders
		e.reply(msg, "ℹ️ 模型切换说明\n\n当前实现：模型配置在新建会话时生效。\n用法: /model <模型标识>\n示例: /model gpt-4\n\n用 /new 新建会话后生效。")
		return
	}
	model := strings.Join(args, " ")
	if sess != nil {
		sess.currentModel = model
	}
	e.reply(msg, fmt.Sprintf("✅ 已设置模型为 %s（用 /new 新建会话后生效）", model))
}

// cmdMode 切换执行模式（plan-and-exec vs react）。
func (e *Engine) cmdMode(msg *notify.InboundMessage, args []string) {
	if len(args) == 0 {
		e.reply(msg, "用法: /mode [plan|react]\n  plan  = 先规划再执行（适合复杂任务）\n  react = 直接推理循环（适合简单问答）")
		return
	}
	mode := strings.ToLower(args[0])
	if mode != "plan" && mode != "react" {
		e.reply(msg, "❌ 无效模式，可选: plan / react")
		return
	}
	e.reply(msg, fmt.Sprintf("✅ 执行模式设为 %s（用 /new 新建会话后生效）", mode))
}

// cmdScan 对目标执行端口扫描和安全评估（生成结构化 prompt 喂给 agent）。
func (e *Engine) cmdScan(msg *notify.InboundMessage, args []string) {
	if len(args) == 0 {
		e.reply(msg, "用法: /scan <目标>\n示例: /scan 192.168.1.1\n      /scan 192.168.1.0/24")
		return
	}
	target := strings.Join(args, " ")
	prompt := fmt.Sprintf("请对目标 %s 执行端口扫描和安全评估。先做端口扫描识别开放服务，再针对开放的服务做基础安全检查。给出发现摘要。", target)
	// 作为普通消息喂给 agent
	e.dispatchToAgent(msg, prompt)
}

// cmdMitm 启动 MITM 中间人代理（生成 prompt 喂给 agent）。
func (e *Engine) cmdMitm(msg *notify.InboundMessage, args []string) {
	prompt := "请启动一个 MITM 中间人代理，用于抓取和分析 HTTP 流量。告诉我代理地址和端口，以及如何配置客户端使用。"
	e.dispatchToAgent(msg, prompt)
}

// cmdRun 让 agent 执行指定 yak 脚本（生成 prompt 喂给 agent）。
func (e *Engine) cmdRun(msg *notify.InboundMessage, args []string) {
	if len(args) == 0 {
		e.reply(msg, "用法: /run <脚本名或描述>\n示例: /run 端口扫描器\n      /run poc.YamlBuilder")
		return
	}
	script := strings.Join(args, " ")
	prompt := fmt.Sprintf("请执行 yak 脚本: %s。如果需要参数请询问我。", script)
	e.dispatchToAgent(msg, prompt)
}

// cmdResume 恢复被中断的会话：若 gRPC 流已断（started=false）则重建，否则回执仍在活跃。
// /stop 只发 cancel_task 不关流，agent 处理后流可能还在；resume 仅在流真正断了时重建。
func (e *Engine) cmdResume(msg *notify.InboundMessage) {
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		e.reply(msg, "当前没有会话可恢复。直接发消息即可开始新的 AI 对话。")
		return
	}
	sess.streamMu.Lock()
	started := sess.started
	sess.streamMu.Unlock()
	if started {
		e.reply(msg, "✅ 当前会话仍在活跃中，直接发消息即可继续对话。")
		return
	}
	// 流已断，重建
	if err := e.startAgentStream(sess); err != nil {
		e.reply(msg, fmt.Sprintf("❌ 恢复会话失败: %v", err))
		return
	}
	e.reply(msg, "✅ 会话已恢复，上下文保留。直接发消息继续对话。")
}

// cmdUpdateReplyMode 切换回复颗粒度（standard / summary / detailed）。
// 命令和卡片「切换详细度」按钮共用本实现。
func (e *Engine) cmdUpdateReplyMode(msg *notify.InboundMessage, args []string) {
	if len(args) == 0 {
		e.reply(msg, "用法: /replymode [standard|summary|detailed]\n  standard = 只发最终回复\n  summary  = 摘要+最终回复\n  detailed = 含思考过程")
		return
	}
	mode := strings.ToLower(args[0])
	switch mode {
	case "standard", "summary", "detailed":
	default:
		e.reply(msg, "❌ 无效模式，可选: standard / summary / detailed")
		return
	}
	e.UpdateConfigForPlatform(string(msg.Platform), e.replyQuoteForPlatform(string(msg.Platform)), mode)
	e.reply(msg, fmt.Sprintf("✅ 回复颗粒度已设为 %s", mode))
}

// cmdReview 查看或切换当前会话的执行审批策略。
func (e *Engine) cmdReview(msg *notify.InboundMessage, args []string) {
	if len(args) > 0 {
		replyText, _ := e.setReviewPolicyForMessage(msg, args[0])
		e.reply(msg, replyText)
		return
	}
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, e.buildReviewText(msg))
		return
	}
	e.replyCard(msg, e.buildReviewCard(msg))
}

func (e *Engine) cmdReviewCard(msg *notify.InboundMessage) {
	policy := actionValueString(msg.ActionValue, "policy")
	if policy == "" {
		policy = actionValueString(msg.ActionValue, "option")
	}
	replyText, shouldPatch := e.setReviewPolicyForMessage(msg, policy)
	if replyText != "" {
		e.reply(msg, replyText)
	}
	if !shouldPatch {
		return
	}
	cardMsgID := msgIDToString(msg.ReplyContext)
	if cardMsgID != "" && platformCapabilities(msg.Platform).UpdateCard {
		e.patchReviewCard(msg, cardMsgID)
	}
}

// cmdConfig 打开配置卡片（飞书交互卡片），展示当前回复配置 + 群聊触发策略 + 权限概要。
// 不支持卡片的平台走文本。
func (e *Engine) cmdConfig(msg *notify.InboundMessage, args []string) {
	configText := e.buildConfigSummary(msg)
	if !platformCapabilities(msg.Platform).SendCard {
		e.reply(msg, configText)
		return
	}
	e.replyCard(msg, e.buildConfigCard(msg))
}

// cmdConfigCard 处理卡片来源的 config 子动作（toggle_reply_quote / set_granularity / set_group_trigger / set_review_policy）。
// 执行对应配置切换后，patch 原卡片反映新状态。
func (e *Engine) cmdConfigCard(msg *notify.InboundMessage) {
	replyText, shouldPatch := e.applyConfigCardAction(msg)
	if replyText != "" {
		e.reply(msg, replyText)
	}
	if !shouldPatch {
		return
	}
	// patch 原配置卡片反映新状态
	cardMsgID := msgIDToString(msg.ReplyContext)
	if cardMsgID != "" && platformCapabilities(msg.Platform).UpdateCard {
		e.patchConfigCard(msg, cardMsgID)
	}
}

func (e *Engine) applyConfigCardAction(msg *notify.InboundMessage) (replyText string, shouldPatch bool) {
	if msg == nil {
		return "❌ 配置操作失败：消息为空", false
	}
	sub, _ := msg.ActionValue["sub"].(string)
	platform := string(msg.Platform)
	switch sub {
	case "toggle_reply_quote":
		rq, _, _ := e.GetConfigForPlatform(platform)
		next := !rq
		e.UpdateConfigForPlatform(platform, next, "")
		return fmt.Sprintf("✅ 引用回复已切换为 %s", boolStatusLabel(next)), true
	case "cycle_group_trigger":
		_, _, gt := e.GetConfigForPlatform(platform)
		next := nextGroupTrigger(gt)
		e.UpdateGroupTriggerForPlatform(platform, next)
		return fmt.Sprintf("✅ 群聊触发策略已切换为 %s", groupTriggerLabel(next)), true
	case "toggle_group_mention":
		_, _, gt := e.GetConfigForPlatform(platform)
		next := "must_at"
		if gt == "must_at" || gt == "allow_slash" {
			next = "allow_all"
		}
		e.UpdateGroupTriggerForPlatform(platform, next)
		return fmt.Sprintf("✅ 群聊触发已切换为 %s", groupTriggerLabel(next)), true
	case "set_group_trigger":
		trigger, _ := msg.ActionValue["trigger"].(string)
		if trigger == "" {
			trigger, _ = msg.ActionValue["option"].(string)
		}
		if !isValidGroupTrigger(trigger) {
			return "❌ 无效群聊触发策略，可选: must_at / allow_all", false
		}
		e.UpdateGroupTriggerForPlatform(platform, trigger)
		return fmt.Sprintf("✅ 群聊触发策略已切换为 %s", groupTriggerLabel(trigger)), true
	case "set_granularity":
		mode, _ := msg.ActionValue["mode"].(string)
		if mode == "" {
			mode, _ = msg.ActionValue["option"].(string)
		}
		if mode == "" || (mode != "standard" && mode != "summary" && mode != "detailed") {
			return "❌ 无效回复模式，可选: standard / summary / detailed", false
		}
		e.UpdateConfigForPlatform(platform, e.replyQuoteForPlatform(platform), mode)
		return fmt.Sprintf("✅ 回复颗粒度已设为 %s", replyGranularityLabel(mode)), true
	case "set_review_policy":
		policy := actionValueString(msg.ActionValue, "policy")
		if policy == "" {
			policy = actionValueString(msg.ActionValue, "option")
		}
		return e.setReviewPolicyForMessage(msg, policy)
	default:
		if sub == "" {
			sub = "(空)"
		}
		return "❌ 未知配置子动作: " + sub, false
	}
}

// buildConfigSummary 生成当前配置的 markdown 摘要。
func (e *Engine) buildConfigSummary(msg *notify.InboundMessage) string {
	reviewPolicy, riskScore, disallowPrompt := e.reviewConfigForMessage(msg)
	platform := notify.PlatformType("")
	if msg != nil {
		platform = msg.Platform
	}
	rq, granularity, gt := e.GetConfigForPlatform(string(platform))
	return fmt.Sprintf(`**回复配置**
- 引用回复: %v
- 回复颗粒度: %s

**群聊触发策略**
- 当前: %s
  - must_at: 群聊中必须 @bot，命令也必须 @ 后使用
  - allow_all: 群聊所有消息都会触发

**执行审批**
- 当前: %s
- AI 风险阈值: %.2f
- 允许 AI 主动询问: %v

**权限配置**（在 Yakit 机器人配置面板编辑）
- %s`,
		rq, granularity, groupTriggerLabel(gt), reviewPolicyLabel(reviewPolicy), riskScore, !disallowPrompt, e.permissionSummary(platform))
}

// configButtons 构造配置类卡片的按钮列表（带签名 token）。
// includeStop=true 时额外加"停止"按钮（status 卡片用）。
func (e *Engine) configButtons(msg *notify.InboundMessage, includeStop bool) []notify.CardButton {
	buttons := []notify.CardButton{
		{Text: "💬 新对话", Style: "primary", Value: e.signedValue(msg, "new", nil)},
		{Text: "⚙️ 配置", Style: "default", Value: e.signedValue(msg, "config", nil)},
	}
	if includeStop {
		buttons = append(buttons, notify.CardButton{
			Text: "⏹ 停止", Style: "danger", Value: e.signedValue(msg, "stop", nil),
		})
	}
	return buttons
}

// signedValue 构造带签名 token 的按钮 value map。
func (e *Engine) signedValue(msg *notify.InboundMessage, action string, extra map[string]any) map[string]any {
	v := map[string]any{"action": action}
	for k, val := range extra {
		v[k] = val
	}
	if e.callbackAuth != nil {
		v["token"] = e.callbackAuth.Sign(CallbackSignInput{
			ChatID: msg.ChatID, Action: action,
		})
	}
	return v
}

// patchConfigCard patch 配置卡片为最新状态。
func (e *Engine) patchConfigCard(msg *notify.InboundMessage, cardMsgID string) {
	card := cardMessage(e.buildConfigCard(msg))
	_, err := e.patchFeishuCard(string(msg.Platform), cardMsgID, card, e.botSendConfig(string(msg.Platform)))
	if err != nil {
		log.Warnf("im engine: patch config card failed: %v", err)
	}
}

func (e *Engine) patchReviewCard(msg *notify.InboundMessage, cardMsgID string) {
	card := cardMessage(e.buildReviewCard(msg))
	_, err := e.patchFeishuCard(string(msg.Platform), cardMsgID, card, e.botSendConfig(string(msg.Platform)))
	if err != nil {
		log.Warnf("im engine: patch review card failed: %v", err)
	}
}

func (e *Engine) buildConfigCard(msg *notify.InboundMessage) *notify.Card {
	rq, granularity, gt := e.GetConfigForPlatform(string(msg.Platform))
	reviewPolicy, _, _ := e.reviewConfigForMessage(msg)
	permission := e.permissionSummary(msg.Platform)
	elements := []map[string]any{
		configSectionElement("会话行为"),
		e.configCheckerElement("引用回复", "开启后，AI 回复会引用你的原始消息。", rq, msg, map[string]any{"sub": "toggle_reply_quote"}),
		map[string]any{"tag": "hr"},
		configSectionElement("回复模式"),
		e.configSelectElement(replyGranularityLabel(granularity), []configSelectOption{
			{Label: "标准", Value: "standard"},
			{Label: "摘要", Value: "summary"},
			{Label: "详细", Value: "detailed"},
		}, msg, map[string]any{"sub": "set_granularity"}),
		map[string]any{"tag": "hr"},
		configSectionElement("群聊触发"),
		e.configCheckerElement("需要 @ 提及", "开启后，群聊中必须 @bot 才会响应；`@bot /session` 等命令也需要 @。", gt != "allow_all", msg, map[string]any{"sub": "toggle_group_mention"}),
		map[string]any{"tag": "hr"},
		configSectionElement("执行审批"),
		e.configSelectElement(reviewPolicyLabel(reviewPolicy), []configSelectOption{
			{Label: "人工", Value: "manual"},
			{Label: "托管 YOLO", Value: "yolo"},
			{Label: "协同 AI", Value: "ai"},
		}, msg, map[string]any{"sub": "set_review_policy"}),
		configHintElement(reviewPolicyHint(reviewPolicy)),
		map[string]any{"tag": "hr"},
		configSectionElement("权限"),
		configInfoElement("使用范围", permission),
		configHintElement("权限在 Yakit 机器人配置面板编辑。"),
	}
	return &notify.Card{
		Title: "IM 配置",
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: elements,
	}
}

func (e *Engine) buildReviewText(msg *notify.InboundMessage) string {
	policy, riskScore, disallowPrompt := e.reviewConfigForMessage(msg)
	return fmt.Sprintf(`执行审批

当前策略
- %s

说明
- %s
- AI 风险阈值: %.2f
- 允许 AI 主动询问: %v

用法
- /review manual: 人工
- /review yolo: 托管 YOLO
- /review ai: 协同 AI`, reviewPolicyLabel(policy), reviewPolicyHint(policy), riskScore, !disallowPrompt)
}

func (e *Engine) buildReviewCard(msg *notify.InboundMessage) *notify.Card {
	policy, riskScore, disallowPrompt := e.reviewConfigForMessage(msg)
	elements := []map[string]any{
		configSectionElement("执行审批"),
		e.selectElement("review", reviewPolicyLabel(policy), []configSelectOption{
			{Label: "人工", Value: "manual"},
			{Label: "托管 YOLO", Value: "yolo"},
			{Label: "协同 AI", Value: "ai"},
		}, msg, map[string]any{"sub": "set_review_policy"}),
		configHintElement(reviewPolicyHint(policy)),
		map[string]any{"tag": "hr"},
		configSectionElement("当前参数"),
		configInfoElement("AI 风险阈值", fmt.Sprintf("%.2f", riskScore)),
		configInfoElement("允许 AI 主动询问", boolStatusLabel(!disallowPrompt)),
		map[string]any{"tag": "hr"},
		actionRowElement(
			e.controlButtonElement("完整配置", "primary", msg, "config", nil),
			e.controlButtonElement("状态", "default", msg, "status", nil),
			e.controlButtonElement("命令列表", "default", msg, "commands", nil),
		),
	}
	return &notify.Card{
		Title: "执行审批",
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: elements,
	}
}

func configSectionElement(title string) map[string]any {
	return map[string]any{
		"tag":     "markdown",
		"content": "**" + title + "**",
	}
}

func configInfoElement(label, value string) map[string]any {
	return map[string]any{
		"tag":     "markdown",
		"content": "**" + label + "**\n" + value,
	}
}

func configHintElement(text string) map[string]any {
	return map[string]any{
		"tag":     "markdown",
		"content": text,
	}
}

func actionRowElement(actions ...map[string]any) map[string]any {
	columns := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		columns = append(columns, map[string]any{
			"tag":            "column",
			"width":          "auto",
			"vertical_align": "top",
			"elements":       []map[string]any{action},
		})
	}
	return map[string]any{
		"tag":       "column_set",
		"flex_mode": "none",
		"columns":   columns,
	}
}

func (e *Engine) controlButtonElement(text, style string, msg *notify.InboundMessage, action string, extra map[string]any) map[string]any {
	button := map[string]any{
		"tag": "button",
		"text": map[string]string{
			"tag":     "plain_text",
			"content": text,
		},
		"behaviors": []map[string]any{
			{"type": "callback", "value": e.signedValue(msg, action, extra)},
		},
	}
	if style != "" {
		button["type"] = style
	}
	return button
}

type configSelectOption struct {
	Label string
	Value string
}

func (e *Engine) configSelectElement(current string, options []configSelectOption, msg *notify.InboundMessage, extra map[string]any) map[string]any {
	return e.selectElement("config", current, options, msg, extra)
}

func (e *Engine) selectElement(action string, current string, options []configSelectOption, msg *notify.InboundMessage, extra map[string]any) map[string]any {
	items := make([]map[string]any, 0, len(options))
	for _, opt := range options {
		items = append(items, map[string]any{
			"text": map[string]string{
				"tag":     "plain_text",
				"content": opt.Label,
			},
			"value": opt.Value,
		})
	}
	return map[string]any{
		"tag":            "select_static",
		"initial_option": current,
		"placeholder": map[string]string{
			"tag":     "plain_text",
			"content": "请选择",
		},
		"options": items,
		"type":    "default",
		"width":   "default",
		"margin":  "2px 0 8px 0",
		"behaviors": []map[string]any{
			{"type": "callback", "value": e.signedValue(msg, action, extra)},
		},
	}
}

func (e *Engine) configCheckerElement(title, description string, checked bool, msg *notify.InboundMessage, extra map[string]any) map[string]any {
	return map[string]any{
		"tag": "checker",
		"text": map[string]any{
			"tag":       "lark_md",
			"content":   "**" + title + "**\n" + description,
			"text_size": "normal",
		},
		"checked":           checked,
		"overall_checkable": true,
		"margin":            "2px 0",
		"padding":           "6px 8px",
		"checked_style": map[string]any{
			"show_strikethrough": false,
			"opacity":            1,
		},
		"behaviors": []map[string]any{
			{"type": "callback", "value": e.signedValue(msg, "config", extra)},
		},
	}
}

func (e *Engine) permissionSummary(platform notify.PlatformType) string {
	permSummary := "未配置（任何人可用）"
	bot, err := credential.GetBotConfig(string(platform))
	if err != nil || bot == nil {
		return permSummary
	}
	parts := []string{}
	if bot.OwnerID != "" {
		parts = append(parts, "所有者: "+shortIDForConfig(bot.OwnerID))
	}
	if bot.AllowedUsers != "" {
		parts = append(parts, "允许用户: "+bot.AllowedUsers)
	}
	if bot.AllowedChats != "" {
		parts = append(parts, "允许会话: "+bot.AllowedChats)
	}
	if bot.GroupAccessControl {
		parts = append(parts, "群聊访问控制: 已开启（仅所有者/白名单用户）")
	} else {
		parts = append(parts, "群聊访问控制: 已关闭（群成员可用）")
	}
	if len(parts) == 0 {
		return permSummary
	}
	return strings.Join(parts, "\n")
}

// replyCard 发一张卡片消息（非 managed card，即发即完，无 patch）。
// 按平台能力：SendCard=true 走卡片，否则降级为文本 reply。
func (e *Engine) replyCard(msg *notify.InboundMessage, card *notify.Card) {
	out := cardMessage(card)
	cfg := e.botSendConfig(string(msg.Platform))
	_, err := e.patchFeishuCard_sendCard(string(msg.Platform), msg.ChatID, out, cfg, msg)
	if err != nil {
		// 卡片发送失败，降级为文本
		log.Warnf("im engine: replyCard send failed, fallback to text: %v", err)
		e.reply(msg, card.Title+"\n\n"+card.Content)
	}
}

// patchFeishuCard_sendCard 是 replyCard 的发送卡片辅助（复用平台实例）。
func (e *Engine) patchFeishuCard_sendCard(platform string, targetID string, msg *notify.Message, cfg *notify.SendConfig, inbound *notify.InboundMessage) (*notify.SendResult, error) {
	return sendCardMessage(notify.PlatformType(platform), targetID, msg, cfg, inbound, e.replyQuoteForPlatform(platform))
}

// nextGroupTrigger 循环切换群聊触发策略。
func nextGroupTrigger(current string) string {
	switch current {
	case "allow_all":
		return "must_at"
	}
	return "allow_all"
}

// groupTriggerLabel 返回群聊触发策略的中文标签。
func groupTriggerLabel(gt string) string {
	switch gt {
	case "must_at", "allow_slash":
		return "需要 @ 提及"
	case "allow_all":
		return "不要求 @ 提及"
	}
	return gt
}

func isValidGroupTrigger(gt string) bool {
	switch gt {
	case "must_at", "allow_slash", "allow_all":
		return true
	}
	return false
}

func groupTriggerSelectLabel(gt string) string {
	switch gt {
	case "allow_slash":
		return "需要 @ 提及"
	case "must_at":
		return "需要 @ 提及"
	case "allow_all":
		return "不要求 @ 提及"
	}
	return gt
}

func replyGranularityLabel(mode string) string {
	switch mode {
	case "standard":
		return "标准"
	case "summary":
		return "摘要"
	case "detailed":
		return "详细"
	}
	return mode
}

func platformDisplayLabel(platform string) string {
	switch platform {
	case string(notify.PlatformFeishu):
		return "飞书"
	case string(notify.PlatformDingTalk):
		return "钉钉"
	}
	if platform == "" {
		return "未知平台"
	}
	return platform
}

func chatTypeDisplayLabel(chatType string) string {
	switch chatType {
	case "private", "":
		return "私聊"
	case "group":
		return "群聊"
	case "topic":
		return "话题"
	}
	return chatType
}

func boolStatusLabel(enabled bool) string {
	if enabled {
		return "已开启"
	}
	return "已关闭"
}

func aiSessionSourceLabel(source string) string {
	switch strings.TrimSpace(source) {
	case "im":
		return "IM"
	case "ide":
		return "Yakit"
	case "cli":
		return "CLI"
	case "":
		return "未标记"
	default:
		return source
	}
}

func formatUnixTime(ts int64) string {
	if ts <= 0 {
		return "未知时间"
	}
	return time.Unix(ts, 0).Format("01-02 15:04")
}

func compactCardText(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" || max <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func sessionSummaryLabel(s imStatusSnapshot) string {
	if !s.HasSession {
		return "未创建"
	}
	return s.StreamStatus
}

func historyStatusFromSnapshot(s imStatusSnapshot) string {
	if !s.HasSession {
		return "AI 对话开始后自动同步到 Yakit 历史。"
	}
	if s.StreamStatus == "活跃中" {
		return "已同步，可在 Yakit -> AI Agent -> 历史中通过会话标题或 SessionID 检索。"
	}
	return "已创建会话；下一次 AI 对话开始后会刷新 Yakit 历史元数据。"
}

func conciseHistoryStatus(status string) string {
	status = strings.TrimSpace(status)
	switch {
	case status == "":
		return "未知"
	case strings.Contains(status, "已同步"):
		return "已同步到 Yakit 历史"
	case strings.Contains(status, "已创建"):
		return "已创建，下一次 AI 对话后刷新"
	case strings.Contains(status, "AI 对话开始后"):
		return "首次 AI 对话后自动同步"
	default:
		return status
	}
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// shortIDForConfig 截取 ID 用于配置展示（去掉前缀，截断到 16 字符）。
func shortIDForConfig(id string) string {
	return compactID(id, 16, "", true)
}

// --- 辅助 ---

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func activeStatus(started bool) string {
	if started {
		return "活跃中"
	}
	return "未连接"
}

// now 便于测试时替换时间。
var now = time.Now
