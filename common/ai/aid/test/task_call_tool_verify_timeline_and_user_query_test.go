package test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestAITaskCallToolStdOut_VerifyTimelineAndUserQuery(t *testing.T) {
	outputToken := uuid.New().String()
	errToken := uuid.New().String()
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)

	userRawInput := utils.RandStringBytes(160)

	coordinator, err := aid.NewCoordinator(
		userRawInput,
		aicommon.WithAgreeYOLO(),
		aicommon.WithTools(aid.PrintTool()),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if !strings.Contains(r.GetPrompt(), userRawInput) {
				prompt := r.GetPrompt()
				fmt.Println(prompt)
				t.Fatal("no user raw input found in prompt")
			}
			return mockedToolCalling(i, r, "print", fmt.Sprintf(`{"@action": "call-tool", "tool": "print", "params": {"output": "%s","err":"%s"}}`, outputToken, errToken))
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
		case <-time.After(500 * time.Second): // 优化：从30秒减少到5秒
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

			if result.Type == schema.EVENT_TYPE_STRUCTURED {
				t := jsonpath.FindFirst(string(result.Content), "$..type")
				if t == "pop_task" {
					// break LOOP
				}
			}

			if utils.MatchAllOfSubString(string(result.Content), "start to generate and feedback tool:") {
				break LOOP
			}

			fmt.Println("review task result:" + result.String())
		}
	}
	require.Contains(t, outBuffer.String(), outputToken, " output should match expected token")
	require.Contains(t,
		errBuffer.String(), errToken, " err output should match expected token")
}
