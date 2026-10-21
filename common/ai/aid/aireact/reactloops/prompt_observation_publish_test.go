package reactloops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type promptProfileCapture struct {
	mu       sync.Mutex
	profiles []PromptObservationStatus
}

func (c *promptProfileCapture) emitter() *aicommon.Emitter {
	return aicommon.NewEmitter(uuid.NewString(), func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.Type != schema.EVENT_TYPE_PROMPT_PROFILE {
			return event, nil
		}
		var status PromptObservationStatus
		if err := json.Unmarshal(event.Content, &status); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.profiles = append(c.profiles, status)
		c.mu.Unlock()
		return event, nil
	})
}

func (c *promptProfileCapture) snapshot() []PromptObservationStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]PromptObservationStatus(nil), c.profiles...)
}

type observedCompressionInvoker struct {
	*mock.MockInvoker
	timeline *aicommon.Timeline
	builds   int
}

func (i *observedCompressionInvoker) AddToTimeline(kind, content string) {
	i.timeline.PushText(i.GetConfig().AcquireId(), "["+kind+"]:\n"+content)
}

func (i *observedCompressionInvoker) AddToTimelineWithPromptProjection(kind, content, projected string) {
	i.timeline.PushTextWithPromptProjection(i.GetConfig().AcquireId(), "["+kind+"]:\n"+content, "["+kind+"]:\n"+projected)
}

func (i *observedCompressionInvoker) AssembleLoopPrompt(_ []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	i.builds++
	prompt := aicommon.RenderTimelineFrozenOpen(i.timeline).PromptText() + input.FrozenUserContext + input.TodoSnapshot
	prompt += input.CompressionGuide
	return &aicommon.LoopPromptAssemblyResult{Prompt: prompt, Sections: []*aicommon.PromptSectionObservation{
		aicommon.NewPromptSectionObservation("timeline", "Timeline", aicommon.PromptSectionRoleTimelineOpen, true, prompt),
	}}, nil
}

func TestMainLoopPublishesOnlyRequestedPromptObservations(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, mode := range []string{"normal", "ai_compression", "provider_overflow", "overflow_no_progress", "ineffective_compression"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, mode), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				capture := &promptProfileCapture{}
				var loop *ReActLoop
				var requests []string
				compressionCalls := 0
				var historyID int64
				limit := 3000
				if mode == "provider_overflow" || mode == "overflow_no_progress" {
					limit = 10000
				}
				cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(),
					aicommon.WithEmitter(capture.emitter()), aicommon.WithTimelineContentLimit(limit),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
					aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
							compressionCalls++
							loop.WaitForInflightObservation()
							require.Len(t, capture.snapshot(), len(requests), "compression builds must not generate profiles")
							retain := ""
							if mode == "ineffective_compression" {
								retain = fmt.Sprint(historyID)
							}
							return contextCompressionTestResponse(t, c, req, aicommon.ContextCompressionAction, map[string]any{
								"summary": "COMPACTED_HISTORY", "retain_timeline": retain, "retire_skill_names": []string{}, "retire_evidence_ids": []string{},
							}, false), nil
						}
						requests = append(requests, req.GetPrompt())
						require.NotContains(t, req.GetPrompt(), "COMPRESSION_MODEL_REASONING")
						require.NotContains(t, req.GetPrompt(), "compress_context")
						require.Equal(t, aicommon.MeasureTokens(req.GetPrompt()), loop.GetLastPromptToken())
						if (mode == "provider_overflow" || mode == "overflow_no_progress") && len(requests) == 1 {
							return nil, promptOverflowErrorFor("openai", "gpt-4o", 5000)
						}
						return contextCompressionTestResponse(t, c, req, "observation_done", map[string]any{}, false), nil
					}))
				cfg.AiServerName, cfg.AiModelName = "openai", "gpt-4o"
				invoker := &observedCompressionInvoker{MockInvoker: mock.NewMockInvoker(ctx), timeline: cfg.Timeline}
				invoker.SetConfig(cfg)
				var err error
				loop, err = NewReActLoop("observation-compression", invoker,
					WithFunctionCallMode(native), WithAllowToolCall(false), WithAllowRAG(false), WithAllowAIForge(false),
					WithAllowPlanAndExec(false), WithAllowUserInteract(false), WithDisableLoopPerception(true),
					WithDisablePeriodicVerification(true), WithDisableIncreaseIteration(true), WithMaxIterations(2),
					WithRegisterLoopAction("require_tool", "unused routing", nil, nil, nil),
					WithRegisterLoopAction("observation_done", "finish observation test", nil, nil,
						func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) { op.Exit() }))
				require.NoError(t, err)
				if mode != "normal" && mode != "overflow_no_progress" {
					historyID = cfg.AcquireId()
					cfg.Timeline.PushText(historyID, strings.Repeat("ORIGINAL_HISTORY ", 2000))
				}
				executeErr := loop.Execute("observation-task", ctx, "continue the task")
				if mode == "overflow_no_progress" {
					require.True(t, aicommon.IsPromptTooLong(executeErr), "an unchanged rebuild should stop without another request")
				} else if mode == "ineffective_compression" {
					require.ErrorContains(t, executeErr, "compression made no progress")
				} else {
					require.NoError(t, executeErr)
				}
				loop.WaitForInflightObservation()
				profiles := capture.snapshot()
				wantRequests, wantCompression := 1, 0
				switch mode {
				case "ai_compression", "provider_overflow", "ineffective_compression":
					wantCompression = 1
				}
				if mode == "provider_overflow" {
					wantRequests = 2
				}
				if mode == "ineffective_compression" {
					wantRequests = 0
				}
				wantIterations := 1
				if mode == "ai_compression" || mode == "provider_overflow" {
					wantIterations = 2
				}
				if mode == "overflow_no_progress" {
					wantIterations = 0
				}
				require.Len(t, requests, wantRequests)
				require.Equal(t, wantCompression, compressionCalls)
				require.Len(t, profiles, len(requests), "only actual main-loop requests produce profiles")
				if mode != "normal" {
					require.Greater(t, invoker.builds, len(requests), "the regression must exercise discarded prompt rebuilds")
				}
				for i, profile := range profiles {
					require.Equal(t, len(requests[i]), profile.PromptBytes)
					require.Equal(t, aicommon.MeasureTokens(requests[i]), profile.PromptTokens)
				}
				if len(profiles) > 0 {
					require.Equal(t, profiles[len(profiles)-1].PromptTokens, loop.GetLastPromptToken())
				}
				require.Equal(t, wantIterations, loop.effectiveIterationCount)
			})
		}
	}
}
