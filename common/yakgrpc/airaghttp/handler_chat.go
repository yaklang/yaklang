package airaghttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// noise 事件类型与 nodeId, 这些对用户无意义, SSE 中一律丢弃
// 关键词: noise event filter, drop stream/consumption/pong/pressure
var chatNoiseTypes = map[string]bool{
	"stream":                true,
	"stream_start":          true,
	"stream_finished":       true,
	"consumption":           true,
	"pong":                  true,
	"pressure":              true,
	"ai_first_byte_cost_ms": true,
	"ai_total_cost_ms":      true,
	"ai_call_summary":       true,
	"prompt_profile":        true,
	// 以下事件要么泄漏本地路径/内部 prompt, 要么是额度/通知噪声, 一律不下发前端
	// 关键词: drop local path leak, notify quota spam, session title noise, reference_material prompt leak
	"notify":                   true,
	"filesystem_pin_directory": true,
	"filesystem_pin_filename":  true,
	"session_title":            true,
	"pin_filename":             true,
	"pin_directory":            true,
	// reference_material 会把注入的技能/系统 prompt 缓存原文 (<|AI_CACHE_SYSTEM|>...) 透出, 属内部信息泄漏
	"reference_material": true,
}

// internalMarkerRe 命中内部 prompt / 缓存 / SCHEMA 等标记, 这些是引擎内部内容, 不应泄漏给前端
// 关键词: internal prompt marker, AI_CACHE_SYSTEM, TRAITS, avoid leak
var internalMarkerRe = regexp.MustCompile(`<\|[A-Z]|AI_CACHE_SYSTEM|TRAITS|高静态|high-static`)

// absLocalPathRe 匹配常见的本地绝对路径 (含 yakit-projects/aispace 等), 用于脱敏
// 关键词: sanitize local path, avoid filesystem leak
var absLocalPathRe = regexp.MustCompile(`(?:[A-Za-z]:\\[^\s"']+|/(?:Users|home|root|var|tmp|opt|private|data)/[^\s"':,)]+)`)

// sanitizeTraceMessage 将消息中的本地绝对路径替换为占位符, 防止文件系统信息泄漏
func sanitizeTraceMessage(s string) string {
	if s == "" {
		return s
	}
	// 命中引擎内部 prompt/缓存标记的整条丢弃 (防系统 prompt 泄漏)
	if internalMarkerRe.MatchString(s) {
		return ""
	}
	out := absLocalPathRe.ReplaceAllString(s, "[local-path]")
	// 兜底: 任意残留含 yakit-projects 的串也屏蔽
	if strings.Contains(out, "yakit-projects") || strings.Contains(out, "aispace") {
		return "[local-path]"
	}
	return out
}

var chatNoiseNodeIds = map[string]bool{
	"stream-finished":       true,
	"ai_first_byte_cost_ms": true,
	"ai_total_cost_ms":      true,
	"ai_call_summary":       true,
	"pressure":              true,
	"system":                true,
	"reference_material":    true,
}

