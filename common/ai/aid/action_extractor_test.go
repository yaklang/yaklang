package aid

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
	"testing"
)

func TestAction_Extractor(t *testing.T) {
	raw := `{
    "type": "object",
    "required": [
        "@action",
        "tasks",
        "main_task",
        "main_task_goal"
    ],
    "properties": {
        "@action": {
            "const": "plan"
        },
        "main_task": {
            "type": "string",
            "description": "对指定目标进行 XSS 漏洞检测，识别输入点并注入测试 payload，输出漏洞分析结论"
        },
        "main_task_goal": {
            "type": "string",
            "description": "完成目标的 XSS 漏洞扫描与验证，判断是否存在反射型、存储型或 DOM 型 XSS 漏洞，并输出有效 payload 和响应结果"
        },
        "tasks": []
    }
}
`

	action, err := aicommon.ExtractActionFromStream(strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, "plan", action.ActionType())
	params := action.GetParams()
	require.True(t, params.Has("main_task"))
	require.True(t, params.Has("main_task_goal"))
	require.True(t, params.Has("tasks"))
}

func TestWaitAction_Extractor(t *testing.T) {
	token := uuid.NewString()
	raw := fmt.Sprintf(`{
    "type": "object",
    "required": [
        "@action",
        "tasks",
        "main_task",
        "main_task_goal"
    ],
    "properties": {
        "@action": {
            "const": "plan"
        },
		"mytest": "%s",
        "main_task": {
            "type": "string",
            "description": "对指定目标进行 XSS 漏洞检测，识别输入点并注入测试 payload，输出漏洞分析结论"
        },
        "main_task_goal": {
            "type": "string",
            "description": "完成目标的 XSS 漏洞扫描与验证，判断是否存在反射型、存储型或 DOM 型 XSS 漏洞，并输出有效 payload 和响应结果"
        },
        "tasks": []
    }
}
`, token)

	action, err := aicommon.ExtractWaitableActionFromStream(context.Background(), strings.NewReader(raw), "plan")
	require.NoError(t, err)

	require.Equal(t, action.WaitString("mytest"), token)

}
