package ssaexport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
	PathID      string      `json:"path_id"`
	Description string      `json:"description"`
	Nodes       []*NodeInfo `json:"nodes"`
	Edges       []*EdgeInfo `json:"edges"`
	DotGraph    string      `json:"dot_graph,omitempty"` // 保留作为可选的可视化格式
}

type NodeInfo struct {
	NodeID          string            `json:"node_id"`
	IRCode          string            `json:"ir_code"`
	SourceCode      string            `json:"source_code"`
	SourceCodeStart int               `json:"source_code_start"`
	CodeRange       *ssaapi.CodeRange `json:"code_range"`
	NodeType        string            `json:"node_type"`   // 节点类型：source, sink, transform, etc.
	RiskLevel       string            `json:"risk_level"`  // 风险等级：high, medium, low, safe
	Description     string            `json:"description"` // 节点描述，便于AI理解
}

type EdgeInfo struct {
	EdgeID      string `json:"edge_id"`
	FromNodeID  string `json:"from_node_id"`
	ToNodeID    string `json:"to_node_id"`
	EdgeType    string `json:"edge_type"`   // 边类型：data_flow, control_flow, call, etc.
	Description string `json:"description"` // 边描述，便于AI理解
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

	// 生成数据流路径信息
	dataFlowPaths, err := getDataFlowPathsForRisk(risk)
	if err != nil {
		log.Errorf("get data flow paths failed for risk %d: %v", risk.ID, err)
	} else {
		exportItem.DataFlowPaths = dataFlowPaths
	}

	return exportItem, nil
}

func getIRProgramByRisk(risk *schema.SSARisk) (*ssadb.IrProgram, error) {
	program, err := ssadb.GetApplicationProgram(risk.ProgramName)
	if err != nil {
		return nil, utils.Errorf("get application program %s failed: %v", risk.ProgramName, err)
	}
	return program, nil
}

func getDataFlowPathsForRisk(risk *schema.SSARisk) ([]*DataFlowPath, error) {
	path := &DataFlowPath{
		PathID:      fmt.Sprintf("path_%d", risk.ID),
		Description: "", // Placeholder, will be filled later
		Nodes:       []*NodeInfo{},
		Edges:       []*EdgeInfo{},
		DotGraph:    "", // Placeholder, will be filled later
	}

	// 尝试从风险信息中生成ValueGraph
	if risk.CodeFragment != "" {
		nodes, edges, dotGraph, err := generateGraphInfoFromRisk(risk)
		if err != nil {
			log.Errorf("generate graph info failed for risk %d: %v", risk.ID, err)
		} else {
			path.Nodes = nodes
			path.Edges = edges
			path.DotGraph = dotGraph
		}
	}

	return []*DataFlowPath{path}, nil
}

func generateGraphInfoFromRisk(risk *schema.SSARisk) ([]*NodeInfo, []*EdgeInfo, string, error) {
	if risk.ResultID == 0 || risk.Variable == "" {
		return []*NodeInfo{}, []*EdgeInfo{}, "", nil
	}

	result, err := ssaapi.LoadResultByID(uint(risk.ResultID))
	if err != nil {
		log.Errorf("load result by id %d failed: %v", risk.ResultID, err)
		return []*NodeInfo{}, []*EdgeInfo{}, "", err
	}

	value, err := result.GetValue(risk.Variable, int(risk.Index))
	if err != nil || value == nil {
		log.Errorf("get value failed for variable %s, index %d: %v", risk.Variable, risk.Index, err)
		return []*NodeInfo{}, []*EdgeInfo{}, "", err
	}

	vg := ssaapi.NewValueGraph(value)
	nodes, edges := coverNodeAndEdgeInfos(vg, risk.ProgramName, risk)
	var buf bytes.Buffer
	vg.GenerateDOT(&buf)
	dotGraph := buf.String()
	return nodes, edges, dotGraph, nil
}

