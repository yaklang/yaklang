package ssaapi

import (
	"bytes"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type SFGraph struct {
	GraphValues ssadb.GraphValues 
}

func NewSFGraph(graphValues ...ssadb.GraphValue)*SFGraph{
	return &SFGraph{
		GraphValues: graphValues,
	}
}

func NewSFGraphWithAuditValues(auditValues ...ssadb.AuditValues)*SFGraph{
	graphValues := make(ssadb.GraphValues, 0)
	for _, val := range auditValues {
		for _, v := range val {
			graphValues = append(graphValues, v)
		}
	}
	return &SFGraph{
		GraphValues: graphValues,
	}
}

func (sg *SFGraph)DotGraph()string  {
	vg := CreateDotGraph(sg.GraphValues...)
	var buf bytes.Buffer
	vg.GenerateDOT(&buf)
	return buf.String()
}

