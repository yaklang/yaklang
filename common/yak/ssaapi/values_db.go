package ssaapi

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type saveValueCtx struct {
	db              *gorm.DB
	saveNodeVisited map[string]struct{}
	saveEdgeVisited map[string]struct{}
	ssadb.AuditNodeStatus

	entryValue *Value
}

type SaveValueOption func(c *saveValueCtx)

func OptionSaveValue_TaskID(taskID string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.TaskId = taskID
	}
}

func OptionSaveValue_ResultID(resultID uint) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ResultId = resultID
	}
}

func OptionSaveValue_ResultVariable(variable string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ResultVariable = variable
	}
}

func OptionSaveValue_ResultAlert(alertMsg string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ResultAlertMsg = alertMsg
	}
}

func OptionSaveValue_RuleName(i string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.RuleName = i
	}
}

func OptionSaveValue_RuleTitle(name string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.RuleTitle = name
	}
}

func OptionSaveValue_ProgramName(name string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ProgramName = name
	}
}

func SaveValue(value *Value, opts ...SaveValueOption) error {
	db := ssadb.GetDB()
	if db == nil {
		return utils.Error("db is nil")
	}
	ctx := &saveValueCtx{
		db:              db,
		saveNodeVisited: make(map[string]struct{}),
		saveEdgeVisited: make(map[string]struct{}),
		entryValue:      value,
	}
	for _, o := range opts {
		o(ctx)
	}
	if ctx.ProgramName == "" {
		return utils.Error("program info is empty")
	}
	return ctx.recursiveSaveValue(value)
}

func (s *saveValueCtx) SaveNode(value *Value) (*ssadb.AuditNode, error) {
	an := &ssadb.AuditNode{
		AuditNodeStatus: s.AuditNodeStatus,
		IsEntryNode:     ValueCompare(value, s.entryValue),
		IRCodeID:        value.GetId(),
		VerboseName:     value.GetVerboseName(),
	}
	if ret := s.db.Save(an).Error; ret != nil {
		return nil, utils.Wrap(ret, "save AuditNode")
	}
	return an, nil
}

func (s *saveValueCtx) recursiveSaveValue(value *Value) error {
	if s == nil {
		return utils.Error("saveValueCtx is nil")
	}

	if value == nil {
		return nil
	}

	var nextVals Values
	nextVals = append(nextVals, value.GetDependOn()...)
	nextVals = append(nextVals, value.GetEffectOn()...)
	// save audit edge
	for _, i := range value.GetDependOn() {
		if !s.theEdgeShouldVisit(value, i, s.ProgramName, ssadb.EdgeType_DependsOn) {
			continue
		}
		edge := ssadb.CreateDependsOnEdge(s.ProgramName, value.GetId(), i.GetId())
		err := s.db.Save(edge).Error
		if err != nil {
			return err
		}
	}
	for _, i := range value.GetEffectOn() {
		if !s.theEdgeShouldVisit(value, i, s.ProgramName, ssadb.EdgeType_EffectsOn) {
			continue
		}
		edge := ssadb.CreateEffectsOnEdge(s.ProgramName, value.GetId(), i.GetId())
		err := s.db.Save(edge).Error
		if err != nil {
			return err
		}
	}
	for _, pred := range value.Predecessors {
		node := pred.Node
		if !s.theEdgeShouldVisit(value, node, s.ProgramName, ssadb.EdgeType_Predecessor) {
			continue
		}
		var step int64
		var label string
		if info := pred.Info; info != nil {
			step = int64(info.Step)
			label = info.Label
		}
		edge := ssadb.CreatePredecessorEdge(s.ProgramName, value.GetId(), node.GetId(), step, label)
		nextVals = append(nextVals, node)
		err := s.db.Save(edge).Error
		if err != nil {
			return err
		}
	}
	// save audit node
	var id string
	idInt := value.GetId()
	if idInt <= 0 {
		id = codec.Sha256(value.String())
	} else {
		id = codec.Sha256(idInt)
	}
	if _, ok := s.saveNodeVisited[id]; ok {
		return nil
	}
	s.saveNodeVisited[id] = struct{}{}
	_, err := s.SaveNode(value)
	if err != nil {
		return err
	}
	for _, i := range nextVals {
		err = s.recursiveSaveValue(i)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *saveValueCtx) theEdgeShouldVisit(from *Value, to *Value, program string, edgeType ssadb.AuditEdgeType) bool {
	if from == nil || to == nil {
		return false
	}
	hash := utils.CalcSha1(from.GetId(), to.GetId(), program, edgeType)
	if _, ok := s.saveEdgeVisited[hash]; ok {
		return false
	}
	s.saveEdgeVisited[hash] = struct{}{}
	return true
}
