package imcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// imSession 是一个 IM 会话的运行时状态，按 (platform + chatID + senderID) 隔离。
type imSession struct {
	sessionKey string
	platform   string
	chatID     string
	senderID   string

	// persistentSessionId 映射到 aid 的 TimelineSessionID，用于多轮上下文 & 跨重启恢复。
	// 首次为 sessionKey；/new 后追加时间戳生成新 ID。
	persistentSessionId string

	// stream 是活跃的 StartAIReAct 双向流；nil 表示尚未建立或已关闭。
	stream AIReActStream
	// streamCancel 关闭当前 gRPC 流的 context（不影响 Engine 整体 ctx）。
	streamCancel context.CancelFunc
	// streamMu 保护 stream 的建立/关闭/发送。
	streamMu sync.Mutex

	// started 表示是否已发送过 IsStart 消息（首条）。
	started bool

	// lastMessageID 最近一条入站消息的飞书 message_id（om_xxx），用于引用回复。
	lastMessageID string

	// currentModel 当前会话使用的 AI 模型标识（供状态展示）。
	currentModel string
	// reviewPolicy 当前会话的执行审批策略：manual / ai / yolo。
	reviewPolicy string
	// aiReviewRiskControlScore 是 reviewPolicy=ai 时的风险阈值。
	aiReviewRiskControlScore float64
	// disallowRequireForUserPrompt 控制 agent 是否禁止主动向 IM 用户提问。
	disallowRequireForUserPrompt bool

	// lastActiveAt 最近一次收到该会话消息的时间。
	lastActiveAt time.Time

	// chatType 会话类型（跨平台归一化）：private / group / topic。
	chatType string
	// chatTitle 人类可读的 IM 场景标题。私聊不承担正式会话命名，正式标题由 AI Session 自动生成。
	chatTitle string
	// yakitSessionTitle 是当前绑定的 Yakit AI Session 标题；通过会话面板恢复历史会话时写入。
	yakitSessionTitle string
	// senderName 发送者展示名（飞书事件不含，可能为空）。
	senderName string
	// threadID 话题/线程 ID（飞书话题消息才有）。
	threadID string

	// sendCh 串行化发往 agent 的输入（同一会话一次只处理一个 turn）。
	sendMu sync.Mutex

	// presenter 渲染 agent 运行输出到 IM 平台（卡片或文本降级）。
	// 在 startAgentStream 时按平台能力选型。
	presenter   RunPresenter
	presenterMu sync.RWMutex
	// curRunCtx 当前 turn 的运行上下文（readAgentOutput 回调 presenter 时用）。
	curRunCtx *RunContext

	// pendingInteractive 记录最近一次等待用户确认的交互请求，供无卡片平台通过 /yes /no 回复。
	pendingMu                 sync.Mutex
	pendingInteractiveID      string
	pendingInteractiveTitle   string
	pendingInteractiveContent string
	pendingInteractiveAt      time.Time
}

// isUserVisibleStream 判断一个流事件的 NodeId 是否应该直接展示给 IM 用户。
// 最终回复和 AI 调用错误都必须展示；其余 intent_summary / next_movements /
// dispatches / plan / directly-answer 等中间过程必须过滤。
// 注意：directly-answer NodeId 含 <|FINAL_ANSWER|> 内部标记，不是纯用户回复。
func isUserVisibleStream(nodeID string) bool {
	switch nodeID {
	case "re-act-loop-answer-payload", "ai-error":
		return true
	}
	return false
}

// shouldShowStream 根据 replyGranularity 颗粒度决定哪些流 NodeId 应展示给用户。
//   - standard/summary：只展示最终回复和 AI 错误（answer-payload / ai-error）
//   - detailed：额外展示思考过程（thought）和工具调用摘要（tool-call-summary）
func (e *Engine) shouldShowStream(platform, nodeID string) bool {
	if isUserVisibleStream(nodeID) {
		return true
	}
	if e.replyGranularityForPlatform(platform) == "detailed" {
		// detailed 模式额外展示思考节点
		switch nodeID {
		case "re-act-loop-thought", "thought", "tool-call-summary", "tool_call_summary":
			return true
		}
	}
	return false
}

