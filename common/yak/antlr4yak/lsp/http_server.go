package lsp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// YakLSPHTTPServer HTTP LSP 服务器
type YakLSPHTTPServer struct {
	grpcServer *yakgrpc.Server
	addr       string
	server     *http.Server
}

// NewYakLSPHTTPServer 创建 HTTP LSP 服务器
func NewYakLSPHTTPServer(grpcServer *yakgrpc.Server, addr string) *YakLSPHTTPServer {
	return &YakLSPHTTPServer{
		grpcServer: grpcServer,
		addr:       addr,
	}
}

// Start 启动 HTTP LSP 服务器
func (s *YakLSPHTTPServer) Start() error {
	mux := http.NewServeMux()

	// LSP endpoint
	mux.HandleFunc("/lsp", s.handleLSP)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"server": "yaklang-lsp-http",
		})
	})

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	log.Infof("yaklang HTTP LSP server starting on %s", s.addr)
	return s.server.ListenAndServe()
}

// Stop 停止服务器
func (s *YakLSPHTTPServer) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

func (s *YakLSPHTTPServer) handleLSP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 打印原始请求体
	log.Infof("================================================================================")
	log.Infof("[LSP HTTP Server] 收到 HTTP 请求")
	log.Infof("[LSP HTTP Server] Method: %s", r.Method)
	log.Infof("[LSP HTTP Server] URL: %s", r.URL.String())
	log.Infof("[LSP HTTP Server] Headers: %+v", r.Header)
	log.Infof("[LSP HTTP Server] Raw Body (原始JSON):")
	log.Infof("%s", string(body))
	log.Infof("================================================================================")

	// 解析 JSON-RPC 请求
	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Errorf("Invalid JSON-RPC request: %v", err)
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	log.Infof("HTTP LSP request: %s (id: %v)", req.Method, req.ID)

	// 处理请求
	var result interface{}
	var rpcErr *rpcError

	switch req.Method {
	case "initialize":
		result = s.handleInitialize(req.Params)
	case "initialized":
		// 客户端初始化完成通知
		w.WriteHeader(http.StatusOK)
		return
	case "shutdown":
		result = nil
	case "exit":
		w.WriteHeader(http.StatusOK)
		return
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
	case "textDocument/diagnostics":
		result, rpcErr = s.handleDiagnostics(req.Params)
	case "textDocument/didOpen", "textDocument/didChange", "textDocument/didSave", "textDocument/didClose":
		// 文档同步通知，不需要响应
		w.WriteHeader(http.StatusOK)
		return
	default:
		log.Warnf("unhandled LSP method: %s", req.Method)
		rpcErr = &rpcError{
			Code:    -32601,
			Message: "Method not found",
		}
	}

	// 发送响应
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	}

	// 打印原始响应
	respBytes, _ := json.MarshalIndent(resp, "", "  ")
	log.Infof("================================================================================")
	log.Infof("[LSP HTTP Server] 发送 HTTP 响应")
	log.Infof("[LSP HTTP Server] Response Body (原始JSON):")
	log.Infof("%s", string(respBytes))
	log.Infof("================================================================================")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *YakLSPHTTPServer) handleInitialize(params json.RawMessage) interface{} {
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
			"name":    "yaklang-lsp-http",
			"version": "1.0.0",
		},
	}
}

