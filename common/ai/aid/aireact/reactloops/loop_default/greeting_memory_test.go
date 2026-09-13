package loop_default

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

type greetingRecallMemory struct {
	aicommon.MemoryTriage
	ai, fast atomic.Int32
}

func (m *greetingRecallMemory) SearchMemory(any, int) (*aicommon.SearchMemoryResult, error) {
	m.ai.Add(1)
	return &aicommon.SearchMemoryResult{}, nil
}

func (m *greetingRecallMemory) SearchMemoryWithoutAI(any, int) (*aicommon.SearchMemoryResult, error) {
	m.fast.Add(1)
	return &aicommon.SearchMemoryResult{}, nil
}

type greetingRecallInvoker struct {
	*postIterationTestInvoker
	midterm atomic.Int32
}

func (i *greetingRecallInvoker) ConsumeAndSearchMidtermMemory() string {
	i.midterm.Add(1)
	return "irrelevant historical memory"
}

func TestMUSTPASS_GreetingAnswersOnceWithoutMemoryRecall(t *testing.T) {
	for _, emptyHTTP := range []bool{false, true} {
		name := "no attachment"
		if emptyHTTP {
			name = "empty HTTP placeholder"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var modelCalls atomic.Int32
			callback := scriptedCallback(
				`{"@action":"directly_answer","answer_payload":"你好！"}`,
				`{"@action":"finish","answer":"done"}`,
			)
			inv := &greetingRecallInvoker{postIterationTestInvoker: newPostIterationTestInvoker(
				func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					modelCalls.Add(1)
					return callback(c, req)
				})}
			inv.GetConfig().SetConfig("DisableIntentRecognition", false)
			memory := &greetingRecallMemory{MemoryTriage: aicommon.NewNoOpMemoryTriage()}
			loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_DEFAULT, inv,
				reactloops.WithAllowRAG(false), reactloops.WithAllowToolCall(false),
				reactloops.WithAllowAIForge(false), reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowUserInteract(false), reactloops.WithMemoryTriage(memory),
				reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true),
			)
			require.NoError(t, err)
			task := aicommon.NewStatefulTaskBase("hello", "你好？", ctx, inv.GetConfig().GetEmitter())
			if emptyHTTP {
				task.SetAttachedDatas([]*aicommon.AttachedResource{nil, aicommon.NewAttachedResource("http_flow", "id", "")})
			}
			require.NoError(t, loop.ExecuteWithExistedTask(task))
			require.EqualValues(t, 1, modelCalls.Load(), "greeting must complete after its first answer")
			require.Zero(t, inv.directAnswerCalls(), "greeting must not trigger another summary model")
			require.Never(t, func() bool { return memory.ai.Load()+memory.fast.Load()+inv.midterm.Load() != 0 },
				100*time.Millisecond, time.Millisecond, "trivial greeting must not launch background recall")
		})
	}
}
