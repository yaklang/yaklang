package coordinator

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionWriteReport(t *testing.T) {
	f := newActionFixture(t, true)
	f.invoke("write_report", map[string]any{"title": "Audit", "markdown": " ", "summary": "summary"}, true)
	require.Empty(t, f.loop.Get("coordinator_report_path"))
	f.invoke("write_report", map[string]any{"title": "Audit", "markdown": "# full report body", "summary": "verified source e1"}, false)
	path := f.loop.Get("coordinator_report_path")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "# full report body", string(data))
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "verified source e1")
	require.NotContains(t, f.cfg.GetSessionEvidenceRendered(), "# full report body")
}
