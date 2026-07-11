// Package imengine 实现「IM 远程控制 yaklang」的常驻引擎。
//
// 它把已配置的 IM bot（飞书/钉钉，凭证来自 credential.BotConfig 扫码/手动落库）
// 接收到的入站消息，路由到两类处理：
//   - 斜杠命令（/help /new /stop /model /scan …）：在本地直接处理，控制会话/模型/执行。
//   - 普通文本：通过内部 AIReAct 双向流喂给 aid AI agent 执行，结果回发到 IM。
//
// 设计参考 cc-connect 的 Platform/Engine/Command 分层，但在 yaklang 内实现，
// agent 一端对接已有的 StartAIReAct gRPC 流（而非 Claude Code CLI）。
package imcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Engine 是 IM 远程控制的核心，常驻后台。
type Engine struct {
	// streamFactory 创建 AIReAct 双向流。
	streamFactory AIReActStreamFactory
	// sessionStore 查询和写入 AI Session 元数据。
	sessionStore AISessionStore
	// queryAISessionFunc 允许测试注入 AI Session 查询；为空时使用 sessionStore.QueryAISession。
	queryAISessionFunc func(context.Context, *ypb.QueryAISessionRequest) (*ypb.QueryAISessionResponse, error)

	mu        sync.Mutex
	sessions  map[string]*imSession     // sessionKey → 会话状态
	platforms map[string]*platformState // platform → 监听状态

	stateSeq           int64
	stateWatchers      map[int]chan *ypb.IMControlStateEvent
	nextStateWatcherID int

	recentInboundMu sync.Mutex
	recentInbound   map[string]time.Time // platform:chat:message_id → first seen time，用于抵御平台重投

	ctx    context.Context
	cancel context.CancelFunc

	// startedAt 是本次 IM 控制启动的水位线；平台补投早于该时间的普通消息会被忽略。
	startedAt time.Time

	// sessionIdleTimeout 会话空闲超时，超时后关闭 gRPC 流释放资源。
	sessionIdleTimeout time.Duration

	// platformsFilter 指定只监听这些平台；空 = 所有已配置且 Enabled 的 bot。
	platformsFilter []string

	// replyQuote bot 回复时是否引用用户消息（false=普通发送）。
	replyQuote bool
	// replyGranularity 回复颗粒度：standard / summary / detailed。
	replyGranularity string
	// groupTrigger 群聊触发策略：must_at（默认）/ allow_all。
	// must_at：群消息必须 @bot 才触发，包括 /session 等命令。
	// allow_all：所有消息触发。allow_slash 仅作为历史配置兼容值保留。
	groupTrigger string
	// reviewPolicy 控制 AI agent 执行敏感动作时的审批策略：manual / ai / yolo。
	reviewPolicy string
	// aiReviewRiskControlScore 是 reviewPolicy=ai 时的风险阈值。
	aiReviewRiskControlScore float64
	// disallowRequireForUserPrompt 控制 agent 是否禁止主动向 IM 用户提问。
	disallowRequireForUserPrompt bool
	// platformConfigs 按平台覆盖运行配置，避免飞书/钉钉机器人设置互相串台。
	platformConfigs map[string]RuntimePlatformConfig

	// callbackAuth 卡片按钮回调的签名验签器（Phase 3）。Start 时初始化。
	callbackAuth *CallbackAuth

	started bool
}

// platformState 记录一个平台的监听状态。
type platformState struct {
	platform   string
	configured bool
	enabled    bool
	connected  bool
	message    string
	updatedAt  time.Time
	cancel     context.CancelFunc
}

// Config 是构造 Engine 的参数。
type Config struct {
	Platforms                    []string // 只监听这些平台；空 = 所有已配置且 Enabled 的 bot
	SessionIdleTimeoutSeconds    int      // 会话空闲超时秒数（默认 1800）
	ReplyQuote                   bool     // bot 回复时是否引用用户消息（默认 true）
	ReplyGranularity             string   // 回复颗粒度：standard / summary / detailed（默认 standard）
	GroupTrigger                 string   // 群聊触发策略：must_at / allow_all（默认 must_at；allow_slash 兼容旧配置）
	ReviewPolicy                 string   // 执行审批策略：manual / ai / yolo（默认 yolo）
	AIReviewRiskControlScore     float64  // AI 审批风险阈值（默认 0.5）
	DisallowRequireForUserPrompt bool     // 是否禁止 agent 主动询问 IM 用户（默认 false）
	PlatformConfigs              map[string]RuntimePlatformConfig
}

type RuntimePlatformConfig struct {
	Platform                     string
	ReplyQuote                   bool
	ReplyGranularity             string
	GroupTrigger                 string
	ReviewPolicy                 string
	AIReviewRiskControlScore     float64
	DisallowRequireForUserPrompt bool
}

const (
	inboundDedupeTTL            = 10 * time.Minute
	inboundPersistentDedupeTTL  = 24 * time.Hour
	inboundMaxMessageAge        = 5 * time.Minute
	inboundStartupSkewAllowance = 30 * time.Second
	inboundStaleLogTimeLayout   = "2006-01-02 15:04:05"
)

