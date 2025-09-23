package sfreport

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type ReportType string

const (
	SarifReportType ReportType = "sarif"

	// echo file will only show the first 100 characters in the report
	IRifyReportType ReportType = "irify"

	// echo file will show the full content in the report
	IRifyFullReportType ReportType = "irify-full"

	IRifyReactReportType ReportType = "irify-react-report"
)

var (
	log = ssalog.Log
)

func ReportTypeFromString(s string) ReportType {
	switch s {
	case "sarif":
		return SarifReportType
	case "irify":
		return IRifyReportType
	case "irify-full":
		return IRifyFullReportType
	case "irify-react-report":
		return IRifyReactReportType
	default:
		log.Warnf("unsupported report type: %s, use sarif as default, you can set [sarif, irify, irify-full] to set report type", s)
		return SarifReportType
	}
}

func ToReportSeverityLevel(level schema.SyntaxFlowSeverity) string {
	switch level {
	case schema.SFR_SEVERITY_INFO:
		return "note"
	case schema.SFR_SEVERITY_LOW, schema.SFR_SEVERITY_WARNING:
		return "warning"
	case schema.SFR_SEVERITY_CRITICAL, schema.SFR_SEVERITY_HIGH:
		return "error"
	default:
		return "note"
	}
}

func GetValueByRisk(ssarisk *schema.SSARisk) (*ssaapi.Value, error) {
	// get result
	result, err := ssaapi.LoadResultByID(uint(ssarisk.ResultID))
	if err != nil {
		log.Errorf("load result by id %d error: %v", ssarisk.ResultID, err)
		return nil, err
	}

	// get value
	value, err := result.GetValue(ssarisk.Variable, int(ssarisk.Index))
	if err != nil {
		log.Errorf("get value by variable %s and index %d error: %v", ssarisk.Variable, ssarisk.Index, err)
		return nil, err
	}

	return value, nil
}

// GenerateDataFlowAnalysis generates comprehensive data flow analysis for a risk
func GenerateDataFlowAnalysis(risk *schema.SSARisk, values ...*ssaapi.Value) (*DataFlowPath, error) {
	if risk.ResultID == 0 || risk.Variable == "" {
		return nil, utils.Errorf("risk has no valid result ID or variable")
	}

	var value *ssaapi.Value
	if len(values) > 0 {
		value = values[0]
	}

	if utils.IsNil(value) {
		var err error
		value, err = GetValueByRisk(risk)
		if err != nil {
			return nil, utils.Errorf("get value by risk failed: %v", err)
		}
	}

	dotGraph := ssaapi.NewDotGraph()
	value.GenerateGraph(dotGraph)
	nodes, edges := coverNodeAndEdgeInfos(dotGraph, risk.ProgramName, risk)

	path := &DataFlowPath{
		PathID:      fmt.Sprintf("path_%d", risk.ID),
		Description: generatePathDescription(risk),
		Nodes:       nodes,
		Edges:       edges,
		DotGraph:    dotGraph.String(),
	}

	return path, nil
}

// generatePathDescription generates a description for the data flow path
func generatePathDescription(risk *schema.SSARisk) string {
	return fmt.Sprintf("Data flow path for %s vulnerability in %s", risk.RiskType, risk.ProgramName)
}

// coverNodeAndEdgeInfos converts graph nodes and edges to NodeInfo and EdgeInfo
func coverNodeAndEdgeInfos(graph *ssaapi.DotGraph, programName string, risk *schema.SSARisk) ([]*NodeInfo, []*EdgeInfo) {
	nodes := make([]*NodeInfo, 0, graph.NodeCount())
	edges := make([]*EdgeInfo, 0)

	graph.ForEach(func(s string, v *ssaapi.Value) {
		codeRange, source := ssaapi.CoverCodeRange(v.GetRange())
		nodeInfo := &NodeInfo{
			NodeID:          s,
			IRCode:          v.String(),
			SourceCode:      source,
			SourceCodeStart: 0,
			CodeRange:       codeRange,
		}
		nodes = append(nodes, nodeInfo)
	})

	edgeCache := make(map[string]struct{})
	for edgeID, edge := range graph.Graph.GetAllEdges() {
		if edge == nil {
			continue
		}

		fromNode := edge.From()
		toNode := edge.To()
		if fromNode == nil || toNode == nil {
			continue
		}

		hash := codec.Sha256(fmt.Sprintf(
			"%d-%d-%s",
			fromNode.ID(),
			toNode.ID(),
			edge.Label,
		))
		if _, ok := edgeCache[hash]; ok {
			continue
		}
		edgeCache[hash] = struct{}{}

		edgeInfo := &EdgeInfo{
			EdgeID:      fmt.Sprintf("e%d", edgeID),
			FromNodeID:  nodeId(fromNode.ID()),
			ToNodeID:    nodeId(toNode.ID()),
			EdgeType:    edge.Label,
			Description: generateEdgeDescription(edge.Label),
		}
		edges = append(edges, edgeInfo)
	}

	return nodes, edges
}

// generateEdgeDescription generates description for edge types
func generateEdgeDescription(edgeLabel string) string {
	switch {
	case strings.Contains(edgeLabel, "depend_on"):
		return "The dependency edge in the dataflow, DependOn indicates which other values the current value depends on. For example, A DependOn B means Value A depends on Value B."
	case strings.Contains(edgeLabel, "effect_on"):
		return "Dependency edge in the dataflow, EffectOn indicates which other values the current value affects. For example, A EffectOn B means Value A affects Value B."
	case strings.Contains(edgeLabel, "call"):
		return "Function call"
	case strings.Contains(edgeLabel, "search-exact"):
		return "Search value from database exactly"
	default:
		return ""
	}
}

// nodeId converts node ID to string format
func nodeId(i int) string {
	return fmt.Sprintf("n%d", i)
}
