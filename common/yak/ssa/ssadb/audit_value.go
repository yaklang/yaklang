package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

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
	auditValue.SearchAndSetEdge(db, programName)
	auditValue.SearchAndSetPredecessor(db, programName)
	return auditValue, nil
}

type AuditValue struct {
	ProgramName  string
	VerboseName  string
	Id           int64
	EffectOn     AuditValues
	DependOn     AuditValues
	Predecessors GraphPredecessors
}

type AuditValues []*AuditValue

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

func (v *AuditValue) SearchAndSetVerboseName(db *gorm.DB, programName string) {
	v.VerboseName = GetAuditNodeVerboseNameById(db, v.Id, programName)
}

func (v *AuditValue) SearchAndSetPredecessor(db *gorm.DB, programName string) {
	for _, edge := range GetEdgePredecessorByFromID(db, v.Id, programName) {
		predecessorValue := &AuditValue{
			Id: edge.ToNode,
		}
		predecessorValue.SearchAndSetVerboseName(db, programName)
		v.Predecessors = append(v.Predecessors, &GraphPredecessor{
			GraphValue: predecessorValue,
			Step:       int(edge.AnalysisStep),
			Label:      edge.AnalysisLabel,
		})
	}
}

func (v *AuditValue) SearchAndSetEdge(db *gorm.DB, programName string) {
	for _, d := range GetEdgeDependOnByFromNodeId(db, v.Id, programName) {
		depend := &AuditValue{
			Id: d,
		}
		depend.SearchAndSetPredecessor(db, programName)
		depend.SearchAndSetVerboseName(db, programName)
		v.DependOn = append(v.DependOn, depend)
	}
	for _, e := range GetEdgeEffectOnByFromNodeId(db, v.Id, programName) {
		effect := &AuditValue{
			Id: e,
		}
		effect.SearchAndSetPredecessor(db, programName)
		effect.SearchAndSetVerboseName(db, programName)
		v.EffectOn = append(v.EffectOn, effect)
	}

}
