package yakgrpc

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
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
	).Error)

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

	traceCtx := httptrace.WithEnabled(httptrace.WithScope(context.Background(), httptrace.Scope{SessionID: sessionID}), true)
	record := httptrace.Start(traceCtx, []byte("POST / HTTP/1.1\r\nHost: fixture\r\n\r\n"), false)
	require.NotNil(t, record)
	record.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	record.Finish(nil, 200, "text/plain", "", true)
	assertTraceCount := func(want int) {
		archive := zip.NewWriter(io.Discard)
		count, err := httptrace.Export(context.Background(), archive, httptrace.DefaultDirectory(), sessionID, nil)
		require.NoError(t, err)
		require.NoError(t, archive.Close())
		require.Equal(t, want, count)
	}
	assertTraceCount(1)
	t.Cleanup(func() { httptrace.RemoveSessions([]string{sessionID}) })
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
	assertTraceCount(0)
}
