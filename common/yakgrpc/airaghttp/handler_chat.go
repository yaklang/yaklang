package airaghttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// noise 事件类型与 nodeId, 这些对用户无意义, SSE 中一律丢弃
// 关键词: noise event filter, drop stream/consumption/pong/pressure
var chatNoiseTypes = map[string]bool{
	"stream":               true,
	"stream_start":         true,
	"stream_finished":      true,
	"consumption":          true,
	"pong":                 true,
	"pressure":             true,
	"ai_first_byte_cost_ms": true,
	"ai_total_cost_ms":     true,
	"ai_call_summary":      true,
	"prompt_profile":       true,
}

var chatNoiseNodeIds = map[string]bool{
	"stream-finished":       true,
	"ai_first_byte_cost_ms": true,
	"ai_total_cost_ms":      true,
	"ai_call_summary":       true,
	"pressure":              true,
	"system":                true,
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

	opts := s.buildAIEngineOptions(ctx, sessionID, emitter)

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

	// AI 服务: 自定义模式覆盖, 否则走全局分级 aiconfig (aiengine 默认启用 TieredAICallback)
	if s.config.UseCustomAIConfig() {
		aiOpts := make([]aispec.AIConfigOption, 0)
		if s.config.AI.Model != "" {
			aiOpts = append(aiOpts, aispec.WithModel(s.config.AI.Model))
		}
		if s.config.AI.APIKey != "" {
			aiOpts = append(aiOpts, aispec.WithAPIKey(s.config.AI.APIKey))
		}
		if s.config.AI.Domain != "" {
			aiOpts = append(aiOpts, aispec.WithDomain(s.config.AI.Domain))
		}
		opts = append(opts, aiengine.WithAIConfig(s.config.AI.Type, aiOpts...))
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
	emitter.safeEmit("log", map[string]interface{}{
		"kind":    kind,
		"label":   label,
		"message": message,
		"type":    evtType,
		"nodeId":  nodeID,
	})
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
			return "", ""
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
