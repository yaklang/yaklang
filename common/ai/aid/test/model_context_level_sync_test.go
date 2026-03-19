package test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAIDToAIReact_ModelContextLevel_Compact(t *testing.T) {
	var capturedPrompt string

	inputChan := make(chan *ypb.AIInputEvent, 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	_, err := aireact.NewTestReAct(
		aicommon.WithEventInputChan(inputChan),
		aicommon.WithModelContextLevel(aicommon.ModelContextLevelCompact),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			if strings.Contains(prompt, "Current Time:") && strings.Contains(prompt, "<|PERSISTENT_") {
				capturedPrompt = prompt
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewTestReAct failed: %v", err)
	}

	inputChan <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "test compact react prompt",
	}

	timeout := time.After(5 * time.Second)
	for capturedPrompt == "" {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for compact react prompt")
		case <-outputChan:
		}
	}

	if !strings.Contains(capturedPrompt, "你处于精简上下文模式下的 ReAct Agent") {
		t.Fatalf("expected compact main-loop instruction, got: %s", capturedPrompt)
	}
	if strings.Contains(capturedPrompt, "## 核心行动准则") {
		t.Fatalf("compact prompt should not contain the standard instruction block, got: %s", capturedPrompt)
	}
	if strings.Contains(capturedPrompt, "## 工具使用指南") {
		t.Fatalf("compact prompt should not contain the standard tool guide block, got: %s", capturedPrompt)
	}
	if strings.Contains(capturedPrompt, "## Core Traits (核心性格)") {
		t.Fatalf("compact prompt should not contain the standard base traits block, got: %s", capturedPrompt)
	}
}
