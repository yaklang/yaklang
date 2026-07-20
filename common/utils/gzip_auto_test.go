package utils

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMUSTPASS_OpenFileAutoGzip(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("YAKRAG-test-payload")

	rawPath := filepath.Join(dir, "a.rag")
	require.NoError(t, os.WriteFile(rawPath, payload, 0o644))

	r, err := OpenFileAutoGzip(rawPath, "YAKRAG")
	require.NoError(t, err)
	rawGot, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.Equal(t, payload, rawGot)

	gzBytes, err := GzipCompress(payload)
	require.NoError(t, err)
	gzPath := filepath.Join(dir, "a.rag.gz")
	require.NoError(t, os.WriteFile(gzPath, gzBytes, 0o644))

	r2, err := OpenFileAutoGzip(gzPath, "YAKRAG")
	require.NoError(t, err)
	gzGot, err := io.ReadAll(r2)
	require.NoError(t, err)
	require.NoError(t, r2.Close())
	require.Equal(t, payload, gzGot)

	// misnamed .gz that is actually raw
	misPath := filepath.Join(dir, "b.gz")
	require.NoError(t, os.WriteFile(misPath, payload, 0o644))
	r3, err := OpenFileAutoGzip(misPath, "YAKRAG")
	require.NoError(t, err)
	misGot, err := io.ReadAll(r3)
	require.NoError(t, err)
	require.NoError(t, r3.Close())
	require.Equal(t, payload, misGot)

	// gzip without .gz suffix
	noSuffix := filepath.Join(dir, "c.bin")
	require.NoError(t, os.WriteFile(noSuffix, gzBytes, 0o644))
	r4, err := OpenFileAutoGzip(noSuffix, "YAKRAG")
	require.NoError(t, err)
	noSuffixGot, err := io.ReadAll(r4)
	require.NoError(t, err)
	require.NoError(t, r4.Close())
	require.Equal(t, payload, noSuffixGot)
}
