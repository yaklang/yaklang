package loop_smart_qa_test

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSmartQAFinalAnswerEmitsMarkdownStreamAndSkipsDirectlyAnswer(t *testing.T) {
	const finalAnswer = "# 研究结论\n\n- 要点一\n- 要点二\n\n这是 smart-qa 最终整理出的完整回答。"

	var aiCallCount int32
	var events []*schema.AiOutputEvent
	var eventsMu sync.Mutex

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			atomic.AddInt32(&aiCallCount, 1)

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"final_answer","answer":` + strconv.Quote(finalAnswer) + `}}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_SMART_QA, reactIns)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("smart-qa-final-answer", ctx, "请整理一份完整结论")
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	require.Equal(t, int32(1), atomic.LoadInt32(&aiCallCount), "final_answer should not trigger a second DirectlyAnswer AI call")
	require.Equal(t, finalAnswer, loop.Get("final_answer"))

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var sawAnswerStream bool
	var sawResult bool
	for _, e := range events {
		if e.NodeId == "re-act-loop-answer-payload" && e.IsStream && e.ContentType == aicommon.TypeTextMarkdown {
			sawAnswerStream = true
		}
		if e.NodeId == "result" {
			result := jsonpath.FindFirst(e.Content, "$.result")
			if utils.InterfaceToString(result) == finalAnswer {
				sawResult = true
			}
		}
	}

	require.True(t, sawAnswerStream, "expected final_answer to emit markdown stream events")
	require.True(t, sawResult, "expected final_answer to emit result-after-stream with the original answer")
}