// msgIDToString 从 InboundMessage.ReplyContext（interface{}）提取回发上下文字符串。
// 飞书: ReplyContext 是 string (om_xxx message_id)。
// 钉钉: ReplyContext 是 replyContext{SessionWebhook, ConversationID} 结构体，
//
//	序列化成 JSON 供 dingtalk.ReplyMessage 解析走 sessionWebhook 轻量回复。
func msgIDToString(rc any) string {
	if rc == nil {
		return ""
	}
	if s, ok := rc.(string); ok {
		return s
	}
	// 钉钉 replyContext 等非 string 类型：JSON 序列化后透传给 ReplyMessage
	if b, err := json.Marshal(rc); err == nil {
		return string(b)
	}
	return ""
}

// imSessionKey 生成会话隔离 key。
func imSessionKey(msg *notify.InboundMessage) string {
	if msg != nil && msg.IsCardAction {
		if sessionKey := actionValueString(msg.ActionValue, "session_key"); sessionKey != "" {
			return sessionKey
		}
	}
	switch msg.ChatType {
	case "topic":
		if strings.TrimSpace(msg.ThreadID) != "" {
			return fmt.Sprintf("%s:%s:%s", msg.Platform, msg.ChatID, msg.ThreadID)
		}
		return fmt.Sprintf("%s:%s", msg.Platform, msg.ChatID)
	case "group":
		return fmt.Sprintf("%s:%s", msg.Platform, msg.ChatID)
	}
	return fmt.Sprintf("%s:%s:%s", msg.Platform, msg.ChatID, msg.SenderID)
}

// touchSession 取或建会话，并更新活跃时间。
func (e *Engine) touchSession(sessionKey string, msg *notify.InboundMessage) {
	e.mu.Lock()
	defer e.mu.Unlock()
	sess, ok := e.sessions[sessionKey]
	if !ok {
		cfg := e.configForPlatformLocked(string(msg.Platform))
		sess = &imSession{
			sessionKey:                   sessionKey,
			platform:                     string(msg.Platform),
			chatID:                       msg.ChatID,
			senderID:                     msg.SenderID,
			persistentSessionId:          sessionKey, // 默认用 sessionKey，/new 时会变更
			lastActiveAt:                 time.Now(),
			lastMessageID:                msgIDToString(msg.ReplyContext),
			chatType:                     msg.ChatType,
			chatTitle:                    buildIMSessionTitle(msg),
			senderName:                   msg.SenderName,
			threadID:                     msg.ThreadID,
			reviewPolicy:                 cfg.ReviewPolicy,
			aiReviewRiskControlScore:     cfg.AIReviewRiskControlScore,
			disallowRequireForUserPrompt: cfg.DisallowRequireForUserPrompt,
		}
		e.sessions[sessionKey] = sess
		e.broadcastStateLocked("session_created")
		return
	}
	sess.lastActiveAt = time.Now()
	sess.lastMessageID = msgIDToString(msg.ReplyContext)
	if msg.SenderID != "" {
		sess.senderID = msg.SenderID
	}
	// 平台可能补全了先前缺失的字段（如后续接入 contact API 拿到 senderName），逐字段更新。
	if msg.ChatType != "" {
		sess.chatType = msg.ChatType
	}
	if msg.SenderName != "" {
		sess.senderName = msg.SenderName
	}
	if msg.ThreadID != "" {
		sess.threadID = msg.ThreadID
	}
}

// buildIMSessionTitle 生成 IM 会话的兜底场景标题。
// 不暴露平台内部 ID；私聊的正式会话名由 AI Session 标题生成流程负责。
func buildIMSessionTitle(msg *notify.InboundMessage) string {
	switch msg.ChatType {
	case "topic":
		return "话题会话"
	case "group":
		return "群聊会话"
	default: // private 或未知
		return "私聊会话"
	}
}