func coverNodeAndEdgeInfos(graph *ssaapi.ValueGraph, programName string, risk *schema.SSARisk) ([]*NodeInfo, []*EdgeInfo) {
	nodes := make([]*NodeInfo, 0, len(graph.Node2Value))
	edges := make([]*EdgeInfo, 0)

	nodeMap := make(map[int]*NodeInfo)
	for id, node := range graph.Node2Value {
		codeRange, source := ssaapi.CoverCodeRange(programName, node.GetRange())
		nodeInfo := &NodeInfo{
			NodeID:          dot.NodeName(id),
			IRCode:          node.String(),
			SourceCode:      source,
			SourceCodeStart: 0,
			CodeRange:       codeRange,
			NodeType:        determineNodeType(node, risk),
			RiskLevel:       determineRiskLevel(node, risk),
			Description:     generateNodeDescription(node, risk),
		}
		nodes = append(nodes, nodeInfo)
		nodeMap[id] = nodeInfo
	}

	edgeID := 1
	for _, edge := range graph.Graph.GetAllEdges() {
		fromNodeLabel := edge.From
		toNodeLabel := edge.To

		var fromNode, toNode *NodeInfo
		for _, node := range nodes {
			if strings.Contains(node.SourceCode, fromNodeLabel) || strings.Contains(node.IRCode, fromNodeLabel) {
				fromNode = node
			}
			if strings.Contains(node.SourceCode, toNodeLabel) || strings.Contains(node.IRCode, toNodeLabel) {
				toNode = node
			}
		}

		if fromNode != nil && toNode != nil {
			edgeInfo := &EdgeInfo{
				EdgeID:      fmt.Sprintf("e%d", edgeID),
				FromNodeID:  fromNode.NodeID,
				ToNodeID:    toNode.NodeID,
				EdgeType:    determineEdgeType(edge.Label),
				Description: generateEdgeDescription(edge.Label),
			}
			edges = append(edges, edgeInfo)
			edgeID++
		}
	}

	return nodes, edges
}

func determineEdgeType(edgeLabel string) string {
	// 直接使用原始的边标签作为边类型
	return edgeLabel
}

func generateEdgeDescription(edgeLabel string) string {
	// 根据边的标签生成描述
	switch {
	case strings.Contains(edgeLabel, "depend_on"):
		return "Data dependency flow"
	case strings.Contains(edgeLabel, "effect_on"):
		return "Data effect flow"
	case strings.Contains(edgeLabel, "call"):
		return "Function call relationship"
	case strings.Contains(edgeLabel, "search-exact"):
		return "Exact search relationship"
	default:
		return "Data flows from source to destination"
	}
}

func determineNodeType(node *ssaapi.Value, risk *schema.SSARisk) string {
	// 根据节点类型和风险类型判断节点类型
	irCode := node.String()
	if strings.Contains(irCode, "Parameter-") {
		return "source"
	}
	if strings.Contains(irCode, "Files.copy") || strings.Contains(irCode, "File.") {
		return "sink"
	}
	return "transform"
}

// todo:need more check
func determineRiskLevel(node *ssaapi.Value, risk *schema.SSARisk) string {
	nodeType := determineNodeType(node, risk)
	if nodeType == "source" {
		return "high"
	}
	if nodeType == "sink" {
		return "high"
	}
	return "medium"
}

// todo:need more desc
func generateNodeDescription(node *ssaapi.Value, risk *schema.SSARisk) string {
	nodeType := determineNodeType(node, risk)
	irCode := node.String()

	switch nodeType {
	case "source":
		return fmt.Sprintf("User input source: %s", irCode)
	case "sink":
		return fmt.Sprintf("Potential vulnerability sink: %s", irCode)
	case "transform":
		return fmt.Sprintf("Data transformation: %s", irCode)
	default:
		return fmt.Sprintf("Data processing node: %s", irCode)
	}
}
