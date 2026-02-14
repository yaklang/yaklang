package aireact

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedToolCallingMultiple(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string, callCount *int) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked thought for multiple tool calls test", "cumulative_summary": "..cumulative-mocked.."}
`, toolName)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		*callCount++
		message := fmt.Sprintf("call %d", *callCount)
		identifier := fmt.Sprintf("call_%d_test", *callCount)
		rsp := i.NewAIResponse()
		// Include identifier field for new directory structure
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action": "call-tool", "identifier": "%s", "params": { "message" : "%s", "output_lines": 2 }}`, identifier, message)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		// 如果调用次数少于2次，继续调用工具
		if *callCount < 2 {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "need more calls", "human_readable_result": "need to call tool again"}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done", "human_readable_result": "completed"}`))
		}
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

// TestReAct_ToolCall_FileEmit_Multiple 测试多次工具调用时的文件命名格式
func TestReAct_ToolCall_FileEmit_Multiple(t *testing.T) {
	toolName := "test_multiple_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	callCount := 0

	// 创建一个工具
	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithNumberParam("output_lines"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			message := params.GetString("message", "default message")
			outputLines := int(params.GetInt("output_lines", 2))

			// 写入 stdout
			for i := 0; i < outputLines; i++ {
				fmt.Fprintf(stdout, "stdout line %d: %s\n", i+1, message)
			}

			// 返回 result
			result := map[string]any{
				"status":  "success",
				"message": message,
				"call":    callCount,
			}
			return result, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingMultiple(i, r, toolName, &callCount)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 发送输入事件
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName + " twice",
		}
	}()

	// 设置超时时间
	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	// 收集所有 emit 的 report 文件路径，按调用次数分组
	reportFilesByCall := make(map[int]string) // callNumber -> filepath
	toolCallCount := 0
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			// 检查文件 emit 事件 - 新格式: tool_calls/{n}_{tool}_{id}.md
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					// 从文件名中提取调用次数
					filename := filepath.Base(filePath)
					nameWithoutExt := strings.TrimSuffix(filename, ".md")
					parts := strings.Split(nameWithoutExt, "_")
					if len(parts) >= 2 {
						callNumber := utils.InterfaceToInt(parts[0])
						if callNumber > 0 {
							reportFilesByCall[callNumber] = filePath
							log.Infof("Emitted report for call %d: %s", callNumber, filePath)
						}
					}
				}
			}

			// 检查工具调用完成
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallCount++
			}

			// 检查任务完成
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallCount >= 2 && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	// 验证至少调用了2次工具
	if toolCallCount < 2 {
		t.Fatalf("Expected at least 2 tool calls, but got %d", toolCallCount)
	}

	// 验证任务完成
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证每次工具调用都有对应的 report 文件
	for callNumber := 1; callNumber <= 2; callNumber++ {
		filePath, ok := reportFilesByCall[callNumber]
		if !ok {
			t.Errorf("Expected report file for call %d, but none was found", callNumber)
			continue
		}

		// 验证文件存在
		if !utils.FileExists(filePath) {
			t.Errorf("Report for call %d does not exist: %s", callNumber, filePath)
			continue
		}

		// 验证文件名格式: {n}_{tool}_{identifier}.md
		filename := filepath.Base(filePath)
		if !strings.HasPrefix(filename, fmt.Sprintf("%d_", callNumber)) {
			t.Errorf("Report for call %d should start with '%d_', got: %s", callNumber, callNumber, filename)
		}
		// 验证文件名包含 identifier
		if !strings.Contains(filename, fmt.Sprintf("call_%d_test", callNumber)) {
			t.Errorf("Report for call %d should contain identifier 'call_%d_test', got: %s", callNumber, callNumber, filename)
		}

		// 验证 report 内容包含关键部分
		reportContent, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read report for call %d: %v", callNumber, err)
			continue
		}
		contentStr := string(reportContent)
		if !strings.Contains(contentStr, "# Tool Call Report:") {
			t.Errorf("Report for call %d should contain '# Tool Call Report:' header", callNumber)
		}
		if !strings.Contains(contentStr, "## Parameters") {
			t.Errorf("Report for call %d should contain '## Parameters' section", callNumber)
		}

		log.Infof("✓ Call %d report: %s", callNumber, filename)
	}

	log.Infof("✓ Multiple tool calls test completed successfully: %d calls, reports: %v", toolCallCount, reportFilesByCall)

	// 清理
	defer func() {
		for _, filePath := range reportFilesByCall {
			if utils.FileExists(filePath) {
				os.Remove(filePath)
			}
		}
	}()
}