// shortID 取 ID 的短形式用于兜底标题：截掉常见前缀（ou_/oc_/om_）后保留尾部，
// 避免标题变成一长串「Feishu DM - ou_abc123def456...」。空串返回 "?"。
func shortID(id string) string {
	return compactID(id, 12, "?", false)
}

func compactID(id string, limit int, empty string, ellipsis bool) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return empty
	}
	for _, p := range []string{"ou_", "oc_", "om_", "on_"} {
		if strings.HasPrefix(id, p) {
			id = strings.TrimPrefix(id, p)
			break
		}
	}
	if limit > 0 && len(id) > limit {
		if ellipsis {
			return id[:limit] + "…"
		}
		return id[:limit]
	}
	return id
}

// getSession 取会话（不加锁，调用方需持 e.mu）。
func (e *Engine) getSession(sessionKey string) *imSession {
	return e.sessions[sessionKey]
}

// closeStream 关闭当前会话的 gRPC 流。
func (s *imSession) closeStream() {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
	}
	s.stream = nil
	s.started = false
	s.clearPendingInteraction()
}

// resetForNew 会话重置（/new）：关闭旧流，生成新 persistentSessionId。
func (s *imSession) resetForNew() {
	s.closeStream()
	s.persistentSessionId = fmt.Sprintf("%s-%d", s.sessionKey, time.Now().Unix())
	s.yakitSessionTitle = ""
	s.clearPendingInteraction()
}

func (s *imSession) setPendingInteraction(req *IMInteractiveRequest) {
	if s == nil || req == nil {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	s.pendingInteractiveID = strings.TrimSpace(req.ID)
	s.pendingInteractiveTitle = strings.TrimSpace(req.Title)
	s.pendingInteractiveContent = strings.TrimSpace(req.Content)
	s.pendingInteractiveAt = now()
}

func (s *imSession) pendingInteraction() (string, bool) {
	if s == nil {
		return "", false
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	id := strings.TrimSpace(s.pendingInteractiveID)
	if id == "" {
		return "", false
	}
	return id, true
}

func (s *imSession) clearPendingInteraction() {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	s.pendingInteractiveID = ""
	s.pendingInteractiveTitle = ""
	s.pendingInteractiveContent = ""
	s.pendingInteractiveAt = time.Time{}
}

// dispatchToAgent 把普通文本消息喂给 aid AI agent（通过 StartAIReAct 双向流）。
// agent 的输出事件会被异步读取并回发到 IM。
func (e *Engine) dispatchToAgent(msg *notify.InboundMessage, content string) {
	sessionKey := imSessionKey(msg)

	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()

	sess.sendMu.Lock()
	defer sess.sendMu.Unlock()

	// 首次：建立 gRPC 流并发 IsStart
	if !sess.started {
		if err := e.startAgentStream(sess); err != nil {
			e.reply(msg, fmt.Sprintf("❌ 启动 AI 会话失败: %v", err))
			return
		}
	} else {
		// 用户可能在 Yakit 历史中删除了当前 session 的 meta，但 IM 内存流仍在。
		// 复用已启动流前补写一次 IM meta，保证下一次对话能重新出现在历史列表。
		e.writeSessionIMMeta(sess)
	}

	// turn 开始：通知 presenter 发占位卡片/打字反应
	runCtx := &RunContext{Session: sess, RunID: newRunID()}
	sess.presenterMu.RLock()
	presenter := sess.presenter
	sess.presenterMu.RUnlock()
	if presenter != nil {
		presenter.OnRunStart(runCtx)
	}
	// 记录当前 run 上下文，供 readAgentOutput 的回调使用
	sess.presenterMu.Lock()
	sess.curRunCtx = runCtx
	sess.presenterMu.Unlock()

	// 发送用户输入
	// 附件下载 + 注入：飞书附件下载到本地，路径按 Yakit 前端同款协议塞进
	// AIInputEvent.AttachedResourceInfo(file/file_path)，由 attached-resource init 识别图片并走 vision 管线。
	var attachedFilePaths []string
	if len(msg.Attachments) > 0 {
		attachedFilePaths = e.downloadAttachments(msg)
		if len(attachedFilePaths) == 0 {
			detail := "附件下载失败，AI 无法读取这次发送的文件或图片。\n\n请检查机器人文件权限、文件是否过大，或稍后重试。"
			if strings.TrimSpace(content) == "" {
				e.replyRecovery(msg, "附件下载失败", detail)
				return
			}
			e.replyRecovery(msg, "附件下载失败", detail+"\n\n本次会继续处理你发送的文本内容。")
		}
	}
	// content 兜底：纯附件消息（文本为空）需给 FreeInput 一个描述，避免 handleFreeValue 拒绝空输入
	freeInput := content
	if freeInput == "" && len(attachedFilePaths) > 0 {
		imgCount, fileCount := 0, 0
		for _, att := range msg.Attachments {
			if att.Type == notify.MsgImage {
				imgCount++
			} else {
				fileCount++
			}
		}
		parts := []string{"[用户发送了"}
		if imgCount > 0 {
			parts = append(parts, fmt.Sprintf("%d张图片", imgCount))
		}
		if fileCount > 0 {
			if imgCount > 0 {
				parts = append(parts, "和")
			}
			parts = append(parts, fmt.Sprintf("%d个文件", fileCount))
		}
		parts = append(parts, "，请查看附件]")
		freeInput = strings.Join(parts, "")
	}

	event := buildFreeInputEvent(freeInput, attachedFilePaths)
	if err := sess.stream.Send(event); err != nil {
		log.Errorf("im engine: send free input to agent failed: %v", err)
		e.reply(msg, fmt.Sprintf("❌ 发送消息给 AI 失败: %v", err))
		// 流坏了，重置以便下次重建
		sess.closeStream()
		return
	}
}

func buildFreeInputEvent(freeInput string, attachedFilePaths []string) *ypb.AIInputEvent {
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   freeInput,
	}
	for _, path := range attachedFilePaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		event.AttachedResourceInfo = append(event.AttachedResourceInfo, &ypb.AttachedResourceInfo{
			Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
			Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
			Value: path,
		})
	}
	return event
}

