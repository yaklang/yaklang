package vulnreport

import (
	"context"
	"time"
)

const DefaultSchemaVersion = "vuln-report/v1"

type VulnerabilityEntity interface {
	GetSourceID() string
	GetHash() string
	GetTitle() string
	GetTitleVerbose() string
	GetSeverity() string
	GetRiskType() string
	GetDescription() string
	GetSolution() string
	GetProgramName() string
	GetCodeSourceURL() string
	GetCodeRange() string
	GetCodeFragment() string
	GetFunctionName() string
	GetLine() int64
	GetFromRule() string
	GetCWEList() []string
	GetTags() []string
	GetLatestDisposalStatus() string
	GetLanguage() string
	GetTaskID() string
	GetRuntimeID() string
	GetRiskFeatureHash() string
	GetCreatedAt() *time.Time
	GetUpdatedAt() *time.Time
	IsPotentialRisk() bool
}

type BuildSnapshotMeta struct {
	SchemaVersion    string
	SourceKind       string
	TemplateID       string
	ReportName       string
	ScopeType        string
	ScopeName        string
	ProjectName      string
	ProgramName      string
	TaskID           string
	TaskCount        int64
	ScanBatch        int64
	GeneratedAt      time.Time
	SourceFinishedAt *time.Time
	Owner            string
	Filters          []string
}

type VulnerabilityReportSnapshot struct {
	SchemaVersion string
	SourceKind    string
	TemplateID    string
	Meta          VulnerabilityReportMeta
	Summary       VulnerabilityReportSummary
	Findings      []VulnerabilityFinding
}

type VulnerabilityReportMeta struct {
	ReportName       string
	ScopeType        string
	ScopeName        string
	ProjectName      string
	ProgramName      string
	TaskID           string
	TaskCount        int64
	ScanBatch        int64
	GeneratedAt      time.Time
	SourceFinishedAt *time.Time
	Owner            string
	Filters          []string
}

type VulnerabilityReportSummary struct {
	Total            int
	Critical         int
	High             int
	Medium           int
	Low              int
	PotentialCount   int
	RiskTypes        []VulnerabilityMetric
	Rules            []VulnerabilityMetric
	Languages        []VulnerabilityMetric
	CWEs             []VulnerabilityMetric
	DisposalStatuses []VulnerabilityMetric
}

type VulnerabilityMetric struct {
	Label string
	Value int
}

type VulnerabilityFinding struct {
	SourceKind           string
	SourceID             string
	Hash                 string
	RiskFeatureHash      string
	Title                string
	TitleVerbose         string
	DisplayTitle         string
	Severity             string
	SeverityLabel        string
	RiskType             string
	Description          string
	Solution             string
	ProgramName          string
	TaskID               string
	RuntimeID            string
	Language             string
	FromRule             string
	CWE                  []string
	Tags                 []string
	CodeSourceURL        string
	CodeRange            string
	CodeFragment         string
	FunctionName         string
	Line                 int64
	LatestDisposalStatus string
	CreatedAt            *time.Time
	UpdatedAt            *time.Time
	IsPotential          bool
}

type ReportTemplate struct {
	ID           string
	Version      string
	DisplayName  string
	Formats      []string
	Capabilities []string
	Metadata     map[string]string
}

type ReportTemplateMeta struct {
	ID           string
	Version      string
	DisplayName  string
	Formats      []string
	Capabilities []string
}

type ReportTemplateProvider interface {
	Get(ctx context.Context, templateID string) (*ReportTemplate, error)
	List(ctx context.Context) ([]*ReportTemplateMeta, error)
}

type ReportRenderer interface {
	Format() string
	Render(ctx context.Context, snapshot *VulnerabilityReportSnapshot, tpl *ReportTemplate) ([]byte, *RenderedMeta, error)
}

type RenderedMeta struct {
	FileName    string
	ContentType string
}
