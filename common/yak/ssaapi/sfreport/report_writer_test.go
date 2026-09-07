package sfreport

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReport_SaveWithoutWriter(t *testing.T) {
	report := NewReport(IRifyReportType)
	report.SetProgramName("source-only")
	report.AddRisks(&Risk{Title: "demo"})
	require.NoError(t, report.Save())
}

func TestSarifReport_SaveWithoutWriter(t *testing.T) {
	report, err := NewSarifReport()
	require.NoError(t, err)
	require.NoError(t, report.Save())

	var buf bytes.Buffer
	require.NoError(t, report.SetWriter(&buf))
	require.NoError(t, report.Save())
	require.NotEmpty(t, buf.Bytes())
}
