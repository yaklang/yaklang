package sfreport

import (
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

type Risk struct {
	// index
	ID   uint   `json:"id"`
	Hash string `json:"hash"`

	// info
	Title        string    `json:"title"`
	TitleVerbose string    `json:"title_verbose"`
	Description  string    `json:"description"`
	Solution     string    `json:"solution"`
	Severity     string    `json:"severity"`
	RiskType     string    `json:"risk_type"`
	Details      string    `json:"details"`
	CVE          string    `json:"cve"`
	Time         time.Time `json:"time"`

	// code info
	CodeSourceURL string `json:"code_source_url"`
	Line          int64  `json:"line"`
	// for select code range
	CodeRange string `json:"code_range"`

	// rule
	RuleName string `json:"rule_name"`
	// program
	ProgramName string `json:"program_name"`

	//TODO: for analyze step
	// AnalyzeSteps []AnalyzeStep `json:"analyze_steps"`
}

func NewRisk(risk *schema.SSARisk) *Risk {
	if risk == nil {
		return &Risk{}
	}

	ret := &Risk{
		ID:   risk.ID,
		Hash: risk.Hash,
		Time: risk.CreatedAt,

		Title:        risk.Title,
		TitleVerbose: risk.TitleVerbose,
		Description:  risk.Description,
		Solution:     risk.Solution,
		Severity:     string(risk.Severity),
		RiskType:     risk.RiskType,
		Details:      risk.Details,
		CVE:          risk.CVE,

		CodeRange: risk.CodeRange,
		Line:      risk.Line,

		ProgramName: risk.ProgramName,
	}

	return ret
}

func (r *Risk) SetRule(rule *Rule) {
	r.RuleName = rule.RuleName
}

func (r *Risk) SetFile(file *File) {
	r.CodeSourceURL = file.Path
}
