package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func analysisJSONRows(t *testing.T, wire []byte) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(wire))
	var rows []map[string]any
	for {
		var row map[string]any
		err := decoder.Decode(&row)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		rows = append(rows, row)
	}
	return rows
}

func analysisHTTPFixture(t *testing.T, requests, responses []string) string {
	t.Helper()
	packets := [][]byte{nativeOpeningPacket(t, 1, 99, 0, "", false, true, false), nativeOpeningPacket(t, 2, 499, 100, "", true, true, false)}
	cseq, sseq := uint32(100), uint32(500)
	for i, request := range requests {
		packets = append(packets, nativeOpeningPacket(t, uint64(len(packets)+1), cseq, sseq, request, false, false, false))
		cseq += uint32(len(request))
		if i < len(responses) {
			response := responses[i]
			packets = append(packets, nativeOpeningPacket(t, uint64(len(packets)+1), sseq, cseq, response, true, false, false))
			sseq += uint32(len(response))
		}
	}
	return fixture(t, "analysis.pcap", packets...)
}

func TestAnalysisCLIExportsAssociatedLiteralCredentialsAndDecodedChunks(t *testing.T) {
	request := "GET /../resource HTTP/1.1\r\nHost: example.test\r\nUser-Agent: curl/test\r\nAuthorization: Basic YWRtaW46cGFzc3dvcmQ=\r\nCookie: session=deadbeef; role=admin\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"
	input := analysisHTTPFixture(t, []string{request}, []string{response})
	dir := filepath.Join(t.TempDir(), "new", "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", dir, "-Y", `http.response.code == 200`))
	var manifest map[string]any
	for _, row := range analysisJSONRows(t, output.Bytes()) {
		if row["record_type"] == "object_export" {
			require.Nil(t, manifest)
			manifest = row
		}
	}
	require.NotNil(t, manifest)
	require.Equal(t, "Basic YWRtaW46cGFzc3dvcmQ=", manifest["authorization"])
	require.Equal(t, "session=deadbeef; role=admin", manifest["cookie"])
	require.Equal(t, "/../resource", manifest["uri"])
	name := manifest["path"].(string)
	require.Equal(t, dir, filepath.Dir(name))
	wire, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, "Wikipedia", string(wire))
	sum := sha256.Sum256(wire)
	require.Equal(t, hex.EncodeToString(sum[:]), manifest["sha256"])
	require.EqualValues(t, 9, manifest["observed_length"])
	require.Nil(t, manifest["declared_length"])
	require.Equal(t, true, manifest["complete"])
	output.Reset()
	require.ErrorContains(t, invoke(&output, "-r", input, "--json", "--export-objects", dir), "file exists")
	unchanged, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, wire, unchanged)
}

func TestAnalysisObjectExportMissingRequestLengthMismatchAndPathSafety(t *testing.T) {
	exporter, err := newObjectExporter(t.TempDir())
	require.NoError(t, err)
	declared := int64(10)
	manifest, err := exporter.export(&objectObservation{id: 7, responseTo: 6, protocol: "http", body: strings.NewReader("short"), declared: &declared, contentType: "text/plain"})
	require.NoError(t, err)
	require.Nil(t, manifest.Authorization)
	require.Nil(t, manifest.Cookie)
	require.Nil(t, manifest.URI)
	require.False(t, manifest.Complete)
	require.True(t, manifest.LengthMismatch)
	for _, uri := range []string{"../../etc/passwd", "/%2e%2e/%2e%2e/escape", "/..\\..\\escape", "/", "/\x00bad", "/CON", "https://example.test/a?x=y"} {
		name := safeObjectName(uri, "application/octet-stream")
		require.NotEmpty(t, name)
		require.Equal(t, name, filepath.Base(name))
		require.NotContains(t, name, "/")
		require.NotContains(t, name, "\\")
		require.NotEqual(t, "..", name)
	}
	for _, completeness := range []string{"headers", "envelope", "incomplete", "encrypted"} {
		e := &pcaputil.ProtocolEvent{Protocol: "http", Completeness: completeness, Session: map[string]any{"Method": "GET", "Headers": map[string][]string{"Authorization": {"Basic value"}}}}
		require.Nil(t, observedObject(e), completeness)
	}
}