// downloadAttachments 下载入站消息中的附件到本地临时目录，返回成功下载的本地路径列表。
// 下载失败只 log warn 并跳过（best-effort，不阻断用户文本发给 agent）。
func (e *Engine) downloadAttachments(msg *notify.InboundMessage) []string {
	if msg == nil || len(msg.Attachments) == 0 {
		return nil
	}
	cfg := e.botSendConfig(string(msg.Platform))
	var paths []string
	for _, att := range msg.Attachments {
		resourceType := "file"
		if att.Type == notify.MsgImage {
			resourceType = "image"
		}
		resp, err := executeNotifyRequest(&notify.Request{
			Platform: msg.Platform,
			Action:   notify.ActionResourcesDownload,
			Resource: &notify.ResourceRef{
				ID:        att.FileKey,
				MessageID: att.MessageID,
				Name:      att.FileName,
				Type:      resourceType,
			},
		}, cfg)
		if err != nil {
			log.Warnf("im engine: download attachment %s (msg=%s) failed: %v", att.FileKey, att.MessageID, err)
			continue
		}
		if resp.Resource != nil && resp.Resource.Path != "" {
			paths = append(paths, resp.Resource.Path)
		}
	}
	return paths
}

// 同时启动一个 goroutine 读取 agent 输出事件并回发到 IM。
func (e *Engine) startAgentStream(sess *imSession) error {
	sess.streamMu.Lock()
	defer sess.streamMu.Unlock()

	ctx, cancel := context.WithCancel(e.ctx)
	if e.streamFactory == nil {
		cancel()
		return fmt.Errorf("ai agent stream factory is not configured")
	}
	stream, err := e.streamFactory.StartAIReAct(ctx)
	if err != nil {
		cancel()
		return fmt.Errorf("open StartAIReAct stream: %w", err)
	}
	if strings.TrimSpace(sess.currentModel) == "" {
		if modelName := currentDefaultAIModelName(); modelName != "" {
			sess.currentModel = modelName
		}
	}

	// 发送 IsStart 配置消息
	startEvent := &ypb.AIInputEvent{
		IsStart: true,
		Params:  e.buildStartParams(sess),
	}
	if err := stream.Send(startEvent); err != nil {
		cancel()
		return fmt.Errorf("send start event: %w", err)
	}

	sess.stream = stream
	sess.streamCancel = cancel
	sess.started = true

	// 按平台能力选型 presenter：支持卡片更新 → FeishuRunPresenter，否则 TextRunPresenter。
	e.selectPresenter(sess)

	// 启动输出读取循环
	go e.readAgentOutput(sess, stream)

	// 回写 IM 元数据到 ai_sessions_v1.im_source，供 Yakit 历史列表展示/按平台筛选。
	// 这里同步写一次，避免 Yakit 列表刚刷新时 source=im 已存在但 im_source 还没落库。
	e.writeSessionIMMeta(sess)
	return nil
}

