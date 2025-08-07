package sfreport

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type Risk struct {
	// 项目信息
	ProjectInfo ProjectInfo `json:"project_info"`
	// 风险基本信息
	BaseInfo BaseInfo `json:"base_info"`
	// 风险详情
	DetailInfo DetailInfo `json:"detail_info"`
	// CVE信息
	CVEInfo CVEInfo `json:"cve_info"`
	// 审计规则信息
	AuditRuleInfo AuditRuleInfo `json:"audit_rule"`
	// 风险触发点代码范围
	RiskTriggerCodeRange RiskTriggerCodeRange `json:"risk_trigger_code_range"`
	// 处置状态
	LatestDisposalStatus string `json:"latest_disposal_status"`
	// 数据流路径信息
	DataFlowPaths []*DataFlowPath `json:"data_flow_paths,omitempty"`
}

// ProjectInfo 项目信息
type ProjectInfo struct {
	ProgramName string `json:"program_name"`
	Language    string `json:"language"`
}

// BaseInfo 风险基本信息
type BaseInfo struct {
	ID        uint      `json:"id"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DetailInfo 风险详情
type DetailInfo struct {
	Title        string `json:"title"`
	TitleVerbose string `json:"title_verbose"`
	Description  string `json:"description"`
	Solution     string `json:"solution"`
	RiskType     string `json:"risk_type"`
	Details      string `json:"details"`
	Severity     string `json:"severity"`
	Tags         string `json:"tags"`
}

// CVEInfo CVE信息
type CVEInfo struct {
	CVE                 string `json:"cve"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
}

// AuditRuleInfo 审计规则信息
type AuditRuleInfo struct {
	RuleName string `json:"rule_name"`
}

// RiskTriggerCodeRange 风险触发点代码范围
type RiskTriggerCodeRange struct {
	CodeSourceUrl string `json:"code_source_url"`
	CodeRange     string `json:"code_range"`
	CodeFragment  string `json:"code_fragment"`
	FunctionName  string `json:"function_name"`
	Line          int64  `json:"line"`
}

// DataFlowPath 数据流路径
type DataFlowPath struct {
	PathID      string      `json:"path_id"`
	Description string      `json:"description"`
	Nodes       []*NodeInfo `json:"nodes"`
	Edges       []*EdgeInfo `json:"edges"`
	DotGraph    string      `json:"dot_graph,omitempty"`
}

type NodeInfo struct {
	NodeID          string            `json:"node_id"`
	IRCode          string            `json:"ir_code"`
	SourceCode      string            `json:"source_code"`
	SourceCodeStart int               `json:"source_code_start"`
	CodeRange       *ssaapi.CodeRange `json:"code_range"`
	//NodeType        string            `json:"node_type"`   // 节点类型 TODO:目前确定source、sink比较难
	//Description     string            `json:"description"` // 节点描述
}

type EdgeInfo struct {
	EdgeID      string `json:"edge_id"`
	FromNodeID  string `json:"from_node_id"`
	ToNodeID    string `json:"to_node_id"`
	EdgeType    string `json:"edge_type"`   // 边类型：data_flow, control_flow, call, etc.
	Description string `json:"description"` // 边描述，便于AI理解
}

func NewRisk(risk *schema.SSARisk) *Risk {
	if risk == nil {
		return &Risk{}
	}

	projectInfo := ProjectInfo{
		ProgramName: risk.ProgramName,
		Language:    getLanguageByRisk(risk),
	}

	// 构建基本信息
	baseInfo := BaseInfo{
		ID:        risk.ID,
		Hash:      risk.Hash,
		CreatedAt: risk.CreatedAt,
		UpdatedAt: risk.UpdatedAt,
	}

	// 构建风险详情
	detailInfo := DetailInfo{
		Title:        risk.Title,
		TitleVerbose: risk.TitleVerbose,
		Description:  risk.Description,
		Solution:     risk.Solution,
		RiskType:     risk.RiskType,
		Details:      risk.Details,
		Severity:     string(risk.Severity),
		Tags:         risk.Tags,
	}

	// 构建CVE信息
	cveInfo := CVEInfo{
		CVE:                 risk.CVE,
		CveAccessVector:     risk.CveAccessVector,
		CveAccessComplexity: risk.CveAccessComplexity,
	}

	// 构建审计规则信息
	auditRuleInfo := AuditRuleInfo{
		RuleName: risk.FromRule,
	}

	// 构建风险触发点代码范围
	riskTriggerInfo := RiskTriggerCodeRange{
		CodeSourceUrl: risk.CodeSourceUrl,
		CodeRange:     risk.CodeRange,
		CodeFragment:  risk.CodeFragment,
		FunctionName:  risk.FunctionName,
		Line:          risk.Line,
	}

	ret := &Risk{
		ProjectInfo:          projectInfo,
		BaseInfo:             baseInfo,
		DetailInfo:           detailInfo,
		CVEInfo:              cveInfo,
		AuditRuleInfo:        auditRuleInfo,
		RiskTriggerCodeRange: riskTriggerInfo,
		LatestDisposalStatus: risk.LatestDisposalStatus,
	}

	// Generate data flow paths if available
	if risk.ResultID != 0 && risk.Variable != "" {
		dataFlowPath, err := GenerateDataFlowAnalysis(risk)
		if err != nil {
			log.Errorf("generate data flow paths failed for risk %d: %v", risk.ID, err)
		} else {
			ret.DataFlowPaths = []*DataFlowPath{dataFlowPath}
		}
	}

	return ret
}

func (r *Risk) GetHash() string {
	return r.BaseInfo.Hash
}

func (r *Risk) GetTitle() string {
	return r.DetailInfo.Title
}

func (r *Risk) GetTitleVerbose() string {
	return r.DetailInfo.TitleVerbose
}

func (r *Risk) GetDescription() string {
	return r.DetailInfo.Description
}

func (r *Risk) GetSolution() string {
	return r.DetailInfo.Solution
}

func (r *Risk) GetSeverity() string {
	return r.DetailInfo.Severity
}

func (r *Risk) GetRiskType() string {
	return r.DetailInfo.RiskType
}

func (r *Risk) GetDetails() string {
	return r.DetailInfo.Details
}

func (r *Risk) GetCVE() string {
	return r.CVEInfo.CVE
}

func (r *Risk) GetTime() time.Time {
	return r.BaseInfo.CreatedAt
}

func (r *Risk) GetCodeSourceURL() string {
	return r.RiskTriggerCodeRange.CodeSourceUrl
}

func (r *Risk) GetLine() int64 {
	return r.RiskTriggerCodeRange.Line
}

func (r *Risk) GetCodeRange() string {
	return r.RiskTriggerCodeRange.CodeRange
}

func (r *Risk) GetProgramName() string {
	return r.ProjectInfo.ProgramName
}

func (r *Risk) GetLanguage() string {
	return r.ProjectInfo.Language
}

func (r *Risk) GetRuleName() string {
	return r.AuditRuleInfo.RuleName
}

func (r *Risk) SetRule(rule *Rule) {
	r.AuditRuleInfo.RuleName = rule.RuleName
}

func (r *Risk) SetFile(file *File) {
	r.RiskTriggerCodeRange.CodeSourceUrl = file.Path
}

func getLanguageByRisk(risk *schema.SSARisk) string {
	program, err := ssadb.GetApplicationProgram(risk.ProgramName)
	if err != nil {
		log.Errorf("get language by risk failed:%v", err)
	}
	return program.Language
}
