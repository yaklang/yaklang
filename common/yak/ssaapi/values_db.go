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

func OptionSaveValue_ResultIndex(index uint) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ResultIndex = index
	}
}

func OptionSaveValue_ResultAlert(alertMsg string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ResultAlertMsg = alertMsg
	}
}

func OptionSaveValue_ResultRiskHash(hash string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.RiskHash = hash
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

// type Cache struct {
// 	mu    sync.Mutex
// 	items map[ssa.Value]*ssadb.AuditNode
// }

// func (c *Cache) Set(key ssa.Value, value *ssadb.AuditNode) {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	c.items[key] = value
// }

// func (c *Cache) Get(key ssa.Value) (*ssadb.AuditNode, bool) {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	val, ok := c.items[key]
// 	return val, ok
// }

// func (c *Cache) ReMove() {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	c.items = make(map[ssa.Value]*ssadb.AuditNode)
// }

// var cache = &Cache{
// 	mu:    sync.Mutex{},
// 	items: make(map[ssa.Value]*ssadb.AuditNode),
// }

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
	// log.Infof("SaveValue: %v: %v", ctx, value)
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
	where := &ssadb.AuditNode{
		AuditNodeStatus: ssadb.AuditNodeStatus{
			ResultId:       an.ResultId,
			ResultVariable: an.ResultVariable,
		},
		IsEntryNode:    an.IsEntryNode,
		IRCodeID:       an.IRCodeID,
		TmpStartOffset: an.TmpStartOffset,
		TmpEndOffset:   an.TmpEndOffset,
	}
	if err := s.db.Where(where).FirstOrCreate(an).Error; err != nil {
		return nil, utils.Wrap(err, "save AuditNode")
	}
	return an, nil
}

func (s *saveValueCtx) getNeighbors(value *Value, visited *map[*Value]map[*Value]bool) []*graph.Neighbor[*Value] {
	if value == nil {
		return nil
	}

	var res []*graph.Neighbor[*Value]
	// for _, i := range value.GetDependOn() {
	// 	res = append(res, graph.NewNeighbor(i, EdgeTypeDependOn))
	// }
	// for _, i := range value.GetEffectOn() {
	// 	res = append(res, graph.NewNeighbor(i, EdgeTypeEffectOn))
	// }
	for _, pred := range value.GetPredecessors() {
		if pred.Node == nil {
			continue
		}
		if IsDataFlowLabel(pred.Info.Label) {
			graph.BuildGraphWithBFS[*ssadb.AuditNode, *Value](
				pred.Node, value,
				s.SaveNode,
				s.getNeighborsDependOn,
				s.getNeighborsEffectOn,
				s.SaveEdge,
				visited,
			)

			neighbor := graph.NewNeighbor(pred.Node, "")
			neighbor.AddExtraMsg("label", pred.Info.Label)
			neighbor.AddExtraMsg("step", int64(pred.Info.Step))
			res = append(res, neighbor)
		} else {
			neighbor := graph.NewNeighbor(pred.Node, EdgeTypePredecessor)
			neighbor.AddExtraMsg("label", pred.Info.Label)
			neighbor.AddExtraMsg("step", int64(pred.Info.Step))
			res = append(res, neighbor)
		}
	}
	return res
}

func (s *saveValueCtx) getNeighborsDependOn(value *Value) map[*Value]*graph.Neighbor[*Value] {
	var res map[*Value]*graph.Neighbor[*Value] = make(map[*Value]*graph.Neighbor[*Value])
	for _, i := range value.GetDependOn() {
		res[i] = graph.NewNeighbor(i, EdgeTypeDependOn)
	}
	return res
}

func (s *saveValueCtx) getNeighborsEffectOn(value *Value) map[*Value]*graph.Neighbor[*Value] {
	var res map[*Value]*graph.Neighbor[*Value] = make(map[*Value]*graph.Neighbor[*Value])
	for _, i := range value.GetEffectOn() {
		res[i] = graph.NewNeighbor(i, EdgeTypeEffectOn)
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
		if err := s.db.Where(edge).FirstOrCreate(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	case EdgeTypeEffectOn:
		edge := from.CreateEffectsOnEdge(s.ProgramName, to.ID)
		if err := s.db.Where(edge).FirstOrCreate(edge).Error; err != nil {
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
		if err := s.db.Where(edge).FirstOrCreate(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	}
}
