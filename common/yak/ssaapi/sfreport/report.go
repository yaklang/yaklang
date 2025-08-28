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

	ProgramName   string    `json:"program_name"`
	ProgramLang   string    `json:"program_lang"`
	Description   string    `json:"description"`
	RepositoryURL string    `json:"repository_url"`
	FileCount     int       `json:"file_count"`
	CodeLineCount int       `json:"code_line_count"`
	ScanStartTime time.Time `json:"scan_start_time"`
	ScanEndTime   time.Time `json:"scan_end_time"`
	// ProgramVersion string `json:"program_version"`
	RiskNums int
	// Program Program
	Rules []*Rule
	Risks map[string]*Risk // hash -> risk
	File  []*File
}

func NewReport(reportType ReportType) *Report {
	now := time.Now()
	return &Report{
		ReportType:    reportType,
		EngineVersion: consts.GetYakVersion(),
		ReportTime:    now,

		// 初始化新增字段的默认值
		ScanStartTime: now,
		ScanEndTime:   now,
		FileCount:     0,
		CodeLineCount: 0,

		Rules: make([]*Rule, 0),
		Risks: make(map[string]*Risk),
		File:  make([]*File, 0),
	}
}

func (r *Report) SetProgramName(programName string) {
	r.ProgramName = programName
}

func (r *Report) SetProgramLang(lang string) {
	r.ProgramLang = lang
}

func (r *Report) SetDescription(description string) {
	r.Description = description
}

func (r *Report) SetRepositoryURL(url string) {
	r.RepositoryURL = url
}

func (r *Report) SetFileCount(count int) {
	r.FileCount = count
}

func (r *Report) SetCodeLineCount(count int) {
	r.CodeLineCount = count
}

func (r *Report) SetScanStartTime(startTime time.Time) {
	r.ScanStartTime = startTime
}

func (r *Report) SetScanEndTime(endTime time.Time) {
	r.ScanEndTime = endTime
}

func (r *Report) SetScanTimes(startTime, endTime time.Time) {
	r.ScanStartTime = startTime
	r.ScanEndTime = endTime
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

// ToSSAProjectReport 将 Report 转换为 SSAProjectReport
func (r *Report) ToSSAProjectReport() *SSAProjectReport {
	ssaReport := &SSAProjectReport{
		// 封面信息
		ProgramName: r.ProgramName,
		ReportTime:  r.ReportTime,

		// 项目信息
		Language:      r.ProgramLang,
		Description:   r.Description,
		RepositoryURL: r.RepositoryURL,
		FileCount:     r.FileCount,
		CodeLineCount: r.CodeLineCount,
		ScanStartTime: r.ScanStartTime,
		ScanEndTime:   r.ScanEndTime,
		TotalRules:    len(r.Rules),
		EngineVersion: r.EngineVersion,

		// 初始化集合
		Risks: make([]*SSAReportRisk, 0, len(r.Risks)),
		Files: make([]*SSAReportFile, 0, len(r.File)),
		Rules: make(map[string]*SSAReportRule),
	}

	// 创建风险哈希到风险对象的映射，便于后续查找
	riskHashMap := make(map[string]*Risk)
	for hash, risk := range r.Risks {
		riskHashMap[hash] = risk
	}

	// 转换风险数据并统计
	ssaReport.convertRisks(r.Risks)

	// 转换文件数据，传递风险映射以便计算统计
	ssaReport.convertFiles(r.File, riskHashMap)

	// 转换规则数据
	ssaReport.convertRules(r.Rules)

	return ssaReport
}

// convertRisks 转换风险数据并进行统计
func (ssr *SSAProjectReport) convertRisks(risks map[string]*Risk) {
	ssr.TotalRisksCount = len(risks)

	// 统计各等级风险数量
	for _, risk := range risks {
		ssaRisk := &SSAReportRisk{
			Title:         risk.GetTitle(),
			TitleVerbose:  risk.GetTitleVerbose(),
			Description:   risk.GetDescription(),
			Solution:      risk.GetSolution(),
			RiskType:      risk.GetRiskType(),
			Severity:      risk.GetSeverity(),
			FromRule:      risk.GetRuleName(),
			CodeSourceUrl: risk.GetCodeSourceURL(),
			CodeRange:     risk.GetCodeRange(),
			Line:          risk.GetLine(),
			CodeFragment:  risk.GetCodeFragment(),
			FunctionName:  risk.GetFunctionName(),
		}
		ssr.Risks = append(ssr.Risks, ssaRisk)

		// 统计各等级风险数量
		switch risk.GetSeverity() {
		case severityCritical:
			ssr.CriticalRisksCount++
		case severityHigh:
			ssr.HighRisksCount++
		case severityMiddle:
			ssr.MiddleRisksCount++
		case severityLow:
			ssr.LowRisksCount++
		}
	}
}

// convertFiles 转换文件数据
func (ssr *SSAProjectReport) convertFiles(files []*File, riskHashMap map[string]*Risk) {
	for _, file := range files {
		ssaFile := &SSAReportFile{
			FilePath:  file.Path,
			Language:  ssr.Language, // 使用报告的语言信息
			LineCount: 0,            // 原结构体中没有行数信息
			RiskCount: len(file.Risks),
			// 风险等级统计需要根据风险哈希来计算
		}

		// 根据文件关联的风险计算各等级数量
		ssr.calculateFileRiskCounts(ssaFile, file.Risks, riskHashMap)
		ssr.Files = append(ssr.Files, ssaFile)
	}
}

// convertRules 转换规则数据
func (ssr *SSAProjectReport) convertRules(rules []*Rule) {
	for _, rule := range rules {
		ssaRule := &SSAReportRule{
			RuleName:    rule.RuleName,
			Title:       "", // 原结构体中没有对应字段
			TitleZh:     "", // 原结构体中没有对应字段
			Severity:    "", // 原结构体中没有对应字段
			Description: rule.Description,
			RiskCount:   len(rule.Risks),
		}
		ssr.Rules[rule.RuleName] = ssaRule
	}
}

// calculateFileRiskCounts 计算文件中各等级风险的数量
func (ssr *SSAProjectReport) calculateFileRiskCounts(ssaFile *SSAReportFile, riskHashes []string, riskHashMap map[string]*Risk) {
	// 根据风险哈希找到对应的风险并统计各等级数量
	for _, hash := range riskHashes {
		if risk, exists := riskHashMap[hash]; exists {
			switch risk.GetSeverity() {
			case severityCritical:
				ssaFile.CriticalCount++
			case severityHigh:
				ssaFile.HighCount++
			case severityMiddle:
				ssaFile.MiddleCount++
			case severityLow:
				ssaFile.LowCount++
			}
		}
	}
}