// New 构造一个未启动的 IM Engine。需要后续调 Start 才会开始监听。
func New(cfg Config) *Engine {
	idle := time.Duration(cfg.SessionIdleTimeoutSeconds) * time.Second
	if cfg.SessionIdleTimeoutSeconds <= 0 {
		idle = 30 * time.Minute
	}
	granularity := cfg.ReplyGranularity
	if granularity == "" {
		granularity = "standard"
	}
	groupTrigger := cfg.GroupTrigger
	if groupTrigger == "" {
		groupTrigger = "must_at"
	}
	reviewPolicy := normalizeReviewPolicy(cfg.ReviewPolicy)
	riskScore := normalizeAIReviewRiskControlScore(cfg.AIReviewRiskControlScore)
	return &Engine{
		sessions:                     map[string]*imSession{},
		platforms:                    map[string]*platformState{},
		stateWatchers:                map[int]chan *ypb.IMControlStateEvent{},
		recentInbound:                map[string]time.Time{},
		sessionIdleTimeout:           idle,
		platformsFilter:              cfg.Platforms,
		replyQuote:                   cfg.ReplyQuote, // false 时不用引用回复；默认在 grpc 层设 true
		replyGranularity:             granularity,
		groupTrigger:                 groupTrigger,
		reviewPolicy:                 reviewPolicy,
		aiReviewRiskControlScore:     riskScore,
		disallowRequireForUserPrompt: cfg.DisallowRequireForUserPrompt,
		platformConfigs:              normalizeRuntimePlatformConfigs(cfg.PlatformConfigs, cfg),
	}
}

func normalizeRuntimePlatformConfigs(raw map[string]RuntimePlatformConfig, fallback Config) map[string]RuntimePlatformConfig {
	if len(raw) == 0 {
		return map[string]RuntimePlatformConfig{}
	}
	out := make(map[string]RuntimePlatformConfig, len(raw))
	for key, cfg := range raw {
		if cfg.Platform == "" {
			cfg.Platform = key
		}
		platform := normalizePlatformConfigKey(cfg.Platform)
		if platform == "" {
			continue
		}
		out[platform] = normalizeRuntimePlatformConfig(cfg, fallback)
	}
	return out
}

func normalizeRuntimePlatformConfig(cfg RuntimePlatformConfig, fallback Config) RuntimePlatformConfig {
	platform := normalizePlatformConfigKey(cfg.Platform)
	granularity := strings.TrimSpace(cfg.ReplyGranularity)
	if granularity == "" {
		granularity = strings.TrimSpace(fallback.ReplyGranularity)
	}
	if granularity == "" {
		granularity = "standard"
	}
	groupTrigger := normalizeRuntimeGroupTrigger(cfg.GroupTrigger)
	if cfg.GroupTrigger == "" {
		groupTrigger = normalizeRuntimeGroupTrigger(fallback.GroupTrigger)
	}
	policy := normalizeReviewPolicy(cfg.ReviewPolicy)
	if strings.TrimSpace(cfg.ReviewPolicy) == "" {
		policy = normalizeReviewPolicy(fallback.ReviewPolicy)
	}
	riskScore := normalizeAIReviewRiskControlScore(cfg.AIReviewRiskControlScore)
	if cfg.AIReviewRiskControlScore <= 0 {
		riskScore = normalizeAIReviewRiskControlScore(fallback.AIReviewRiskControlScore)
	}
	return RuntimePlatformConfig{
		Platform:                     platform,
		ReplyQuote:                   cfg.ReplyQuote,
		ReplyGranularity:             granularity,
		GroupTrigger:                 groupTrigger,
		ReviewPolicy:                 policy,
		AIReviewRiskControlScore:     riskScore,
		DisallowRequireForUserPrompt: cfg.DisallowRequireForUserPrompt,
	}
}

func normalizeRuntimeGroupTrigger(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "allow_all":
		return "allow_all"
	case "must_at", "allow_slash":
		return "must_at"
	}
	return "must_at"
}

