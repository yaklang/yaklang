package aireact

import (
	"bytes"
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func mockedRequireBlueprint_BASIC(config aicommon.AICallerConfigIf, flag string) (*aicommon.AIResponse, error) {
	rsp := config.NewAIResponse()
	rs := bytes.NewBufferString(`
{"@action": "object", "next_action": {
	"type": "require_ai_blueprint",
	"require_ai_blueprint_payload": "..[mocked_answer` + flag + `]..",
}, "human_readable_thought": "mocked thought` + flag + `", "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
	rsp.EmitOutputStream(rs)
	rsp.Close()
	return rsp, nil
}

func TestReAct_RequireBlueprint(t *testing.T) {
	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedRequireBlueprint_BASIC(i, flag)
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
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		close(in)
	}()
	after := time.After(5 * time.Second)

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())

		case <-after:
			break LOOP
		}
	}

	timeline := ins.DumpTimeline()
	fmt.Println(timeline)
}