// handleChat GET|POST /chat SSE Agentic RAG 流式问答
// 关键词: SSE chat, aiengine.InvokeReAct, focus knowledge_enhance, 429 busy
func (s *RAGHTTPServer) handleChat(w http.ResponseWriter, r *http.Request) {
	clientAddr := r.RemoteAddr
	question := s.readQuestion(r)

	if question == "" {
		writeJSONError(w, http.StatusBadRequest, "missing question (use ?q=... or POST body {question:...})")
		return
	}
	log.Infof("/chat incoming from %s question=%q", clientAddr, question)

	// SSE 响应头 (CORS 头已由中间件写入)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	emitter := newSSEEmitter(w)
	if emitter == nil {
		writeJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// 抢并发信号量, 失败立刻 429
	if !s.acquireSlot() {
		log.Warnf("concurrency limit reached, reject %s", clientAddr)
		w.WriteHeader(http.StatusTooManyRequests)
		emitter.safeEmit("error", map[string]interface{}{"code": 429, "message": "server is busy, please retry later"})
		emitter.safeEmit("end", map[string]interface{}{"reason": "server_busy", "ok": false})
		return
	}

	w.WriteHeader(http.StatusOK)

	sessionID := "airaghttp-" + utils.RandStringBytes(12)
	startTime := time.Now()

	defer func() {
		if rec := recover(); rec != nil {
			log.Errorf("chat handler panic: %v", rec)
			emitter.safeEmit("error", map[string]interface{}{"code": 500, "message": utils.InterfaceToString(rec)})
		}
		s.releaseSlot()
		// 会话结束: 删除本次 session 在数据库中的全部痕迹 (会话/运行时/检查点/事件/计划), 避免数据爆炸
		// 关键词: cleanup ai session, delete session data, avoid data explosion
		cleanupSessionData(consts.GetGormProjectDatabase(), sessionID)
		log.Infof("/chat finished for %s session=%s inflight=%d", clientAddr, sessionID, s.getInflight())
	}()

	emitter.safeEmit("start", map[string]interface{}{
		"question":        question,
		"collectionCount": len(s.readyCollections),
		"collections":     s.readyCollections,
		"ai":              s.aiModeInfo(),
	})

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.config.Timeout)*time.Second)
	defer cancel()

	// 为本次会话分配独立临时工作目录, 结束后整体删除, 避免 aispace session 产物堆积撑爆磁盘
	// 关键词: ephemeral workdir, cleanup artifacts, avoid disk explosion
	workdir := filepath.Join(os.TempDir(), "airaghttp-sessions", sessionID)
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		log.Warnf("create session workdir failed: %v", err)
		workdir = ""
	} else {
		defer func() {
			if rmErr := os.RemoveAll(workdir); rmErr != nil {
				log.Warnf("cleanup session workdir failed: %v", rmErr)
			} else {
				log.Infof("session workdir cleaned: %s", workdir)
			}
		}()
	}

	opts := s.buildAIEngineOptions(ctx, sessionID, emitter)
	if workdir != "" {
		opts = append(opts, aiengine.WithWorkdir(workdir))
	}

	log.Infof("invoking aiengine.InvokeReAct for %s ...", clientAddr)
	var invokeErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				invokeErr = utils.Errorf("invoke panic: %v", rec)
			}
		}()
		invokeErr = aiengine.InvokeReAct(question, opts...)
	}()

	durationMs := time.Since(startTime).Milliseconds()
	if invokeErr != nil {
		log.Errorf("InvokeReAct failed for %s: %v", clientAddr, invokeErr)
		emitter.safeEmit("error", map[string]interface{}{"code": 500, "message": "invoke failed: " + invokeErr.Error()})
	}

	emitter.safeEmit("end", map[string]interface{}{"durationMs": durationMs, "ok": invokeErr == nil})
}

// buildCustomAICallback 根据自定义 AI 配置构造统一回调, 用于覆盖 React loop 的质量/速度优先回调.
// 与 aiengine.WithAIConfig 内部构造方式一致 (LoadChater + AIChatToAICallbackType).
// 关键词: build custom ai callback, LoadChater, override tiered callback
func buildCustomAICallback(typ string, opts []aispec.AIConfigOption) aicommon.AICallbackType {
	chatter, err := ai.LoadChater(typ, opts...)
	if err != nil {
		log.Warnf("load custom ai chatter (type=%s) failed: %v", typ, err)
		return nil
	}
	return aicommon.AIChatToAICallbackType(chatter)
}

// cleanupSessionData 删除某次会话在项目库中的全部关联数据 (会话元信息/运行时/检查点/输出事件/计划执行).
// 用于 /chat 结束后立即清理, 防止只读问答服务长期堆积 session 数据撑爆磁盘.
// 关键词: DeleteAISession, cleanup session, avoid data explosion
func cleanupSessionData(db *gorm.DB, sessionID string) {
	if db == nil || sessionID == "" {
		return
	}
	runtimes, events, err := yakit.DeleteAISession(db, sessionID)
	if err != nil {
		log.Warnf("cleanup ai session %s failed: %v", sessionID, err)
		return
	}
	log.Infof("ai session cleaned: session=%s runtimes=%d events=%d", sessionID, runtimes, events)
}

