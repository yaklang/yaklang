package sfreport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers: build minimal Report structs for unit testing without SSA compilation ---

func makeTestReport(risks map[string]*Risk, files []*File) *Report {
	r := NewReport(IRifyFullReportType)
	r.ProgramName = "test-program"
	r.Risks = risks
	r.File = files
	for _, f := range files {
		if f != nil && f.IrSourceHash != "" {
			r.fileByHash[f.IrSourceHash] = f
			r.IrSourceHashes[f.IrSourceHash] = struct{}{}
		}
	}
	return r
}

func makeFile(hash, path, content string, riskHashes []string) *File {
	return &File{
		Path:         path,
		IrSourceHash: hash,
		Content:      content,
		Length:       int64(len(content)),
		LineCount:    strings.Count(content, "\n") + 1,
		Risks:        riskHashes,
	}
}

func makeRisk(hash, title, severity, program string, flows []*DataFlowPath) *Risk {
	return &Risk{
		Hash:        hash,
		Title:       title,
		Severity:    severity,
		ProgramName: program,
		DataFlowPaths: flows,
	}
}

func makeDataFlowPath(desc string) *DataFlowPath {
	return &DataFlowPath{
		Description: desc,
		Nodes: []*NodeInfo{
			{NodeID: "n1", IRCode: "var x", IRSourceHash: "fh1", StartOffset: 0, EndOffset: 5, IsEntryNode: true},
			{NodeID: "n2", IRCode: "sink(x)", IRSourceHash: "fh1", StartOffset: 10, EndOffset: 17},
		},
		Edges: []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "data-flow"},
		},
	}
}

// --- Tests ---

func TestConvertSingleResultToStreamParts_NilResult(t *testing.T) {
	parts, err := ConvertSingleResultToStreamParts(nil, StreamPartsOptions{})
	require.NoError(t, err)
	assert.Nil(t, parts)
}

func TestBuildFiles_Basic(t *testing.T) {
	f1 := makeFile("hash1", "/src/a.java", "class A {}", []string{"r1", "r2"})
	f2 := makeFile("hash2", "/src/b.java", "class B {}", []string{"r1"})
	report := makeTestReport(nil, []*File{f1, f2})

	out := &StreamSingleResultParts{}
	riskFiles := buildFiles(out, report, true, nil, false)

	assert.Len(t, out.Files, 2)
	assert.Contains(t, riskFiles, "r1")
	assert.Contains(t, riskFiles, "r2")
	assert.ElementsMatch(t, []string{"hash1", "hash2"}, riskFiles["r1"])
	assert.Equal(t, []string{"hash1"}, riskFiles["r2"])

	// Verify deterministic ordering by hash.
	assert.Equal(t, "hash1", out.Files[0].IrSourceHash)
	assert.Equal(t, "hash2", out.Files[1].IrSourceHash)
}

func TestBuildFiles_WithoutFile(t *testing.T) {
	f1 := makeFile("hash1", "/src/a.java", "class A {}", nil)
	report := makeTestReport(nil, []*File{f1})

	out := &StreamSingleResultParts{}
	riskFiles := buildFiles(out, report, false, nil, false)

	assert.Empty(t, out.Files)
	assert.Empty(t, riskFiles)
}

func TestBuildFiles_DedupAcrossStream(t *testing.T) {
	streamKey := "test-dedup-files"
	defer ResetStreamFileDedup(streamKey)

	f1 := makeFile("hash1", "/src/a.java", "class A {}", []string{"r1"})
	report := makeTestReport(nil, []*File{f1})

	dedup := getStreamDedupState(streamKey)

	// First call: file should be included.
	out1 := &StreamSingleResultParts{}
	buildFiles(out1, report, true, dedup, true)
	assert.Len(t, out1.Files, 1)

	// Second call with same streamKey: file should be deduped.
	out2 := &StreamSingleResultParts{}
	riskFiles := buildFiles(out2, report, true, dedup, true)
	assert.Empty(t, out2.Files, "file should be deduped on second call")
	assert.Contains(t, riskFiles, "r1", "risk→file association should still exist")
}

func TestBuildFiles_InvalidUTF8(t *testing.T) {
	content := "hello\x80world"
	f1 := makeFile("hash1", "/src/bad.bin", content, nil)
	report := makeTestReport(nil, []*File{f1})

	out := &StreamSingleResultParts{}
	buildFiles(out, report, true, nil, false)

	require.Len(t, out.Files, 1)
	// Content should be escaped to valid UTF-8.
	assert.True(t, isValidUTF8(out.Files[0].Content))
	assert.NotEqual(t, content, out.Files[0].Content)
}