func (s *YakLSPHTTPServer) handleCompletion(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text,omitempty"` // 支持直接传递文档内容
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

	// 优先使用直接传递的文档内容，否则从文件读取
	code := p.TextDocument.Text
	if code == "" {
		var err error
		code, err = readDocumentContent(p.TextDocument.URI)
		if err != nil {
			log.Errorf("read document failed: %v", err)
			return []interface{}{}, nil
		}
	}

	// 使用 memedit 获取光标位置的单词（复刻 yakit 客户端逻辑）
	editor := memedit.NewMemEditor(code)
	// LSP 坐标从 0 开始，memedit 坐标从 1 开始，所以都需要 +1
	position := editor.GetPositionByLine(p.Position.Line+1, p.Position.Character+1)

	// 使用 GetWordWithPointAtPosition 获取包含 "." 的完整单词
	wordText, wordStart, wordEnd := editor.GetWordWithPointAtPosition(position)

	// 关键修复：确保 endPosition 包含点号
	// 问题：GrpcRangeToSSARange 会根据 StartLine/StartColumn/EndLine/EndColumn 重新从源码中提取文本
	// 所以我们必须确保 endPosition 指向正确的位置，使得提取的文本包含点号
	endPosition := wordEnd
	cursorOffset := editor.GetOffsetByPosition(position)

	// 情况1：光标在点号之后（例如 "ai.|"，光标在第3列，点号在第2列）
	// 此时 cursorOffset 指向点号后面，我们需要检查点号是否存在
	if cursorOffset > 0 {
		charBeforeCursor := editor.GetTextFromOffset(cursorOffset-1, cursorOffset)
		if charBeforeCursor == "." {
			// 光标在点号后面，endPosition 应该指向光标位置（点号后面）
			endPosition = position
		}
	}

	// 情况2：光标在点号之前（例如 "ai|."，光标在第2列）
	// 此时我们需要检查光标后面是否有点号
	if cursorOffset < editor.CodeLength() {
		charAtCursor := editor.GetTextFromOffset(cursorOffset, cursorOffset+1)
		if charAtCursor == "." {
			// 光标在点号前面，endPosition 应该指向点号后面
			endPosition = editor.GetPositionByOffset(cursorOffset + 1)
		}
	}

	rangeCode := editor.GetTextFromRange(editor.GetRangeByPosition(wordStart, endPosition))

	log.Infof("--------------------------------------------------------------------------------")
	log.Infof("[LSP HTTP Completion] 处理补全请求")
	log.Infof("[LSP HTTP Completion] Position (LSP): Line %d Col %d", p.Position.Line, p.Position.Character)
	log.Infof("[LSP HTTP Completion] Position (memedit +1): Line %d Col %d", p.Position.Line+1, p.Position.Character+1)
	log.Infof("[LSP HTTP Completion] WordText: %q", wordText)
	log.Infof("[LSP HTTP Completion] RangeCode: %q", rangeCode)
	log.Infof("[LSP HTTP Completion] WordStart: Line %d Col %d", wordStart.GetLine(), wordStart.GetColumn())
	log.Infof("[LSP HTTP Completion] WordEnd: Line %d Col %d", wordEnd.GetLine(), wordEnd.GetColumn())
	log.Infof("[LSP HTTP Completion] EndPosition (after fix): Line %d Col %d", endPosition.GetLine(), endPosition.GetColumn())
	log.Infof("[LSP HTTP Completion] Code length: %d bytes", len(code))
	log.Infof("[LSP HTTP Completion] Code (first 200 chars): %q", truncateString(code, 200))
	log.Infof("--------------------------------------------------------------------------------")

	// 调用 gRPC 服务获取补全（完全复刻 yakit 客户端参数）
	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.COMPLETION,
		YakScriptType: "yak",
		YakScriptCode: code, // 完整代码
		Range: &ypb.Range{
			Code:        rangeCode, // 关键：这里是光标处的单词（如 "rag."）
			StartLine:   int64(wordStart.GetLine()),
			StartColumn: int64(wordStart.GetColumn()),
			EndLine:     int64(endPosition.GetLine()),
			EndColumn:   int64(endPosition.GetColumn()),
		},
	}

	log.Infof("--------------------------------------------------------------------------------")
	log.Infof("[LSP HTTP Completion] 调用 gRPC YaklangLanguageSuggestion")
	log.Infof("[LSP HTTP Completion] gRPC Request:")
	log.Infof("  InspectType: %s", yakgrpc.COMPLETION)
	log.Infof("  YakScriptType: yak")
	log.Infof("  YakScriptCode: %q (length: %d)", truncateString(code, 200), len(code))
	log.Infof("  Range.Code: %q", rangeCode)
	log.Infof("  Range.StartLine: %d", req.Range.StartLine)
	log.Infof("  Range.StartColumn: %d", req.Range.StartColumn)
	log.Infof("  Range.EndLine: %d", req.Range.EndLine)
	log.Infof("  Range.EndColumn: %d", req.Range.EndColumn)
	log.Infof("--------------------------------------------------------------------------------")

	resp, err := s.grpcServer.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		log.Errorf("get completion failed: %v", err)
		return []interface{}{}, nil
	}

	log.Infof("--------------------------------------------------------------------------------")
	log.Infof("[LSP HTTP Completion] gRPC 响应")
	log.Infof("[LSP HTTP Completion] Got %d suggestions for word %q", len(resp.SuggestionMessage), wordText)
	if len(resp.SuggestionMessage) > 0 {
		log.Infof("[LSP HTTP Completion] First 5 suggestions:")
		for i := 0; i < min(5, len(resp.SuggestionMessage)); i++ {
			log.Infof("  %d. %s (kind: %s)", i+1, resp.SuggestionMessage[i].Label, resp.SuggestionMessage[i].Kind)
		}
	}
	log.Infof("--------------------------------------------------------------------------------")

	// 转换为 LSP CompletionItem
	items := make([]map[string]interface{}, 0, len(resp.SuggestionMessage))
	for _, item := range resp.SuggestionMessage {
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

	return items, nil
}

