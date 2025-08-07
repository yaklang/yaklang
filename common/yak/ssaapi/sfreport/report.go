package sfreport

import (
	"encoding/json"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

type Report struct {
	// info
	ReportType    ReportType `json:"report_type"`
	EngineVersion string     `json:"engine_version"`
	ReportTime    time.Time  `json:"report_time"`

	ProgramName string `json:"program_name"`
	// ProgramVersion string `json:"program_version"`
	RiskNums int
	// Program Program
	Rules []*Rule
	Risks map[string]*Risk // hash -> risk
	File  []*File
}

func NewReport(reportType ReportType) *Report {
	return &Report{
		ReportType:    reportType,
		EngineVersion: consts.GetYakVersion(),
		ReportTime:    time.Now(),

		Rules: make([]*Rule, 0),
		Risks: make(map[string]*Risk),
		File:  make([]*File, 0),
	}
}

func (r *Report) SetProgramName(programName string) {
	r.ProgramName = programName
}

func (r *Report) AddRules(rule ...*Rule) {
	r.Rules = append(r.Rules, rule...)
}

func (r *Report) GetRule(ruleName string) *Rule {
	for _, rule := range r.Rules {
		if rule.RuleName == ruleName {
			return rule
		}
	}
	return nil
}

func (r *Report) AddRisks(risk ...*Risk) {
	if r.Risks == nil {
		r.Risks = make(map[string]*Risk)
	}
	for _, risk := range risk {
		// set program from risk if not set in report
		if r.ProgramName == "" && risk.GetProgramName() != "" {
			r.ProgramName = risk.GetProgramName()
		}

		r.Risks[risk.GetHash()] = risk
	}
}

func (r *Report) GetRisk(hash string) *Risk {
	return r.Risks[hash]
}

func (r *Report) AddFile(file *File) {
	r.File = append(r.File, file)
}

func (r *Report) GetFile(path string) *File {
	for _, file := range r.File {
		if file.Path == path {
			return file
		}
	}
	return nil
}

func (r *Report) Write(w io.Writer) error {
	jsonData, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = w.Write(jsonData)
	return err
}
