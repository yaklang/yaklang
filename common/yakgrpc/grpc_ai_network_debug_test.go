package yakgrpc

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestAINetworkDebugExportAndSessionOption(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	path, db, err := consts.GetTempTestDatabase()
	require.NoError(t, err)
	defer func() { db.Close(); os.Remove(path) }()
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)
	event := &schema.AiOutputEvent{CoordinatorId: "runtime-a", SessionId: "session-a", Type: schema.EVENT_TYPE_STREAM, EventUUID: "event-a", Content: []byte("answer"), AIModelVerboseName: "verbose-model", VizSource: "reason_content", RecoveryIndexID: "recovery-id"}
	require.NoError(t, db.Create(event).Error)
	for _, session := range []string{"session-a", "session-b"} {
		ctx := httptrace.WithEnabled(httptrace.WithScope(context.Background(), httptrace.Scope{SessionID: session, RuntimeID: "runtime-a", CallID: "call-a"}), true)
		record := httptrace.Start(ctx, []byte("POST /chat HTTP/1.1\r\nHost: local\r\n\r\n"), false)
		require.NotNil(t, record)
		record.Write([]byte("HTTP/1.1 429 Too Many Requests\r\nRetry-After: 60\r\nContent-Length: 5\r\n\r\nlimit"))
		record.Finish(nil, 429, "", "60", true)
	}
	server := &Server{projectDatabase: db}
	response, err := server.ExportAILogs(context.Background(), &ypb.ExportAILogsRequest{SessionID: "session-a", ExportDataTypes: []string{"output_event"}, OutputPath: filepath.Join(t.TempDir(), "export.zip")})
	require.NoError(t, err)
	zipReader, err := zip.OpenReader(response.FilePath)
	require.NoError(t, err)
	defer zipReader.Close()
	networkFiles := 0
	eventsFound := false
	for _, file := range zipReader.File {
		reader, err := file.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(reader)
		reader.Close()
		require.NoError(t, err)
		if strings.HasPrefix(file.Name, "network/") {
			networkFiles++
		}
		if file.Name == "AIOutputEvent.json" {
			var events []*schema.AiOutputEvent
			require.NoError(t, json.Unmarshal(b, &events))
			require.Len(t, events, 1)
			require.Equal(t, "session-a", events[0].SessionId)
			require.Equal(t, event.VizSource, events[0].VizSource)
			require.Equal(t, event.AIModelVerboseName, events[0].AIModelVerboseName)
			require.Equal(t, event.RecoveryIndexID, events[0].RecoveryIndexID)
			eventsFound = true
		}
		if filepath.Base(file.Name) == "metadata.json" {
			var meta httptrace.Metadata
			require.NoError(t, json.Unmarshal(b, &meta))
			require.Equal(t, "session-a", meta.SessionID)
			require.Equal(t, 429, meta.Status)
		}
	}
	require.True(t, eventsFound)
	require.Equal(t, 3, networkFiles)
	// Explicit false can turn off a previously cached session opt-in. An old
	// frontend omitting the optional field does not accidentally change it.
	cached := &ypb.AIStartParams{NetworkDebug: proto.Bool(true)}
	require.True(t, yakit.MergeCachedAISessionStartParams(cached, &ypb.AIStartParams{}).GetNetworkDebug())
	require.False(t, yakit.MergeCachedAISessionStartParams(cached, &ypb.AIStartParams{NetworkDebug: proto.Bool(false)}).GetNetworkDebug())
	require.False(t, yakit.OverlayAISessionStartParams(cached, &ypb.AIStartParams{NetworkDebug: proto.Bool(false)}).GetNetworkDebug())
}
