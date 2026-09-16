package aireact

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type contextModePromptBuilder func(*reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error)

func (f contextModePromptBuilder) Build(p *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
	return f(p)
}

type contextModeMemory struct {
	aicommon.MemoryTriage
	searches atomic.Int32
}

func (m *contextModeMemory) SearchMemory(any, int) (*aicommon.SearchMemoryResult, error) {
	m.searches.Add(1)
	return &aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{{Id: "parent-memory", Content: "PARENT_MEMORY_SENTINEL"}}}, nil
}

func (m *contextModeMemory) SearchMemoryWithoutAI(origin any, limit int) (*aicommon.SearchMemoryResult, error) {
	return m.SearchMemory(origin, limit)
}

// Use the production child runtime and LiteForge path. A mocked child invoker
// would miss LiteForge's independent persistent-session restoration entirely.
func TestSubAgentContextMode_RealRuntimeAndGoalElaboration(t *testing.T) {
	for _, mode := range []string{reactloops.SubAgentContextTaskOnly, reactloops.SubAgentContextFork} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			sessionID := "subagent-context-" + ksuid.New().String()
			persisted := aicommon.NewTimeline(nil, nil)
			persisted.PushText(1, "PERSISTED_PARENT_SENTINEL")
			serialized, err := aicommon.MarshalTimeline(persisted)
			require.NoError(t, err)
			db := consts.GetGormProjectDatabase()
			row := &schema.AIAgentRuntime{Uuid: sessionID, PersistentSession: sessionID, QuotedTimeline: strconv.Quote(serialized)}
			_, err = yakit.CreateOrUpdateAIAgentRuntime(db, row)
			require.NoError(t, err)
			t.Cleanup(func() { db.Unscoped().Where("uuid = ?", sessionID).Delete(&schema.AIAgentRuntime{}) })

			var mu sync.Mutex
			var elaborationPrompts, runtimePrompts []string
			memory := &contextModeMemory{MemoryTriage: aicommon.NewNoOpMemoryTriage()}
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithDisablePerception(true), aicommon.WithDisableIntentRecognition(true),
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithMemoryTriage(memory),
				aicommon.WithUserPresetPrompt("HOST_POLICY_SENTINEL"), aicommon.WithPlanPrompt("PARENT_PLAN_PROMPT_SENTINEL"),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					prompt := req.GetPrompt()
					body := `{"@action":"finish"}`
					mu.Lock()
					if strings.Contains(prompt, "You are preparing a task brief") {
						elaborationPrompts = append(elaborationPrompts, prompt)
						body = `{"@action":"sub_react_agent_goal_elaboration","goal":"EXPLICIT_CHILD_GOAL","result_contract":"EXPLICIT_RESULT_CONTRACT"}`
					} else {
						runtimePrompts = append(runtimePrompts, prompt)
					}
					mu.Unlock()
					response := c.NewAIResponse()
					response.EmitOutputStream(bytes.NewBufferString(body))
					response.Close()
					return response, nil
				}))
			// Parent startup has already restored its session. Goal elaboration
			// must use the submitted snapshot without rereading this DB history.
			cfg.PersistentSessionId = sessionID
			cfg.InitStatus.SetPersistentSessionRestored(true)
			cfg.GetTimeline().PushText(cfg.AcquireId(), "LIVE_PARENT_SENTINEL")
			cfg.AppendFrozenBlockPartition("plan_document", "Plan Document", "PARENT_FROZEN_PLAN_SENTINEL", aicommon.PlanDocumentFrozenPartitionOrder)
			cfg.SetUserInputHistory([]schema.AIAgentUserInputRecord{{Round: 1, Timestamp: time.Now(), UserInput: "PARENT_USER_HISTORY_SENTINEL"}})
			parent := mock.NewMockInvoker(ctx)
			parent.SetConfig(cfg)
			task := aicommon.NewStatefulTaskBase("parent", "PARENT_TASK_INPUT_SENTINEL", ctx, cfg.GetEmitter(), true)
			var childConfig *aicommon.Config
			builder := contextModePromptBuilder(func(p *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
				if _, ok := p.Invoker.(*ReAct); !ok {
					t.Errorf("expected the real ReAct child runtime, got %T", p.Invoker)
				}
				childConfig = p.Invoker.GetConfig().(*aicommon.Config)
				options := reactloops.BasicAICommonConfigOption(childConfig)
				options = append(options,
					reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
					reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
					reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableIncreaseIteration(true))
				return reactloops.NewReActLoop("context-mode-child", p.Invoker, options...)
			})
			results := reactloops.DispatchSubAgents(parent, task,
				[]reactloops.SubAgentJob{{Identifier: "child", ContextMode: mode, Goal: "EXPLICIT_CHILD_GOAL", ResultContract: "EXPLICIT_RESULT_CONTRACT"}},
				reactloops.SubAgentOptions{TimelineMode: reactloops.SubAgentTimelineFork, ElaborateGoals: true, LoopBuilder: builder})
			require.Len(t, results, 1)
			require.NoError(t, results[0].ExecErr)
			require.Equal(t, "completed", results[0].Record.Status)
			mu.Lock()
			elaboration := strings.Join(elaborationPrompts, "\n")
			runtime := strings.Join(runtimePrompts, "\n")
			elaborationCount, runtimeCount := len(elaborationPrompts), len(runtimePrompts)
			mu.Unlock()
			require.Equal(t, 1, elaborationCount)
			require.Equal(t, 1, runtimeCount)
			for _, prompt := range []string{elaboration, runtime} {
				require.Contains(t, prompt, "EXPLICIT_CHILD_GOAL")
				require.Contains(t, prompt, "EXPLICIT_RESULT_CONTRACT")
			}
			require.Contains(t, runtime, "HOST_POLICY_SENTINEL")
			if mode == reactloops.SubAgentContextTaskOnly {
				for _, marker := range []string{"PERSISTED_PARENT_SENTINEL", "LIVE_PARENT_SENTINEL", "PARENT_FROZEN_PLAN_SENTINEL", "PARENT_PLAN_PROMPT_SENTINEL", "PARENT_USER_HISTORY_SENTINEL", "PARENT_TASK_INPUT_SENTINEL", "PARENT_MEMORY_SENTINEL"} {
					require.NotContains(t, elaboration, marker)
					require.NotContains(t, runtime, marker)
				}
				require.Empty(t, childConfig.PersistentSessionId)
				require.Empty(t, childConfig.PlanPrompt)
				require.Empty(t, aicommon.FrozenBlockPartitionsFromConfig(childConfig))
				require.Equal(t, "noop", childConfig.MemoryTriage.GetSessionID())
				require.Zero(t, memory.searches.Load(), "task_only must not query the inherited memory store")
			} else {
				require.NotContains(t, elaboration, "PERSISTED_PARENT_SENTINEL", "goal elaboration must use the submitted snapshot without supplementing it from the persistent session")
				require.Contains(t, elaboration, "LIVE_PARENT_SENTINEL")
				require.Contains(t, runtime, "LIVE_PARENT_SENTINEL")
				require.Contains(t, runtime, "PARENT_FROZEN_PLAN_SENTINEL")
				require.Same(t, memory, childConfig.MemoryTriage)
			}
		})
	}
}
