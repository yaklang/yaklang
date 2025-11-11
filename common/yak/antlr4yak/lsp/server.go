package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// LSP JSON-RPC 2.0 structures
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// YakLSPServer LSP 服务器
type YakLSPServer struct {
	grpcServer *yakgrpc.Server
	reader     *bufio.Reader
	writer     io.Writer

	// 文档管理和调度
	docMgr    *DocumentManager
	scheduler *EditScheduler
}

// NewYakLSPServer 创建 LSP 服务器
func NewYakLSPServer(grpcServer *yakgrpc.Server) *YakLSPServer {
	docMgr := NewDocumentManager()
	scheduler := NewEditScheduler(docMgr)

	return &YakLSPServer{
		grpcServer: grpcServer,
		reader:     bufio.NewReader(os.Stdin),
		writer:     os.Stdout,
		docMgr:     docMgr,
		scheduler:  scheduler,
	}
}

// Start 启动 LSP 服务器
func (s *YakLSPServer) Start() error {
	log.Info("yaklang LSP server starting...")

	for {
		// 读取 Content-Length header
		headers := make(map[string]string)
		for {
			line, err := s.reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}

			line = strings.TrimSpace(line)
			if line == "" {
				break
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		contentLength := 0
		if cl, ok := headers["Content-Length"]; ok {
			fmt.Sscanf(cl, "%d", &contentLength)
		}

		if contentLength == 0 {
			continue
		}

		// 读取消息体
		content := make([]byte, contentLength)
		_, err := io.ReadFull(s.reader, content)
		if err != nil {
			return err
		}

		// 处理请求
		go s.handleRequest(content)
	}
}

func (s *YakLSPServer) handleRequest(content []byte) {
	var req jsonRPCRequest
	if err := json.Unmarshal(content, &req); err != nil {
		log.Errorf("parse json-rpc request failed: %v", err)
		return
	}

	log.Debugf("[LSP] request: %s", req.Method)

	var result interface{}
	var rpcErr *rpcError

	switch req.Method {
	case "initialize":
		result = s.handleInitialize(req.Params)
	case "initialized":
		// 客户端初始化完成通知
		return
	case "shutdown":
		result = nil
	case "exit":
		os.Exit(0)
	case "textDocument/completion":
		result, rpcErr = s.handleCompletion(req.Params)
	case "textDocument/hover":
		result, rpcErr = s.handleHover(req.Params)
	case "textDocument/signatureHelp":
		result, rpcErr = s.handleSignatureHelp(req.Params)
	case "textDocument/definition":
		result, rpcErr = s.handleDefinition(req.Params)
	case "textDocument/references":
		result, rpcErr = s.handleReferences(req.Params)
	case "textDocument/didOpen":
		s.handleDidOpen(req.Params)
		return
	case "textDocument/didChange":
		s.handleDidChange(req.Params)
		return
	case "textDocument/didSave":
		s.handleDidSave(req.Params)
		return
	case "textDocument/didClose":
		s.handleDidClose(req.Params)
		return
	default:
		log.Warnf("unhandled LSP method: %s", req.Method)
		rpcErr = &rpcError{
			Code:    -32601,
			Message: "Method not found",
		}
	}

	// 发送响应
	if req.ID != nil {
		s.sendResponse(req.ID, result, rpcErr)
	}
}

func (s *YakLSPServer) handleInitialize(params json.RawMessage) interface{} {
	return map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": map[string]interface{}{
				"openClose": true,
				"change":    1, // Full sync
				"save":      map[string]interface{}{"includeText": true},
			},
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", "("},
				"resolveProvider":   false,
			},
			"hoverProvider":              true,
			"signatureHelpProvider":      map[string]interface{}{"triggerCharacters": []string{"(", ","}},
			"definitionProvider":         true,
			"referencesProvider":         true,
			"documentFormattingProvider": true,
		},
		"serverInfo": map[string]interface{}{
			"name":    "yaklang-lsp",
			"version": "1.0.0",
		},
	}
}

