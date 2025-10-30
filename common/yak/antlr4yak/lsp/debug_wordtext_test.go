package lsp

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDebugWordText(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testCode := `rag.`

	t.Logf("测试代码: %q", testCode)
	t.Logf("长度: %d", len(testCode))
	t.Logf("字符: %v", []rune(testCode))

	// 测试不同的 Range 位置
	testCases := []struct {
		name        string
		startLine   int64
		startColumn int64
		endLine     int64
		endColumn   int64
	}{
		{"光标在 rag. 后面（4,4）", 1, 4, 1, 4},
		{"光标在点前面（3,3）", 1, 3, 1, 3},
		{"光标在点上（3,4）", 1, 3, 1, 4},
		{"选中 rag（0,3）", 1, 0, 1, 3},
		{"选中 rag.（0,4）", 1, 0, 1, 4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &ypb.YaklangLanguageSuggestionRequest{
				InspectType:   "completion",
				YakScriptType: "yak",
				YakScriptCode: testCode,
				Range: &ypb.Range{
					Code:        testCode,
					StartLine:   tc.startLine,
					StartColumn: tc.startColumn,
					EndLine:     tc.endLine,
					EndColumn:   tc.endColumn,
				},
			}

			_, err := server.YaklangLanguageSuggestion(context.Background(), req)
			if err != nil {
				t.Logf("  Error: %v", err)
			}
		})
	}
}
