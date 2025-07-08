package aid

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestAITaskCallToolStdOut(t *testing.T) {
	outputToken := uuid.New().String()
	errToken := uuid.New().String()
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		WithAgreeYOLO(true),
		WithTools(PrintTool()),
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: print`, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "call-tool", "tool": "print", "params": {"output": "%s","err":"%s"}}`, outputToken, errToken)))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "print"}`))
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

LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 100 {
				break LOOP
			}
			fmt.Println("result:" + result.String())

			if result.Type == schema.EVENT_TYPE_STREAM {
				if result.NodeId == "tool-print-stdout" {
					require.True(t, result.DisableMarkdown)
					outBuffer.Write(result.StreamDelta)
				}
				if result.NodeId == "tool-print-stderr" {
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
