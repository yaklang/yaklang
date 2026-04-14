package test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestPlanRetry(t *testing.T) {
	var count int64
	planJSON := `{
    "@action": "plan_from_document",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {"subtask_name": "遍历目标目录", "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"},
        {"subtask_name": "筛选最大文件", "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"},
        {"subtask_name": "输出结果", "subtask_goal": "将最大文件的路径和大小以可读格式输出"}
    ]
}`
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			outputChan <- e
		}),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithAIAutoRetry(2),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			n := atomic.AddInt64(&count, 1)
			if n <= 2 {
				return nil, utils.Errorf("mock, unknown err[%v]", n)
			}
			prompt := req.GetPrompt()
			if rsp, err := tryHandleNewPlanFlowPrompt(config, prompt, planJSON); rsp != nil {
				return rsp, err
			}
			rsp := config.NewAIResponse()
			defer rsp.Close()
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "ok"}`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("recovered from panic in ins.Run(): %v\n", r)
			}
		}()
		err := ins.Run()
		if err != nil {
			fmt.Printf("ins.Run() error: %v\n", err)
		}
	}()

	retryWithDupSeqId := false
	parsedTask := false
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

LOOP:
	for {
		select {
		case <-timeout.C:
			t.Fatal("timeout")
		case output := <-outputChan:
			msg := output.String()
			if !strings.Contains(msg, `input_consumption`) {
				fmt.Println(msg)
			}
			if strings.Contains(msg, `prepare to retry call ai`) && !retryWithDupSeqId {
				retryWithDupSeqId = true
				continue
			}
			if strings.Contains(msg, `plan_review_require]`) && retryWithDupSeqId {
				parsedTask = true
				break LOOP
			}
		}
	}
	assert.True(t, retryWithDupSeqId)
	assert.True(t, parsedTask)
	fmt.Println("==========================================================================================")
	testRecoverPlanRetry(t, ins.Config.Id)
}

func testRecoverPlanRetry(t *testing.T, uid string) {
	planJSON := `{
    "@action": "plan_from_document",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {"subtask_name": "遍历目标目录", "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"},
        {"subtask_name": "筛选最大文件", "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"},
        {"subtask_name": "输出结果", "subtask_goal": "将最大文件的路径和大小以可读格式输出"}
    ]
}`
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)
	ins, err := aid.NewFastRecoverCoordinator(
		uid,
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			outputChan <- e
		}),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithAIAutoRetry(2),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if rsp, err := tryHandleNewPlanFlowPrompt(config, prompt, planJSON); rsp != nil {
				return rsp, err
			}
			rsp := config.NewAIResponse()
			defer rsp.Close()
			if isNextActionDecisionPrompt(prompt) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "object",
    "next_action": {"type": "finish", "answer_payload": "task done"},
    "cumulative_summary": "done",
    "human_readable_thought": "done"
}`))
				return rsp, nil
			}
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "ok"}`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("recovered from panic in testRecoverPlanRetry ins.Run(): %v\n", r)
			}
		}()
		err := ins.Run()
		if err != nil {
			fmt.Printf("testRecoverPlanRetry ins.Run() error: %v\n", err)
		}
	}()

	retryWithDupSeqId := false
	parsedTask := false
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

LOOP:
	for {
		select {
		case <-timeout.C:
			t.Fatal("timeout")
		case output := <-outputChan:
			msg := output.String()
			if !strings.Contains(msg, `input_consumption`) {
				fmt.Println(msg)
			}
			if strings.Contains(msg, `prepare to retry call ai`) && !retryWithDupSeqId {
				retryWithDupSeqId = true
				continue
			}
			if strings.Contains(msg, `遍历目标目录`) {
				parsedTask = true
				break LOOP
			}
		}
	}
	assert.False(t, retryWithDupSeqId)
	assert.True(t, parsedTask)
}