func normalizePlatformConfigKey(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

// SetAIBackend 注入 AI Agent 流工厂和 AI Session 存储入口。
func (e *Engine) SetAIBackend(factory AIReActStreamFactory, store AISessionStore) {
	e.streamFactory = factory
	e.sessionStore = store
}

// Start 启动 IM Engine：校验 AI 后端 + 对每个已配置 bot 启动 Receive 循环。
// 返回启动结果消息。
func (e *Engine) Start() error {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return fmt.Errorf("im engine already started")
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.startedAt = now()
	e.mu.Unlock()

	// 0) 初始化卡片回调签名验签器（Phase 3）
	if e.callbackAuth == nil {
		e.callbackAuth = NewCallbackAuth(nil) // nil 时内部派生默认密钥
	}

	// 1) 校验 AI 后端。
	if e.streamFactory == nil {
		if e.cancel != nil {
			e.cancel()
		}
		return fmt.Errorf("ai agent stream factory is not configured")
	}

	// 2) 加载已配置的 bot 凭证
	bots, err := credential.ListBotConfigs()
	if err != nil {
		return fmt.Errorf("list bot configs: %w", err)
	}

	started := 0
	for _, bot := range bots {
		if !bot.Enabled {
			continue
		}
		platform := notify.PlatformType(bot.Platform)
		if !e.platformAllowed(bot.Platform) {
			continue
		}
		// 只有声明 events:receive 的平台才能收消息（飞书/钉钉）。
		if !isPlatformReceivable(platform) {
			continue
		}
		if err := e.startReceive(platform, bot); err != nil {
			log.Errorf("im engine: start receive for %s failed: %v", platform, err)
			e.setPlatformState(string(platform), false, err.Error())
			continue
		}
		started++
	}

	if started == 0 {
		e.mu.Lock()
		e.started = true
		e.broadcastStateLocked("engine_started_without_platform")
		e.mu.Unlock()
		return fmt.Errorf("no receivable IM platform started (configured: %d bots, filter: %v)", len(bots), e.platformsFilter)
	}

	e.mu.Lock()
	e.started = true
	e.broadcastStateLocked("engine_started")
	e.mu.Unlock()
	log.Infof("im engine started: %d platform(s) listening", started)

	// 启动会话空闲回收
	go e.idleReaper()
	return nil
}

// startReceive 为一个平台启动长连接接收循环。
func (e *Engine) startReceive(platform notify.PlatformType, bot *credential.BotConfig) error {
	sendCfg := bot.ToSendConfig()
	desc, err := descriptorForPlatform(notify.Platform(platform))
	if err != nil {
		return err
	}
	reg := notify.NewRegistry()
	reg.Register(desc)
	client := notify.NewClient(notify.WithRegistry(reg), notify.WithSendConfig(sendCfg))

	ctx, cancel := context.WithCancel(e.ctx)
	e.setPlatformState(string(platform), false, "connecting")
	e.setPlatformCancel(string(platform), cancel)

	go func() {
		defer cancel()
		err := client.Stream(ctx, &notify.Request{
			Platform: notify.Platform(platform),
			Action:   notify.ActionEventsReceive,
		}, func(ev notify.Event) {
			eventPlatform := string(ev.Platform)
			if eventPlatform == "" {
				eventPlatform = string(platform)
			}
			switch ev.Type {
			case notify.EventConnected:
				e.setPlatformState(eventPlatform, true, "connected")
				return
			case notify.EventError:
				message := "reconnecting"
				if ev.Err != nil {
					message = fmt.Sprintf("reconnecting: %v", ev.Err)
				}
				e.setPlatformState(eventPlatform, false, message)
				return
			}
			if ev.Message != nil {
				e.handleMessage(ev.Message)
			}
		})
		if err != nil && err != context.Canceled {
			log.Warnf("im engine: %s receive loop exited: %v", platform, err)
			e.setPlatformState(string(platform), false, fmt.Sprintf("disconnected: %v", err))
		} else {
			e.setPlatformState(string(platform), false, "stopped")
		}
	}()
	return nil
}

// handleMessage 是所有平台入站消息的统一入口，由 notify event stream 回调。
// 既处理普通文本/斜杠命令，也处理卡片按钮回调（IsCardAction）。
func (e *Engine) handleMessage(msg *notify.InboundMessage) {
	if msg == nil {
		return
	}

	// 卡片按钮回调：解析 action.value → IMAction → handleAction（不走 agent）
	if msg.IsCardAction {
		e.handleCardAction(msg)
		return
	}

	content := strings.TrimSpace(msg.Text)
	messageID := inboundDedupeMessageID(msg)
	eventTime, eventAge := inboundEventTimeForLog(msg)
	log.Infof("im engine: inbound from %s chat=%s sender=%s msg_id=%s event_time=%s age=%s len=%d attachments=%d",
		msg.Platform, msg.ChatID, msg.SenderID, messageID, eventTime, eventAge, len(content), len(msg.Attachments))
	// 纯文本无附件时跳过；有附件（如纯图片消息）即使文本为空也继续
	if content == "" && len(msg.Attachments) == 0 {
		return
	}
	if e.shouldSkipStaleInbound(msg) {
		return
	}
	if e.shouldSkipDuplicateInbound(msg, messageID) {
		return
	}
	e.tryBackfillOwnerFromPrivateMessage(msg)

	// 先定位会话，让权限判断能识别群聊/话题场景；群聊会话按 chat/thread 共享。
	sessionKey := imSessionKey(msg)
	e.touchSession(sessionKey, msg)

	// 权限校验（普通消息入口与卡片回调入口一致）
	if ok, reason := e.checkPermission(string(msg.Platform), msg.ChatID, msg.SenderID); !ok {
		e.replyRecovery(msg, "无权限", "无权限："+reason+"\n\n权限范围可在 Yakit 机器人配置面板调整。")
		return
	}

	// 群聊触发策略过滤（私聊不受限）
	if !e.shouldTriggerInGroup(msg, content) {
		return
	}
	content = normalizeGroupMentionedContent(msg, content)
	if content == "" && len(msg.Attachments) == 0 {
		return
	}

	// 更新会话活跃时间
	e.touchSession(sessionKey, msg)

	// 给用户消息加 ✅ 表情回应，提供「已收到、正在处理」的即时反馈（异步，不阻塞主流程）
	go e.addAckReaction(msg)

	// 斜杠命令优先
	if strings.HasPrefix(content, "/") {
		if e.handleCommand(msg, content) {
			return
		}
		// 未识别的斜杠命令 fall through 到 agent（与 cc-connect 一致）
	}

	// 普通消息 → 喂给 AI agent
	e.dispatchToAgent(msg, content)
}

func (e *Engine) tryBackfillOwnerFromPrivateMessage(msg *notify.InboundMessage) {
	if msg == nil {
		return
	}
	senderID := strings.TrimSpace(msg.SenderID)
	if senderID == "" || e.chatTypeForAccess(msg) != "private" {
		return
	}
	bot, err := credential.GetBotConfig(string(msg.Platform))
	if err != nil || bot == nil {
		return
	}
	if strings.TrimSpace(bot.OwnerID) != "" || strings.TrimSpace(bot.AllowedUsers) != "" {
		return
	}
	bot.OwnerID = senderID
	saved, err := credential.SaveBotConfig(bot)
	if err != nil {
		log.Warnf("im engine: backfill bot owner failed platform=%s sender=%s: %v", msg.Platform, senderID, err)
		return
	}
	log.Infof("im engine: backfilled bot owner platform=%s owner=%s", msg.Platform, shortIDForConfig(senderID))
	go func() {
		if err := e.SendOnboardingWelcomeReply(saved, msg); err != nil {
			log.Warnf("im engine: send onboarding welcome after owner backfill failed platform=%s owner=%s: %v", msg.Platform, shortIDForConfig(senderID), err)
		}
	}()
}

func (e *Engine) shouldSkipDuplicateInbound(msg *notify.InboundMessage, messageID string) bool {
	if msg == nil || msg.IsCardAction {
		return false
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	key := fmt.Sprintf("%s:%s:%s", msg.Platform, msg.ChatID, messageID)
	nowTime := now()

	e.recentInboundMu.Lock()
	for k, seenAt := range e.recentInbound {
		if nowTime.Sub(seenAt) > inboundDedupeTTL {
			delete(e.recentInbound, k)
		}
	}
	if _, ok := e.recentInbound[key]; ok {
		log.Warnf("im engine: duplicate inbound ignored platform=%s chat=%s sender=%s msg_id=%s",
			msg.Platform, msg.ChatID, msg.SenderID, messageID)
		e.recentInboundMu.Unlock()
		return true
	}
	e.recentInboundMu.Unlock()

	if duplicated, err := e.markPersistentInboundSeen(msg, messageID, nowTime); err != nil {
		log.Warnf("im engine: persistent inbound dedupe failed platform=%s chat=%s sender=%s msg_id=%s: %v",
			msg.Platform, msg.ChatID, msg.SenderID, messageID, err)
	} else if duplicated {
		log.Warnf("im engine: persisted duplicate inbound ignored platform=%s chat=%s sender=%s msg_id=%s",
			msg.Platform, msg.ChatID, msg.SenderID, messageID)
		e.recentInboundMu.Lock()
		e.recentInbound[key] = nowTime
		e.recentInboundMu.Unlock()
		return true
	}

	e.recentInboundMu.Lock()
	e.recentInbound[key] = nowTime
	e.recentInboundMu.Unlock()
	return false
}

func (e *Engine) shouldSkipStaleInbound(msg *notify.InboundMessage) bool {
	if msg == nil || msg.IsCardAction || msg.EventTime.IsZero() {
		return false
	}
	e.mu.Lock()
	startedAt := e.startedAt
	e.mu.Unlock()
	if !startedAt.IsZero() {
		watermark := startedAt.Add(-inboundStartupSkewAllowance)
		if msg.EventTime.Before(watermark) {
			log.Warnf("im engine: stale inbound ignored platform=%s chat=%s sender=%s msg_id=%s event_time=%s started_at=%s",
				msg.Platform, msg.ChatID, msg.SenderID, inboundDedupeMessageID(msg),
				msg.EventTime.Format(inboundStaleLogTimeLayout), startedAt.Format(inboundStaleLogTimeLayout))
			return true
		}
	}
	age := now().Sub(msg.EventTime)
	if age <= inboundMaxMessageAge {
		return false
	}
	log.Warnf("im engine: old inbound ignored platform=%s chat=%s sender=%s msg_id=%s event_time=%s age=%s max_age=%s",
		msg.Platform, msg.ChatID, msg.SenderID, inboundDedupeMessageID(msg),
		msg.EventTime.Format(inboundStaleLogTimeLayout), age.Round(time.Second), inboundMaxMessageAge)
	return true
}

func inboundEventTimeForLog(msg *notify.InboundMessage) (string, string) {
	if msg == nil || msg.EventTime.IsZero() {
		return "", ""
	}
	return msg.EventTime.Format(inboundStaleLogTimeLayout), now().Sub(msg.EventTime).Round(time.Second).String()
}

func inboundDedupeMessageID(msg *notify.InboundMessage) string {
	if msg == nil || msg.ReplyContext == nil {
		return ""
	}
	switch msg.Platform {
	case notify.PlatformFeishu:
		if s, ok := msg.ReplyContext.(string); ok {
			return strings.TrimSpace(s)
		}
	case notify.PlatformDingTalk:
		return extractStringFieldFromJSON(msg.ReplyContext, "MsgID", "msgId", "msg_id", "openMsgId", "OpenMsgID")
	}
	return ""
}

func extractStringFieldFromJSON(v any, keys ...string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return ""
	}
	for _, key := range keys {
		if s, _ := m[key].(string); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// handleCardAction 处理卡片按钮回调。验签 + 权限校验后，从 action.value 取 action 类型，
// 构造 IMAction（与命令共用 handleAction），保证「卡片停止按钮」和「/stop」走同一逻辑。
func (e *Engine) handleCardAction(msg *notify.InboundMessage) {
	startedAt := time.Now()
	actionStr, _ := msg.ActionValue["action"].(string)
	if actionStr == "" {
		log.Warnf("im engine: card action rejected platform=%s chat=%s sender=%s reason=missing_action value=%v",
			msg.Platform, msg.ChatID, msg.SenderID, msg.ActionValue)
		e.replyRecovery(msg, "未知操作", "按钮回调缺少 action 字段。\n\n请重新输入 /help 打开控制入口。")
		return
	}
	runID, _ := msg.ActionValue["run_id"].(string)
	subAction, _ := msg.ActionValue["sub"].(string)
	sessionID, _ := msg.ActionValue["session_id"].(string)
	outcome := "handled"
	defer func() {
		log.Infof("im engine: card action finished platform=%s chat=%s sender=%s action=%s sub=%s run_id=%s session_id=%s outcome=%s cost=%s",
			msg.Platform, msg.ChatID, msg.SenderID, actionStr, subAction, runID, sessionID, outcome, time.Since(startedAt))
	}()
	log.Infof("im engine: card action received platform=%s chat=%s sender=%s action=%s sub=%s run_id=%s session_id=%s",
		msg.Platform, msg.ChatID, msg.SenderID, actionStr, subAction, runID, sessionID)

	applyCardActionSessionHints(msg)
	// 卡片回调也要定位会话（用于 stop 等需要操作当前 run 的动作）
	sessionKey := imSessionKey(msg)
	if actionStr != string(ActionReviewDecision) || e.sessionExists(sessionKey) {
		e.touchSession(sessionKey, msg)
	}
	e.tryBackfillOwnerFromPrivateMessage(msg)

	// 先做权限校验，再验签。stop 等 one-shot token 会在验签时消费 nonce；
	// 无权限用户点击群聊卡片时不能消耗 owner 后续操作所需的 token。
	if ok, reason := e.checkPermission(string(msg.Platform), msg.ChatID, msg.SenderID); !ok {
		outcome = "permission_denied"
		log.Warnf("im engine: card action permission denied platform=%s chat=%s sender=%s action=%s reason=%s",
			msg.Platform, msg.ChatID, msg.SenderID, actionStr, reason)
		e.replyRecovery(msg, "无权限", "无权限："+reason+"\n\n权限范围可在 Yakit 机器人配置面板调整。")
		return
	}

	// 1. 验签（防伪造 token）
	if e.callbackAuth != nil {
		token, _ := msg.ActionValue["token"].(string)
		if token == "" {
			outcome = "auth_missing_token"
			log.Warnf("im engine: card action auth failed platform=%s chat=%s sender=%s action=%s reason=missing_token",
				msg.Platform, msg.ChatID, msg.SenderID, actionStr)
			e.reply(msg, "❌ 按钮校验失败：缺少签名 token")
			return
		}
		result := e.callbackAuth.Verify(token, CallbackVerifyExpected{
			RunID:  runID,
			ChatID: msg.ChatID,
			Action: actionStr,
		})
		if !result.OK {
			outcome = "auth_" + result.Reason
			log.Warnf("im engine: card action auth failed platform=%s chat=%s sender=%s action=%s reason=%s",
				msg.Platform, msg.ChatID, msg.SenderID, actionStr, result.Reason)
			e.reply(msg, fmt.Sprintf("❌ 按钮校验失败：%s", result.Reason))
			return
		}
	}

	// update_reply_mode 的 mode 参数从 ActionValue 取
	var args []string
	if mode, ok := msg.ActionValue["mode"].(string); ok && mode != "" {
		args = []string{mode}
	}
	if policy, ok := msg.ActionValue["policy"].(string); ok && policy != "" {
		args = []string{policy}
	}
	if len(args) == 0 {
		if option, ok := msg.ActionValue["option"].(string); ok && option != "" && actionStr == string(ActionReview) {
			args = []string{option}
		}
	}
	if !knownIMAction(IMActionType(actionStr)) {
		outcome = "unknown_action"
		log.Warnf("im engine: card action unknown platform=%s chat=%s sender=%s action=%s value=%v",
			msg.Platform, msg.ChatID, msg.SenderID, actionStr, msg.ActionValue)
	}
	e.handleAction(IMAction{
		Type:   IMActionType(actionStr),
		Source: "card",
		Msg:    msg,
		RunID:  runID,
		Args:   args,
	})
}

func applyCardActionSessionHints(msg *notify.InboundMessage) {
	if msg == nil || !msg.IsCardAction {
		return
	}
	if msg.ChatType == "" {
		msg.ChatType = actionValueString(msg.ActionValue, "chat_type")
	}
	if msg.ThreadID == "" {
		msg.ThreadID = actionValueString(msg.ActionValue, "thread_id")
	}
}

func (e *Engine) sessionExists(sessionKey string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessions[sessionKey] != nil
}

// addAckReaction 给入站消息加一个表情回应（如果平台支持）。
// 飞书：用 emoji 短标识 "OK"（👍），通过 im/v1/messages/{id}/reactions API。
// 钉钉：用自定义文字表情 "开始任务"（通过 /v1.0/robot/emotion/reply API，需 replyContext JSON 含 msgId）。
func (e *Engine) addAckReaction(msg *notify.InboundMessage) {
	// 钉钉 ReplyContext 是 replyContext 结构体（含 msgId），飞书是 string（om_xxx）。
	// 都通过 msgIDToString 序列化成字符串供平台实现使用。
	msgID := msgIDToString(msg.ReplyContext)
	if msgID == "" {
		return
	}
	caps := platformCapabilities(msg.Platform)
	if !caps.Reactions {
		return // 平台不支持 reaction，静默跳过
	}
	bot, err := credential.GetBotConfig(string(msg.Platform))
	if err != nil || bot == nil {
		return
	}
	emojiType := "OK" // 飞书 emoji: OK = 👍
	if msg.Platform == notify.PlatformDingTalk {
		emojiType = "开始任务" // 钉钉自定义文字表情
	}
	err = addReaction(msg.Platform, msgID, emojiType, bot.ToSendConfig())
	if err != nil {
		log.Warnf("im engine: add reaction failed for %s: %v", msg.Platform, err)
	}
}

// UpdateConfig 热更新回复配置（replyQuote / replyGranularity / groupTrigger），无需重启 IM Engine 或断开长连接。
// 用于前端切换「转发回复」开关、「回复颗粒度」下拉框、「群聊触发策略」时即时生效。
func (e *Engine) UpdateConfig(replyQuote bool, granularity string) {
	e.UpdateConfigForPlatform("", replyQuote, granularity)
}

func (e *Engine) UpdateConfigForPlatform(platform string, replyQuote bool, granularity string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	platform = normalizePlatformConfigKey(platform)
	if platform != "" {
		cfg := e.configForPlatformLocked(platform)
		cfg.Platform = platform
		cfg.ReplyQuote = replyQuote
		if granularity != "" {
			cfg.ReplyGranularity = granularity
		}
		e.platformConfigs[platform] = cfg
		log.Infof("im engine: platform config updated platform=%s replyQuote=%v granularity=%s", platform, cfg.ReplyQuote, cfg.ReplyGranularity)
		return
	}
	e.replyQuote = replyQuote
	if granularity != "" {
		e.replyGranularity = granularity
	}
	log.Infof("im engine: config updated (replyQuote=%v, granularity=%s)", replyQuote, e.replyGranularity)
}

// UpdateGroupTrigger 热更新群聊触发策略。
func (e *Engine) UpdateGroupTrigger(strategy string) {
	e.UpdateGroupTriggerForPlatform("", strategy)
}

func (e *Engine) UpdateGroupTriggerForPlatform(platform, strategy string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	platform = normalizePlatformConfigKey(platform)
	if strategy != "" {
		strategy = normalizeRuntimeGroupTrigger(strategy)
		if platform != "" {
			cfg := e.configForPlatformLocked(platform)
			cfg.Platform = platform
			cfg.GroupTrigger = strategy
			e.platformConfigs[platform] = cfg
			log.Infof("im engine: platform group trigger updated platform=%s strategy=%s", platform, cfg.GroupTrigger)
			return
		}
		e.groupTrigger = strategy
	}
	log.Infof("im engine: group trigger updated to %s", e.groupTrigger)
}

// GetConfig 返回当前回复配置快照（供 /config 卡片展示）。
func (e *Engine) GetConfig() (replyQuote bool, granularity, groupTrigger string) {
	return e.GetConfigForPlatform("")
}

func (e *Engine) GetConfigForPlatform(platform string) (replyQuote bool, granularity, groupTrigger string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.configForPlatformLocked(platform)
	return cfg.ReplyQuote, cfg.ReplyGranularity, cfg.GroupTrigger
}

func (e *Engine) configForPlatformLocked(platform string) RuntimePlatformConfig {
	cfg := RuntimePlatformConfig{
		ReplyQuote:                   e.replyQuote,
		ReplyGranularity:             e.replyGranularity,
		GroupTrigger:                 normalizeRuntimeGroupTrigger(e.groupTrigger),
		ReviewPolicy:                 e.reviewPolicy,
		AIReviewRiskControlScore:     e.aiReviewRiskControlScore,
		DisallowRequireForUserPrompt: e.disallowRequireForUserPrompt,
	}
	platform = normalizePlatformConfigKey(platform)
	if platform == "" {
		return cfg
	}
	if platformCfg, ok := e.platformConfigs[platform]; ok {
		platformCfg.Platform = platform
		return platformCfg
	}
	return cfg
}

func (e *Engine) replyQuoteForPlatform(platform string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.configForPlatformLocked(platform).ReplyQuote
}

func (e *Engine) replyGranularityForPlatform(platform string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.configForPlatformLocked(platform).ReplyGranularity
}

// shouldTriggerInGroup 按 groupTrigger 策略判断群聊消息是否应触发 bot。
// 私聊（chatType=private 或空）始终触发；群聊/话题按策略过滤。
func (e *Engine) shouldTriggerInGroup(msg *notify.InboundMessage, content string) bool {
	if msg.ChatType != "group" && msg.ChatType != "topic" {
		return true // 私聊始终触发
	}
	_, _, strategy := e.GetConfigForPlatform(string(msg.Platform))
	switch strategy {
	case "allow_all":
		return true
	case "must_at", "allow_slash":
		return msg.MentionBot
	}
	return msg.MentionBot
}

func normalizeGroupMentionedContent(msg *notify.InboundMessage, content string) string {
	content = strings.TrimSpace(content)
	if msg == nil || !isGroupChatType(msg.ChatType) || !msg.MentionBot {
		return content
	}
	if strings.HasPrefix(content, "<at") {
		if idx := strings.Index(content, "</at>"); idx >= 0 {
			return strings.TrimSpace(content[idx+len("</at>"):])
		}
	}
	if strings.HasPrefix(content, "@") {
		parts := strings.Fields(content)
		if len(parts) > 1 {
			return strings.TrimSpace(strings.Join(parts[1:], " "))
		}
		return ""
	}
	return content
}

// RestartPlatform 用数据库里的最新凭证重启指定平台的监听。
// 用于 bot 凭证变更（重新扫码/手动修改）后刷新长连接，无需重启整个 IM Engine。
// 如果该平台没在监听，则启动它；如果 bot 配置被删除或禁用，则停止监听。
func (e *Engine) RestartPlatform(platform string) error {
	if platform == "" {
		return fmt.Errorf("platform is required")
	}
	p := notify.PlatformType(platform)

	e.mu.Lock()
	if !e.started {
		e.mu.Unlock()
		return fmt.Errorf("im engine not started")
	}
	// 停掉该平台的旧监听
	if ps, ok := e.platforms[platform]; ok && ps.cancel != nil {
		ps.cancel()
		ps.connected = false
		ps.message = "restarting"
		ps.updatedAt = now()
		e.broadcastStateLocked("platform_restarting")
	}
	e.mu.Unlock()

	// 从数据库取最新凭证
	bot, err := credential.GetBotConfig(platform)
	if err != nil {
		return fmt.Errorf("get bot config: %w", err)
	}
	if bot == nil || !bot.Enabled {
		// bot 被删除或禁用，只停止不重启
		log.Infof("im engine: platform %s stopped (bot removed or disabled)", platform)
		e.setPlatformState(platform, false, "bot removed or disabled")
		return nil
	}
	if !isPlatformReceivable(p) {
		return fmt.Errorf("platform %q is not receivable", platform)
	}

	// 用新凭证重启监听
	if err := e.startReceive(p, bot); err != nil {
		return fmt.Errorf("restart receive: %w", err)
	}
	log.Infof("im engine: platform %s restarted with new credentials", platform)
	return nil
}

// Stop 停止 IM Engine，关闭所有平台监听和会话流。
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.started {
		e.mu.Unlock()
		return
	}
	e.started = false
	ctxCancel := e.cancel
	// 取消所有平台监听
	for _, ps := range e.platforms {
		if ps.cancel != nil {
			ps.cancel()
		}
		ps.connected = false
		ps.message = "stopped"
		ps.updatedAt = now()
	}
	// 关闭所有会话流
	for key, sess := range e.sessions {
		sess.closeStream()
		delete(e.sessions, key)
	}
	e.broadcastStateLocked("engine_stopped")
	e.closeStateWatchersLocked()
	e.mu.Unlock()

	if ctxCancel != nil {
		ctxCancel()
	}
	log.Info("im engine stopped")
}

// State 返回当前 IM Engine 的运行状态快照。
func (e *Engine) State() *ypb.IMControlState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stateLocked()
}

// SubscribeState 订阅 IM Engine 状态变化。订阅建立后会立即返回一份完整快照；
// 后续每次状态变化继续推送完整快照，避免前端维护增量合并状态。
func (e *Engine) SubscribeState() (int, <-chan *ypb.IMControlStateEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := e.nextStateWatcherID
	e.nextStateWatcherID++
	ch := make(chan *ypb.IMControlStateEvent, 8)
	e.stateWatchers[id] = ch
	ch <- e.stateEventLocked("snapshot")
	return id, ch
}

func (e *Engine) UnsubscribeState(id int) { e.removeStateWatcher(id) }

func (e *Engine) stateLocked() *ypb.IMControlState {
	resp := &ypb.IMControlState{
		Running:            e.started,
		ActiveSessionCount: int32(len(e.sessions)),
	}
	for _, ps := range e.platforms {
		updatedAt := ps.updatedAt
		if updatedAt.IsZero() {
			updatedAt = now()
		}
		resp.Platforms = append(resp.Platforms, &ypb.IMControlPlatformState{
			Platform:        ps.platform,
			Label:           platformStateLabel(ps.platform),
			Configured:      ps.configured,
			Enabled:         ps.enabled,
			Connected:       ps.connected,
			Transport:       platformStateTransport(ps.platform),
			Level:           platformStateLevel(ps.connected, ps.message),
			Message:         ps.message,
			UpdatedAtUnixMs: updatedAt.UnixMilli(),
		})
	}
	for _, sess := range e.sessions {
		resp.Sessions = append(resp.Sessions, &ypb.IMControlSessionInfo{
			SessionKey:   sess.sessionKey,
			Platform:     sess.platform,
			ChatID:       sess.chatID,
			SenderID:     sess.senderID,
			LastActiveAt: sess.lastActiveAt.Unix(),
			CurrentModel: sess.currentModel,
			ChatType:     sess.chatType,
			ChatTitle:    sess.chatTitle,
			SenderName:   sess.senderName,
			ThreadID:     sess.threadID,
		})
	}
	return resp
}

func (e *Engine) stateEventLocked(reason string) *ypb.IMControlStateEvent {
	return &ypb.IMControlStateEvent{
		Sequence:        e.stateSeq,
		TimestampUnixMs: now().UnixMilli(),
		Reason:          reason,
		State:           e.stateLocked(),
	}
}

func (e *Engine) broadcastStateLocked(reason string) {
	if len(e.stateWatchers) == 0 {
		e.stateSeq++
		return
	}
	e.stateSeq++
	event := e.stateEventLocked(reason)
	for _, ch := range e.stateWatchers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (e *Engine) closeStateWatchersLocked() {
	for id, ch := range e.stateWatchers {
		delete(e.stateWatchers, id)
		close(ch)
	}
}

func (e *Engine) removeStateWatcher(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch, ok := e.stateWatchers[id]
	if !ok {
		return
	}
	delete(e.stateWatchers, id)
	close(ch)
}

func platformStateLabel(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case string(notify.PlatformFeishu), "lark":
		return "飞书"
	case string(notify.PlatformDingTalk):
		return "钉钉"
	default:
		if strings.TrimSpace(platform) == "" {
			return "IM"
		}
		return platform
	}
}

func platformStateTransport(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case string(notify.PlatformFeishu), "lark":
		return "WebSocket"
	case string(notify.PlatformDingTalk):
		return "Stream"
	default:
		return "Stream"
	}
}

