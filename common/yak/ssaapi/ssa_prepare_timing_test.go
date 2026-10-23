package ssaapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeReportsSeparatePreparationPhases(t *testing.T) {
	t.Setenv("YAK_SSA_SOURCE_CACHE", "0")
	root := t.TempDir()
	for name, source := range map[string]string{"A.java": "class A { int f() { return 7; } }", "view.jsp": `<p><%= request.getParameter("q") %></p>`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := MaterializeLocalProject(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 2 || result.Errors != 0 {
		t.Fatalf("result: %+v", result)
	}
	m := result.Timings
	if m.SourceCount != 1 || m.TemplateCount != 1 || m.WalkErrors != 0 || m.TemplateErrors != 0 {
		t.Fatalf("phase counts: %+v", m)
	}
	if m.WalkSeconds <= 0 || m.ArchivePrepareSeconds <= 0 || m.TemplatePrepareSeconds <= 0 || m.TemplateTranslationWorkerSeconds <= 0 || m.TemplateJavaWorkerSeconds <= 0 || m.StoreWorkerSeconds <= 0 {
		t.Fatalf("missing measured phase: %+v", m)
	}
	if m.DiskWriteWorkerSeconds != 0 || m.DiskWriteBytes != 0 {
		t.Fatalf("disabled disk tier was reported as disk writes: %+v", m)
	}
	if total := m.WalkSeconds + m.ArchivePrepareSeconds + m.TemplatePrepareSeconds + m.ReleaseSeconds; total > result.Elapsed.Seconds() {
		t.Fatalf("serial phase durations %.6f exceed total %.6f", total, result.Elapsed.Seconds())
	}
}
