package aid

import (
	"github.com/stretchr/testify/require"
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

	action, err := ExtractActionFromStream(strings.NewReader(raw), "plan")
	require.NoError(t, err)
	require.Equal(t, "plan", action.ActionType())
	params := action.params
	require.True(t, params.Has("main_task"))
	require.True(t, params.Has("main_task_goal"))
	require.True(t, params.Has("tasks"))
}
