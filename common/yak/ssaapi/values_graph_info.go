package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils/dot"
)

// NodeInfo represents information about a node in the value graph
type NodeInfo struct {
	NodeID          string     `json:"node_id"`
	IRCode          string     `json:"ir_code"`
	SourceCode      string     `json:"source_code"`
	SourceCodeStart int        `json:"source_code_start"`
	CodeRange       *CodeRange `json:"code_range"`
}

func (info *NodeInfo) Hash() string {
	if info == nil {
		return ""
	}
	return codec.Md5(info.IRCode + info.CodeRange.JsonString())
}

// ValueGraphInfo encapsulates all the graph information including paths, dot graph, and node info
type ValueGraphInfo struct {
	GraphPaths [][]string
	DotGraph   string

	NodeInfos []*NodeInfo
	NodeID    string
}

// GetNodeInfos generates NodeInfo for all nodes in the ValueGraph
func (g *ValueGraph) GetNodeInfos() []*NodeInfo {
	res := make([]*NodeInfo, 0, len(g.Node2Value))
	for id, node := range g.Node2Value {
		codeRange, source := CoverCodeRange(node.GetRange())
		ret := &NodeInfo{
			NodeID:     dot.NodeName(id),
			IRCode:     node.String(),
			SourceCode: source,
			CodeRange:  codeRange,
		}
		res = append(res, ret)
	}
	return res
}

// GenerateValueGraphInfo creates comprehensive graph information for a value
func (g *ValueGraph) GenerateValueGraphInfo(value *Value) (*ValueGraphInfo, error) {
	nodeID, exists := g.Value2Node[value]
	if !exists {
		return nil, fmt.Errorf("value not found in graph")
	}

	// Generate graph paths
	graphPaths := g.DeepFirstGraphPrev(value)

	// Generate DOT graph
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	dotGraph := buf.String()

	// Generate node infos
	nodeInfos := g.GetNodeInfos()
	return &ValueGraphInfo{
		GraphPaths: graphPaths,
		DotGraph:   dotGraph,
		NodeInfos:  nodeInfos,
		NodeID:     dot.NodeName(nodeID),
	}, nil
}

func (g *ValueGraph) GenerateValuesGraphInfo(values Values) (map[string]*ValueGraphInfo, error) {
	result := make(map[string]*ValueGraphInfo)
	for _, value := range values {
		info, err := g.GenerateValueGraphInfo(value)
		if err != nil {
			return nil, err
		}
		result[value.GetUUID()] = info
	}
	return result, nil
}
