package aid

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestSearchAIYakTool(t *testing.T) {
	coordinator, err := NewCoordinator("test", WithAiToolsSearchTool())
	assert.NilError(t, err)

	tools, err := coordinator.config.aiToolManager.GetAllTools()
	assert.NilError(t, err)

	hasSearchTool := false
	hasMemoryTool := false

	for _, tool := range tools {
		if tool.Name == "tools_search" {
			hasSearchTool = true
		}
		if strings.Contains(tool.Name, "memory") {
			hasMemoryTool = true
		}
	}

	assert.Assert(t, hasSearchTool)
	assert.Assert(t, hasMemoryTool)
}

func TestDecodeBase64BySearchTool(t *testing.T) {
	stateKeyword := [][2]string{
		// 规划任务
		{
			"你是一个输出JSON的任务规划的工具", `{
  "@action": "plan",
  "main_task": "解密提供的Base64字符串。",
  "main_task_goal": "成功解密给定的Base64编码字符串，并得到明文内容。",
  "tasks": [
    {
      "subtask_name": "使用base64工具解码字符串",
      "subtask_goal": "检查decode工具的输出，确认其是否为可读的明文，尝试理解其含义，如果解码失败（例如，出现乱码或无意义字符），则考虑其他的解密方案。"
    }
  ],
  "query": ""
}
`,
		},
		// 申请搜索工具
		{
			`[-] "解密提供的Base64字符串。" `, `{"tool": "tools_search", "@action": "require-tool"}`,
		},
		// 调用搜索工具
		{
			"只生成一个有效的请求参数", `
	{
  "tool": "tools_search",
  "@action": "call-tool",
  "params": {
    "query": "base64解码工具"
  }
}`,
		},
		// 搜索工具执行
		{
			"你是一个智能关键词匹配助手", `
	[
      {
        "tool": "decode",
        "reason": "可以用于base64解码"
      }
    ]`,
		},
		// 判断任务情况
		{
			"成功执行了外部工具", `{"@action": "require-more-tool"}`,
		},
		// 申请解码工具
		{
			"你是一个任务执行助手，根据既定的任务清单", `{"tool": "decode", "@action": "require-tool"}`,
		},
		// 调用解码工具
		{
			"请根据Schema描述构造有效JSON对象来调用此工具，系统会执行工具内容", `
	{
  "tool": "decode",
  "@action": "call-tool",
  "params": {
    "text": "eWFrbGFuZw==",
    "type": "base64"
  }
}`,
		},
		{
			"成功执行了外部工具", `{"@action": "finished"}`,
		},
		{
			"你是一个按Schema输出JSON的上下文总结者", `
			{
			"@action": "summary",
			"short_summary": "成功解码了Base64字符串",
			"long_summary": "成功解码了Base64字符串，并得到了明文内容"
			}`,
		},
		{
			"你是一个输出 Markdown 计划书和报告的工具", "ok",
		},
	}

	currentStateIndex := 0
	coordinator, err := NewCoordinator("帮我解码一个base64编码的字符串: eWFrbGFuZw==",
		WithAiToolsSearchTool(),
		WithYOLO(),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			prompt := request.GetPrompt()
			pair := stateKeyword[currentStateIndex]
			pateKeyword := pair[0]
			aiRsp := pair[1]
			if strings.Contains(prompt, pateKeyword) {
				currentStateIndex++
				rsp := config.NewAIResponse()
				rsp.EmitOutputStream(strings.NewReader(aiRsp))
				rsp.Close()
				return rsp, nil
			} else {
				t.Fatalf("pateKeyword: %s, prompt: %s", pateKeyword, prompt)
				return nil, nil
			}
		}),
	)
	assert.NilError(t, err)
	coordinator.Run()
}