func (s *YakLSPHTTPServer) handleHover(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text,omitempty"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	code := p.TextDocument.Text
	if code == "" {
		var err error
		code, err = readDocumentContent(p.TextDocument.URI)
		if err != nil {
			return nil, nil
		}
	}

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

func (s *YakLSPHTTPServer) handleSignatureHelp(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text,omitempty"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	code := p.TextDocument.Text
	if code == "" {
		var err error
		code, err = readDocumentContent(p.TextDocument.URI)
		if err != nil {
			return nil, nil
		}
	}

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

func (s *YakLSPHTTPServer) handleDefinition(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text,omitempty"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	code := p.TextDocument.Text
	if code == "" {
		var err error
		code, err = readDocumentContent(p.TextDocument.URI)
		if err != nil {
			return nil, nil
		}
	}

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.DEFINITION,
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

	resp, err := s.grpcServer.YaklangLanguageFind(context.Background(), req)
	if err != nil || resp == nil {
		return nil, nil
	}

	// 返回定义位置
	result := map[string]interface{}{
		"uri":    resp.URI,
		"ranges": make([]map[string]interface{}, 0, len(resp.Ranges)),
	}

	for _, r := range resp.Ranges {
		result["ranges"] = append(result["ranges"].([]map[string]interface{}), map[string]interface{}{
			"startLine":   r.StartLine,
			"startColumn": r.StartColumn,
			"endLine":     r.EndLine,
			"endColumn":   r.EndColumn,
		})
	}

	return result, nil
}

func (s *YakLSPHTTPServer) handleReferences(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text,omitempty"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
		Context struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	code := p.TextDocument.Text
	if code == "" {
		var err error
		code, err = readDocumentContent(p.TextDocument.URI)
		if err != nil {
			return []interface{}{}, nil
		}
	}

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.REFERENCES,
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

	resp, err := s.grpcServer.YaklangLanguageFind(context.Background(), req)
	if err != nil || resp == nil {
		return []interface{}{}, nil
	}

	// 返回引用位置
	result := map[string]interface{}{
		"uri":    resp.URI,
		"ranges": make([]map[string]interface{}, 0, len(resp.Ranges)),
	}

	for _, r := range resp.Ranges {
		result["ranges"] = append(result["ranges"].([]map[string]interface{}), map[string]interface{}{
			"startLine":   r.StartLine,
			"startColumn": r.StartColumn,
			"endLine":     r.EndLine,
			"endColumn":   r.EndColumn,
		})
	}

	return result, nil
}

func (s *YakLSPHTTPServer) handleDiagnostics(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		TextDocument struct {
			URI        string `json:"uri"`
			LanguageID string `json:"languageId"`
			Text       string `json:"text"`
		} `json:"textDocument"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	// 使用提供的文本，如果没有则尝试从 URI 读取
	code := p.TextDocument.Text
	if code == "" {
		var err error
		code, err = readDocumentContent(p.TextDocument.URI)
		if err != nil {
			return []interface{}{}, nil
		}
	}

	// 确定脚本类型
	scriptType := p.TextDocument.LanguageID
	if scriptType == "" {
		scriptType = "yak"
	}

	// 调用静态分析接口
	req := &ypb.StaticAnalyzeErrorRequest{
		Code:       []byte(code),
		PluginType: scriptType,
	}

	resp, err := s.grpcServer.StaticAnalyzeError(context.Background(), req)
	if err != nil {
		log.Errorf("static analyze error failed: %v", err)
		return []interface{}{}, nil
	}

	// 转换为诊断信息
	diagnostics := make([]map[string]interface{}, 0, len(resp.Result))
	for _, item := range resp.Result {
		diagnostic := map[string]interface{}{
			"startLineNumber": item.StartLineNumber,
			"endLineNumber":   item.EndLineNumber,
			"startColumn":     item.StartColumn,
			"endColumn":       item.EndColumn,
			"message":         string(item.Message),
			"rawMessage":      string(item.RawMessage),
			"severity":        item.Severity,
			"tag":             item.Tag,
		}
		diagnostics = append(diagnostics, diagnostic)
	}

	return diagnostics, nil
}

// StartLSPHTTPServer 启动 HTTP LSP 服务器的便捷函数
func StartLSPHTTPServer(grpcServer *yakgrpc.Server, addr string) error {
	server := NewYakLSPHTTPServer(grpcServer, addr)
	return server.Start()
}

// 辅助函数：截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// 辅助函数：返回两个整数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