// readQuestion 从 query 参数 q 或 POST body {question}/{q} 中读取问题
func (s *RAGHTTPServer) readQuestion(r *http.Request) string {
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		return q
	}
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	if v, ok := obj["question"]; ok {
		if str := strings.TrimSpace(utils.InterfaceToString(v)); str != "" {
			return str
		}
	}
	if v, ok := obj["q"]; ok {
		return strings.TrimSpace(utils.InterfaceToString(v))
	}
	return ""
}

// buildAIEngineOptions 构造 aiengine 选项 (focus=knowledge_enhance + 知识库挂载 + 事件回调)
func (s *RAGHTTPServer) buildAIEngineOptions(ctx context.Context, sessionID string, emitter *sseEmitter) []aiengine.AIEngineConfigOption {
	opts := []aiengine.AIEngineConfigOption{
		aiengine.WithContext(ctx),
		aiengine.WithFocus("knowledge_enhance"),
		aiengine.WithYOLOMode(),
		aiengine.WithAllowUserInteract(false),
		aiengine.WithMaxIteration(s.config.MaxIteration),
		aiengine.WithLanguage(s.config.Language),
		aiengine.WithDisableToolUse(true),
		aiengine.WithDisableMCPServers(true),
		aiengine.WithDisableAIForge(true),
		aiengine.WithSessionID(sessionID),
		aiengine.WithDebugMode(s.config.Debug),
	}

	for _, kb := range s.readyCollections {
		opts = append(opts, aiengine.WithAttachedKnowledgeBase(kb))
	}

	// AI 服务选择 (按优先级分档以节约成本):
	//   - 速度优先(speed): React loop 内搜索/记忆/压缩等高频调用走此通道. 用 ai_lightweight
	//     配置的小尺寸模型; 未配置则回退内置 memfit-light-free. 避免高质模型带来的高成本.
	//   - 质量优先(quality): 关键推理/最终回答. 用 ai 配置的高质模型; 未配置则回退轻量模型.
	// 关键: 仅 WithAIConfig 不足以覆盖 React loop 内部按"质量/速度优先"取的分级回调,
	//       必须同时通过 ExtOptions 注入对应优先级回调才能真正生效.
	// 关键词: quality vs speed channel, ai_lightweight, cost saving
	var lightCb aicommon.AICallbackType
	if s.config.IsLightweightAIConfigured() {
		lwOpts := s.config.LightweightAISpecOptions()
		lightCb = buildCustomAICallback(s.config.AILightweight.Type, lwOpts)
	}
	if lightCb == nil {
		lightCb = aicommon.MustGetLightweightAIModelCallback()
	}

	qualityCb := lightCb
	if s.config.IsAIConfigured() {
		aiOpts := s.config.AISpecOptions()
		opts = append(opts, aiengine.WithAIConfig(s.config.AI.Type, aiOpts...))
		if cb := buildCustomAICallback(s.config.AI.Type, aiOpts); cb != nil {
			qualityCb = cb
		}
	}

	extOpts := make([]aicommon.ConfigOption, 0, 4)
	if qualityCb != nil {
		extOpts = append(extOpts, aicommon.WithQualityPriorityAICallback(qualityCb))
	}
	if lightCb != nil {
		extOpts = append(extOpts, aicommon.WithSpeedPriorityAICallback(lightCb))
	}
	// 禁用记忆系统: 用 no-op triage 替换默认 aimem, 彻底关闭记忆构建/入库/检索, 更快更省
	// 关键词: disable memory, NoOpMemoryTriage, no memory build/store/search
	if s.config.DisableMemory {
		extOpts = append(extOpts, aicommon.WithNoOpMemoryTriage())
	}
	// 自定义系统/预设提示词: 作为 USER_PRESET 注入每次请求 (超长由引擎自动截断)
	// 关键词: custom system prompt, WithUserPresetPrompt, persona
	if prompt := strings.TrimSpace(s.config.SystemPrompt); prompt != "" {
		extOpts = append(extOpts, aicommon.WithUserPresetPrompt(prompt))
	}
	if len(extOpts) > 0 {
		opts = append(opts, aiengine.WithExtOptions(extOpts...))
	}

	opts = append(opts,
		aiengine.WithOnEvent(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent) {
			s.onChatEvent(emitter, event)
		}),
		aiengine.WithOnStream(func(_ aicommon.AIEngineOperator, _ *schema.AiOutputEvent, nodeID string, data []byte) {
			s.onChatStream(emitter, nodeID, data)
		}),
	)
	return opts
}