func (e *Engine) buildStartParams(sess *imSession) *ypb.AIStartParams {
	if sess == nil {
		return &ypb.AIStartParams{
			Source:                       "im",
			ReviewPolicy:                 e.reviewPolicy,
			AIReviewRiskControlScore:     e.aiReviewRiskControlScore,
			DisallowRequireForUserPrompt: e.disallowRequireForUserPrompt,
		}
	}
	return &ypb.AIStartParams{
		TimelineSessionID:            sess.persistentSessionId,
		Source:                       "im",
		AIModelName:                  strings.TrimSpace(sess.currentModel),
		ReviewPolicy:                 sess.reviewPolicy,
		AIReviewRiskControlScore:     sess.aiReviewRiskControlScore,
		DisallowRequireForUserPrompt: sess.disallowRequireForUserPrompt,
	}
}

// writeSessionIMMeta 把当前会话的结构化 IM 元数据写入项目库。
// 在 startAgentStream 成功后异步调用。
func (e *Engine) writeSessionIMMeta(sess *imSession) {
	if e.sessionStore == nil || sess == nil || sess.persistentSessionId == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := e.sessionStore.UpdateAISessionIMMeta(ctx, &ypb.UpdateAISessionIMMetaRequest{
		SessionID: sess.persistentSessionId,
		Meta: &ypb.IMSourceMeta{
			Platform:   sess.platform,
			ChatType:   sess.chatType,
			ChatTitle:  sess.chatTitle,
			SenderName: sess.senderName,
			ThreadID:   sess.threadID,
		},
	})
	if err != nil {
		log.Warnf("im engine: write session im meta for %s failed: %v", sess.sessionKey, err)
	}
}

// newRunID 生成一个 turn 的唯一运行 ID（用于卡片按钮 callback 关联当前 run）。
func newRunID() string {
	return fmt.Sprintf("run-%d", now().UnixNano())
}

func currentDefaultAIModelName() string {
	configs := consts.GetIntelligentAIConfigs()
	if len(configs) == 0 || configs[0] == nil {
		return ""
	}
	if model := strings.TrimSpace(configs[0].GetModelName()); model != "" {
		return model
	}
	for _, kv := range configs[0].GetExtraParams() {
		if kv.GetKey() == consts.ModelExtraParamKey {
			return strings.TrimSpace(kv.GetValue())
		}
	}
	return ""
}

// selectPresenter 按平台能力给会话选 presenter。
// 支持卡片更新 → FeishuRunPresenter（managed card + 节流 patch），否则 TextRunPresenter（逐段文本，零行为变更）。
func (e *Engine) selectPresenter(sess *imSession) {
	caps := platformCapabilities(notify.PlatformType(sess.platform))
	deps := e.buildPresenterDeps(sess, caps)

	var p RunPresenter
	if caps.UpdateCard && caps.SendCard {
		p = newFeishuRunPresenter(deps)
	} else {
		p = newTextRunPresenter(deps, e.replyGranularityForPlatform(sess.platform))
	}
	sess.presenterMu.Lock()
	sess.presenter = p
	sess.presenterMu.Unlock()
}

