package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestPlanPhaseRealReviewRevisionStaleCardAndEditedApproval(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			db, err := utils.CreateTempTestDatabaseInMemory()
			require.NoError(t, err)
			defer db.Close()
			require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}, &schema.AiCheckpoint{}, &schema.AIAgentRuntime{}).Error)
			var s *coordinator.Session
			var calls, reviews atomic.Int64
			var oldID string
			input := make(chan *ypb.AIInputEvent, 8)
			rejectedOld := make(chan bool, 1)
			model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				switch calls.Add(1) {
				case 1:
					return protocolResponse(cfg, req, native, "create_plan", map[string]any{"plan": map[string]any{"name": "检查", "goal": "提交审核", "tasks": []any{map[string]any{"name": "核对", "goal": "核对来源", "identifier": "check"}}}, "plan_document": "初始文档"})
				case 2:
					return protocolResponse(cfg, req, native, "submit_plan", map[string]any{})
				case 3:
					if state := s.Snapshot(); state.Phase != coordinator.PhasePlan || state.ReviewPending {
						return nil, fmt.Errorf("revision did not unlock PLAN")
					}
					return protocolResponse(cfg, req, native, "modify_plan", map[string]any{"document": "修订后的文档"})
				case 4:
					return protocolResponse(cfg, req, native, "submit_plan", map[string]any{})
				default:
					return protocolResponse(cfg, req, native, "finish", map[string]any{})
				}
			}
			s, err = coordinator.NewSession(ctx, "规划并提交审核", aicommon.WithWorkdir(t.TempDir()), aicommon.WithPersistentSessionId("review-boundary"), aicommon.WithEnableFunctionCallMode(native), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false), aicommon.WithForceManualPlanReview(true), aicommon.WithEventInputChan(input), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAICallback(model), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				if e.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
					return
				}
				var payload map[string]any
				_ = json.Unmarshal(e.Content, &payload)
				id := payload["id"].(string)
				if reviews.Add(1) == 1 {
					oldID = id
					input <- &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: `{"suggestion":"unclear","extra_prompt":"明确只读边界"}`}
					return
				}
				go func() {
					input <- &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: oldID, InteractiveJSONInput: `{"suggestion":"continue"}`}
					select {
					case <-time.After(50 * time.Millisecond):
					case <-ctx.Done():
						return
					}
					state := s.Snapshot()
					rejectedOld <- oldID != id && state.Phase == coordinator.PhasePlan && state.ReviewPending && calls.Load() == 4
					reply, _ := json.Marshal(map[string]any{"suggestion": "continue", "plans": map[string]any{"root_task": payload["plans"].(map[string]any)["root_task"], "document": "用户最终编辑的文档"}})
					// Duplicate delivery to the same endpoint must not create another approval.
					replyEvent := &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: string(reply)}
					input <- replyEvent
					input <- replyEvent
				}()
			}))
			require.NoError(t, err)
			s.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(s.Id, db)
			require.NoError(t, s.RunPlanOnly())
			require.True(t, <-rejectedOld, "old card must not approve the next review")
			require.EqualValues(t, 2, reviews.Load())
			require.Equal(t, coordinator.PhaseExec, s.Snapshot().Phase)
			require.Equal(t, "用户最终编辑的文档", s.Snapshot().Plan.Document)
			for _, a := range s.Snapshot().Attempts {
				require.Zero(t, a.ID)
			}
		})
	}
}