func (s *YakLSPServer) handleCompletion(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
		Context struct {
			TriggerKind      int    `json:"triggerKind"`
			TriggerCharacter string `json:"triggerCharacter,omitempty"`
		} `json:"context,omitempty"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	// 读取文档内容
	code, err := s.getDocumentContent(p.TextDocument.URI)
	if err != nil {
		log.Errorf("read document failed: %v", err)
		return []interface{}{}, nil
	}

	// P0 高优先级请求：尝试使用缓存快速响应，后台更新
	// 检查是否有文档状态
	if doc, ok := s.docMgr.GetDocument(p.TextDocument.URI); ok {
		ssaCache := doc.GetSSACache()
		// 计算当前代码的哈希
		currentHash := ComputeCodeHash(code)

		// 如果有 SSA 缓存且语义哈希匹配（或缓存不太旧），可以直接使用
		if ssaCache != nil {
			cacheAge := time.Since(ssaCache.CreatedAt)
			// 对于 Completion，允许使用稍旧的缓存（5秒内）
			if ssaCache.Hash == currentHash.Semantic || (ssaCache.Stale && cacheAge < 5*time.Second) {
				log.Debugf("[LSP] using cached SSA for completion (age: %v, stale: %v)", cacheAge, ssaCache.Stale)
				// 使用缓存，后台触发更新
				if ssaCache.Hash != currentHash.Semantic {
					go s.scheduler.ScheduleAnalysis(p.TextDocument.URI, "yak")
				}
			} else {
				// 缓存过期太久，请求驱动立即编译（但有超时）
				log.Debugf("[LSP] cache too stale, requesting immediate compilation")
				_, _ = s.scheduler.RequestDrivenAnalysis(p.TextDocument.URI, "yak", 3*time.Second)
			}
		} else {
			// 没有缓存，触发编译但不等待
			log.Debugf("[LSP] no cache available, triggering background compilation")
			go s.scheduler.ScheduleImmediateAnalysis(p.TextDocument.URI, "yak")
		}
	}

	// 调用 gRPC 服务获取补全
	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.COMPLETION,
		YakScriptType: "yak",
		YakScriptCode: code,
		Range: &ypb.Range{
			Code:        code,
			StartLine:   int64(p.Position.Line + 1),
			StartColumn: int64(p.Position.Character),
			EndLine:     int64(p.Position.Line + 1),
			EndColumn:   int64(p.Position.Character),
		},
	}

	resp, err := s.grpcServer.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		log.Errorf("get completion failed: %v", err)
		return []interface{}{}, nil
	}

	// 转换为 LSP CompletionItem 并去重
	items := make([]map[string]interface{}, 0, len(resp.SuggestionMessage))
	seen := make(map[string]bool) // 用于去重

	for _, item := range resp.SuggestionMessage {
		// 使用 label 作为去重键
		if seen[item.Label] {
			continue
		}
		seen[item.Label] = true

		completionItem := map[string]interface{}{
			"label":  item.Label,
			"kind":   convertCompletionKind(item.Kind),
			"detail": item.Description,
		}
		if item.InsertText != "" {
			completionItem["insertText"] = item.InsertText
		}
		if item.DefinitionVerbose != "" {
			completionItem["documentation"] = map[string]interface{}{
				"kind":  "markdown",
				"value": item.DefinitionVerbose,
			}
		}
		items = append(items, completionItem)
	}

	if len(seen) < len(resp.SuggestionMessage) {
		log.Debugf("[LSP Completion] deduplicated: %d -> %d items", len(resp.SuggestionMessage), len(items))
	}

	return items, nil
}

func (s *YakLSPServer) handleHover(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	code, err := s.getDocumentContent(p.TextDocument.URI)
	if err != nil {
		return nil, nil
	}

	// P0 请求：快速响应
	s.ensureSSACache(p.TextDocument.URI, code, 5*time.Second)

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.HOVER,
		YakScriptType: "yak",
		YakScriptCode: code,
		Range: &ypb.Range{
			Code:        code,
			StartLine:   int64(p.Position.Line + 1),
			StartColumn: int64(p.Position.Character),
			EndLine:     int64(p.Position.Line + 1),
			EndColumn:   int64(p.Position.Character),
		},
	}

	resp, err := s.grpcServer.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil || len(resp.SuggestionMessage) == 0 {
		return nil, nil
	}

	// 合并所有提示信息
	var contents []string
	for _, item := range resp.SuggestionMessage {
		if item.DefinitionVerbose != "" {
			contents = append(contents, item.DefinitionVerbose)
		}
	}

	if len(contents) == 0 {
		return nil, nil
	}

	return map[string]interface{}{
		"contents": map[string]interface{}{
			"kind":  "markdown",
			"value": strings.Join(contents, "\n\n---\n\n"),
		},
	}, nil
}

func (s *YakLSPServer) handleSignatureHelp(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	code, err := s.getDocumentContent(p.TextDocument.URI)
	if err != nil {
		return nil, nil
	}

	// P0 请求：快速响应
	s.ensureSSACache(p.TextDocument.URI, code, 5*time.Second)

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.SIGNATURE,
		YakScriptType: "yak",
		YakScriptCode: code,
		Range: &ypb.Range{
			Code:        code,
			StartLine:   int64(p.Position.Line + 1),
			StartColumn: int64(p.Position.Character),
			EndLine:     int64(p.Position.Line + 1),
			EndColumn:   int64(p.Position.Character),
		},
	}

	resp, err := s.grpcServer.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil || len(resp.SuggestionMessage) == 0 {
		return nil, nil
	}

	signatures := make([]map[string]interface{}, 0, len(resp.SuggestionMessage))
	for _, item := range resp.SuggestionMessage {
		sig := map[string]interface{}{
			"label": item.Label,
		}
		if item.DefinitionVerbose != "" {
			sig["documentation"] = map[string]interface{}{
				"kind":  "markdown",
				"value": item.DefinitionVerbose,
			}
		}
		signatures = append(signatures, sig)
	}

	return map[string]interface{}{
		"signatures":      signatures,
		"activeSignature": 0,
		"activeParameter": 0,
	}, nil
}

func (s *YakLSPServer) handleDefinition(params json.RawMessage) (interface{}, *rpcError) {
	// TODO: 实现跳转到定义
	return nil, nil
}

func (s *YakLSPServer) handleReferences(params json.RawMessage) (interface{}, *rpcError) {
	// TODO: 实现查找引用
	return []interface{}{}, nil
}

// handleDidOpen 处理文档打开事件
func (s *YakLSPServer) handleDidOpen(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI        string `json:"uri"`
			LanguageID string `json:"languageId"`
			Version    int    `json:"version"`
			Text       string `json:"text"`
		} `json:"textDocument"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		log.Errorf("[LSP] failed to parse didOpen params: %v", err)
		return
	}

	log.Debugf("[LSP] didOpen: %s (version: %d)", p.TextDocument.URI, p.TextDocument.Version)
	s.docMgr.OpenDocument(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)
	s.scheduler.ScheduleAnalysis(p.TextDocument.URI, "yak")
}