func TestAnalysisCLIPhysicalOnlyAndFrameTransportFilters(t *testing.T) {
	request := "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"
	input := fixture(t, "gap.pcap", nativeOpeningPacket(t, 1, 99, 0, "", false, true, false), nativeOpeningPacket(t, 2, 100, 0, "GE", false, false, false), nativeOpeningPacket(t, 3, 104, 0, request[4:], false, false, false))
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json-physical-only"))
	rows := analysisJSONRows(t, output.Bytes())
	require.Len(t, rows, 3)
	for i, row := range rows {
		require.NotContains(t, row, "record_type")
		require.EqualValues(t, i+1, row["number"])
	}
	output.Reset()
	require.NoError(t, invoke(&output, "-r", input, "--json"))
	found := false
	for _, row := range analysisJSONRows(t, output.Bytes()) {
		if row["record_type"] == "protocol_publication" {
			found = true
			require.Equal(t, false, row["physical"])
		}
	}
	require.True(t, found)
	input = analysisHTTPFixture(t, []string{request}, []string{"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"})
	for _, expr := range []string{`frame.number <= 2`, `tcp.dstport == 12011`, `ip.src == 192.0.2.1 and tcp.dstport == 12011`} {
		output.Reset()
		require.NoError(t, invoke(&output, "-r", input, "--json-physical-only", "-Y", expr))
		rows = analysisJSONRows(t, output.Bytes())
		require.Len(t, rows, 2, expr)
	}
	output.Reset()
	require.NoError(t, invoke(&output, "-r", input, "--json", "-Y", `pdu.content contains "GET /"`))
	matches := 0
	for _, row := range analysisJSONRows(t, output.Bytes()) {
		if row["record_type"] == nil {
			matches++
			require.EqualValues(t, 3, row["number"])
		}
	}
	require.Equal(t, 1, matches)
}

func TestAnalysisCLIStatisticsCountFilteredFramesAndPDUs(t *testing.T) {
	input := analysisHTTPFixture(t, []string{"GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"}, []string{"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"})
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json"))
	pduCount := 0
	flowIDs := map[float64]bool{}
	for _, row := range analysisJSONRows(t, output.Bytes()) {
		if pdus, ok := row["pdus"].([]any); ok {
			pduCount += len(pdus)
		}
		if id, ok := row["tcp_flow_id"].(float64); ok {
			flowIDs[id] = true
		}
	}
	output.Reset()
	require.NoError(t, invoke(&output, "-r", input, "--stats", "proto", "--stats", "conv", "--stats", "io", "--stats", "endpoints", "--json"))
	rows := analysisJSONRows(t, output.Bytes())
	require.Len(t, rows, 4)
	for _, record := range rows {
		entries := record["rows"].([]any)
		switch record["kind"] {
		case "proto":
			count := 0
			for _, value := range entries {
				entry := value.(map[string]any)
				count += int(entry["pdus"].(float64))
				if entry["protocol"] == "http" {
					require.EqualValues(t, 2, entry["frames"])
				}
			}
			require.Equal(t, pduCount, count)
		case "conv":
			require.Len(t, entries, len(flowIDs))
			require.Contains(t, entries[0].(map[string]any)["source"], ":")
		case "io":
			require.Len(t, entries, 1)
			require.EqualValues(t, 4, entries[0].(map[string]any)["packets"])
		}
	}
	output.Reset()
	require.NoError(t, invoke(&output, "-r", input, "--stats", "io", "--stats-interval", "1s", "--json", "-Y", "frame.number <= 2"))
	rows = analysisJSONRows(t, output.Bytes())
	require.EqualValues(t, 2, rows[0]["rows"].([]any)[0].(map[string]any)["packets"])
	output.Reset()
	require.NoError(t, invoke(&output, "-r", input, "--stats", "proto"))
	require.Contains(t, output.String(), "PROTOCOL")
	require.Contains(t, output.String(), "PDUS")
	for _, args := range [][]string{{"--stats", "bad"}, {"--stats", "io", "--stats-interval", "0s"}, {"--json-physical-only", "--stats", "proto"}} {
		output.Reset()
		require.Error(t, invoke(&output, append([]string{"-r", input}, args...)...))
	}
	stats, err := newTrafficStatistics([]string{"io"}, time.Second)
	require.NoError(t, err)
	require.NoError(t, stats.add(packetSummary{Time: time.Unix(-1, 500000000), CapturedLength: 10}))
	require.Contains(t, stats.buckets, int64(-1))
}
