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

	// 解析 JSON-RPC 请求
	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
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

	// 🔥 关键修复：如果光标后面紧跟着 "."，需要把它包含进来
	// 这样 Range.Code 才会包含点号，trimSourceCode 才能正确检测 PointSuffix
	rangeCode := wordText
	endPosition := wordEnd
	endOffset := editor.GetOffsetByPosition(wordEnd)
	if endOffset < editor.CodeLength() {
		nextChar := editor.GetTextFromOffset(endOffset, endOffset+1)
		if nextChar == "." {
			rangeCode = wordText + "."
			endPosition = editor.GetPositionByLine(wordEnd.GetLine(), wordEnd.GetColumn()+1)
		}
	}

	log.Infof("[LSP HTTP Completion] Position: Line %d Col %d, WordText: %q, RangeCode: %q",
		p.Position.Line+1, p.Position.Character, wordText, rangeCode)

	// 调用 gRPC 服务获取补全（完全复刻 yakit 客户端参数）
	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   yakgrpc.COMPLETION,
		YakScriptType: "yak",
		YakScriptCode: code, // 完整代码
		Range: &ypb.Range{
			Code:        rangeCode, // ⚠️ 关键：这里是光标处的单词（如 "rag."）
			StartLine:   int64(wordStart.GetLine()),
			StartColumn: int64(wordStart.GetColumn()),
			EndLine:     int64(endPosition.GetLine()),
			EndColumn:   int64(endPosition.GetColumn()),
		},
	}

	resp, err := s.grpcServer.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		log.Errorf("get completion failed: %v", err)
		return []interface{}{}, nil
	}

	log.Infof("[LSP HTTP Completion] Got %d suggestions for word %q", len(resp.SuggestionMessage), wordText)

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
