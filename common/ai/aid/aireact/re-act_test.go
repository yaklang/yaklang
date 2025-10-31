package aireact

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_QueueEnqueue(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			time.Sleep(20 * time.Second)
			return nil, nil
		}),
		aicommon.WithDebug(true),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
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

func TestReAct_CounterForQueueEnqueueFlushed(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 100)
	ctx, cancel := context.WithCancel(context.Background())
	atomicCount := new(int32)
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			time.Sleep(20 * time.Second)
			return nil, nil
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			fmt.Println("===============================")
			fmt.Println("Event: ", e.String())
			fmt.Println("===============================")
			if e.NodeId == "react_task_enqueue" {
				if atomic.AddInt32(atomicCount, 1) >= 3 {
					cancel()
				}
			}
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
	after := time.After(3 * time.Second)
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		case <-after:
			break LOOP
		}
	}

	count := atomic.LoadInt32(atomicCount)
	if count <= 2 {
		t.Fatal("task enqueue count is less than 2, got " + fmt.Sprint(count))
	}
}

func TestReAct_QueueEnqueueDequeue(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	haveTaskEnqueue := false

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
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

func TestAcc(t *testing.T) {

}
