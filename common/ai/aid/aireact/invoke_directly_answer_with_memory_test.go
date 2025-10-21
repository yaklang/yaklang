package aireact

import (
	"bytes"
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
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
		close(in)
	}()
	after := time.After(10 * time.Second)

	haveResult := false

	haveMemoryAdd := false
	removeMem := 0
	addMem := 0
	taskFinished := false
	haveMemorySearched := false
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

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					//break LOOP
					taskFinished = true
				}
			}

			if addMem > 50 && taskFinished && removeMem > 0 {
				break LOOP
			}
		case <-after:
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

	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, flag) {
		t.Fatal("timeline does not contain flag", flag)
	}
	fmt.Println(timeline)
}
