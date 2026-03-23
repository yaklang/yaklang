package schema

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReportToRecordAppliesRecordMeta(t *testing.T) {
	report := &Report{}
	report.Title("IRify 报告")
	report.Owner("alice")
	report.From("ssa-scan")
	report.Markdown("# hello")

	finishedAt := time.Unix(1710000000, 0)
	report.SetRecordMeta(&ReportRecordMeta{
		ReportType:       "ssa-scan",
		ScopeType:        "task",
		ScopeName:        "JavaSecLab 第6批",
		ProjectName:      "JavaSecLab",
		TaskID:           "task-1",
		TaskCount:        1,
		ScanBatch:        6,
		RiskTotal:        12,
		RiskCritical:     1,
		RiskHigh:         2,
		RiskMedium:       5,
		RiskLow:          4,
		SourceFinishedAt: &finishedAt,
	})

	record, err := report.ToRecord()
	require.NoError(t, err)
	require.Equal(t, "ssa-scan", record.ReportType)
	require.Equal(t, "task", record.ScopeType)
	require.Equal(t, "JavaSecLab 第6批", record.ScopeName)
	require.Equal(t, "JavaSecLab", record.ProjectName)
	require.Equal(t, "task-1", record.TaskID)
	require.EqualValues(t, 6, record.ScanBatch)
	require.EqualValues(t, 12, record.RiskTotal)
	require.EqualValues(t, 1, record.TaskCount)
	require.NotNil(t, record.SourceFinishedAt)
	require.Equal(t, finishedAt.Unix(), record.SourceFinishedAt.Unix())
}
