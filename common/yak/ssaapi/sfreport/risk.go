package sfreport

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type Risk struct {
	// index
	ID   uint   `json:"id"`
	Hash string `json:"hash"`

	// info
	Title        string             `json:"title"`
	TitleVerbose string             `json:"title_verbose"`
	Description  string             `json:"description"`
	Solution     string             `json:"solution"`
	Severity     string             `json:"severity"`
	RiskType     string             `json:"risk_type"`
	Details      string             `json:"details"`
	CVE          string             `json:"cve"`
	CWE          schema.StringArray `json:"cwe"`
	Time         time.Time          `json:"time"`
	Language     string             `json:"language"`
	// code info
	CodeSourceURL string `json:"code_source_url"`
	Line          int64  `json:"line"`
	// for select code range
	CodeRange    string `json:"code_range"`
	CodeFragment string `json:"code_fragment"`
	FunctionName string `json:"function_name"`

	// rule
	RuleName string `json:"rule_name"`
	// program
	ProgramName string `json:"program_name"`
	// 处置状态
	LatestDisposalStatus string `json:"latest_disposal_status"`
	// 数据流路径信息
	DataFlowPaths []*DataFlowPath `json:"data_flow_paths,omitempty"`
	// RiskFeatureHash
	RiskFeatureHash string `json:"risk_feature_hash"`
}

func NewRisk(ssarisk *schema.SSARisk, r *Report, value ...*ssaapi.Value) *Risk {
	if ssarisk == nil {
		return &Risk{}
	}

	risk := &Risk{
		ID:   ssarisk.ID,
		Hash: ssarisk.Hash,
		Time: ssarisk.CreatedAt,

		Title:        ssarisk.Title,
		TitleVerbose: ssarisk.TitleVerbose,
		Description:  ssarisk.Description,
		Solution:     ssarisk.Solution,
		Severity:     string(ssarisk.Severity),
		RiskType:     ssarisk.RiskType,
		Details:      ssarisk.Details,
		CVE:          ssarisk.CVE,
		CWE:          ssarisk.CWE,
		Language:     ssarisk.Language,

		CodeRange:     ssarisk.CodeRange,
		CodeFragment:  ssarisk.CodeFragment,
		CodeSourceURL: ssarisk.CodeSourceUrl,
		FunctionName:  ssarisk.FunctionName,
		Line:          ssarisk.Line,

		ProgramName:          ssarisk.ProgramName,
		LatestDisposalStatus: ssarisk.LatestDisposalStatus,
		RiskFeatureHash:      ssarisk.RiskFeatureHash,
	}

	// Generate data flow paths if available
	if r.ReportType == IRifyFullReportType || r.config.showDataflowPath {
		if ssarisk.ResultID != 0 && ssarisk.Variable != "" {
			dataFlowPath, err := GenerateDataFlowAnalysis(ssarisk, value...)
			if err != nil {
				log.Errorf("generate data flow paths failed for risk %d: %v", ssarisk.ID, err)
			} else {
				risk.DataFlowPaths = []*DataFlowPath{dataFlowPath}
			}
		}
	}

	return risk
}

func (r *Risk) SaveToDB(db *gorm.DB) error {
	if db == nil {
		return utils.Error("Save Risk to DB failed: db is nil")
	}

	ssaRisk := &schema.SSARisk{
		Hash:                r.Hash,
		Title:               r.Title,
		TitleVerbose:        r.TitleVerbose,
		Description:         r.Description,
		Solution:            r.Solution,
		RiskType:            r.RiskType,
		Details:             r.Details,
		Severity:            schema.SyntaxFlowSeverity(r.Severity),
		Language:            r.Language,
		IsPotential:         false,
		CVE:                 r.CVE,
		CWE:                 r.CWE,
		IsRead:              false,
		Ignore:              false,
		UploadOnline:        false,
		CveAccessVector:     "",
		CveAccessComplexity: "",
		Tags:                "",
		FromRule:            r.RuleName,
		ProgramName:         r.ProgramName,
		CodeSourceUrl:       r.CodeSourceURL,
		CodeRange:           r.CodeRange,
		CodeFragment:        r.CodeFragment,
		FunctionName:        r.FunctionName,
		Line:                r.Line,
		// RuntimeId:            "",
		// ResultID:             uint64(r.ResultID),
		// Variable:             r.Variable,
		// Index:                int64(r.Index),
		LatestDisposalStatus: r.LatestDisposalStatus,
		RiskFeatureHash:      r.RiskFeatureHash,
	}
	err := yakit.CreateSSARisk(db, ssaRisk)
	if err != nil {
		return utils.Wrapf(err, "Save Risk to DB failed")
	}

	// save data flow paths
	saver := newSaveDataFlowCtx(db, r.Hash)
	for _, dataFlowPath := range r.DataFlowPaths {
		saver.SaveDataFlow(dataFlowPath)
	}
	return nil
}

func (r *Risk) GetHash() string {
	return r.Hash
}

func (r *Risk) GetTitle() string {
	return r.Title
}

func (r *Risk) GetTitleVerbose() string {
	return r.TitleVerbose
}

func (r *Risk) GetDescription() string {
	return r.Description
}

func (r *Risk) GetSolution() string {
	return r.Solution
}

func (r *Risk) GetSeverity() string {
	return r.Severity
}

func (r *Risk) GetRiskType() string {
	return r.RiskType
}

func (r *Risk) GetDetails() string {
	return r.Details
}

func (r *Risk) GetCVE() string {
	return r.CVE
}

func (r *Risk) GetCodeSourceURL() string {
	return r.CodeSourceURL
}

func (r *Risk) GetLine() int64 {
	return r.Line
}

func (r *Risk) GetCodeRange() string {
	return r.CodeRange
}

func (r *Risk) GetCodeFragment() string {
	return r.CodeFragment
}

func (r *Risk) GetFunctionName() string {
	return r.FunctionName
}

func (r *Risk) GetCWE() []string {
	return r.CWE
}

func (r *Risk) GetProgramName() string {
	return r.ProgramName
}

func (r *Risk) GetLanguage() string {
	return r.Language
}

func (r *Risk) GetRuleName() string {
	return r.RuleName
}

func (r *Risk) SetRule(rule *Rule) {
	r.RuleName = rule.RuleName
}

func (r *Risk) SetFile(file *File) {
	// 不覆盖 CodeSourceURL，因为它已经从 SSARisk 中正确设置
	// r.CodeSourceURL = file.Path
}