// onChatStream 处理流式 token, 只保留 thought / answer 两类 nodeId
func (s *RAGHTTPServer) onChatStream(emitter *sseEmitter, nodeID string, data []byte) {
	chunk := string(data)
	if chunk == "" {
		return
	}
	switch nodeID {
	case "re-act-loop-thought":
		emitter.safeEmit("thought", map[string]interface{}{"chunk": chunk})
	case "re-act-loop-answer-payload":
		emitter.safeEmit("answer", map[string]interface{}{"chunk": chunk})
	}
}

// onChatEvent 处理非流式事件, 映射为 log / error 事件
func (s *RAGHTTPServer) onChatEvent(emitter *sseEmitter, event *schema.AiOutputEvent) {
	if event == nil || event.IsStream {
		return
	}
	evtType := string(event.Type)
	if evtType == "" || chatNoiseTypes[evtType] {
		return
	}
	nodeID := event.NodeId
	if chatNoiseNodeIds[nodeID] {
		return
	}

	message := extractEventMessage(string(event.Content), event.IsJson)

	if evtType == "fail_react_task" || evtType == "fail_plan_and_execution" {
		emitter.safeEmit("error", map[string]interface{}{"code": 500, "message": message, "nodeId": nodeID})
		return
	}

	kind, label := classifyEvent(evtType, nodeID)
	if kind == "" {
		return
	}
	if message == "" {
		return
	}
	// 路径脱敏: 若整条消息被判定为本地路径, 直接丢弃, 否则替换内嵌路径
	message = sanitizeTraceMessage(message)
	if message == "" || message == "[local-path]" {
		return
	}
	// progress 消息形如 "执行搜索中 - search_knowledge:semantic / executing search ...", 仅保留前半段中文短语
	if kind == "progress" {
		message = cleanProgressMessage(message)
		if message == "" {
			return
		}
	}
	emitter.safeEmit("log", map[string]interface{}{
		"kind":    kind,
		"label":   label,
		"message": message,
		"type":    evtType,
		"nodeId":  nodeID,
	})
}

// cleanProgressMessage 清洗 loading status 文案, 去掉英文副本与技术后缀, 保留简洁短语
// 形如: "执行搜索中 - search_knowledge:semantic / executing search - mode:semantic" -> "执行搜索中"
//
//	"初始化 / initializing..." -> "初始化"
//	"压缩搜索结果中 - compressing search result" -> "压缩搜索结果中"
func cleanProgressMessage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, " / "); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, " - "); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// extractEventMessage 从结构化内容中提取可读消息
func extractEventMessage(rawContent string, isJson bool) string {
	if rawContent == "" {
		return ""
	}
	if !isJson {
		return rawContent
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(rawContent), &obj); err != nil {
		return rawContent
	}
	for _, key := range []string{"message", "content", "value", "title", "path", "filename", "payload"} {
		if v, ok := obj[key]; ok {
			if str := utils.InterfaceToString(v); str != "" {
				return str
			}
		}
	}
	return ""
}

// classifyEvent 根据事件类型与 nodeId 推导 kind/label; kind 为空表示丢弃
func classifyEvent(evtType, nodeID string) (kind string, label string) {
	if evtType == "structured" {
		switch {
		case nodeID == "re-act-loading-status-key" || nodeID == "status":
			// loading status 是有意义的进度: 搜索中/压缩中/评估中, 作为 progress 步骤下发
			// 关键词: surface loading status, progress step, 获取资料/正在压缩
			return "progress", "progress"
		case nodeID == "timeline_item":
			return "timeline", "timeline"
		case nodeID == "session_title":
			return "title", "session_title"
		case strings.HasPrefix(nodeID, "react_task_"):
			return "task", "task"
		case strings.Contains(nodeID, "knowledge") || strings.Contains(nodeID, "search"):
			return "search", "search"
		default:
			return "event", nodeID
		}
	}
	switch evtType {
	case "knowledge", "task_knowledge":
		return "search", "search"
	case "thought":
		return "thought", "thought"
	case "plan":
		return "plan", "plan"
	default:
		return "event", evtType
	}
}
