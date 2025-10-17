package ssaapi

type GraphInfo struct {
	NodeID    string
	Graph     string
	GraphInfo []*NodeInfo
	GraphPath [][]string
}

type NodeInfo struct {
	NodeID          string     `json:"node_id"`
	IRCode          string     `json:"ir_code"`
	SourceCode      string     `json:"source_code"`
	SourceCodeStart int        `json:"source_code_start"`
	CodeRange       *CodeRange `json:"code_range"`
}

func (v *Value) GetGraphInfo(graphs ...*DotGraph) *GraphInfo {
	var graph *DotGraph
	if len(graphs) > 0 {
		graph = graphs[0]
	} else {
		graph = NewDotGraph()
		v.GenerateGraph(graph)
	}

	ret := &GraphInfo{
		NodeID:    graph.NodeName(v),
		Graph:     graph.String(),
		GraphInfo: make([]*NodeInfo, 0),
		GraphPath: make([][]string, 0),
	}

	// info
	graph.ForEach(func(s string, v *Value) {
		codeRange, source := CoverCodeRange(v.GetRange())
		info := &NodeInfo{
			NodeID:     s,
			IRCode:     v.String(),
			SourceCode: source,
			CodeRange:  codeRange,
		}
		ret.GraphInfo = append(ret.GraphInfo, info)
	})

	// path
	ret.GraphPath = graph.DeepFirstGraphPrev(v)

	return ret
}
