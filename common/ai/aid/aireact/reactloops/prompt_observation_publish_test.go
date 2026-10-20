package reactloops

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

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
	return aicommon.NewEmitter("prompt-profile-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
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

func (i *observedCompressionInvoker) AssembleLoopPrompt(_ []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	i.builds++
	prompt := aicommon.RenderTimelineFrozenOpen(i.timeline).PromptText() + input.FrozenUserContext + input.TodoSnapshot
	return &aicommon.LoopPromptAssemblyResult{Prompt: prompt, Sections: []*aicommon.PromptSectionObservation{
		aicommon.NewPromptSectionObservation("timeline", "Timeline", aicommon.PromptSectionRoleTimelineOpen, true, prompt),
	}}, nil
}

func TestMainLoopPublishesOnlyRequestedPromptObservations(t *testing.T) {
	for _, mode := range []string{"normal", "fold_only", "ai_compression", "provider_overflow", "overflow_no_progress"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			capture := &promptProfileCapture{}
			var loop *ReActLoop
			var requests []string
			compressionCalls := 0
			limit := 1200
			if mode == "provider_overflow" || mode == "overflow_no_progress" {
				limit = 10000
			}
			cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithEmitter(capture.emitter()), aicommon.WithTimelineContentLimit(limit),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if req.GetCallerLabel() == aicommon.CallerLabelTimelineCompress {
						compressionCalls++
						loop.WaitForInflightObservation()
						require.Len(t, capture.snapshot(), len(requests), "compression builds must not generate profiles")
						response := c.NewAIResponse()
						response.EmitOutputStream(strings.NewReader(`{"@action":"compress_timeline","summary":"COMPACTED_HISTORY","retain_timeline":"","retire_skill_names":[],"retire_evidence_ids":[]}`))
						response.Close()
						return response, nil
					}
					requests = append(requests, req.GetPrompt())
					require.Equal(t, aicommon.MeasureTokens(req.GetPrompt()), loop.GetLastPromptToken())
					if (mode == "provider_overflow" || mode == "overflow_no_progress") && len(requests) == 1 {
						return nil, promptOverflowErrorFor("openai", "gpt-4o", 5000)
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(`{"@action":"observation_done"}`))
					response.Close()
					return response, nil
				}))
			cfg.AiServerName, cfg.AiModelName = "openai", "gpt-4o"
			invoker := &observedCompressionInvoker{MockInvoker: mock.NewMockInvoker(ctx), timeline: cfg.Timeline}
			invoker.SetConfig(cfg)
			var err error
			loop, err = NewReActLoop("observation-compression", invoker,
				WithFunctionCallMode(false), WithAllowToolCall(false), WithAllowRAG(false), WithAllowAIForge(false),
				WithAllowPlanAndExec(false), WithAllowUserInteract(false), WithDisableLoopPerception(true),
				WithDisablePeriodicVerification(true), WithDisableIncreaseIteration(true),
				WithRegisterLoopAction("require_tool", "unused routing", nil, nil, nil),
				WithRegisterLoopAction("observation_done", "finish observation test", nil, nil,
					func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) { op.Exit() }))
			require.NoError(t, err)
			if mode == "fold_only" {
				for _, content := range []string{strings.Repeat("OBSOLETE_EVIDENCE ", 2000), "current evidence"} {
					payload, err := json.Marshal(map[string]string{"id": "e1", "content": content})
					require.NoError(t, err)
					require.True(t, cfg.Timeline.PushPromotable(cfg.AcquireId(), aicommon.TimelinePromotedKindEvidence,
						aicommon.TimelinePromotedTargetSemiDynamic1, "e1", aicommon.TimelinePromotedOperationUpsert, string(payload)))
				}
			} else if mode != "normal" && mode != "overflow_no_progress" {
				cfg.Timeline.PushText(cfg.AcquireId(), strings.Repeat("ORIGINAL_HISTORY ", 2000))
			}
			executeErr := loop.Execute("observation-task", ctx, "continue the task")
			if mode == "overflow_no_progress" {
				require.True(t, aicommon.IsPromptTooLong(executeErr), "an unchanged rebuild should stop without another request")
			} else {
				require.NoError(t, executeErr)
			}
			loop.WaitForInflightObservation()
			profiles := capture.snapshot()
			wantRequests, wantCompression := 1, 0
			if mode == "ai_compression" || mode == "provider_overflow" {
				wantCompression = 1
			}
			if mode == "provider_overflow" {
				wantRequests = 2
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
			require.Equal(t, profiles[len(profiles)-1].PromptTokens, loop.GetLastPromptToken())
		})
	}
}
