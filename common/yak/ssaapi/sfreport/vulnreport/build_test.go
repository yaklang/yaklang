package vulnreport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockEntity struct {
	sourceID        string
	hash            string
	title           string
	titleVerbose    string
	severity        string
	riskType        string
	description     string
	solution        string
	programName     string
	codeSourceURL   string
	codeRange       string
	codeFragment    string
	functionName    string
	line            int64
	fromRule        string
	cwe             []string
	tags            []string
	disposalStatus  string
	language        string
	taskID          string
	runtimeID       string
	riskFeatureHash string
	createdAt       *time.Time
	updatedAt       *time.Time
	isPotential     bool
}

func (m mockEntity) GetSourceID() string             { return m.sourceID }
func (m mockEntity) GetHash() string                 { return m.hash }
func (m mockEntity) GetTitle() string                { return m.title }
func (m mockEntity) GetTitleVerbose() string         { return m.titleVerbose }
func (m mockEntity) GetSeverity() string             { return m.severity }
func (m mockEntity) GetRiskType() string             { return m.riskType }
func (m mockEntity) GetDescription() string          { return m.description }
func (m mockEntity) GetSolution() string             { return m.solution }
func (m mockEntity) GetProgramName() string          { return m.programName }
func (m mockEntity) GetCodeSourceURL() string        { return m.codeSourceURL }
func (m mockEntity) GetCodeRange() string            { return m.codeRange }
func (m mockEntity) GetCodeFragment() string         { return m.codeFragment }
func (m mockEntity) GetFunctionName() string         { return m.functionName }
func (m mockEntity) GetLine() int64                  { return m.line }
func (m mockEntity) GetFromRule() string             { return m.fromRule }
func (m mockEntity) GetCWEList() []string            { return m.cwe }
func (m mockEntity) GetTags() []string               { return m.tags }
func (m mockEntity) GetLatestDisposalStatus() string { return m.disposalStatus }
func (m mockEntity) GetLanguage() string             { return m.language }
func (m mockEntity) GetTaskID() string               { return m.taskID }
func (m mockEntity) GetRuntimeID() string            { return m.runtimeID }
func (m mockEntity) GetRiskFeatureHash() string      { return m.riskFeatureHash }
func (m mockEntity) GetCreatedAt() *time.Time        { return m.createdAt }
func (m mockEntity) GetUpdatedAt() *time.Time        { return m.updatedAt }
func (m mockEntity) IsPotentialRisk() bool           { return m.isPotential }

func TestBuildSnapshotFromEntities(t *testing.T) {
	now := time.Date(2026, 3, 17, 16, 0, 0, 0, time.UTC)
	later := now.Add(5 * time.Minute)
	snapshot, err := BuildSnapshotFromEntities(context.Background(), []mockEntity{
		{
			sourceID:        "1",
			hash:            "hash-1",
			title:           "SQL Injection",
			titleVerbose:    "SQL Injection Verbose",
			severity:        "high",
			riskType:        "sql_injection",
			description:     "desc-1",
			solution:        "sol-1",
			programName:     "demo-project",
			codeSourceURL:   "file://demo/main.go",
			codeRange:       "10:1-10:8",
			codeFragment:    "danger()",
			functionName:    "main",
			line:            10,
			fromRule:        "rule-a",
			cwe:             []string{"CWE-89"},
			tags:            []string{"owasp:a03", "backend"},
			disposalStatus:  "not_set",
			language:        "go",
			taskID:          "task-1",
			riskFeatureHash: "rfh-1",
			createdAt:       &now,
			updatedAt:       &later,
		},
		{
			sourceID:        "2",
			hash:            "hash-2",
			title:           "Command Injection",
			severity:        "warning",
			riskType:        "command_injection",
			description:     "desc-2",
			solution:        "sol-2",
			programName:     "demo-project",
			codeSourceURL:   "file://demo/exec.go",
			codeRange:       "20:1-20:8",
			codeFragment:    "exec()",
			functionName:    "run",
			line:            20,
			fromRule:        "rule-b",
			cwe:             []string{"CWE-77", "CWE-78"},
			tags:            []string{"shell"},
			disposalStatus:  "fixed",
			language:        "go",
			taskID:          "task-2",
			riskFeatureHash: "rfh-2",
			createdAt:       &now,
			updatedAt:       &later,
			isPotential:     true,
		},
	}, &BuildSnapshotMeta{
		SourceKind:  "distributed_ssa",
		TemplateID:  "default-v1",
		ReportName:  "Demo 报告",
		GeneratedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, DefaultSchemaVersion, snapshot.SchemaVersion)
	require.Equal(t, "distributed_ssa", snapshot.SourceKind)
	require.Equal(t, "default-v1", snapshot.TemplateID)
	require.Equal(t, "Demo 报告", snapshot.Meta.ReportName)
	require.Equal(t, "demo-project", snapshot.Meta.ProjectName)
	require.Equal(t, int64(2), snapshot.Meta.TaskCount)
	require.Equal(t, 2, snapshot.Summary.Total)
	require.Equal(t, 1, snapshot.Summary.High)
	require.Equal(t, 1, snapshot.Summary.Medium)
	require.Equal(t, 1, snapshot.Summary.PotentialCount)
	require.Len(t, snapshot.Findings, 2)
	require.Equal(t, "SQL Injection Verbose", snapshot.Findings[0].DisplayTitle)
	require.Equal(t, "高危", snapshot.Findings[0].SeverityLabel)
	require.Equal(t, "中危", snapshot.Findings[1].SeverityLabel)
	require.ElementsMatch(t, []VulnerabilityMetric{
		{Label: "command_injection", Value: 1},
		{Label: "sql_injection", Value: 1},
	}, snapshot.Summary.RiskTypes)
	require.Equal(t, "go", snapshot.Summary.Languages[0].Label)
	require.Equal(t, 1, snapshot.Summary.CWEs[0].Value)
}
