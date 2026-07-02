package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestBuildDiscoveryReferenceCatalog_IncludesReconPath(t *testing.T) {
	state := &model.AuditState{
		ProjectPath:   "/proj",
		TechStack:     "PHP",
		ReconFilePath: "/proj/audit/recon.md",
		ReconOutline:  "## Modules\n- sqli\n- xss",
	}
	cat := model.VulnCategory{ID: "xss_injection", Name: "XSS"}
	catalog := BuildDiscoveryReferenceCatalog(state, cat)
	require.GreaterOrEqual(t, len(catalog), 2)
	require.Equal(t, "recon_report", catalog[0].ID)
	require.Equal(t, "/proj/audit/recon.md", catalog[0].Path)
}

func TestEvaluateDiscoveryQuality_FlowCentricWeak(t *testing.T) {
	cat := model.VulnCategory{ID: "xss_injection", Name: "XSS"}
	q := EvaluateDiscoveryQuality(cat, 1, 1)
	require.Equal(t, "weak", q.Level)
}

func TestEvaluateDiscoveryQuality_SinkCentricGood(t *testing.T) {
	cat := model.VulnCategory{ID: "sql_injection", Name: "SQLi"}
	q := EvaluateDiscoveryQuality(cat, 5, 1)
	require.Equal(t, "good", q.Level)
}

func TestFormatDeepDiscoveryGuidance_Empty(t *testing.T) {
	cat := model.VulnCategory{ID: "xss_injection", Name: "XSS"}
	msg := FormatDeepDiscoveryGuidance(cat, DiscoveryQuality{Level: "empty", Reason: "0 candidates", Attempt: 1})
	require.Contains(t, msg, "read_recon_notes")
	require.Contains(t, msg, "数据流型")
}

func TestBuildFastContextQuery_FlowCentric(t *testing.T) {
	q := BuildFastContextQuery(model.VulnCategory{ID: "xss_injection", Name: "XSS"})
	require.Contains(t, q, "数据流型")
	require.Contains(t, q, "Sink")
}
