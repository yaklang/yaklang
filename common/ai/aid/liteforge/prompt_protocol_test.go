package liteforge

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

func TestOutputProtocolInstructionAndWireName(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, name := range []string{"result", "memory/triage"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, name), func(t *testing.T) {
				_, err := Execute(context.Background(), Request{ActionName: name, Schema: openSchema,
					StaticInstruction: "提取已确认的长期约束", Prompt: "本次输入只允许读取文件"},
					testOptions(native, func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						projection := aiprojection.ProjectAndObserve("protocol-instruction-test", req.GetPrompt())
						require.NotNil(t, projection)
						system := fmt.Sprint(projection.Messages[0].Content)
						require.NotContains(t, system, "提取已确认的长期约束")
						require.NotContains(t, system, "本次输入只允许读取文件")
						if native {
							require.Contains(t, system, "普通 assistant content 不是结果通道")
							require.NotContains(t, system, "# 文本提交协议")
							require.Len(t, projection.Tools, 1)
							wireName := projection.Tools[0].Function.Name
							require.Contains(t, req.GetPrompt(), "必须实际调用 `"+wireName+"` 一次")
							require.NotContains(t, system, "必须实际调用 `"+wireName+"`")
						} else {
							require.Contains(t, system, "# 文本提交协议")
							require.NotContains(t, system, "# 参数流常见边界")
							require.Empty(t, projection.Tools)
						}
						return response(c, req, native, `{"summary":"原意保留","payload":null}`), nil
					})...)
				require.NoError(t, err)
			})
		}
	}
}
