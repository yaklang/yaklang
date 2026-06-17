package loop_code_security_audit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditState_PersistAndLoadFromWorkDir(t *testing.T) {
	tmp := t.TempDir()
	auditDir := filepath.Join(tmp, "audit")
	require.NoError(t, os.MkdirAll(auditDir, 0o755))

	state := NewAuditState()
	state.WorkDir = tmp
	state.SetProjectInfo("/tmp/demo", "demo")
	state.TechStack = "go"
	state.SetFinalReport("# report\n")
	state.SetFinalReportPath(filepath.Join(auditDir, "security_audit_report.md"))
	require.NoError(t, os.WriteFile(state.GetFinalReportPath(), []byte("# report\n"), 0o644))

	require.NoError(t, state.PersistToAuditDir(auditDir))

	loaded, ok := TryLoadAuditStateFromWorkDir(tmp)
	require.True(t, ok)
	require.NotNil(t, loaded)
	require.Equal(t, AuditPhaseDone, loaded.GetPhase())
	require.Equal(t, "demo", loaded.ProjectName)
	require.Equal(t, "/tmp/demo", loaded.ProjectPath)
	require.Equal(t, "go", loaded.TechStack)
	require.Equal(t, state.GetFinalReportPath(), loaded.GetFinalReportPath())
}

func TestTryLoadAuditStateFromWorkDir_ReconstructFromReportOnly(t *testing.T) {
	tmp := t.TempDir()
	auditDir := filepath.Join(tmp, "audit")
	require.NoError(t, os.MkdirAll(auditDir, 0o755))

	reportPath := filepath.Join(auditDir, "security_audit_report.md")
	require.NoError(t, os.WriteFile(reportPath, []byte("# legacy report\n"), 0o644))

	loaded, ok := TryLoadAuditStateFromWorkDir(tmp)
	require.True(t, ok)
	require.Equal(t, AuditPhaseDone, loaded.GetPhase())
	require.Equal(t, reportPath, loaded.GetFinalReportPath())
	require.Contains(t, loaded.GetFinalReport(), "legacy report")
}

func TestTryLoadAuditStateFromWorkDir_NotFound(t *testing.T) {
	tmp := t.TempDir()
	_, ok := TryLoadAuditStateFromWorkDir(tmp)
	require.False(t, ok)
}
