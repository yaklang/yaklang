package sharkcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalysisCLIJSONFileWritesStrictJSONLWithoutStdout(t *testing.T) {
	request := "POST /login HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 15\r\n\r\nusername=admin"
	input := analysisHTTPFixture(t, []string{request}, []string{"HTTP/1.1 204 No Content\r\n\r\n"})
	path := filepath.Join(t.TempDir(), "analysis.jsonl")

	var stdout bytes.Buffer
	require.NoError(t, invoke(&stdout, "-r", input, "--json-file", path))
	require.Empty(t, stdout.String())

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	rows := analysisJSONRows(t, mustReadFile(t, path))
	require.NotEmpty(t, rows)
}

func TestAnalysisCLIJSONFileIsExclusive(t *testing.T) {
	input := analysisHTTPFixture(t, []string{"GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"}, []string{"HTTP/1.1 204 No Content\r\n\r\n"})
	path := filepath.Join(t.TempDir(), "analysis.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0600))

	var stdout bytes.Buffer
	err := invoke(&stdout, "-r", input, "--json-file", path)
	require.ErrorContains(t, err, "file exists")
	require.Empty(t, stdout.String())
	require.Equal(t, "keep", string(mustReadFile(t, path)))
}

func TestAnalysisCLIJSONFileWithObjectExport(t *testing.T) {
	body := "username=admin&password=plaintext"
	request := fmt.Sprintf("POST /login HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	input := analysisHTTPFixture(t, []string{request}, []string{"HTTP/1.1 204 No Content\r\n\r\n"})
	path := filepath.Join(t.TempDir(), "analysis.jsonl")
	objects := filepath.Join(t.TempDir(), "objects")

	var stdout bytes.Buffer
	require.NoError(t, invoke(&stdout, "-r", input, "--json-file", path, "--export-objects", objects))
	require.Empty(t, stdout.String())
	manifests := 0
	for _, row := range analysisJSONRows(t, mustReadFile(t, path)) {
		if row["record_type"] == "object_export" {
			manifests++
			require.Equal(t, "http_body", row["object_type"])
			require.Equal(t, body, string(mustReadFile(t, row["path"].(string))))
		}
	}
	require.Equal(t, 1, manifests)
}

func TestAnalysisCLIJSONFileRejectsLegacyJSONCombination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analysis.jsonl")
	var stdout bytes.Buffer
	err := invoke(&stdout, "-r", "unused.pcap", "--json", "--json-file", path)
	require.ErrorContains(t, err, "cannot be combined")
	require.Empty(t, stdout.String())
	_, statErr := os.Stat(path)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