// handleDidChange 处理文档变更事件
func (s *YakLSPServer) handleDidChange(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"` // 全量同步模式
		} `json:"contentChanges"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		log.Errorf("[LSP] failed to parse didChange params: %v", err)
		return
	}

	if len(p.ContentChanges) == 0 {
		return
	}

	// 全量更新（LSP 配置为 Full sync）
	newText := p.ContentChanges[0].Text
	log.Debugf("[LSP] didChange: %s (version: %d, size: %d bytes)",
		p.TextDocument.URI, p.TextDocument.Version, len(newText))

	s.docMgr.UpdateDocument(p.TextDocument.URI, p.TextDocument.Version, newText)
	s.scheduler.ScheduleAnalysis(p.TextDocument.URI, "yak")
}

// handleDidSave 处理文档保存事件
func (s *YakLSPServer) handleDidSave(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Text string `json:"text,omitempty"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		log.Errorf("[LSP] failed to parse didSave params: %v", err)
		return
	}

	log.Debugf("[LSP] didSave: %s", p.TextDocument.URI)
	// 保存时立即触发高优先级分析
	s.scheduler.ScheduleImmediateAnalysis(p.TextDocument.URI, "yak")
}

// handleDidClose 处理文档关闭事件
func (s *YakLSPServer) handleDidClose(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		log.Errorf("[LSP] failed to parse didClose params: %v", err)
		return
	}

	log.Debugf("[LSP] didClose: %s", p.TextDocument.URI)
	s.docMgr.CloseDocument(p.TextDocument.URI)
}

