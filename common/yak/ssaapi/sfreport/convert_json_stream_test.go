package sfreport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func populatedReport(t *testing.T, risks int) *Report {
	t.Helper()
	report := NewReport(IRifyFullReportType, WithDataflowPath(true), WithFileContent(true))
	report.SetProgramName("demo")
	report.SetProgramLang(ssaconfig.JAVA)
	report.SetDescription("desc")
	report.SetRepositoryURL("https://example.invalid/repo")
	report.SetFileCount(2)
	report.SetCodeLineCount(120)
	report.RiskNums = risks
	for i := 0; i < risks; i++ {
		report.AddRules(&Rule{
			RuleName: fmt.Sprintf("rule-%d", i),
			Language: ssaconfig.JAVA,
			Title:    fmt.Sprintf("title-%d", i),
			Severity: "high",
			Content:  "rule content",
			Risks:    []string{fmt.Sprintf("hash-%d", i)},
		})
		report.AddRisks(&Risk{
			ID:           uint(i + 1),
			Hash:         fmt.Sprintf("hash-%d", i),
			Title:        fmt.Sprintf("risk-%d", i),
			Description:  "a description with <html> & \"quotes\"",
			Severity:     "high",
			RiskType:     "sqli",
			CWE:          schema.StringArray{"CWE-89"},
			CodeRange:    "1-2",
			CodeFragment: "exec(sql)",
			RuleName:     fmt.Sprintf("rule-%d", i),
			DataFlowPaths: []*DataFlowPath{{
				Description: "flow",
				Nodes:       []*NodeInfo{{NodeID: "a", IRCode: "a"}},
				Edges:       []*EdgeInfo{{FromNodeID: "a", ToNodeID: "b"}},
				DotGraph:    "digraph {}",
			}},
		})
	}
	report.AddFile(&File{
		Path:         "/src/Main.java",
		Length:       42,
		Hash:         map[string]string{"md5": "m", "sha1": "s", "sha256": "ss"},
		IrSourceHash: "ir-1",
		Content:      "class Main {}",
		LineCount:    3,
		Risks:        []string{"hash-0"},
	})
	report.AddFile(&File{
		Path:         "/src/Util.java",
		IrSourceHash: "ir-2",
		Content:      "class Util {}",
		LineCount:    1,
	})
	return report
}

// TestReport_StreamedDocumentMatchesMarshalIndent proves the streaming writer
// is a pure replacement for json.MarshalIndent: same bytes, same order, same
// escaping, and the same null-versus-empty distinction for unset collections.
func TestReport_StreamedDocumentMatchesMarshalIndent(t *testing.T) {
	cases := []struct {
		name   string
		report *Report
	}{
		{name: "empty", report: NewReport(IRifyReportType)},
		{name: "nil collections", report: &Report{ReportType: IRifyReportType}},
		{name: "single risk", report: populatedReport(t, 1)},
		{name: "many risks", report: populatedReport(t, 32)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := json.MarshalIndent(tc.report, "", "  ")
			require.NoError(t, err)

			var got bytes.Buffer
			require.NoError(t, tc.report.writeJSONSnapshot(&got))
			require.Equal(t, string(want), got.String())
		})
	}
}

// TestReport_StreamedDocumentRoundTrips proves a reader sees exactly the same
// report, not only the same bytes.
func TestReport_StreamedDocumentRoundTrips(t *testing.T) {
	report := populatedReport(t, 8)
	var buf bytes.Buffer
	require.NoError(t, report.writeJSONSnapshot(&buf))

	var decoded Report
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, report.ProgramName, decoded.ProgramName)
	require.Equal(t, report.ProgramLang, decoded.ProgramLang)
	require.Len(t, decoded.Risks, len(report.Risks))
	require.Len(t, decoded.Rules, len(report.Rules))
	require.Len(t, decoded.File, len(report.File))
	require.Equal(t, "sqli", decoded.Risks["hash-3"].RiskType)
	require.Len(t, decoded.Risks["hash-3"].DataFlowPaths, 1)
}

// A snapshot that fails midway must never replace the previous document.
func TestReport_SaveKeepsPreviousDocumentWhenStreamingFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	defer file.Close()

	// The first snapshot succeeds and becomes the document to protect.
	require.NoError(t, writeReportSnapshot(file, func(out io.Writer) error {
		_, err := out.Write([]byte(`{"previous":true}`))
		return err
	}))

	err = writeReportSnapshot(file, func(out io.Writer) error {
		_, _ = out.Write([]byte(`{"partial":`))
		return errors.New("stream broke")
	})
	require.Error(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `{"previous":true}`, string(data),
		"a failed snapshot must leave the last complete document in place")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file may survive a failed snapshot")
}

// A destination that cannot be rewound must not be left holding half a
// document either.
func TestReport_FailedSnapshotDropsPartialBuffer(t *testing.T) {
	buf := &SnapshotBuffer{}
	err := writeReportSnapshot(buf, func(out io.Writer) error {
		_, _ = out.Write([]byte(`{"partial":`))
		return errors.New("stream broke")
	})
	require.Error(t, err)
	require.Zero(t, buf.buf.Len(), "the partial document must be discarded")
}

// A checkpoint runs while the scan keeps adding findings. The document must
// stay valid JSON and the streamed bytes must be identical to a full marshal
// taken under the same lock.
func TestReport_StreamedSnapshotIsStableUnderConcurrentWrites(t *testing.T) {
	report := populatedReport(t, 64)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			select {
			case <-stop:
				return
			default:
			}
			report.AddRisks(&Risk{Hash: fmt.Sprintf("live-%d", i), Title: "live"})
			runtime.Gosched()
		}
	}()

	for round := 0; round < 3; round++ {
		var buf bytes.Buffer
		require.NoError(t, report.writeJSONSnapshot(&buf))
		var decoded Report
		require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded),
			"concurrent findings must never produce a partial document")
	}
	close(stop)
	wg.Wait()
}

// The streamed writer lists the document fields by hand, so a new exported
// field on Report would be silently dropped from every snapshot. This guard
// fails as soon as the struct and the writer disagree.
func TestReport_StreamedDocumentCoversEveryField(t *testing.T) {
	report := populatedReport(t, 1)
	var buf bytes.Buffer
	require.NoError(t, report.writeJSONSnapshot(&buf))
	document := buf.String()

	typ := reflect.TypeOf(report).Elem()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Tag.Get("json") == "-" {
			continue
		}
		name := field.Name
		if tag := field.Tag.Get("json"); tag != "" {
			name = strings.Split(tag, ",")[0]
			if name == "" {
				name = field.Name
			}
		}
		require.Contains(t, document, fmt.Sprintf("\n  %q: ", name),
			"field %s is missing from the streamed document", field.Name)
	}
}
