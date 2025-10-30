package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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
}

// NewYakLSPServer 创建 LSP 服务器
func NewYakLSPServer(grpcServer *yakgrpc.Server) *YakLSPServer {
	return &YakLSPServer{
		grpcServer: grpcServer,
		reader:     bufio.NewReader(os.Stdin),
		writer:     os.Stdout,
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

	log.Infof("LSP request: %s", req.Method)

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
	case "textDocument/didOpen", "textDocument/didChange", "textDocument/didSave", "textDocument/didClose":
		// 文档同步通知，不需要响应
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
	code, err := readDocumentContent(p.TextDocument.URI)
	if err != nil {
		log.Errorf("read document failed: %v", err)
		return []interface{}{}, nil
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

	code, err := readDocumentContent(p.TextDocument.URI)
	if err != nil {
		return nil, nil
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

	code, err := readDocumentContent(p.TextDocument.URI)
	if err != nil {
		return nil, nil
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

func (s *YakLSPServer) handleDefinition(params json.RawMessage) (interface{}, *rpcError) {
	// TODO: 实现跳转到定义
	return nil, nil
}

func (s *YakLSPServer) handleReferences(params json.RawMessage) (interface{}, *rpcError) {
	// TODO: 实现查找引用
	return []interface{}{}, nil
}

func readDocumentContent(uri string) (string, error) {
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
