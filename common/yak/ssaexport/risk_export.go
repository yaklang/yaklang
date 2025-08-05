package ssaexport

import (
	"encoding/json"
	"os"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
)

var log = ssalog.Log

// RiskExportData 风险导出
type RiskExportData struct {
	// 基本信息
	ExportTime time.Time `json:"export_time"`
	TotalRisks int       `json:"total_risks"`

	// 风险列表
	Risks []*RiskExportItem `json:"risks"`
}

// RiskExportItem 单个风险导出项
type RiskExportItem struct {
	// 项目信息
	ProjectInformation ProjectInformation `json:"project_information"`
	// 风险基本信息
	BaseInformation BaseInformation `json:"base_information"`
	// 风险详情
	DetailInformation DetailInformation `json:"detail_information"`
	// CVE信息
	CVEInformation CVEInformation `json:"cve_information"`
	// 审计规则信息
	AuditRuleInformation AuditRuleInformation `json:"audit_rule"`
	// 代码位置信息
	CodeRangeInformation CodeRangeInformation `json:"code_range_information"`
	// 处置状态
	LatestDisposalStatus string `json:"latest_disposal_status"`
	// 数据流路径信息
	DataFlowPaths []*DataFlowPath `json:"data_flow_paths"`
}

type ProjectInformation struct {
	ProgramName string `json:"program_name"`
	Language    string `json:"language"`
}

type BaseInformation struct {
	ID        uint      `json:"id"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DetailInformation struct {
	Title           string `json:"title"`
	TitleVerbose    string `json:"title_verbose"`
	Description     string `json:"description"`
	Solution        string `json:"solution"`
	RiskType        string `json:"risk_type"`
	RiskTypeVerbose string `json:"risk_type_verbose"`
	Details         string `json:"details"`
	Severity        string `json:"severity"`
	IsPotential     bool   `json:"is_potential"`
	Tags            string `json:"tags"`
}

type CVEInformation struct {
	CVE                 string `json:"cve"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
}

type AuditRuleInformation struct {
	RuleName string `json:"rule_name"`
}

type CodeRangeInformation struct {
	CodeSourceUrl string `json:"code_source_url"`
	CodeRange     string `json:"code_range"`
	CodeFragment  string `json:"code_fragment"`
	FunctionName  string `json:"function_name"`
	Line          int64  `json:"line"`
}

// DataFlowPath 数据流路径
type DataFlowPath struct {
	PathID      string          `json:"path_id"`
	Description string          `json:"description"`
	Nodes       []*DataFlowNode `json:"nodes"`
	Edges       []*DataFlowEdge `json:"edges"`
	DotGraph    string          `json:"dot_graph"`
}

// DataFlowNode 数据流节点
type DataFlowNode struct {
	NodeID         uint   `json:"node_id"`
	IsEntryNode    bool   `json:"is_entry_node"`
	IRCodeID       int64  `json:"ir_code_id"`
	TmpValue       string `json:"tmp_value"`
	VerboseName    string `json:"verbose_name"`
	ResultVariable string `json:"result_variable"`
	ResultIndex    uint   `json:"result_index"`
	RiskHash       string `json:"risk_hash"`
}

// DataFlowEdge 数据流边
type DataFlowEdge struct {
	EdgeID        uint   `json:"edge_id"`
	FromNode      uint   `json:"from_node"`
	ToNode        uint   `json:"to_node"`
	EdgeType      string `json:"edge_type"`
	AnalysisStep  int64  `json:"analysis_step"`
	AnalysisLabel string `json:"analysis_label"`
}

// ExportSSARisksToJSON 导出风险到JSON文件
func ExportSSARisksToJSON(risks []*schema.SSARisk, outputPath string) error {
	exportData := &RiskExportData{
		ExportTime: time.Now(),
		TotalRisks: len(risks),
		Risks:      make([]*RiskExportItem, 0, len(risks)),
	}

	for _, risk := range risks {
		exportItem, err := buildRiskExportItem(risk)
		if err != nil {
			log.Errorf("build risk export item failed for risk %d: %v", risk.ID, err)
			continue
		}
		exportData.Risks = append(exportData.Risks, exportItem)
	}

	jsonData, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return utils.Errorf("marshal json failed: %v", err)
	}

	err = os.WriteFile(outputPath, jsonData, 0644)
	if err != nil {
		return utils.Errorf("write file failed: %v", err)
	}
	return nil
}

func buildRiskExportItem(risk *schema.SSARisk) (*RiskExportItem, error) {
	program, err := getIRProgramByRisk(risk)
	if err != nil {
		return nil, err
	}

	exportItem := &RiskExportItem{
		ProjectInformation: ProjectInformation{
			ProgramName: program.ProgramName,
			Language:    program.Language,
		},
		BaseInformation: BaseInformation{
			ID:        risk.ID,
			Hash:      risk.Hash,
			CreatedAt: risk.CreatedAt,
			UpdatedAt: risk.UpdatedAt,
		},
		DetailInformation: DetailInformation{
			Title:        risk.Title,
			TitleVerbose: risk.TitleVerbose,
			Description:  risk.Description,
			Solution:     risk.Solution,
			RiskType:     risk.RiskType,
			Details:      risk.Details,
			Severity:     string(risk.Severity),
			Tags:         risk.Tags,
		},
		CVEInformation: CVEInformation{
			CVE:                 risk.CVE,
			CveAccessVector:     risk.CveAccessVector,
			CveAccessComplexity: risk.CveAccessComplexity,
		},
		AuditRuleInformation: AuditRuleInformation{
			RuleName: risk.FromRule,
		},
		CodeRangeInformation: CodeRangeInformation{
			CodeSourceUrl: risk.CodeSourceUrl,
			CodeRange:     risk.CodeRange,
			CodeFragment:  risk.CodeFragment,
			FunctionName:  risk.FunctionName,
			Line:          risk.Line,
		},
		LatestDisposalStatus: risk.LatestDisposalStatus,
	}
	return exportItem, nil
}

func getDataFlowPathsForRisk(risk *schema.Risk) ([]*DataFlowPath, error) {
	return nil, nil
}

func buildDataFlowPath(startNode *ssadb.AuditNode, pathType string) (*DataFlowPath, error) {
	return nil, nil
}

func generateDotGraphFromAuditNodes(nodes []*DataFlowNode, edges []*DataFlowEdge) string {
	return ""
}

func getIRProgramByRisk(risk *schema.SSARisk) (*ssadb.IrProgram, error) {
	program, err := ssadb.GetApplicationProgram(risk.ProgramName)
	if err != nil {
		return nil, utils.Errorf("get application program %s failed: %v", risk.ProgramName, err)
	}
	return program, nil
}
