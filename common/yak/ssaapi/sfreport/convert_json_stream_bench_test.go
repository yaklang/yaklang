package sfreport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// benchmarkReport builds a report shaped like a javacms-core scan: many small
// findings, a bounded rule set and a large file table.
func benchmarkReport(risks, files int, elementBytes int) *Report {
	report := NewReport(IRifyReportType)
	report.SetProgramName("bench")
	report.SetProgramLang(ssaconfig.JAVA)
	body := "select * from users where id = ${id}"
	for i := 0; i < risks; i++ {
		name := fmt.Sprintf("rule-%d", i%131)
		report.AddRules(&Rule{
			RuleName: name,
			Language: ssaconfig.JAVA,
			Title:    "SQL injection",
			Severity: "high",
			Content:  body,
			Risks:    []string{fmt.Sprintf("hash-%d", i)},
		})
		report.AddRisks(&Risk{
			ID:           uint(i + 1),
			Hash:         fmt.Sprintf("hash-%d", i),
			Title:        "SQL injection",
			Description:  body,
			Solution:     body,
			Severity:     "high",
			RiskType:     "sqli",
			CWE:          schema.StringArray{"CWE-89"},
			CodeRange:    "10-20",
			CodeFragment: body,
			RuleName:     name,
			ProgramName:  "bench",
		})
	}
	for i := 0; i < files; i++ {
		report.AddFile(&File{
			Path:         fmt.Sprintf("/src/main/java/File%d.java", i),
			IrSourceHash: fmt.Sprintf("ir-%d", i),
			LineCount:    42,
			Content:      strings.Repeat("x", elementBytes),
		})
	}
	return report
}

// BenchmarkReportSnapshot compares the two ways of turning a report into one
// document: a full json.MarshalIndent, or the streamed writer that replaced it.
//
// The streamed writer exists for memory, not speed: a marshal allocates about
// 3.5x the document, which on javacms-core meant several GB per checkpoint.
func BenchmarkReportSnapshot(b *testing.B) {
	report := benchmarkReport(400, 400, 0)
	b.Run("marshal_indent", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				b.Fatal(err)
			}
			if len(data) == 0 {
				b.Fatal("empty document")
			}
		}
	})
	b.Run("stream", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := report.writeJSONSnapshot(io.Discard); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkReportSnapshotLarge uses the shape that matters: a big file table
// dominates the document, exactly like javacms-core (408MB, 2,590 files).
func BenchmarkReportSnapshotLarge(b *testing.B) {
	report := benchmarkReport(2000, 1200, 900)
	var size bytes.Buffer
	if err := report.writeJSONSnapshot(&size); err != nil {
		b.Fatal(err)
	}
	b.Logf("document=%dMB", size.Len()>>20)

	b.Run("marshal_indent", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				b.Fatal(err)
			}
			if len(data) == 0 {
				b.Fatal("empty document")
			}
		}
	})
	b.Run("stream", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := report.writeJSONSnapshot(io.Discard); err != nil {
				b.Fatal(err)
			}
		}
	})
}
