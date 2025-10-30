package lsp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

// TestHTTPCompletion_RagDot 测试 rag. 补全（模拟 TS 客户端）
func TestHTTPCompletion_RagDot(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建 HTTP LSP 服务器
	lspServer := NewYakLSPHTTPServer(server, "127.0.0.1:0")

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lspServer.handleLSP(w, r)
	}))
	defer httpServer.Close()

	t.Logf("HTTP 测试服务器: %s", httpServer.URL)

	// 测试代码: rag.
	testCode := `rag.`

	// 构造请求（模拟 TS 客户端）
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "textDocument/completion",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":  "file:///test.yak",
				"text": testCode, // TS 客户端直接传递文档内容
			},
			"position": map[string]interface{}{
				"line":      0, // TS 客户端：line 从 0 开始
				"character": 4, // rag. 后面
			},
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	t.Logf("请求内容:\n%s", string(jsonData))

	// 发送请求
	resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var result struct {
		JSONRPC string        `json:"jsonrpc"`
		ID      int           `json:"id"`
		Result  []interface{} `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("LSP error: %s", result.Error.Message)
	}

	t.Logf("✅ 返回 %d 个补全项", len(result.Result))

	// 查找 BuildIndexKnowledgeFromFile
	found := false
	for _, item := range result.Result {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if label, ok := itemMap["label"].(string); ok {
			if label == "BuildIndexKnowledgeFromFile" {
				found = true
				t.Logf("✅ 找到 BuildIndexKnowledgeFromFile!")
				if insertText, ok := itemMap["insertText"].(string); ok {
					t.Logf("   InsertText: %s", insertText)
				}
				break
			}
		}
	}

	if !found {
		t.Errorf("❌ 未找到 BuildIndexKnowledgeFromFile")
		t.Logf("前 20 个补全项:")
		for i, item := range result.Result {
			if i >= 20 {
				break
			}
			itemMap, _ := item.(map[string]interface{})
			label, _ := itemMap["label"].(string)
			kind, _ := itemMap["kind"].(float64)
			t.Logf("  [%d] %s (kind: %.0f)", i+1, label, kind)
		}
	}
}

// TestHTTPCompletion_StrDot 测试 str. 补全（字符串方法）
func TestHTTPCompletion_StrDot(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建 HTTP LSP 服务器
	lspServer := NewYakLSPHTTPServer(server, "127.0.0.1:0")

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lspServer.handleLSP(w, r)
	}))
	defer httpServer.Close()

	// 测试代码: str = "hello"\nstr.
	testCode := `str = "hello"
str.`

	// 构造请求
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/completion",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":  "file:///test.yak",
				"text": testCode,
			},
			"position": map[string]interface{}{
				"line":      1, // 第二行（从 0 开始）
				"character": 4, // str. 后面
			},
		},
	}

	jsonData, _ := json.Marshal(reqBody)

	// 发送请求
	resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var result struct {
		JSONRPC string        `json:"jsonrpc"`
		ID      int           `json:"id"`
		Result  []interface{} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	t.Logf("✅ 返回 %d 个补全项", len(result.Result))

	// 查找常见字符串方法
	expectedMethods := []string{"ToUpper", "ToLower", "Split"}
	for _, method := range expectedMethods {
		found := false
		for _, item := range result.Result {
			itemMap, _ := item.(map[string]interface{})
			if label, ok := itemMap["label"].(string); ok {
				if label == method {
					found = true
					t.Logf("✅ 找到方法: %s", method)
					break
				}
			}
		}
		if !found {
			t.Errorf("❌ 未找到方法: %s", method)
		}
	}

	// 显示前 10 个补全项
	t.Logf("\n前 10 个补全项:")
	for i, item := range result.Result {
		if i >= 10 {
			break
		}
		itemMap, _ := item.(map[string]interface{})
		label, _ := itemMap["label"].(string)
		insertText, _ := itemMap["insertText"].(string)
		t.Logf("  [%d] %s -> %s", i+1, label, insertText)
	}
}

// TestHTTPCompletion_XEqualRagDot 测试 x = rag; x. 补全
func TestHTTPCompletion_XEqualRagDot(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建 HTTP LSP 服务器
	lspServer := NewYakLSPHTTPServer(server, "127.0.0.1:0")

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lspServer.handleLSP(w, r)
	}))
	defer httpServer.Close()

	// 测试代码: x = rag\nx.
	testCode := `x = rag
x.`

	// 构造请求
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/completion",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":  "file:///test.yak",
				"text": testCode,
			},
			"position": map[string]interface{}{
				"line":      1, // 第二行
				"character": 2, // x. 后面
			},
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	t.Logf("测试代码:\n%s", testCode)
	t.Logf("光标位置: line=1, character=2")

	// 发送请求
	resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var result struct {
		JSONRPC string        `json:"jsonrpc"`
		ID      int           `json:"id"`
		Result  []interface{} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	t.Logf("✅ 返回 %d 个补全项", len(result.Result))

	// 查找 BuildIndexKnowledgeFromFile
	found := false
	for _, item := range result.Result {
		itemMap, _ := item.(map[string]interface{})
		if label, ok := itemMap["label"].(string); ok {
			if label == "BuildIndexKnowledgeFromFile" {
				found = true
				t.Logf("✅ 找到 BuildIndexKnowledgeFromFile!")
				if insertText, ok := itemMap["insertText"].(string); ok {
					t.Logf("   InsertText: %s", insertText)
				}
				break
			}
		}
	}

	if !found {
		t.Logf("❌ 未找到 BuildIndexKnowledgeFromFile")
		t.Logf("前 20 个补全项:")
		for i, item := range result.Result {
			if i >= 20 {
				break
			}
			itemMap, _ := item.(map[string]interface{})
			label, _ := itemMap["label"].(string)
			t.Logf("  [%d] %s", i+1, label)
		}
		t.Errorf("期望找到 BuildIndexKnowledgeFromFile，但实际未找到")
	}
}
