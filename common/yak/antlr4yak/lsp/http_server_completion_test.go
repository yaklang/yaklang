package lsp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPServerCompletion_RagLibrary(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: rag.
	testCode := `rag.`

	// 构造 LSP 补全请求参数
	params := struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}{}
	params.TextDocument.URI = "inmemory://test.yak"
	params.Position.Line = 0
	params.Position.Character = 4 // rag. 后面

	// 由于 handleCompletion 需要从 URI 读取文件，我们直接测试 gRPC
	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        testCode, // "rag."
			StartLine:   1,
			StartColumn: 1, // rag 开始
			EndLine:     1,
			EndColumn:   5, // rag. 结束（包含点）
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	t.Logf("✅ 返回 %d 个补全项", len(resp.SuggestionMessage))

	// 验证必须包含 BuildIndexKnowledgeFromFile
	found := false
	var foundItem *ypb.SuggestionDescription
	for _, item := range resp.SuggestionMessage {
		if item.Label == "BuildIndexKnowledgeFromFile" {
			found = true
			foundItem = item
			break
		}
	}

	if !found {
		t.Errorf("❌ 未找到 BuildIndexKnowledgeFromFile")
		t.Logf("所有补全项:")
		for i, item := range resp.SuggestionMessage {
			t.Logf("  [%d] %s", i+1, item.Label)
		}
		t.FailNow()
	}

	t.Logf("✅ 找到 BuildIndexKnowledgeFromFile")
	t.Logf("   Label: %s", foundItem.Label)
	t.Logf("   Kind: %s", foundItem.Kind)
	t.Logf("   InsertText: %s", foundItem.InsertText)
	t.Logf("   Description: %s", foundItem.Description)

	// 验证 InsertText 是否包含 snippet 格式
	if foundItem.InsertText == "" {
		t.Errorf("❌ InsertText 为空")
	}

	// 检查是否包含参数占位符 ${1:...}
	if !strings.Contains(foundItem.InsertText, "${") {
		t.Logf("⚠️  InsertText 不包含 snippet 格式: %s", foundItem.InsertText)
	} else {
		t.Logf("✅ InsertText 包含 snippet 格式")
	}
}

func TestHTTPServerCompletion_StringMethods(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: str.
	testCode := `str = "hello"
str.`

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        "str.", // 只传递 str.
			StartLine:   2,
			StartColumn: 1, // str 开始
			EndLine:     2,
			EndColumn:   5, // str. 结束（包含点）
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	t.Logf("✅ 返回 %d 个补全项", len(resp.SuggestionMessage))

	// 验证包含常见字符串方法
	expectedMethods := []string{"ToUpper", "ToLower", "Split", "Contains"}
	for _, method := range expectedMethods {
		found := false
		for _, item := range resp.SuggestionMessage {
			if strings.Contains(item.Label, method) {
				found = true
				t.Logf("✅ 找到方法: %s (InsertText: %s)", item.Label, item.InsertText)
				break
			}
		}
		if !found {
			t.Errorf("❌ 未找到方法: %s", method)
		}
	}

	// 显示前 10 个补全项
	t.Logf("\n前 10 个补全项:")
	for i, item := range resp.SuggestionMessage {
		if i >= 10 {
			break
		}
		t.Logf("  [%d] %s - %s", i+1, item.Label, item.InsertText)
	}
}

func TestHTTPServerCompletion_SnippetFormat(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: 函数补全
	testCode := `println`

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        testCode,
			StartLine:   1,
			StartColumn: 1, // println 开始
			EndLine:     1,
			EndColumn:   8, // println 结束（7个字符，所以到第8列）
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	t.Logf("✅ 返回 %d 个补全项", len(resp.SuggestionMessage))

	// 查找 println 相关的补全
	for _, item := range resp.SuggestionMessage {
		if item.Label == "println" || strings.HasPrefix(item.Label, "println") {
			t.Logf("找到: %s", item.Label)
			t.Logf("  InsertText: %s", item.InsertText)
			t.Logf("  Kind: %s", item.Kind)

			// 检查是否包含 snippet 格式
			if strings.Contains(item.InsertText, "${") {
				t.Logf("  ✅ 包含 snippet 格式")
			} else {
				t.Logf("  ⚠️  不包含 snippet 格式")
			}
		}
	}
}

func TestHTTPServerCompletion_AllLibraries(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试多个标准库
	libraries := []struct {
		name           string
		code           string
		line           int64
		startCol       int64
		endCol         int64
		expectedMethod string
	}{
		{
			name:           "rag",
			code:           "rag.",
			line:           1,
			startCol:       1, // rag 开始
			endCol:         5, // rag. 结束（包含点）
			expectedMethod: "BuildIndexKnowledgeFromFile",
		},
		{
			name:           "str",
			code:           "str.",
			line:           1,
			startCol:       1,
			endCol:         5,
			expectedMethod: "Split",
		},
		{
			name:           "http",
			code:           "http.",
			line:           1,
			startCol:       1,
			endCol:         6,
			expectedMethod: "Get",
		},
		{
			name:           "file",
			code:           "file.",
			line:           1,
			startCol:       1,
			endCol:         6,
			expectedMethod: "ReadFile",
		},
	}

	for _, lib := range libraries {
		t.Run(lib.name, func(t *testing.T) {
			req := &ypb.YaklangLanguageSuggestionRequest{
				InspectType:   "completion",
				YakScriptType: "yak",
				YakScriptCode: lib.code,
				Range: &ypb.Range{
					Code:        lib.code,
					StartLine:   lib.line,
					StartColumn: lib.startCol,
					EndLine:     lib.line,
					EndColumn:   lib.endCol,
				},
			}

			resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
			if err != nil {
				t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
			}

			t.Logf("✅ %s: 返回 %d 个补全项", lib.name, len(resp.SuggestionMessage))

			// 查找期望的方法
			found := false
			for _, item := range resp.SuggestionMessage {
				if strings.Contains(item.Label, lib.expectedMethod) {
					found = true
					t.Logf("  ✅ 找到 %s", lib.expectedMethod)
					t.Logf("     InsertText: %s", item.InsertText)
					break
				}
			}

			if !found {
				t.Errorf("  ❌ 未找到 %s", lib.expectedMethod)
				t.Logf("  前 10 个补全项:")
				for i, item := range resp.SuggestionMessage {
					if i >= 10 {
						break
					}
					t.Logf("    [%d] %s", i+1, item.Label)
				}
			}
		})
	}
}

func TestHTTPServerCompletion_JSONOutput(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: rag.
	testCode := `rag.`

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        testCode,
			StartLine:   1,
			StartColumn: 1, // rag 开始
			EndLine:     1,
			EndColumn:   5, // rag. 结束（包含点）
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	// 输出前 5 个补全项的 JSON 格式
	t.Logf("\nJSON 输出 (前 5 个补全项):")
	count := 5
	if len(resp.SuggestionMessage) < count {
		count = len(resp.SuggestionMessage)
	}

	jsonData, err := json.MarshalIndent(resp.SuggestionMessage[:count], "", "  ")
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	t.Logf("\n%s", string(jsonData))

	// 验证 JSON 结构
	var items []map[string]interface{}
	if err := json.Unmarshal(jsonData, &items); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	for i, item := range items {
		t.Logf("\n补全项 %d:", i+1)
		if label, ok := item["Label"].(string); ok {
			t.Logf("  Label: %s", label)
		}
		if insertText, ok := item["InsertText"].(string); ok {
			t.Logf("  InsertText: %s", insertText)
		}
		if kind, ok := item["Kind"].(string); ok {
			t.Logf("  Kind: %s", kind)
		}
	}
}
