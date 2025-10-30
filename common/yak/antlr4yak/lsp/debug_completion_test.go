package lsp

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDebugCompletion_RagDot(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: rag.
	testCode := `rag.`

	t.Logf("测试代码: %q", testCode)
	t.Logf("光标位置: Line 1, Column 4 (rag. 后面)")

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        testCode,
			StartLine:   1,
			StartColumn: 1, // rag 开始位置（从 1 开始计数）
			EndLine:     1,
			EndColumn:   5, // rag. 结束位置（包含点）
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	t.Logf("\n========================================")
	t.Logf("返回 %d 个补全项", len(resp.SuggestionMessage))
	t.Logf("========================================\n")

	// 显示前 20 个补全项
	t.Logf("前 20 个补全项:")
	for i, item := range resp.SuggestionMessage {
		if i >= 20 {
			break
		}
		t.Logf("[%d] Label=%s, InsertText=%s, Kind=%s",
			i+1, item.Label, item.InsertText, item.Kind)
	}

	// 搜索 BuildIndexKnowledgeFromFile
	t.Logf("\n搜索 'BuildIndexKnowledgeFromFile':")
	found := false
	for i, item := range resp.SuggestionMessage {
		if item.Label == "BuildIndexKnowledgeFromFile" {
			found = true
			t.Logf("找到! 位置: [%d]", i+1)
			t.Logf("   Label: %s", item.Label)
			t.Logf("   InsertText: %s", item.InsertText)
			t.Logf("   Kind: %s", item.Kind)
			t.Logf("   Description: %s", item.Description)
			break
		}
	}
	if !found {
		t.Errorf("未找到 BuildIndexKnowledgeFromFile")
	}

	// 搜索所有包含 "rag" 的补全项
	t.Logf("\n所有包含 'rag' 的补全项:")
	for i, item := range resp.SuggestionMessage {
		if item.Label == "rag" {
			t.Logf("[%d] Label=%s, InsertText=%s, Kind=%s",
				i+1, item.Label, item.InsertText, item.Kind)
		}
	}
}

func TestDebugCompletion_RagNoDot(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: rag (没有点)
	testCode := `rag`

	t.Logf("测试代码: %q", testCode)
	t.Logf("光标位置: Line 1, Column 3 (rag 后面)")

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        testCode,
			StartLine:   1,
			StartColumn: 3, // rag 后面
			EndLine:     1,
			EndColumn:   3,
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	t.Logf("\n========================================")
	t.Logf("返回 %d 个补全项", len(resp.SuggestionMessage))
	t.Logf("========================================\n")

	// 显示前 10 个补全项
	t.Logf("前 10 个补全项:")
	for i, item := range resp.SuggestionMessage {
		if i >= 10 {
			break
		}
		t.Logf("[%d] Label=%s, InsertText=%s, Kind=%s",
			i+1, item.Label, item.InsertText, item.Kind)
	}

	// 搜索 rag 库
	t.Logf("\n搜索 'rag' 库:")
	found := false
	for i, item := range resp.SuggestionMessage {
		if item.Label == "rag" {
			found = true
			t.Logf("找到! 位置: [%d]", i+1)
			t.Logf("   Label: %s", item.Label)
			t.Logf("   InsertText: %s", item.InsertText)
			t.Logf("   Kind: %s", item.Kind)
			break
		}
	}
	if !found {
		t.Errorf("未找到 rag 库")
	}
}

func TestDebugCompletion_RagWithCode(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 测试代码: 赋值后再补全
	testCode := `x = rag
x.`

	t.Logf("测试代码:\n%s", testCode)
	t.Logf("光标位置: Line 2, Column 2 (x. 后面)")

	req := &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   "completion",
		YakScriptType: "yak",
		YakScriptCode: testCode,
		Range: &ypb.Range{
			Code:        "x.", // 只传递 x.
			StartLine:   2,
			StartColumn: 1, // x 开始位置
			EndLine:     2,
			EndColumn:   3, // x. 结束位置（包含点）
		},
	}

	resp, err := server.YaklangLanguageSuggestion(context.Background(), req)
	if err != nil {
		t.Fatalf("YaklangLanguageSuggestion failed: %v", err)
	}

	t.Logf("\n========================================")
	t.Logf("返回 %d 个补全项", len(resp.SuggestionMessage))
	t.Logf("========================================\n")

	// 显示所有补全项
	t.Logf("所有补全项:")
	for i, item := range resp.SuggestionMessage {
		t.Logf("[%d] Label=%s, InsertText=%s, Kind=%s",
			i+1, item.Label, item.InsertText, item.Kind)
		if i >= 20 {
			t.Logf("... (%d more)", len(resp.SuggestionMessage)-i-1)
			break
		}
	}

	// 搜索 BuildIndexKnowledgeFromFile
	t.Logf("\n搜索 'BuildIndexKnowledgeFromFile':")
	found := false
	for i, item := range resp.SuggestionMessage {
		if item.Label == "BuildIndexKnowledgeFromFile" {
			found = true
			t.Logf("找到! 位置: [%d]", i+1)
			t.Logf("   Label: %s", item.Label)
			t.Logf("   InsertText: %s", item.InsertText)
			t.Logf("   Kind: %s", item.Kind)
			break
		}
	}
	if found {
		t.Logf("测试通过：x = rag; x. 能正确补全")
	} else {
		t.Errorf("x = rag; x. 无法补全 rag 的方法")
	}
}