func platformStateLevel(connected bool, message string) string {
	if connected {
		return "ok"
	}
	msg := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(msg, "stopped"), strings.Contains(msg, "disabled"), strings.Contains(msg, "removed"):
		return "disabled"
	case msg == "", strings.Contains(msg, "connecting"), strings.Contains(msg, "reconnecting"), strings.Contains(msg, "starting"):
		return "warning"
	default:
		return "error"
	}
}

// --- 内部辅助 ---

func (e *Engine) platformAllowed(platform string) bool {
	if len(e.platformsFilter) == 0 {
		return true
	}
	for _, p := range e.platformsFilter {
		if strings.EqualFold(p, platform) {
			return true
		}
	}
	return false
}

func (e *Engine) setPlatformState(platform string, connected bool, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps, ok := e.platforms[platform]
	if !ok {
		ps = &platformState{platform: platform, configured: true, enabled: true}
		e.platforms[platform] = ps
	}
	ps.connected = connected
	ps.message = message
	ps.configured = true
	ps.updatedAt = now()
	if strings.Contains(strings.ToLower(message), "disabled") || strings.Contains(strings.ToLower(message), "removed") {
		ps.enabled = false
	} else if message != "" {
		ps.enabled = true
	}
	e.broadcastStateLocked("platform_state_changed")
}

func (e *Engine) setPlatformCancel(platform string, cancel context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps, ok := e.platforms[platform]
	if !ok {
		ps = &platformState{platform: platform, configured: true, enabled: true}
		e.platforms[platform] = ps
	}
	ps.cancel = cancel
}

// idleReaper 定期回收空闲会话，关闭其 gRPC 流。
func (e *Engine) idleReaper() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.reapIdleSessions()
		}
	}
}

func (e *Engine) reapIdleSessions() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	reaped := false
	for key, sess := range e.sessions {
		if now.Sub(sess.lastActiveAt) > e.sessionIdleTimeout {
			sess.closeStream()
			delete(e.sessions, key)
			reaped = true
			log.Infof("im engine: reaped idle session %s", key)
		}
	}
	if reaped {
		e.broadcastStateLocked("session_reaped")
	}
}
