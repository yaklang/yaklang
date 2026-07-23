package scannode

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

func decodeArtifactToParts(t *testing.T, codec string, build *SSAArtifactBuildResult) []sfreport.SSAResultParts {
	t.Helper()
	raw, err := os.ReadFile(build.ArtifactPath)
	require.NoError(t, err)

	var plain []byte
	switch codec {
	case "zstd":
		dec, err := zstd.NewReader(nil)
		require.NoError(t, err)
		defer dec.Close()
		plain, err = dec.DecodeAll(raw, nil)
		require.NoError(t, err)
	case "gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		require.NoError(t, err)
		defer gr.Close()
		plain, err = io.ReadAll(gr)
		require.NoError(t, err)
	case "identity":
		plain = raw
	default:
		t.Fatalf("unsupported codec in test: %s", codec)
	}

	dec := json.NewDecoder(bufio.NewReader(bytes.NewReader(plain)))
	parts := make([]sfreport.SSAResultParts, 0)
	for {
		var p sfreport.SSAResultParts
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		parts = append(parts, p)
	}
	return parts
}

func TestSSAArtifactCollector_PersistNDJSONParts(t *testing.T) {
	c := NewSSAArtifactCollector("task-1", "runtime-1", "sub-1")
	defer c.Cleanup()

	riskRaw := json.RawMessage(`{"hash":"rh1","title":"sql injection","severity":"high","risk_type":"sql","program_name":"demo"}`)
	parts := &sfreport.SSAResultParts{
		ProgramName: "demo",
		ReportType:  string(sfreport.IRifyFullReportType),
		Files: []*sfreport.File{
			{Path: "src/A.java", IrSourceHash: "fh1", Content: "class A {}", Risks: []string{"rh1"}},
		},
		Dataflows: []*sfreport.SSADataflowPart{
			{DataflowHash: "df1", Payload: json.RawMessage(`{"description":"path","nodes":[],"edges":[]}`)},
		},
		Risks: []*sfreport.SSARiskPart{
			{RiskHash: "rh1", RiskJSON: riskRaw, FileHashes: []string{"fh1"}, DataflowHashes: []string{"df1"}},
		},
	}
	raw, err := json.Marshal(parts)
	require.NoError(t, err)
	require.NoError(t, c.AddStreamPayload(string(raw)))

	build, err := c.BuildCompressedArtifact("identity")
	require.NoError(t, err)
	require.NotNil(t, build)
	require.Equal(t, spec.SSAArtifactFormatPartsNDJSONV1, build.ArtifactFormat)
	require.EqualValues(t, 1, build.RiskCount)
	require.EqualValues(t, 1, build.FileCount)
	require.EqualValues(t, 1, build.FlowCount)

	decoded := decodeArtifactToParts(t, "identity", build)
	require.Len(t, decoded, 1)
	require.Equal(t, "demo", decoded[0].ProgramName)
	require.Len(t, decoded[0].Risks, 1)
	require.Len(t, decoded[0].Files, 1)
	require.Len(t, decoded[0].Dataflows, 1)
}

func TestSSAArtifactCollector_Codec_Zstd(t *testing.T) {
	c := NewSSAArtifactCollector("task-1", "runtime-1", "sub-1")
	defer c.Cleanup()
	parts := &sfreport.SSAResultParts{
		ProgramName: "demo",
		ReportType:  string(sfreport.IRifyFullReportType),
		Risks: []*sfreport.SSARiskPart{{
			RiskHash: "rh1",
			RiskJSON: json.RawMessage(`{"hash":"rh1","title":"x","severity":"high"}`),
		}},
	}
	raw, err := json.Marshal(parts)
	require.NoError(t, err)
	require.NoError(t, c.AddStreamPayload(string(raw)))

	build, err := c.BuildCompressedArtifact("zstd")
	require.NoError(t, err)
	require.Equal(t, int64(1), build.RiskCount)

	decoded := decodeArtifactToParts(t, "zstd", build)
	require.Len(t, decoded, 1)
	require.Len(t, decoded[0].Risks, 1)
}

func TestSSAArtifactCollector_Codec_Gzip(t *testing.T) {
	c := NewSSAArtifactCollector("task-1", "runtime-1", "sub-1")
	defer c.Cleanup()
	parts := &sfreport.SSAResultParts{
		ProgramName: "demo",
		ReportType:  string(sfreport.IRifyFullReportType),
		Risks: []*sfreport.SSARiskPart{{
			RiskHash: "rh1",
			RiskJSON: json.RawMessage(`{"hash":"rh1","title":"x","severity":"high"}`),
		}},
	}
	raw, err := json.Marshal(parts)
	require.NoError(t, err)
	require.NoError(t, c.AddStreamPayload(string(raw)))

	build, err := c.BuildCompressedArtifact("gzip")
	require.NoError(t, err)
	decoded := decodeArtifactToParts(t, "gzip", build)
	require.Len(t, decoded, 1)
	require.Len(t, decoded[0].Risks, 1)
}

