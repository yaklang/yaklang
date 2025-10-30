package test

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/yak/depinjector"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

// func TestSearchAIYakTool(t *testing.T) {
// 	coordinator, err := aid.NewCoordinator("test", aid.WithAiToolsSearchTool())
// 	assert.NilError(t, err)

// 	tools, err := coordinator.config.aiToolManager.GetSuggestedTools()
// 	assert.NilError(t, err)

// 	hasSearchTool := false
// 	hasMemoryTool := false

// 	for _, tool := range tools {
// 		if tool.Name == "tools_search" {
// 			hasSearchTool = true
// 		}
// 		if strings.Contains(tool.Name, "memory") {
// 			hasMemoryTool = true
// 		}
// 	}

// 	assert.Assert(t, hasSearchTool)
// 	assert.Assert(t, hasMemoryTool)
// }

func TestDecodeBase64BySearchTool(t *testing.T) {
	depinjector.DependencyInject()

	taskId := string(utils.RandStringBytes(10))
	summaryId := string(utils.RandStringBytes(10))
	stateKeyword := []struct {
		name    string
		matcher any
		aiRsp   string
	}{
		// 规划任务
		{
			"规划任务",
			"你是一个输出JSON的任务规划的工具", `{
  "@action": "plan",
  "main_task": "` + taskId + `",
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
			"申请搜索工具",
			taskId, `{"tool": "tools_search", "@action": "require-tool"}`,
		},
		// 调用搜索工具
		{
			"调用搜索工具",
			nil, `
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
			"执行搜索工具",
			"你是一个智能关键词匹配助手", `
	{
		"@action": "keyword_search",
		"matches": [
			{
				"tool": "decode",
				"matched_keywords": ["base64解码"]
			}
		]
	}`,
		},
		// 判断任务情况
		{
			"判断任务情况",
			nil, `{"@action": "continue-current-task"}`,
		},
		// 申请解码工具
		{
			"申请解码工具",
			nil, `{"tool": "decode", "@action": "require-tool"}`,
		},
		// 调用解码工具
		{
			"调用解码工具",
			nil, `
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
			"判断任务完成情况",
			"status_summary", `{"@action": "proceed-next-task"}`,
		},
		{
			"task summary",
			"\"short_summary\", \"long_summary\"", `{
			"@action": "summary",
			"short_summary": "` + summaryId + `",
			"long_summary": "` + summaryId + `"
		}`,
		},
		{
			"输出任务报告",
			nil, "ok",
		},
	}

	currentStateIndex := 0
	coordinator, err := aid.NewCoordinator("帮我解码一个base64编码的字符串: eWFrbGFuZw==",
		aicommon.WithEnableToolManagerAISearch(true),
		aicommon.WithAgreeYOLO(),
		aicommon.WithEnableToolsName("decode"),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			pair := stateKeyword[currentStateIndex]
			matcher := pair.matcher
			aiRsp := pair.aiRsp
			switch ret := matcher.(type) {
			case string:
				if strings.Contains(prompt, ret) {
					currentStateIndex++
					rsp := config.NewAIResponse()
					rsp.EmitOutputStream(strings.NewReader(aiRsp))
					rsp.Close()
					return rsp, nil
				} else {
					t.Fatalf("run step `%s` failed", pair.name)
					return nil, nil
				}
			case func(string) bool:
				if ret(prompt) {
					currentStateIndex++
					rsp := config.NewAIResponse()
					rsp.EmitOutputStream(strings.NewReader(aiRsp))
					rsp.Close()
					return rsp, nil
				} else {
					t.Fatalf("run step `%s` failed", pair.name)
					return nil, nil
				}
			default:
				currentStateIndex++
				rsp := config.NewAIResponse()
				rsp.EmitOutputStream(strings.NewReader(aiRsp))
				rsp.Close()
				return rsp, nil
			}
		}),
	)
	assert.NilError(t, err)
	coordinator.Run()
}
