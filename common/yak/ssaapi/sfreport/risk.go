package sfreport

import (
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
	Language     string    `json:"language"`
	// code info
	CodeSourceURL string `json:"code_source_url"`
	Line          int64  `json:"line"`
	// for select code range
	CodeRange string `json:"code_range"`

	// rule
	RuleName string `json:"rule_name"`
	// program
	ProgramName string `json:"program_name"`
	// 处置状态
	LatestDisposalStatus string `json:"latest_disposal_status"`
	// 数据流路径信息
	DataFlowPaths []*DataFlowPath `json:"data_flow_paths,omitempty"`
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
		Language:     risk.Language,

		CodeRange: risk.CodeRange,
		Line:      risk.Line,

		ProgramName:          risk.ProgramName,
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
	r.CodeSourceURL = file.Path
}
