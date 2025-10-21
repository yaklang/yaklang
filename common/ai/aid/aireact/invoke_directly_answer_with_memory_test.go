package aireact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedFreeInputOutputWithMemory(config aicommon.AICallerConfigIf, flag string) (*aicommon.AIResponse, error) {
	rsp := config.NewAIResponse()
	rs := bytes.NewBufferString(`
{"@action": "object", "next_action": {
	"type": "directly_answer",
	"answer_payload": "..[mocked_answer` + flag + `]..",
}, "human_readable_thought": "mocked thought` + flag + `", "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
	rsp.EmitOutputStream(rs)
	rsp.Close()
	return rsp, nil
}

func TestReAct_DirectlyWithMemory(t *testing.T) {
	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewTestReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedFreeInputOutputWithMemory(i, flag)
		}),
		WithDebug(false),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithMemoryPoolSize(300),
	)

	if o, ok := ins.memoryTriage.(*aimem.MockMemoryTriage); ok {
		o.SetOverSearch(true)
	}

	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		// 不要立即关闭 channel，等待任务完成后发送同步请求
	}()
	after := time.After(10 * time.Second)

	haveResult := false

	haveMemoryAdd := false
	removeMem := 0
	addMem := 0
	taskFinished := false
	haveMemorySearched := false
	haveMemoryContext := false
	memoryContextSent := false
LOOP:
	for {
		select {
		case e := <-out:
			switch e.Type {
			case string(schema.EVENT_TYPE_MEMORY_ADD_CONTEXT), string(schema.EVENT_TYPE_MEMORY_REMOVE_CONTEXT):
			default:
				fmt.Println(e.String())
			}
			if e.NodeId == "result" {
				result := jsonpath.FindFirst(e.GetContent(), "$..result")
				if strings.Contains(utils.InterfaceToString(result), flag) {
					haveResult = true
				}
			}

			if e.Type == string(schema.EVENT_TYPE_MEMORY_ADD_CONTEXT) {
				haveMemoryAdd = true
				addMem++
			}

			if e.Type == string(schema.EVENT_TYPE_MEMORY_REMOVE_CONTEXT) {
				removeMem++
			}

			if e.Type == string(schema.EVENT_TYPE_MEMORY_SEARCH_QUICKLY) {
				haveMemorySearched = true
			}

			if e.Type == string(schema.EVENT_TYPE_MEMORY_CONTEXT) {
				haveMemoryContext = true
				// 验证 memory context 响应内容
				totalMemories := jsonpath.FindFirst(e.GetContent(), "$..total_memories")
				totalSize := jsonpath.FindFirst(e.GetContent(), "$..total_size")
				memoryPoolLimit := jsonpath.FindFirst(e.GetContent(), "$..memory_pool_limit")
				memorySessionID := jsonpath.FindFirst(e.GetContent(), "$..memory_session_id")

				if totalMemories == nil {
					t.Fatal("Expected total_memories in memory context response")
				}
				if totalSize == nil {
					t.Fatal("Expected total_size in memory context response")
				}
				if memoryPoolLimit == nil {
					t.Fatal("Expected memory_pool_limit in memory context response")
				}
				if memorySessionID == nil {
					t.Fatal("Expected memory_session_id in memory context response")
				}

				// 验证总大小小于配置的 memory size
				sizeInt := utils.InterfaceToInt(totalSize)
				limitInt := utils.InterfaceToInt(memoryPoolLimit)
				if sizeInt > limitInt {
					t.Fatalf("Memory total size %d should not exceed limit %d", sizeInt, limitInt)
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					//break LOOP
					taskFinished = true

					// 任务完成后发送 MEMORY CONTEXT SYNC 请求
					if !memoryContextSent {
						memoryContextSent = true
						// 立即发送同步请求
						in <- &ypb.AIInputEvent{
							IsSyncMessage: true,
							SyncType:      SYNC_TYPE_MEMORY_CONTEXT,
						}
					}
				}
			}

			if addMem > 50 && taskFinished && removeMem > 0 && haveMemoryContext {
				close(in) // 关闭输入 channel
				break LOOP
			}
		case <-after:
			close(in) // 超时时也关闭 channel
			break LOOP
		}
	}

	if !haveResult {
		t.Fatal("Expected to have at least one result event, but got none")
	}
	if !haveMemoryAdd {
		t.Fatal("Expected to have memory add event, but got none")
	}
	if !taskFinished {
		t.Fatal("Expected the task to be finished, but it was not")
	}
	if addMem < 50 {
		t.Fatal("Expected to have more than 50 memory add events, but got", addMem)
	}
	if removeMem <= 0 {
		t.Fatal("Expected to have some memory remove events, but got none")
	}
	if !haveMemorySearched {
		t.Fatal("Expected to have memory searched event, but got none")
	}
	if !haveMemoryContext {
		t.Fatal("Expected to have memory context event, but got none")
	}

	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, flag) {
		t.Fatal("timeline does not contain flag", flag)
	}
	fmt.Println(timeline)
}