// buildPresenterDeps 组装 presenter 依赖（发送消息/卡片/patch 能力的函数闭包）。
// 复用平台 driver Factory 创建的 Client 实例。
func (e *Engine) buildPresenterDeps(sess *imSession, caps notify.PlatformCapabilities) PresenterDeps {
	cfg := e.botSendConfig(sess.platform)
	platform := notify.PlatformType(sess.platform)

	deps := PresenterDeps{
		Config: cfg,
		Send: func(platform notify.PlatformType, chatID, messageID, text string) error {
			req, err := buildTextRequest(platform, chatID, messageID, text, e.replyQuoteForPlatform(string(platform)))
			if err != nil {
				return err
			}
			_, err = executeNotifyRequest(req, cfg)
			return err
		},
	}
	if caps.SendCard {
		deps.SendCard = func(msg *notify.Message, c *notify.SendConfig) (string, error) {
			action := notify.ActionMessagesSend
			if sess.lastMessageID != "" && e.replyQuoteForPlatform(sess.platform) {
				action = notify.ActionMessagesReply
			}
			req, err := buildMessageRequest(platform, action, sess.chatID, sess.lastMessageID, inferFeishuReceiveIDType(sess.chatID), msg)
			if err != nil {
				return "", err
			}
			resp, err := executeNotifyRequest(req, c)
			if err != nil && action == notify.ActionMessagesReply {
				log.Warnf("im engine: send managed card by reply failed, fallback to normal send: %v", err)
				req, err = buildMessageRequest(platform, notify.ActionMessagesSend, sess.chatID, "", inferFeishuReceiveIDType(sess.chatID), msg)
				if err != nil {
					return "", err
				}
				resp, err = executeNotifyRequest(req, c)
			}
			if err != nil {
				return "", err
			}
			return resp.MessageID, nil
		}
	}
	if caps.UpdateCard {
		deps.PatchCard = func(messageID string, msg *notify.Message, c *notify.SendConfig) error {
			_, err := e.patchFeishuCard(sess.platform, messageID, msg, c)
			return err
		}
	}
	if e.callbackAuth != nil {
		deps.SignToken = func(input CallbackSignInput) string {
			return e.callbackAuth.Sign(input)
		}
	}
	return deps
}

// botSendConfig 取某平台的 bot 发送配置（凭证等）。从 BotConfig 构造。
func (e *Engine) botSendConfig(platform string) *notify.SendConfig {
	bots, err := credential.ListBotConfigs()
	if err != nil {
		return &notify.SendConfig{}
	}
	for _, b := range bots {
		if string(b.Platform) == platform && b.Enabled {
			return b.ToSendConfig()
		}
	}
	return &notify.SendConfig{}
}

// patchFeishuCard 调飞书 PatchCard 更新已发卡片。复用平台实例避免每次新建。
func (e *Engine) patchFeishuCard(platform, messageID string, msg *notify.Message, cfg *notify.SendConfig) (*notify.SendResult, error) {
	return patchCardMessage(notify.PlatformType(platform), messageID, msg, cfg)
}

// streamSegment 累积单个 event_writer_id 的流式文本。
// 对齐 yakit 前端 grpcAIMessageHandlers.ts：每个 event_writer_id 是一个独立 UI 节点，
// stream-finished 时该节点结束。IM 端同样按 writer_id 分桶，段结束发一条消息。
type streamSegment struct {
	buf    strings.Builder
	nodeID string
}

