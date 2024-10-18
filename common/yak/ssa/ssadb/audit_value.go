package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type AuditValue struct {
	ProgramName  string
	VerboseName  string
	Id           int64
	EffectOn     AuditValues
	DependOn     AuditValues
	Predecessors GraphPredecessors
}

type AuditValues []*AuditValue

func GetAuditValuesByIds(db *gorm.DB, ids []int64, programName string) (AuditValues, error) {
	values := make(AuditValues, 0)
	for _, id := range ids {
		value, err := GetAuditValueById(db, id, programName)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	return values, nil
}

func GetAuditValueById(db *gorm.DB, id int64, programName string) (*AuditValue, error) {
	value := GetIrCodeById(db, id)
	if value == nil {
		return nil, utils.Errorf("can't find ircode by id %d", id)
	}
	auditValue := &AuditValue{
		Id: id,
	}
	auditValue.searchAndSetEdge(db, programName, make(map[int64]struct{}))
	return auditValue, nil
}

func (v *AuditValue) GetId() int64 {
	return v.Id
}

func (v *AuditValue) GetEffectOnGraphValues() GraphValues {
	graphValues := make(GraphValues, 0)
	for _, val := range v.EffectOn {
		graphValues = append(graphValues, val)
	}
	return graphValues
}

func (v *AuditValue) GetDependOnGraphValues() GraphValues {
	graphValues := make(GraphValues, 0)
	for _, val := range v.DependOn {
		graphValues = append(graphValues, val)
	}
	return graphValues
}

func (v *AuditValue) GetGraphPredecessors() GraphPredecessors {
	return v.Predecessors
}

func (v *AuditValue) GetVerboseName() string {
	return v.VerboseName
}

func (v *AuditValue) searchAndSetVerboseName(db *gorm.DB, programName string) {
	v.VerboseName = GetAuditNodeVerboseNameById(db, v.Id, programName)
}

func (v *AuditValue) searchAndSetEdge(db *gorm.DB, programName string, m map[int64]struct{}) {
	if _, ok := m[v.Id]; ok {
		return
	}
	m[v.Id] = struct{}{}
	v.searchAndSetVerboseName(db, programName)
	for _, d := range GetEdgeDependOnByFromNodeId(db, v.Id, programName) {
		depend := &AuditValue{
			Id: d,
		}
		depend.searchAndSetEdge(db, programName, m)
		v.DependOn = append(v.DependOn, depend)
	}
	for _, e := range GetEdgeEffectOnByFromNodeId(db, v.Id, programName) {
		effect := &AuditValue{
			Id: e,
		}
		effect.searchAndSetEdge(db, programName, m)
		v.EffectOn = append(v.EffectOn, effect)
	}
	for _, edge := range GetEdgePredecessorByFromID(db, v.Id, programName) {
		predecessorValue := &AuditValue{
			Id: edge.ToNode,
		}
		predecessorValue.searchAndSetEdge(db, programName, m)
		v.Predecessors = append(v.Predecessors, &GraphPredecessor{
			GraphValue: predecessorValue,
			Step:       int(edge.AnalysisStep),
			Label:      edge.AnalysisLabel,
		})
	}

}
