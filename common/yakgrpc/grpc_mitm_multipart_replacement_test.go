package yakgrpc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestManualLargeRequestReplacementStore_MultipartPart(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)
	target, err := newManualLargeRequestReplacementTarget(false, 2)
	require.NoError(t, err)

	require.NoError(t, store.consume(false, 2, []byte("first-"), true, false, false))
	activePath := store.active[target].path
	require.FileExists(t, activePath)
	require.True(t, store.hasActive())
	require.False(t, store.hasCompleted())

	require.NoError(t, store.consume(false, 2, []byte("second"), false, true, false))
	require.False(t, store.hasActive())
	require.True(t, store.hasCompleted())
	completedPath := store.multipartPaths()[2]
	require.Equal(t, activePath, completedPath)
	content, err := os.ReadFile(completedPath)
	require.NoError(t, err)
	require.Equal(t, []byte("first-second"), content)
	// Starting another upload for the same part discards the prior completed
	// file and supports an empty replacement file in one message.
	require.NoError(t, store.consume(false, 2, nil, true, true, false))
	require.NoFileExists(t, completedPath)
	emptyPath := store.multipartPaths()[2]
	require.FileExists(t, emptyPath)
	info, err := os.Stat(emptyPath)
	require.NoError(t, err)
	require.Zero(t, info.Size())

	require.NoError(t, store.consume(false, 2, nil, false, false, true))
	require.False(t, store.hasCompleted())
	require.NoFileExists(t, emptyPath)
}

func TestManualLargeRequestReplacementStore_RequestBody(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)

	require.NoError(t, store.consume(true, 0, []byte("raw-body"), true, true, false))
	bodyPath := store.bodyPath()
	require.FileExists(t, bodyPath)
	content, err := os.ReadFile(bodyPath)
	require.NoError(t, err)
	require.Equal(t, []byte("raw-body"), content)
	require.Empty(t, store.multipartPaths())
}

func TestManualLargeRequestReplacementStoreRejectsChunkBeforeStart(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)

	err := store.consume(false, 0, []byte("orphan"), false, false, false)
	require.ErrorContains(t, err, "was not started")
}

func TestRenderMITMSubmittedRequest_FileAndUserFuzzTag(t *testing.T) {
	original := []byte{0xff, 0x00, 'A'}
	originalPath := filepath.Join(t.TempDir(), "original.bin")
	require.NoError(t, os.WriteFile(originalPath, original, 0o600))
	packet := []byte("POST /{{int(7)}} HTTP/1.1\r\nHost: example.test\r\nContent-Length: 999\r\n\r\n{{file(" + originalPath + ")}}")

	rendered, err := renderMITMSubmittedRequest(packet)
	require.NoError(t, err)
	require.Contains(t, string(rendered), "POST /7 HTTP/1.1")
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, original, body)
	require.Equal(t, fmt.Sprint(len(original)), lowhttp.GetHTTPPacketHeader(rendered, "Content-Length"))
}

func TestRenderMITMV2SubmittedRequest_ResourceAndUserFuzzTag(t *testing.T) {
	original := []byte{0xff, 0x00, 'A'}
	originalPath := filepath.Join(t.TempDir(), "original.bin")
	require.NoError(t, os.WriteFile(originalPath, original, 0o600))
	tag := "{{file(" + originalPath + ")}}"
	packet := []byte("POST /{{int(7)}} HTTP/1.1\r\nHost: example.test\r\nContent-Length: 1\r\n\r\n" + tag)

	rendered, resourceCount, err := renderMITMV2SubmittedRequest(packet, originalPath, false, nil)
	require.NoError(t, err)
	require.Equal(t, 1, resourceCount)
	require.Contains(t, string(rendered), "POST /7 HTTP/1.1")
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.True(t, bytes.Equal(original, body))
	require.Equal(t, "3", lowhttp.GetHTTPPacketHeader(rendered, "Content-Length"))
}

func TestRenderMITMV2SubmittedRequest_InlineUnquoteBinaryHexEdit(t *testing.T) {
	// This is the backend half of the Binary-chip contract. The frontend sends
	// one textual unquote tag after a HEX edit; SendPacket must expand it before
	// fixing Content-Length and writing to the target server.
	edited := append(bytes.Repeat([]byte{0x11}, 16), []byte{'P', 'K', 0x03, 0x04}...)
	tag := lowhttp.ToUnquoteFuzzTagForce(edited)
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/zip\r\nContent-Length: 999\r\n\r\n" + tag)

	rendered, resourceCount, err := renderMITMV2SubmittedRequest(packet, "", false, nil)
	require.NoError(t, err)
	require.Zero(t, resourceCount)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, edited, body)
	require.Equal(t, fmt.Sprint(len(edited)), lowhttp.GetHTTPPacketHeader(rendered, "Content-Length"))
	require.NotContains(t, body, []byte(`\x11`))
}

func TestRenderMITMV2SubmittedRequest_AppliesCompletedReplacement(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	originalPath := filepath.Join(t.TempDir(), "original.bin")
	require.NoError(t, os.WriteFile(originalPath, []byte("original"), 0o600))
	tag := "{{file(" + originalPath + ")}}"
	packet := []byte("POST / HTTP/1.1\r\nHost: example.test\r\n\r\n" + tag)

	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)
	require.NoError(t, store.consume(true, 0, []byte("replacement"), true, true, false))
	rendered, resourceCount, err := renderMITMV2SubmittedRequest(packet, originalPath, false, store)
	require.NoError(t, err)
	require.Equal(t, 1, resourceCount)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, []byte("replacement"), body)
}

func TestRenderMITMV2SubmittedRequest_ReplacementMarkerRemovedFailsClosed(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)
	require.NoError(t, store.consume(true, 0, []byte("replacement"), true, true, false))

	_, _, err := renderMITMV2SubmittedRequest(
		[]byte("POST / HTTP/1.1\r\nHost: example.test\r\n\r\nbody"),
		filepath.Join(t.TempDir(), "original.bin"),
		false,
		store,
	)
	require.ErrorContains(t, err, "file tag was removed")
}

func TestRenderMITMV2SubmittedRequest_MissingOriginalFileFailsClosed(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.bin")
	packet := []byte("POST / HTTP/1.1\r\nHost: example.test\r\n\r\n{{file(" + missingPath + ")}}")

	_, _, err := renderMITMV2SubmittedRequest(packet, missingPath, false, nil)
	require.ErrorContains(t, err, "large request body sidecar")
}