// readAgentOutput 持续读取 agent 的 AIOutputEvent，只提取用户可见的最终回复回发到 IM。
//
// 过滤策略（对齐 yakit ai-re-act 前端 grpcAIMessageHandlers.ts 的呈现逻辑）：
//   - stream_start：解析 Content 取 event_writer_id，新建独立 segment（仅非 IsSystem/IsReason 且 shouldShowStream）
//   - stream：用 ev.EventUUID（= event_writer_id）把 delta 累加进对应 segment；IsSystem/IsReason 走 thoughtBuf（detailed 模式）
//   - structured + stream-finished：解析 Content 取 event_writer_id + is_reason/is_system，flush 对应 segment 发一条 IM 消息
//   - result（after_stream=false 且无对应已 flush 的流）：兜底一次性发送
//   - thought / 工具调用 / 会话管理 / structured 内部状态等全部跳过
//
// 关键：每个 event_writer_id 段在 stream-finished 时独立发一条消息，与前端"每个 writer_id
// 一个 AI 响应节点"完全一致。agent 跑几轮就发几条，忠实呈现，不做跨段去重。
func (e *Engine) readAgentOutput(sess *imSession, stream AIReActStream) {
	// 取当前 turn 的 presenter + run context（dispatchToAgent 在发输入前已设置）。
	getPresenterCtx := func() (RunPresenter, *RunContext) {
		sess.presenterMu.RLock()
		defer sess.presenterMu.RUnlock()
		return sess.presenter, sess.curRunCtx
	}

	for {
		ev, err := stream.Recv()
		if err != nil {
			// 流结束前兜底 flush
			if p, rc := getPresenterCtx(); p != nil && rc != nil {
				p.Flush(rc)
			}
			if err != context.Canceled {
				log.Debugf("im engine: agent stream ended for %s: %v", sess.sessionKey, err)
			}
			sess.streamMu.Lock()
			sess.started = false
			sess.stream = nil
			if sess.streamCancel != nil {
				sess.streamCancel()
				sess.streamCancel = nil
			}
			sess.streamMu.Unlock()
			return
		}

		presenter, rc := getPresenterCtx()
		if presenter == nil || rc == nil {
			continue // turn 尚未开始（理论上不会到这），丢弃
		}
		if model := strings.TrimSpace(ev.GetAIModelName()); model != "" {
			sess.currentModel = model
		}
		if ev.GetIsSync() {
			continue
		}

		evType := ev.GetType()
		if req := parseIMInteractiveRequest(ev); req != nil {
			if shouldSuppressReviewInteraction(sess, req) {
				continue
			}
			presenter.OnRunInteraction(rc, req)
			continue
		}
		switch evType {
		case "stream_start":
			if ev.GetIsSystem() || ev.GetIsReason() {
				continue
			}
			if !e.shouldShowStream(sess.platform, ev.GetNodeId()) {
				continue
			}
			// stream_start 仅建 segment 索引；presenter 内部按需建。
			// 这里不产事件，让 stream delta 自带 EventUUID 时再建（presenter 自管）。
			continue

		case "stream":
			if ev.GetIsSystem() || ev.GetIsReason() {
				// detailed 模式思考过程：传给 presenter 标记 IsReason
				if e.replyGranularityForPlatform(sess.platform) != "detailed" {
					continue
				}
				if delta := string(ev.GetStreamDelta()); delta != "" {
					presenter.OnRunDelta(rc, RunEvent{
						Type:     RunEventDelta,
						WriterID: ev.GetEventUUID(),
						NodeID:   ev.GetNodeId(),
						Delta:    delta,
						IsReason: true,
					})
				}
				continue
			}
			if !e.shouldShowStream(sess.platform, ev.GetNodeId()) {
				continue
			}
			writerID := ev.GetEventUUID()
			if writerID == "" {
				continue
			}
			if delta := string(ev.GetStreamDelta()); delta != "" {
				presenter.OnRunDelta(rc, RunEvent{
					Type:     RunEventDelta,
					WriterID: writerID,
					NodeID:   ev.GetNodeId(),
					Delta:    delta,
				})
			}

		case "structured":
			if ev.GetNodeId() != "stream-finished" {
				continue
			}
			var finInfo struct {
				EventWriterID string `json:"event_writer_id"`
				IsReason      bool   `json:"is_reason"`
				IsSystem      bool   `json:"is_system"`
			}
			_ = json.Unmarshal(ev.GetContent(), &finInfo)
			if finInfo.IsSystem {
				continue
			}
			presenter.OnRunSegmentFinished(rc, RunEvent{
				Type:     RunEventSegmentFinished,
				WriterID: finInfo.EventWriterID,
				IsReason: finInfo.IsReason,
			})

		case "result":
			rawContent := strings.TrimSpace(string(ev.GetContent()))
			if rawContent == "" {
				continue
			}
			presenter.OnRunResult(rc, RunEvent{
				Type: RunEventResult,
				Text: rawContent,
			})

		case "thought":
			continue

		case "fail_react_task", "fail_plan_and_execution", "api_request_failed", "ai_call_failure":
			content := strings.TrimSpace(string(ev.GetContent()))
			if content != "" {
				presenter.OnRunError(rc, RunEvent{
					Type: RunEventError,
					Text: content,
				})
			}

		default:
			continue
		}
	}
}

