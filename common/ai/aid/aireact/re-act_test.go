package aireact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_QueueEnqueue(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			time.Sleep(20 * time.Second)
			return nil, nil
		}),
		WithDebug(true),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 3; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		close(in)
	}()
	after := time.After(5 * time.Second)
	haveTaskEnqueue := false
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.GetNodeId() == "react_task_enqueue" {
				haveTaskEnqueue = true
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}

	if !haveTaskEnqueue {
		t.Fatal("task not enqueue")
	}
}

func TestReAct_QueueEnqueueCount(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			time.Sleep(20 * time.Second)
			return nil, nil
		}),
		WithDebug(true),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 3; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		close(in)
	}()
	after := time.After(5 * time.Second)
	haveTaskEnqueue := false
	count := 0
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.GetNodeId() == "react_task_enqueue" {
				haveTaskEnqueue = true
				count++
				if count > 1 {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}

	if !haveTaskEnqueue {
		t.Fatal("task not enqueue")
	}
	if count <= 1 {
		t.Fatal("task enqueue count is less than 2, got " + fmt.Sprint(count))
	}
}

func TestReAct_QueueEnqueueDequeue(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	haveTaskEnqueue := false

	ins, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if strings.Contains(req.GetPrompt(), `如果你觉得问题比较简单，直接回答 Example`) {
				for {
					time.Sleep(30 * time.Millisecond)
					if haveTaskEnqueue {
						break
					}
				}
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{
"@action": "object", "next_action": {"type": "directly_answer"}, "answer_payload": "..[your-answer]..", 
"human_readable_thought": "a"}`))
				rsp.Close()
				return rsp, nil
			}
			time.Sleep(100 * time.Second)
			return nil, nil
		}),
		WithDebug(false),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 3; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		close(in)
	}()
	after := time.After(5 * time.Second)
	haveTaskDequeue := false
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.GetNodeId() == "react_task_enqueue" {
				haveTaskEnqueue = true
			}
			if e.GetNodeId() == "react_task_dequeue" {
				haveTaskDequeue = true
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}

	if !haveTaskEnqueue {
		t.Fatal("task not enqueue")
	}
	if !haveTaskDequeue {
		t.Fatal("task not dequeue")
	}
}
