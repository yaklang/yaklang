package ssaapi

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type saveValueCtx struct {
	db      *gorm.DB
	visited map[string]struct{}
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
		db:         db,
		visited:    make(map[string]struct{}),
		entryValue: value,
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

	var vals Values
	vals = append(vals, value.DependOn...)
	vals = append(vals, value.EffectOn...)

	for _, i := range value.DependOn {
		edge := ssadb.CreateEffectsOnEdge(s.ProgramName, value.GetId(), i.GetId())
		edge.FromVerboseName = value.GetVerboseName()
		edge.ToVerboseName = i.GetVerboseName()
		s.db.Save(edge)
	}

	for _, i := range value.EffectOn {
		edge := ssadb.CreateEffectsOnEdge(s.ProgramName, value.GetId(), i.GetId())
		edge.FromVerboseName = value.GetVerboseName()
		edge.ToVerboseName = i.GetVerboseName()
		s.db.Save(edge)
	}

	for _, pred := range value.Predecessors {
		var step int64
		var label string
		if info := pred.Info; info != nil {
			step = int64(info.Step)
			label = info.Label
		}
		edge := ssadb.CreatePEdge(s.ProgramName, value.GetId(),pred.Node.GetId(), step, label)
		edge.FromVerboseName = value.GetVerboseName()
		edge.ToVerboseName = pred.Node.GetVerboseName()
		vals = append(vals, pred.Node)
		s.db.Save(edge)
	}

	var id string
	idInt := value.GetId()
	if idInt <= 0 {
		id = codec.Sha256(value.String())
	} else {
		id = codec.Sha256(idInt)
	}

	if _, ok := s.visited[id]; ok {
		return nil
	}
	s.visited[id] = struct{}{}

	_, err := s.SaveNode(value)
	if err != nil {
		return err
	}

	for _, i := range vals {
		_ = s.recursiveSaveValue(i)
	}
	return nil
}