// internalTagRe 匹配所有 AITAG 内部标记（<|FINAL_ANSWER_xxx|> ... <|FINAL_ANSWER_END_xxx|>、
// <|AI_CACHE_xxx|>、<|FACTS_xxx|> 等），这些是 AI agent 的传输层标记，不应展示给 IM 用户。
var internalTagRe = regexp.MustCompile(`<\|[A-Z_]+(_[A-Za-z0-9]+)?\|>`)

// cleanIMText 清理发给 IM 用户的消息文本：
//  1. 剥离所有 AITAG 内部标记（<|FINAL_ANSWER_xxx|> / <|FINAL_ANSWER_END_xxx|> / <|AI_CACHE_xxx|> 等）
//  2. 修复字面量 \n / \t（AI 偶尔输出反斜杠+n 而非真实换行符）
//  3. 去除首尾空白
func cleanIMText(text string) string {
	// 剥离内部标记
	text = internalTagRe.ReplaceAllString(text, "")
	// 修复字面量 \n / \t / \r（AI 偶尔输出转义序列而非真实控制字符）
	text = strings.ReplaceAll(text, "\\n", "\n")
	text = strings.ReplaceAll(text, "\\t", "\t")
	text = strings.ReplaceAll(text, "\\r", "\r")
	// 压缩连续空行（3+ 换行 → 2 换行）
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// reply 向 IM 回发一条文本消息（便捷方法），引用回复用户消息。
func (e *Engine) reply(msg *notify.InboundMessage, text string) {
	messageID := msgIDToString(msg.ReplyContext)
	if msg.IsCardAction {
		// Card action 的 ReplyContext 是承载按钮的卡片 message_id，不是用户消息。
		// 用它走 reply API 容易失败，且会让按钮点击看起来只有 loading 没有反馈。
		messageID = ""
	}
	e.sendRaw(string(msg.Platform), msg.ChatID, messageID, text)
}

// sendRaw 用已配置的 bot 凭证向指定平台/会话发送文本，带重试（最多 3 次）。
// 如果提供了 messageID，优先使用引用回复（飞书 reply API / 钉钉 sessionWebhook），让回复挂在用户消息下方。
// 网络抖动导致的偶发超时会自动重试，避免用户收不到回复。
func (e *Engine) sendRaw(platform, chatID, messageID, text string) {
	p := notify.PlatformType(platform)
	bot, err := credential.GetBotConfig(string(p))
	if err != nil || bot == nil {
		log.Errorf("im engine: no bot config for %s: %v", platform, err)
		return
	}
	cfg := bot.ToSendConfig()

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := sendText(p, chatID, messageID, text, cfg, e.replyQuoteForPlatform(platform))
		if err == nil {
			return
		}
		log.Warnf("im engine: send reply to %s/%s failed (attempt %d/%d): %v", platform, chatID, i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
		}
	}
	log.Errorf("im engine: send reply to %s/%s failed after %d retries", platform, chatID, maxRetries)
}

// inferFeishuReceiveIDType 根据飞书 ID 前缀推断 receive_id_type。
// oc_ = chat_id, ou_ = open_id, on_ = union_id, u_ = user_id。
// 非飞书平台或无法识别时留空（由平台默认处理）。
func inferFeishuReceiveIDType(targetID string) string {
	switch {
	case strings.HasPrefix(targetID, "oc_"):
		return "chat_id"
	case strings.HasPrefix(targetID, "ou_"):
		return "open_id"
	case strings.HasPrefix(targetID, "on_"):
		return "union_id"
	case strings.HasPrefix(targetID, "u_"):
		return "user_id"
	}
	return ""
}
