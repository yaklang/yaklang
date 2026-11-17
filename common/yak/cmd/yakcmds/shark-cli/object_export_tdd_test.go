package sharkcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func objectExportManifests(t *testing.T, wire []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, row := range analysisJSONRows(t, wire) {
		if row["record_type"] == "object_export" {
			out = append(out, row)
		}
	}
	return out
}

func TestAnalysisCLIObjectExportHTTP1EdgeCases(t *testing.T) {
	t.Run("chunked request body", func(t *testing.T) {
		body := "username=admin&password=plaintext"
		request := fmt.Sprintf("POST /chunked HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/x-www-form-urlencoded\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n0\r\n\r\n", len(body), body)
		input := analysisHTTPFixture(t, []string{request}, []string{"HTTP/1.1 204 No Content\r\n\r\n"})
		dir := filepath.Join(t.TempDir(), "objects")
		var output bytes.Buffer
		require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", dir))
		manifests := objectExportManifests(t, output.Bytes())
		require.Len(t, manifests, 1)
		require.Equal(t, "request", manifests[0]["direction"])
		require.Nil(t, manifests[0]["declared_length"])
		wire, err := os.ReadFile(manifests[0]["path"].(string))
		require.NoError(t, err)
		require.Equal(t, body, string(wire))
	})

	for _, tc := range []struct {
		name, request, response string
	}{
		{"HEAD representation length", "HEAD /head HTTP/1.1\r\nHost: example.test\r\n\r\n", "HTTP/1.1 200 OK\r\nContent-Length: 4096\r\n\r\n"},
		{"204 no content", "GET /none HTTP/1.1\r\nHost: example.test\r\n\r\n", "HTTP/1.1 204 No Content\r\n\r\n"},
		{"304 not modified", "GET /cache HTTP/1.1\r\nHost: example.test\r\n\r\n", "HTTP/1.1 304 Not Modified\r\nContent-Length: 4096\r\n\r\n"},
		{"302 zero length", "GET /redirect HTTP/1.1\r\nHost: example.test\r\n\r\n", "HTTP/1.1 302 Found\r\nLocation: /next\r\nContent-Length: 0\r\n\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := analysisHTTPFixture(t, []string{tc.request}, []string{tc.response})
			dir := filepath.Join(t.TempDir(), "objects")
			var output bytes.Buffer
			require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", dir))
			require.Empty(t, objectExportManifests(t, output.Bytes()))
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}

func TestAnalysisCLIObjectExportPipelinedHTTP1Associations(t *testing.T) {
	bodyOne, bodyTwo := "first=1", "second=2"
	requestOne := fmt.Sprintf("POST /one HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: %d\r\n\r\n%s", len(bodyOne), bodyOne)
	requestTwo := fmt.Sprintf("POST /two HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: %d\r\n\r\n%s", len(bodyTwo), bodyTwo)
	responseOne := "HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\none"
	responseTwo := "HTTP/1.1 201 Created\r\nContent-Length: 3\r\n\r\ntwo"
	packets := [][]byte{
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 499, 100, "", true, true, false),
		nativeOpeningPacket(t, 3, 100, 500, requestOne+requestTwo, false, false, false),
		nativeOpeningPacket(t, 4, 500, 100+uint32(len(requestOne)+len(requestTwo)), responseOne+responseTwo, true, false, false),
	}
	input := fixture(t, "pipelined.pcap", packets...)
	dir := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", dir))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 4)
	want := map[string]string{
		"request:/one":  bodyOne,
		"response:/one": "one",
		"request:/two":  bodyTwo,
		"response:/two": "two",
	}
	for _, manifest := range manifests {
		key := manifest["direction"].(string) + ":" + manifest["uri"].(string)
		body, ok := want[key]
		require.True(t, ok, key)
		wire, err := os.ReadFile(manifest["path"].(string))
		require.NoError(t, err)
		require.Equal(t, body, string(wire), key)
	}
}

func TestAnalysisCLIObjectExportHTTP2MultiplexedBodies(t *testing.T) {
	input, err := trafficfixture.Materialize(top30Sessions+"http2-multiplex.pcap", t.TempDir())
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", dir))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 3)
	want := map[string]string{
		"request:/two":  "body",
		"response:/one": "first",
		"response:/two": "second",
	}
	for _, manifest := range manifests {
		key := manifest["direction"].(string) + ":" + manifest["uri"].(string)
		body, ok := want[key]
		require.True(t, ok, key)
		wire, err := os.ReadFile(manifest["path"].(string))
		require.NoError(t, err)
		require.Equal(t, body, string(wire), key)
	}

	dir = filepath.Join(t.TempDir(), "post-only")
	output.Reset()
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", dir, "-Y", `http.request.method == "POST"`))
	manifests = objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 1)
	require.Equal(t, "request", manifests[0]["direction"])
	require.Equal(t, "/two", manifests[0]["uri"])
	wire, err := os.ReadFile(manifests[0]["path"].(string))
	require.NoError(t, err)
	require.Equal(t, "body", string(wire))
}

func TestSafeObjectNameAvoidsWindowsDeviceNames(t *testing.T) {
	for _, uri := range []string{"/CON", "/con.txt", "/NUL", "/COM1", "/LPT9"} {
		name := safeObjectName(uri, "application/octet-stream")
		base := name[:len(name)-len(filepath.Ext(name))]
		require.NotContains(t, []string{"CON", "NUL", "COM1", "LPT9"}, strings.ToUpper(base), uri)
	}
}
