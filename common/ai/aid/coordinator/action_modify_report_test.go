package coordinator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatorActionModifyReport(t *testing.T) {
	f := completedActionFixture(t)
	f.invoke("create_report", map[string]any{"title": "报告", "document": "# Report\nverified\n"}, false)
	path := f.c.Snapshot().Report.Path
	f.invoke("modify_report", map[string]any{"document_patch": "--- coordinator-report.md\n+++ coordinator-report.md\n@@ -1,2 +1,2 @@\n # Report\n-verified\n+verified with evidence\n"}, false)
	require.Equal(t, "# Report\nverified with evidence\n", f.c.Snapshot().Report.Document)
	require.Equal(t, path, f.c.Snapshot().Report.Path)
	f.invoke("modify_report", map[string]any{"document_patch": "--- unrelated.md\n+++ unrelated.md\n@@ -1 +1 @@\n-a\n+b\n"}, true)
	require.Equal(t, "# Report\nverified with evidence\n", f.c.Snapshot().Report.Document)
	f.invoke("modify_report", map[string]any{"document": "new body", "document_patch": "bad"}, true)
}
