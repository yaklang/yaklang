package coordinator_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPlanPhaseMockerRestoreDoesNotInitializeAgain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}, &schema.AiCheckpoint{}, &schema.AIAgentRuntime{}).Error)
	var builds atomic.Int64
	id := uuid.NewString()
	opts := []aicommon.ConfigOption{aicommon.WithID(id), aicommon.WithPersistentSessionId("mocker-restore"), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithDisallowMCPServers(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false), aicommon.WithAgreeYOLO(), coordinator.WithPlanMocker(func(*coordinator.Session) *coordinator.PlanResponse {
		builds.Add(1)
		return &coordinator.PlanResponse{Document: "预设文档", RootTask: &coordinator.PlanNode{Name: "检查", Goal: "调查来源", Identifier: "root", Subtasks: []*coordinator.PlanNode{{Name: "读来源", Goal: "核对来源", Identifier: "read"}}}}
	})}
	for iteration := 0; iteration < 2; iteration++ {
		var calls int
		var s *coordinator.Session
		callback := aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls++
			if calls > 2 {
				return nil, fmt.Errorf("restore did not converge")
			}
			if s.Snapshot().Phase == coordinator.PhasePlan {
				return nativeResponse(cfg, req, "submit_plan", map[string]any{})
			}
			return nativeResponse(cfg, req, "finish", map[string]any{})
		})
		s, err = coordinator.NewSession(ctx, "仅规划", append(append([]aicommon.ConfigOption{}, opts...), callback)...)
		require.NoError(t, err)
		s.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(id, db)
		require.NoError(t, s.RunPlanOnly())
		require.EqualValues(t, 1, builds.Load())
		require.Equal(t, coordinator.PhaseExec, s.Snapshot().Phase)
		if iteration == 1 {
			require.Equal(t, 1, calls, "restore starts with the persisted plan, not create/submit")
		}
	}
}
