package httptrace

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestExportIsolationAndActiveSnapshot(t *testing.T) {
	root := t.TempDir()
	var active *Record
	for _, session := range []string{"session-a", "session-b"} {
		ctx := WithEnabled(WithDirectory(WithScope(context.Background(), Scope{SessionID: session, RuntimeID: session + "-runtime"}), root), true)
		r := Start(ctx, []byte("POST / HTTP/1.1\r\nHost: local\r\n\r\n"), false)
		r.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nhalf"))
		if session == "session-a" {
			active = r
		} else {
			r.Finish(nil, 200, "", "", true)
		}
	}
	defer active.Finish(nil, 200, "", "", true)
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	n, err := Export(context.Background(), z, root, "session-a", nil)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, z.Close())
	reader, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	require.Len(t, reader.File, 3)
	for _, file := range reader.File {
		f, err := file.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(f)
		f.Close()
		require.NoError(t, err)
		if filepath.Base(file.Name) == "metadata.json" {
			var meta Metadata
			require.NoError(t, json.Unmarshal(b, &meta))
			require.Equal(t, "session-a", meta.SessionID)
			require.Nil(t, meta.Ended)
			require.False(t, meta.TransportComplete)
		}
	}
}

func TestCaptureFailureDoesNotBreakNetworkWriter(t *testing.T) {
	ctx := WithEnabled(WithDirectory(context.Background(), t.TempDir()), true)
	r := Start(ctx, nil, false)
	require.NotNil(t, r)
	require.NoError(t, r.response.Close())
	n, err := r.Write([]byte("response"))
	require.NoError(t, err)
	require.Equal(t, 8, n)
	r.Finish(nil, 200, "", "", true)
	b, err := os.ReadFile(filepath.Join(r.dir, "metadata.json"))
	require.NoError(t, err)
	var meta Metadata
	require.NoError(t, json.Unmarshal(b, &meta))
	require.NotEmpty(t, meta.CaptureError)
}

func TestRemoveSessionsIsolation(t *testing.T) {
	// Unique identities keep this check separate from any user's diagnostic files.
	sessions := []string{t.Name() + uuid.NewString(), t.Name() + uuid.NewString()}
	t.Cleanup(func() { _ = RemoveSessions(sessions) })
	var dirs []string
	for _, session := range sessions {
		ctx := WithEnabled(WithScope(context.Background(), Scope{SessionID: session}), true)
		r := Start(ctx, []byte("GET / HTTP/1.1\r\nHost: fixture\r\n\r\n"), false)
		require.NotNil(t, r)
		r.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
		r.Finish(nil, 200, "", "", true)
		dirs = append(dirs, r.dir)
	}
	require.NoError(t, RemoveSessions([]string{"", sessions[0]}))
	_, err := os.Stat(dirs[0])
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(dirs[1], "response.http"))
}