// getDocumentContent 从 DocumentManager 或文件系统获取文档内容
func (s *YakLSPServer) getDocumentContent(uri string) (string, error) {
	// 优先从 DocumentManager 获取
	if doc, ok := s.docMgr.GetDocument(uri); ok {
		return doc.GetContent(), nil
	}

	// 如果不在缓存中，从文件系统读取
	return readDocumentContentFromFile(uri)
}

// ensureSSACache 确保 SSA 缓存可用（智能策略）
func (s *YakLSPServer) ensureSSACache(uri string, code string, maxStaleAge time.Duration) {
	doc, ok := s.docMgr.GetDocument(uri)
	if !ok {
		return
	}

	ssaCache := doc.GetSSACache()
	currentHash := ComputeCodeHash(code)

	if ssaCache != nil {
		cacheAge := time.Since(ssaCache.CreatedAt)
		// 如果缓存匹配或不太旧，可以使用
		if ssaCache.Hash == currentHash.Semantic || (ssaCache.Stale && cacheAge < maxStaleAge) {
			// 使用缓存，后台触发更新（如果需要）
			if ssaCache.Hash != currentHash.Semantic {
				go s.scheduler.ScheduleAnalysis(uri, "yak")
			}
			return
		}
		// 缓存过期太久，请求驱动立即编译
		go func() {
			_, _ = s.scheduler.RequestDrivenAnalysis(uri, "yak", 3*time.Second)
		}()
	} else {
		// 没有缓存，触发编译但不等待
		go s.scheduler.ScheduleImmediateAnalysis(uri, "yak")
	}
}

func readDocumentContent(uri string) (string, error) {
	return readDocumentContentFromFile(uri)
}

func readDocumentContentFromFile(uri string) (string, error) {
	// 移除 file:// 前缀
	filePath := strings.TrimPrefix(uri, "file://")

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func convertCompletionKind(kind string) int {
	// LSP CompletionItemKind
	switch kind {
	case "Function":
		return 3
	case "Method":
		return 2
	case "Variable":
		return 6
	case "Field":
		return 5
	case "Keyword":
		return 14
	case "Module":
		return 9
	case "Class":
		return 7
	case "Constant":
		return 21
	default:
		return 1 // Text
	}
}

func (s *YakLSPServer) sendResponse(id interface{}, result interface{}, err *rpcError) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	data, jsonErr := json.Marshal(resp)
	if jsonErr != nil {
		log.Errorf("marshal response failed: %v", jsonErr)
		return
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.writer.Write([]byte(header))
	s.writer.Write(data)

	// 确保数据被刷新到 stdout
	if flusher, ok := s.writer.(interface{ Flush() error }); ok {
		flusher.Flush()
	}
}

// StartLSPServer 启动 LSP 服务器的便捷函数
func StartLSPServer(grpcServer *yakgrpc.Server) error {
	server := NewYakLSPServer(grpcServer)
	return server.Start()
}
