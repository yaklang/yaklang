package taskstack

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestDefaultSummaryAICallback(t *testing.T) {
	// 创建一个模拟的TaskSystemContext
	ctx := &TaskSystemContext{
		CurrentTask: &Task{
			AICallback: func(req *AIRequest) (*AIResponse, error) {
				// 模拟AI回调返回一个简单的总结
				resp := NewAIResponse()
				defer resp.Close()
				resp.EmitOutputStream(bytes.NewReader([]byte("这是一个测试总结")))
				return resp, nil
			},
		},
	}

	// 准备测试数据
	testDetails := []aispec.ChatDetail{
		{
			Role:    "user",
			Content: "这是一个需要总结的测试内容",
		},
	}

	// 调用被测试的函数
	result, err := DefaultSummaryAICallback(ctx, testDetails...)

	// 验证结果
	if err != nil {
		t.Fatalf("DefaultSummaryAICallback返回错误: %v", err)
	}
	require.NoError(t, err, "DefaultSummaryAICallback返回了错误")
	require.NotNil(t, result, "result 不能为 nil")
	// 读取结果内容
	resultBytes, err := io.ReadAll(result)
	require.NoError(t, err, "读取结果时出错")

	// 验证结果内容
	if string(resultBytes) != "这是一个测试总结" {
		t.Errorf("预期结果为 '这是一个测试总结'，但得到 '%s'", string(resultBytes))
	}
}
