package yakgrpc

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// readMITMBinaryRepositoryFixture makes realistic, checked-in binary files
// reusable across gRPC upload and MITM outcome tests without depending on the
// process working directory or a developer-local Downloads folder.
func readMITMBinaryRepositoryFixture(t *testing.T, pathFromRepositoryRoot ...string) []byte {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	fixturePath := filepath.Join(append([]string{repositoryRoot}, pathFromRepositoryRoot...)...)
	content, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	require.NotEmpty(t, content)
	return content
}

func requireRealZIPFixture(t *testing.T, content []byte) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)
	require.NotEmpty(t, reader.File)
	var expandedSize int64
	for _, entry := range reader.File {
		file, err := entry.Open()
		require.NoError(t, err)
		n, readErr := io.Copy(io.Discard, file)
		closeErr := file.Close()
		require.NoError(t, readErr)
		require.NoError(t, closeErr)
		expandedSize += n
	}
	require.Greater(t, expandedSize, int64(0))
	require.False(t, utf8.Valid(content), "the fixture must exercise binary editor/export behavior")
}

func requireRealPDFFixture(t *testing.T, content []byte) {
	t.Helper()
	require.True(t, bytes.HasPrefix(content, []byte("%PDF-")))
	require.True(t, bytes.Contains(content, []byte("%%EOF")))
	require.Greater(t, len(content), 1024)
	require.False(t, utf8.Valid(content), "the fixture must exercise binary editor/export behavior")
}
