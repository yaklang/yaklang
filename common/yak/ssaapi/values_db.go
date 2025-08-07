package ssaapi

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
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

	ctx        context.Context
	entryValue *Value

	visitedNode map[*Value]*ssadb.AuditNode

	nodeSaver *databasex.Save[*ssadb.AuditNode]
	edgeSaver *databasex.Save[*ssadb.AuditEdge]
}

type SaveValueOption func(c *saveValueCtx)

func OptionSaveValue_TaskID(taskID string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.TaskId = taskID
	}
}

func OptionSaveValue_Context(ctx context.Context) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ctx = ctx
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

	nodeSaver := databasex.NewSave[*ssadb.AuditNode](
		func(nodes []*ssadb.AuditNode) {
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, node := range nodes {
					if err := tx.Create(node).Error; err != nil {
						log.Errorf("save audit node failed: %v", err)
					}
				}
				return nil
			})
		},
		databasex.WithBufferSize(100),
		databasex.WithSaveSize(20),
	)

	edgeSaver := databasex.NewSave[*ssadb.AuditEdge](
		func(edges []*ssadb.AuditEdge) {
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, edge := range edges {
					if err := tx.Create(edge).Error; err != nil {
						log.Errorf("save audit edge failed: %v", err)
					}
				}
				return nil
			})
		},
		databasex.WithBufferSize(100),
		databasex.WithSaveSize(20),
	)

	saveValueConfig := &saveValueCtx{
		db:          db,
		entryValue:  value,
		visitedNode: make(map[*Value]*ssadb.AuditNode),
		nodeSaver:   nodeSaver,
		edgeSaver:   edgeSaver,
	}
	for _, o := range opts {
		o(saveValueConfig)
	}
	if saveValueConfig.ProgramName == "" {
		return utils.Error("program info is empty")
	}
	err := graph.BuildGraphWithDFS[*ssadb.AuditNode, *Value](
		saveValueConfig.ctx,
		value,
		saveValueConfig.CreateNode,
		saveValueConfig.getNeighbors,
		saveValueConfig.SaveEdge,
	)

	if err != nil {
		saveValueConfig.nodeSaver.Close()
		saveValueConfig.edgeSaver.Close()
		return err
	}

	saveValueConfig.nodeSaver.Close()
	saveValueConfig.edgeSaver.Close()
	return nil
}

func (s *saveValueCtx) CreateNode(value *Value) (*ssadb.AuditNode, error) {
	if node, ok := s.visitedNode[value]; ok {
		return node, nil
	}
	if value == nil {
		return nil, utils.Error("value is nil")
	}
	an := &ssadb.AuditNode{
		UUID:            uuid.NewString(),
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
			hash := editor.GetIrSourceHash()
			an.TmpValueFileHash = hash
			an.TmpStartOffset = R.GetStartOffset()
			an.TmpEndOffset = R.GetEndOffset()
		}
	}
	s.visitedNode[value] = an
	s.nodeSaver.Save(an)
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
		label := pred.Info.Label
		if IsDataFlowLabel(label) {
			var neighbor *graph.Neighbor[*Value]
			if s.saveDataFlow(pred.Node, value, label) {
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
func (s *saveValueCtx) saveDataFlow(from *Value, to *Value, label string) bool {
	var getNext func(v *Value) []*Value
	var saveNode func(from, to *ssadb.AuditNode)

	switch label {
	case Predecessors_TopDefLabel:
		getNext = func(v *Value) []*Value {
			return v.GetDependOn()
		}
		saveNode = func(from, to *ssadb.AuditNode) {
			s.SaveEdge(from, to, EdgeTypeDependOn, nil)
			s.SaveEdge(to, from, EdgeTypeEffectOn, nil)
		}
	case Predecessors_BottomUseLabel:
		getNext = func(v *Value) []*Value {
			return v.GetEffectOn()
		}
		saveNode = func(from, to *ssadb.AuditNode) {
			s.SaveEdge(from, to, EdgeTypeEffectOn, nil)
			s.SaveEdge(to, from, EdgeTypeDependOn, nil)
		}
	default:
		return false
	}

	var paths [][]*Value
	ctx, cancel := context.WithTimeout(context.Background(), MAXTime)
	_ = cancel
	paths = graph.GraphPathWithTarget(ctx, from, to, func(v *Value) []*Value {
		return getNext(v)
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
			fromNode, err := s.CreateNode(path[i])
			if err != nil {
				log.Errorf("failed to save node: %v", err)
				continue
			}

			toNode, err := s.CreateNode(path[i+1])
			if err != nil {
				log.Errorf("failed to save node: %v", err)
				continue
			}
			saveNode(fromNode, toNode)
		}
	}

	return true
}

func (s *saveValueCtx) SaveEdge(from *ssadb.AuditNode, to *ssadb.AuditNode, edgeType string, extraMsg map[string]interface{}) {
	if from == nil || to == nil {
		return
	}
	switch ValidEdgeType(edgeType) {
	case EdgeTypeDependOn:
		edge := from.CreateDependsOnEdge(s.ProgramName, to.UUID)
		s.edgeSaver.Save(edge)
	case EdgeTypeEffectOn:
		edge := from.CreateEffectsOnEdge(s.ProgramName, to.UUID)
		s.edgeSaver.Save(edge)
	case EdgeTypePredecessor:
		var (
			label string
			step  int64
		)
		if extraMsg != nil {
			label = extraMsg["label"].(string)
			step = extraMsg["step"].(int64)
		}
		edge := from.CreatePredecessorEdge(s.ProgramName, to.UUID, step, label)
		s.edgeSaver.Save(edge)
	}
}
