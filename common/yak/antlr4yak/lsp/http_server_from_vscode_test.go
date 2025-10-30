package lsp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc"
)

// TestHTTPServerCompletion_FromVSCode 测试从 VSCode 实际发送的补全请求
// 这些测试用例直接复制自 VSCode 的实际请求，确保服务器能正确处理
func TestHTTPServerCompletion_FromVSCode(t *testing.T) {
	// 创建 gRPC 服务器
	grpcServer, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建 HTTP LSP 服务器
	lspServer := NewYakLSPHTTPServer(grpcServer, "127.0.0.1:0")

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lspServer.handleLSP(w, r)
	}))
	defer httpServer.Close()

	t.Logf("HTTP 测试服务器: %s", httpServer.URL)

	t.Run("VSCode Case: rag. in if block", func(t *testing.T) {
		// 这是用户在 VSCode 中的实际输入场景：
		// Line 0: if a {
		// Line 1:     rag.█  (光标在点号后面)
		// Line 2: }
		//
		// VSCode 发送的请求参数（原封不动）：
		// {
		//   "textDocument": {
		//     "uri": "file:///Users/v1ll4n/Projects/yaklang-support/sampleWorkspace/test.yak",
		//     "text": "if a {\n    rag.\n}"
		//   },
		//   "position": {
		//     "line": 1,        // 第 2 行 (从 0 开始)
		//     "character": 8    // 第 8 个字符位置 (点号后面)
		//   }
		// }

		// 构造请求（原封不动复制 VSCode 的实际请求）
		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1761810212018,
			"method":  "textDocument/completion",
			"params": map[string]interface{}{
				"textDocument": map[string]interface{}{
					"uri":  "file:///Users/v1ll4n/Projects/yaklang-support/sampleWorkspace/test.yak",
					"text": "if a {\n    rag.\n}",
				},
				"position": map[string]interface{}{
					"line":      1,
					"character": 8,
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
			JSONRPC string                   `json:"jsonrpc"`
			ID      int64                    `json:"id"`
			Result  []map[string]interface{} `json:"result"`
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

		items := result.Result

		// 验证：应该返回 rag 库的方法（数量可能会随着库的更新而变化）
		assert.Greater(t, len(items), 40, "应该返回大量的 rag 库方法")
		t.Logf("返回了 %d 个 rag 库方法补全项", len(items))

		// 验证：必须包含 BuildIndexKnowledgeFromFile
		hasTarget := false
		for _, item := range items {
			if label, ok := item["label"].(string); ok {
				if label == "BuildIndexKnowledgeFromFile" {
					hasTarget = true
					// 验证 insertText 格式
					if insertText, ok := item["insertText"].(string); ok {
						assert.Contains(t, insertText, "BuildIndexKnowledgeFromFile(")
						assert.Contains(t, insertText, "kbName")
						assert.Contains(t, insertText, "path")
					}
					break
				}
			}
		}
		assert.True(t, hasTarget, "必须包含 BuildIndexKnowledgeFromFile 方法")

		t.Logf("✅ VSCode case 'rag.' 补全成功，返回 %d 个补全项", len(items))
	})

	t.Run("VSCode Case: str. at line start", func(t *testing.T) {
		// 场景：光标在行首，输入 str.
		// Line 0: str.█
		//
		// VSCode 请求：
		// position: { line: 0, character: 4 }

		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      100,
			"method":  "textDocument/completion",
			"params": map[string]interface{}{
				"textDocument": map[string]interface{}{
					"uri":  "file:///test.yak",
					"text": "str.",
				},
				"position": map[string]interface{}{
					"line":      0,
					"character": 4,
				},
			},
		}

		jsonData, _ := json.Marshal(reqBody)

		resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Result []map[string]interface{} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		items := result.Result

		// str 库有 157 个方法
		assert.Greater(t, len(items), 150, "str 库应该返回 150+ 个方法")

		// 验证包含常见方法
		labels := extractLabels(items)
		assert.Contains(t, labels, "Split")
		assert.Contains(t, labels, "ToUpper")
		assert.Contains(t, labels, "ToLower")

		t.Logf("✅ VSCode case 'str.' 补全成功，返回 %d 个补全项", len(items))
	})

	t.Run("VSCode Case: http. with indentation", func(t *testing.T) {
		// 场景：在缩进后输入 http.
		// Line 0: func test() {
		// Line 1:     http.█
		// Line 2: }

		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      200,
			"method":  "textDocument/completion",
			"params": map[string]interface{}{
				"textDocument": map[string]interface{}{
					"uri":  "file:///test.yak",
					"text": "func test() {\n    http.\n}",
				},
				"position": map[string]interface{}{
					"line":      1,
					"character": 9,
				},
			},
		}

		jsonData, _ := json.Marshal(reqBody)

		resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Result []map[string]interface{} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		items := result.Result

		// http 库有 39 个方法
		assert.Greater(t, len(items), 35, "http 库应该返回 35+ 个方法")

		// 验证包含常见方法
		labels := extractLabels(items)
		assert.Contains(t, labels, "Get")
		assert.Contains(t, labels, "Post")
		assert.Contains(t, labels, "Request")

		t.Logf("✅ VSCode case 'http.' 补全成功，返回 %d 个补全项", len(items))
	})

	t.Run("VSCode Case: multiple lines with empty lines", func(t *testing.T) {
		// 场景：复杂的多行代码
		// Line 0: a = 1
		// Line 1:
		// Line 2: rag.█
		// Line 3:
		// Line 4: b = 2

		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      300,
			"method":  "textDocument/completion",
			"params": map[string]interface{}{
				"textDocument": map[string]interface{}{
					"uri":  "file:///test.yak",
					"text": "a = 1\n\nrag.\n\nb = 2",
				},
				"position": map[string]interface{}{
					"line":      2,
					"character": 4,
				},
			},
		}

		jsonData, _ := json.Marshal(reqBody)

		resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Result []map[string]interface{} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		items := result.Result

		assert.Greater(t, len(items), 40, "应该返回 rag 库的所有方法")

		// 验证包含关键方法
		labels := extractLabels(items)
		assert.Contains(t, labels, "BuildIndexKnowledgeFromFile")
		assert.Contains(t, labels, "BuildCollectionFromFile")

		t.Logf("✅ VSCode case 'rag.' (多行) 补全成功，返回 %d 个补全项", len(items))
	})

	t.Run("VSCode Case: nested blocks", func(t *testing.T) {
		// 场景：嵌套的代码块
		// Line 0: for i in range(10) {
		// Line 1:     if i > 5 {
		// Line 2:         str.█
		// Line 3:     }
		// Line 4: }

		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      400,
			"method":  "textDocument/completion",
			"params": map[string]interface{}{
				"textDocument": map[string]interface{}{
					"uri":  "file:///test.yak",
					"text": "for i in range(10) {\n    if i > 5 {\n        str.\n    }\n}",
				},
				"position": map[string]interface{}{
					"line":      2,
					"character": 12,
				},
			},
		}

		jsonData, _ := json.Marshal(reqBody)

		resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Result []map[string]interface{} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		items := result.Result

		assert.Greater(t, len(items), 150, "str 库应该返回 150+ 个方法")

		labels := extractLabels(items)
		assert.Contains(t, labels, "Split")

		t.Logf("✅ VSCode case 'str.' (嵌套块) 补全成功，返回 %d 个补全项", len(items))
	})
}

// extractLabels 从补全项中提取所有 label
func extractLabels(items []map[string]interface{}) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		if label, ok := item["label"].(string); ok {
			labels = append(labels, label)
		}
	}
	return labels
}
