package aid

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAITaskCallToolStdOut(t *testing.T) {
	outputToken := uuid.New().String()
	errToken := uuid.New().String()
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		aicommon.WithAgreeYOLO(),
		aicommon.WithTools(PrintTool()),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			// 处理工具调用参数生成阶段
			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: print`, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "call-tool", "tool": "print", "params": {"output": "%s","err":"%s"}}`, outputToken, errToken)))
				return rsp, nil
			}
			// 处理任务执行阶段
			if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "print"}`))
				return rsp, nil
			}
			// 处理决策阶段 - 检查更多的决策阶段特征
			if utils.MatchAllOfSubString(request.GetPrompt(), `review当前任务的执行情况`, `决策`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `刚使用了一个工具来帮助你完成任务`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `continue-current-task`, `proceed-next-task`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `task-failed`, `task-skipped`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `"enum": ["continue-current-task"`) ||
				utils.MatchAllOfSubString(request.GetPrompt(), `工具的结果如下，产生结果时间为`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "proceed-next-task"}`))
				return rsp, nil
			}

			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
			rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "在给定路径下寻找体积最大的文件",
    "main_task_goal": "识别 /Users/v1ll4n/Projects/yaklang 目录中占用存储空间最多的文件，并展示其完整路径与大小信息",
    "tasks": [
        {
            "subtask_name": "扫描目录结构",
            "subtask_goal": "递归遍历 /Users/v1ll4n/Projects/yaklang 目录下所有文件，记录每个文件的位置和占用空间"
        },
        {
            "subtask_name": "计算文件大小",
            "subtask_goal": "遍历所有文件，计算每个文件的大小"
        }
    ]
}
			`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	count := 0
	var outBuffer = bytes.NewBuffer(nil)
	var errBuffer = bytes.NewBuffer(nil)
	var toolCallID string

LOOP:
	for {
		select {
		case <-time.After(5 * time.Second): // 优化：从30秒减少到5秒
			break LOOP
		case result := <-outputChan:
			count++
			if count > 100 {
				break LOOP
			}
			fmt.Println("result:" + result.String())

			if result.Type == schema.EVENT_TOOL_CALL_START {
				toolCallID = result.CallToolID
				continue
			}

			if result.Type == schema.EVENT_TOOL_CALL_DONE || result.Type == schema.EVENT_TOOL_CALL_ERROR || result.Type == schema.EVENT_TOOL_CALL_USER_CANCEL {
				// 不要立即清空toolCallID，因为 stdout 和 stderr 是流事件，是异步的
				// toolCallID = ""
				continue
			}
			if result.Type == schema.EVENT_TYPE_STREAM {
				if result.NodeId == "tool-print-stdout" {
					require.Equal(t, toolCallID, result.CallToolID)
					require.True(t, result.DisableMarkdown)
					outBuffer.Write(result.StreamDelta)
				}
				if result.NodeId == "tool-print-stderr" {
					require.Equal(t, toolCallID, result.CallToolID)
					require.True(t, result.DisableMarkdown)
					errBuffer.Write(result.StreamDelta)
				}
			}
			if utils.MatchAllOfSubString(string(result.Content), "start to generate and feedback tool:") {
				break LOOP
			}
			fmt.Println("review task result:" + result.String())
		}
	}
	require.Equalf(t, outputToken, outBuffer.String(), " output should match expected token")
	require.Equalf(t, errToken, errBuffer.String(), " err output should match expected token")
}
