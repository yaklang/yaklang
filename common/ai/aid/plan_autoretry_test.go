package aid

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestPlanRetry(t *testing.T) {
	count := 0
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	ins, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithEventHandler(func(e *Event) {
			outputChan <- e
		}),
		WithAIAutoRetry(2),
		WithAICallback(func(config *Config, req *AIRequest) (*AIResponse, error) {
			count++
			if count > 2 {
				rsp := config.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewReader([]byte(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {
            "subtask_name": "遍历目标目录",
            "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"
        },
        {
            "subtask_name": "筛选最大文件",
            "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"
        },
        {
            "subtask_name": "输出结果",
            "subtask_goal": "将最大文件的路径和大小以可读格式输出"
        }
    ]
}
`)))
				rsp.Close()
				return rsp, nil
			}
			return nil, utils.Errorf("mock, unknown err[%v]", count)
		}),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	go func() {
		err := ins.Run()
		if err != nil {
			t.Fatal(err)
		}
	}()

	retryWithDupSeqId := false
	parsedTask := false
LOOP:
	for {
		select {
		case <-time.After(time.Second * 3):
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
			//if retryWithDupSeqId {
			//	fmt.Println(spew.Sdump(msg))
			//}
			if strings.Contains(msg, `plan_review_require]`) && retryWithDupSeqId {
				parsedTask = true
				break LOOP
			}
		}
	}
	assert.True(t, retryWithDupSeqId)
	assert.True(t, parsedTask)
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	fmt.Println("==========================================================================================")
	testRecoverPlanRetry(t, ins.config.id)
}

func testRecoverPlanRetry(t *testing.T, uid string) {
	count := 0
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	ins, err := NewFastRecoverCoordinator(
		uid,
		WithEventInputChan(inputChan),
		WithEventHandler(func(e *Event) {
			outputChan <- e
		}),
		WithAIAutoRetry(2),
		WithAICallback(func(config *Config, req *AIRequest) (*AIResponse, error) {
			return nil, utils.Errorf("mock, unknown err[%v]", count)
		}),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	go func() {
		err := ins.Run()
		if err != nil {
			t.Fatal(err)
		}
	}()

	retryWithDupSeqId := false
	parsedTask := false
LOOP:
	for {
		select {
		case <-time.After(time.Second * 3):
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
