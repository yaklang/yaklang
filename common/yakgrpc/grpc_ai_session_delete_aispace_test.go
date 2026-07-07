package yakgrpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_DeleteAISession_RemovesAISpaceWorkDir(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&schema.AISession{},
		&schema.AIAgentRuntime{},
		&schema.AiCheckpoint{},
		&schema.AiOutputEvent{},
		&schema.AiProcessAndAiEvent{},
	))

	sessionID := "sess-" + uuid.NewString()
	workDir := filepath.Join(consts.GetDefaultAISpaceDir(), "grpc-delete-"+uuid.NewString())
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "artifact.txt"), []byte("x"), 0o644))
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	_, err = yakit.CreateOrUpdateAISessionMeta(db, sessionID, "delete-test")
	require.NoError(t, err)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{
		Uuid:              uuid.NewString(),
		PersistentSession: sessionID,
		WorkDir:           workDir,
	}).Error)

	srv := &Server{projectDatabase: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := srv.DeleteAISession(ctx, &ypb.DeleteAISessionRequest{
		Filter: &ypb.DeleteAISessionFilter{
			SessionID: []string{sessionID},
		},
	})
	require.NoError(t, err)
	require.Contains(t, resp.GetExtraMessage(), "deleted_workdirs=1")

	_, statErr := os.Stat(workDir)
	require.True(t, os.IsNotExist(statErr))
}
