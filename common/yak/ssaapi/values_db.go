package ssaapi

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/graph"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type saveValueCtx struct {
	db *gorm.DB
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
		entryValue: value,
	}
	for _, o := range opts {
		o(ctx)
	}
	if ctx.ProgramName == "" {
		return utils.Error("program info is empty")
	}
	err := graph.BuildGraphWithDFS[*ssadb.AuditNode, *Value](
		value,
		ctx.SaveNode,
		ctx.getNeighbors,
		ctx.SaveEdge,
	)
	return err
}

func (s *saveValueCtx) SaveNode(value *Value) (*ssadb.AuditNode, error) {
	if value == nil {
		return nil, utils.Error("value is nil")
	}
	an := &ssadb.AuditNode{
		AuditNodeStatus: s.AuditNodeStatus,
		IsEntryNode:     value == s.entryValue,
		IRCodeID:        value.GetId(),
		TmpStartOffset:  -1,
		TmpEndOffset:    -1,
	}
	if value.GetId() == -1 {
		R := value.GetRange()
		an.TmpValue = yakunquote.TryUnquote(value.String())
		if R != nil {
			editor := R.GetEditor()
			if editor == nil {
				log.Errorf("%v: CreateOffset: rng or editor is nil", value.GetVerboseName())
				return an, nil
			}
			hash := editor.GetIrSourceHash(value.GetProgramName())
			an.TmpValueFileHash = hash
			an.TmpStartOffset = R.GetStartOffset()
			an.TmpEndOffset = R.GetEndOffset()
		}
	}
	if ret := s.db.Save(an).Error; ret != nil {
		return nil, utils.Wrap(ret, "save AuditNode")
	}
	return an, nil
}

func (s *saveValueCtx) getNeighbors(value *Value) []*graph.Neighbor[*Value] {
	if value == nil {
		return nil
	}

	var res []*graph.Neighbor[*Value]
	for _, i := range value.DependOn {
		res = append(res, graph.NewNeighbor(i, EdgeTypeDependOn))
	}
	for _, i := range value.EffectOn {
		res = append(res, graph.NewNeighbor(i, EdgeTypeEffectOn))
	}
	for _, pred := range value.Predecessors {
		neighbor := graph.NewNeighbor(pred.Node, EdgeTypePredecessor)
		neighbor.AddExtraMsg("label", pred.Info.Label)
		neighbor.AddExtraMsg("step", int64(pred.Info.Step))
		res = append(res, neighbor)
	}
	return res
}

func (s *saveValueCtx) SaveEdge(from *ssadb.AuditNode, to *ssadb.AuditNode, edgeType string, extraMsg map[string]interface{}) {
	if from == nil || to == nil {
		return
	}
	switch ValidEdgeType(edgeType) {
	case EdgeTypeDependOn:
		edge := from.CreateDependsOnEdge(s.ProgramName, to.ID)
		if err := s.db.Save(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	case EdgeTypeEffectOn:
		edge := from.CreateEffectsOnEdge(s.ProgramName, to.ID)
		if err := s.db.Save(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	case EdgeTypePredecessor:
		var (
			label string
			step  int64
		)
		if extraMsg != nil {
			label = extraMsg["label"].(string)
			step = extraMsg["step"].(int64)
		}
		edge := from.CreatePredecessorEdge(s.ProgramName, to.ID, step, label)
		if err := s.db.Save(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}

	}
}
