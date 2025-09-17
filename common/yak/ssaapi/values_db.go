package ssaapi

import (
	"context"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/utils"
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
	saveValueConfig := &saveValueCtx{
		db:          db,
		entryValue:  value,
		visitedNode: make(map[*Value]*ssadb.AuditNode),
	}
	for _, o := range opts {
		o(saveValueConfig)
	}
	if saveValueConfig.ProgramName == "" {
		return utils.Error("program info is empty")
	}
	graph := NewDBGraph(saveValueConfig)
	graph.getOrCreateNode(value, true)
	err := value.GenerateGraph(graph, saveValueConfig.ctx)
	return err
}

type DBGraph struct {
	*saveValueCtx
}

var _ Graph = (*DBGraph)(nil)

func NewDBGraph(ctx *saveValueCtx) *DBGraph {
	return &DBGraph{
		saveValueCtx: ctx,
	}
}

func (g *DBGraph) getOrCreateNode(value *Value, isEntry ...bool) (*ssadb.AuditNode, error) {
	entry := false
	if len(isEntry) > 0 {
		entry = isEntry[0]
	}
	if node, ok := g.visitedNode[value]; ok {
		return node, nil
	}
	if value == nil {
		return nil, utils.Error("value is nil")
	}

	an := &ssadb.AuditNode{
		AuditNodeStatus: g.AuditNodeStatus,
		// IsEntryNode:     ValueCompare(value, g.entryValue),
		IsEntryNode:    entry,
		IRCodeID:       value.GetId(),
		TmpStartOffset: -1,
		TmpEndOffset:   -1,
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
	if ret := g.db.Save(an).Error; ret != nil {
		return nil, utils.Wrap(ret, "save AuditNode")
	}
	g.visitedNode[value] = an
	return an, nil
}

func (g *DBGraph) CreateEdge(edge Edge) error {
	fromNode, err := g.getOrCreateNode(edge.From)
	if err != nil {
		return utils.Errorf("create from node failed: %v", err)
	}
	toNode, err := g.getOrCreateNode(edge.To)
	if err != nil {
		return utils.Errorf("create to node failed: %v", err)
	}

	msg := edge.Msg
	switch edge.Kind {
	case EdgeTypeDependOn:
		edge := fromNode.CreateDependsOnEdge(g.ProgramName, toNode.ID)
		if err := g.db.Save(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	case EdgeTypeEffectOn:
		edge := fromNode.CreateEffectsOnEdge(g.ProgramName, toNode.ID)
		if err := g.db.Save(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	// case EdgeTypeDataflow:
	// 	edge := fromNode.CreateDataFlowEdge(g.ProgramName, toNode.ID)
	// 	if err := g.db.Save(edge).Error; err != nil {
	// 		log.Errorf("save AuditEdge failed: %v", err)
	// 	}
	case EdgeTypePredecessor:
		var (
			label string
			step  int64
		)
		if msg != nil {
			if l, ok := msg["label"].(string); ok {
				label = l
			}
			if s, ok := msg["step"].(int64); ok {
				step = s
			}
		}
		edge := fromNode.CreatePredecessorEdge(g.ProgramName, toNode.ID, step, label)
		if err := g.db.Save(edge).Error; err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	}
	return nil
}
