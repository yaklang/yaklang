package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_DeleteAISession_RemovesTrajectories(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected_session", true: "all_sessions"}[all], func(t *testing.T) {
			db, err := utils.CreateTempTestDatabaseInMemory()
			require.NoError(t, err)
			defer db.Close()
			require.NoError(t, db.AutoMigrate(&schema.AISession{}, &schema.AIAgentRuntime{}, &schema.AiCheckpoint{},
				&schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{}, &schema.AISessionPlanAndExec{}, &schema.AITrajectory{}).Error)
			for _, session := range []string{"trajectory_%", "trajectory_other"} {
				_, err := yakit.CreateOrUpdateAISessionMeta(db, session, "trajectory test")
				require.NoError(t, err)
				require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: session, PersistentSession: session}).Error)
				for _, kind := range []string{schema.AITrajectoryCompression, schema.AITrajectoryTerminal} {
					_, err := yakit.SaveAITrajectory(db, &schema.AITrajectory{PersistentSessionID: session, TaskID: "task", Kind: kind,
						TrajectoryKey: "compression", Prompt: "full source", Response: "result"})
					require.NoError(t, err)
				}
			}
			server := &Server{projectDatabase: db}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err = server.DeleteAISession(ctx, &ypb.DeleteAISessionRequest{DeleteAll: all,
				Filter: &ypb.DeleteAISessionFilter{SessionID: []string{"trajectory_%"}}})
			require.NoError(t, err)
			var rows []*schema.AITrajectory
			require.NoError(t, db.Unscoped().Find(&rows).Error)
			if all {
				require.Empty(t, rows)
			} else {
				require.Len(t, rows, 2)
				for _, row := range rows {
					require.Equal(t, "trajectory_other", row.PersistentSessionID)
				}
			}
		})
	}
}