func TestAppendKeyValueParams_SkipScannodeInternalKeys(t *testing.T) {
	node := &ScanNode{}
	args := node.appendKeyValueParams(nil, map[string]interface{}{
		"task-id":                   "t1",
		"_scannode_ssa_upload_url":  "http://localhost/upload",
		"_scannode_ssa_object_key":  "ssa/tasks/t1/report.json.gz",
		"_scannode_ssa_codec":       "gzip",
		"rule_snapshot_id":          "rulesnapshot-a",
		"_scannode_unknown_setting": "x",
	})

	joined := map[string]struct{}{}
	for _, a := range args {
		joined[a] = struct{}{}
	}
	_, hasTask := joined["--task-id"]
	_, hasRuleSnapshot := joined["--rule_snapshot_id"]
	_, hasInternalObjectKey := joined["--_scannode_ssa_object_key"]
	_, hasInternalCodec := joined["--_scannode_ssa_codec"]

	require.True(t, hasTask)
	require.True(t, hasRuleSnapshot)
	require.False(t, hasInternalObjectKey)
	require.False(t, hasInternalCodec)
}

func TestAppendKeyValueParams_ExpandsStringSliceValues(t *testing.T) {
	node := &ScanNode{}
	args := node.appendKeyValueParams(nil, map[string]interface{}{
		"port-preset": []string{"top100"},
		"ports":       "2080",
	})

	require.Equal(t, []string{"top100"}, valuesAfterFlag(args, "--port-preset"))
	require.NotContains(t, args, `["top100"]`)
	require.Equal(t, []string{"2080"}, valuesAfterFlag(args, "--ports"))
}

func valuesAfterFlag(args []string, flag string) []string {
	values := make([]string, 0)
	for index := 0; index < len(args)-1; index++ {
		if args[index] == flag {
			values = append(values, args[index+1])
		}
	}
	return values
}

func TestUploadMetricsAccumulation(t *testing.T) {
	c := NewSSAArtifactCollector("task-1", "runtime-1", "sub-1")

	c.recordUploadMs(500)
	c.recordUploadMs(300)
	c.recordTicketFetchMs(120)
	c.recordTicketFetchMs(80)
	c.recordRetry()
	c.recordRetry()
	c.recordRetry()
	c.recordSegment()
	c.recordSegment()

	m := c.snapshotUploadMetrics()
	if m.TotalUploadMs != 800 {
		t.Errorf("totalUploadMs = %d, want 800", m.TotalUploadMs)
	}
	if m.TicketFetchMs != 200 {
		t.Errorf("ticketFetchMs = %d, want 200", m.TicketFetchMs)
	}
	if m.Retries != 3 {
		t.Errorf("retries = %d, want 3", m.Retries)
	}
	if m.Segments != 2 {
		t.Errorf("segments = %d, want 2", m.Segments)
	}
}

func TestUploadMetricsConcurrency(t *testing.T) {
	c := NewSSAArtifactCollector("task-1", "runtime-1", "sub-1")

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			c.recordUploadMs(1)
			c.recordRetry()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	m := c.snapshotUploadMetrics()
	if m.TotalUploadMs != 100 {
		t.Errorf("totalUploadMs = %d, want 100", m.TotalUploadMs)
	}
	if m.Retries != 100 {
		t.Errorf("retries = %d, want 100", m.Retries)
	}
}

func TestBuildReadyEventPopulatesMetrics(t *testing.T) {
	c := NewSSAArtifactCollector("task-1", "runtime-1", "sub-1")
	c.recordUploadMs(1500)
	c.recordTicketFetchMs(200)
	c.recordSegment()
	c.recordRetry()
	c.setUploadBytes(102400, 32000)

	build := &SSAArtifactBuildResult{
		ObjectKey:        "ssa/task-1/artifact",
		Codec:            "zstd",
		ArtifactFormat:   "ssa-result-segments-manifest-v1",
		CompressedSize:   32000,
		UncompressedSize: 102400,
		SHA256:           "abc123",
	}
	event := c.BuildReadyEvent(build, 5000, 42)
	if event == nil {
		t.Fatal("BuildReadyEvent returned nil")
	}
	if len(event.Metrics) == 0 {
		t.Fatal("event.Metrics is empty, expected upload metrics JSON")
	}

	var metrics map[string]any
	if err := json.Unmarshal(event.Metrics, &metrics); err != nil {
		t.Fatalf("failed to parse metrics JSON: %v", err)
	}
	if metrics["total_upload_ms"] != float64(1500) {
		t.Errorf("metrics.total_upload_ms = %v, want 1500", metrics["total_upload_ms"])
	}
	if metrics["ticket_fetch_ms"] != float64(200) {
		t.Errorf("metrics.ticket_fetch_ms = %v, want 200", metrics["ticket_fetch_ms"])
	}
	if metrics["segments"] != float64(1) {
		t.Errorf("metrics.segments = %v, want 1", metrics["segments"])
	}
	if metrics["retries"] != float64(1) {
		t.Errorf("metrics.retries = %v, want 1", metrics["retries"])
	}
}
