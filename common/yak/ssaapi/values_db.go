package ssaapi

import (
	"context"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/graph"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

const (
	MAXTime         = time.Millisecond * 500
	MaxPathElements = 10
)

type Dtype int

const (
	DT_None Dtype = iota
	DT_DependOn
	DT_EffectOn
)

type saveValueCtx struct {
	db *gorm.DB
	ssadb.AuditNodeStatus

	entryValue *Value

	visitedNode map[*Value]*ssadb.AuditNode
	dtype       Dtype
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

func SaveValue(value *Value, opts ...SaveValueOption) error {
	db := ssadb.GetDB()
	if db == nil {
		return utils.Error("db is nil")
	}
	ctx := &saveValueCtx{
		db:          db,
		entryValue:  value,
		visitedNode: make(map[*Value]*ssadb.AuditNode),
		dtype:       DT_None,
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
	if node, ok := s.visitedNode[value]; ok {
		return node, nil
	}
	if value == nil {
		return nil, utils.Error("value is nil")
	}

	if s.dtype == DT_None {
		if value.GetDependOn() != nil {
			s.dtype = DT_DependOn
		} else if value.GetEffectOn() != nil {
			s.dtype = DT_EffectOn
		}
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
	s.visitedNode[value] = an
	return an, nil
}

func (s *saveValueCtx) getNeighbors(value *Value) []*graph.Neighbor[*Value] {
	if value == nil {
		return nil
	}

	var res []*graph.Neighbor[*Value]
	for _, pred := range value.GetPredecessors() {
		if pred.Node == nil {
			continue
		}
		if IsDataFlowLabel(pred.Info.Label) {
			var neighbor *graph.Neighbor[*Value]
			if s.saveDataFlow(pred.Node, value) {
				neighbor = graph.NewNeighbor(pred.Node, "") // ignore this edge in dot graph
			} else {
				neighbor = graph.NewNeighbor(pred.Node, EdgeTypePredecessor)
			}
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

// from is the source node, to is the target node, from -> xxx -> to
func (s *saveValueCtx) saveDataFlow(from *Value, to *Value) bool {
	f := func(v *Value) []*Value {
		return nil
	}

	if s.dtype == DT_DependOn {
		f = func(v *Value) []*Value {
			return v.GetEffectOn()
		}
	} else if s.dtype == DT_EffectOn {
		f = func(v *Value) []*Value {
			return v.GetDependOn()
		}
	}

	var paths [][]*Value
	ctx, cancel := context.WithTimeout(context.Background(), MAXTime)
	_ = cancel
	paths = graph.GraphPathWithTarget(ctx, from, to, func(v *Value) []*Value {
		return f(v)
	})

	totalElements := 0
	for _, innerSlice := range paths {
		totalElements += len(innerSlice) // 累加所有内层切片长度
	}

	if totalElements == 0 {
		log.Warnf("saveDataFlow:  paths is empty, maybe timeout")
		return false
	}
	if totalElements > MaxPathElements {
		log.Warnf("saveDataFlow:  paths is too many: %v", totalElements)
		return false
	}

	for _, path := range paths {
		// log.Infof("saveDataFlow: %v", path)
		// save dataflow path
		for i := 0; i < len(path)-1; i++ {
			fromNode, err := s.SaveNode(path[i])
			if err != nil {
				log.Errorf("failed to save node: %v", err)
				continue
			}

			toNode, err := s.SaveNode(path[i+1])
			if err != nil {
				log.Errorf("failed to save node: %v", err)
				continue
			}

			if s.dtype == DT_DependOn {
				s.SaveEdge(fromNode, toNode, EdgeTypeDependOn, nil)
				s.SaveEdge(toNode, fromNode, EdgeTypeEffectOn, nil)
			} else if s.dtype == DT_EffectOn {
				s.SaveEdge(fromNode, toNode, EdgeTypeEffectOn, nil)
				s.SaveEdge(toNode, fromNode, EdgeTypeDependOn, nil)
			}

		}
	}
	// elapsed := time.Since(start)
	// if elapsed > MaxTime {
	// 	log.Warnf("saveDataFlow:  save paths cost [%v] paths: %v", elapsed, totalElements)
	// }

	return true
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