func TestBuildFiles_DuplicateHashInFileList(t *testing.T) {
	f1 := makeFile("hash1", "/src/a.java", "class A {}", []string{"r1"})
	f2 := makeFile("hash1", "/src/a.java", "class A {}", []string{"r1"}) // duplicate
	report := makeTestReport(nil, []*File{f1, f2})

	out := &StreamSingleResultParts{}
	buildFiles(out, report, true, nil, false)

	assert.Len(t, out.Files, 1, "duplicate files with same hash should be deduped")
}

func TestBuildDataflowsAndRisks_SinglePass(t *testing.T) {
	flow := makeDataFlowPath("taint from source to sink")
	r1 := makeRisk("risk1", "SQL Injection", "high", "test-program", []*DataFlowPath{flow})
	r2 := makeRisk("risk2", "XSS", "medium", "test-program", nil)

	riskFiles := map[string][]string{
		"risk1": {"fh1"},
	}
	report := makeTestReport(map[string]*Risk{"risk1": r1, "risk2": r2}, nil)

	out := &StreamSingleResultParts{
		ProgramName: "test-program",
	}
	buildDataflowsAndRisks(out, report, nil, false, riskFiles)

	assert.Len(t, out.Risks, 2)
	assert.GreaterOrEqual(t, len(out.Dataflows), 1, "should have at least 1 dataflow from risk1")

	// Find risk1 part and verify references.
	var risk1Part *StreamRiskPart
	for _, rp := range out.Risks {
		if rp.RiskHash == "risk1" {
			risk1Part = rp
			break
		}
	}
	require.NotNil(t, risk1Part)
	assert.Equal(t, []string{"fh1"}, risk1Part.FileHashes)
	assert.NotEmpty(t, risk1Part.DataflowHashes)

	// Verify RiskJSON doesn't contain data_flow_paths.
	var riskData map[string]interface{}
	require.NoError(t, json.Unmarshal(risk1Part.RiskJSON, &riskData))
	assert.Nil(t, riskData["data_flow_paths"], "DataFlowPaths should be stripped from RiskJSON")
	assert.Equal(t, "risk1", riskData["hash"])
}

func TestBuildDataflowsAndRisks_DedupDataflow(t *testing.T) {
	streamKey := "test-dedup-flow"
	defer ResetStreamFileDedup(streamKey)

	flow := makeDataFlowPath("same flow")
	r1 := makeRisk("risk1", "SQLi", "high", "test-program", []*DataFlowPath{flow})
	r2 := makeRisk("risk2", "SQLi-2", "high", "test-program", []*DataFlowPath{flow})

	report := makeTestReport(map[string]*Risk{"risk1": r1, "risk2": r2}, nil)
	dedup := getStreamDedupState(streamKey)

	out := &StreamSingleResultParts{}
	buildDataflowsAndRisks(out, report, dedup, true, nil)

	// Same flow content → same hash → should appear only once in Dataflows.
	assert.Len(t, out.Dataflows, 1)
	// But both risks should still reference it.
	for _, rp := range out.Risks {
		assert.NotEmpty(t, rp.DataflowHashes)
	}
}

func TestDedupStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"b", "a", "b", "c", "a"}, []string{"a", "b", "c"}},
		{"with_spaces", []string{" a ", "a", " b"}, []string{"a", "b"}},
		{"with_empty", []string{"a", "", "  ", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupStrings(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStreamPartsJSON_EmptyRisks(t *testing.T) {
	opts := StreamPartsOptions{
		ReportType: IRifyFullReportType,
	}
	// nil result → no payload.
	j, stats, err := ConvertSingleResultToStreamPartsJSON(nil, opts)
	require.NoError(t, err)
	assert.Empty(t, j)
	assert.Equal(t, false, stats["has_payload"])
}

func TestStreamDedupState_MarkSeen(t *testing.T) {
	st := &streamDedupState{seen: make(map[string]struct{})}

	assert.True(t, st.markSeen("file:", "h1"))
	assert.False(t, st.markSeen("file:", "h1"), "second call should be deduped")
	assert.True(t, st.markSeen("flow:", "h1"), "different prefix should not conflict")
	assert.True(t, st.markSeen("file:", "h2"), "different key should pass")
}

func TestStreamDedupState_NilSafe(t *testing.T) {
	var st *streamDedupState
	assert.True(t, st.markSeen("file:", "h1"), "nil state should return true for non-empty key")
	assert.False(t, st.markSeen("file:", ""), "nil state should return false for empty key")
}

func isValidUTF8(s string) bool {
	for len(s) > 0 {
		r, size := rune(s[0]), 1
		if r >= 0x80 {
			var ok bool
			r, size = decodeRuneInString(s)
			_ = ok
			if r == 0xFFFD && size == 1 {
				return false
			}
		}
		s = s[size:]
	}
	return true
}

func decodeRuneInString(s string) (rune, int) {
	r := []rune(s[:4])
	if len(r) == 0 {
		return 0xFFFD, 1
	}
	return r[0], len(string(r[0]))
}
