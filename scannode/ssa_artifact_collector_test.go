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
		"ruleset-hash":              "abc",
		"_scannode_unknown_setting": "x",
	})

	joined := map[string]struct{}{}
	for _, a := range args {
		joined[a] = struct{}{}
	}
	_, hasTask := joined["--task-id"]
	_, hasRuleset := joined["--ruleset-hash"]
	_, hasInternalUpload := joined["--_scannode_ssa_upload_url"]
	_, hasInternalCodec := joined["--_scannode_ssa_codec"]

	require.True(t, hasTask)
	require.True(t, hasRuleset)
	require.False(t, hasInternalUpload)
	require.False(t, hasInternalCodec)
}
