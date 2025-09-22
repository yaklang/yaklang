package ssaapi

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
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
	database *auditDatabase
	ssadb.AuditNodeStatus

	ctx        context.Context
	entryValue *Value

	visitedNode map[*Value]*ssadb.AuditNode

	isMemoryCompile bool
}

func newSaveValueCtx(opts ...SaveValueOption) *saveValueCtx {
	saveValueConfig := &saveValueCtx{
		visitedNode: make(map[*Value]*ssadb.AuditNode),
		ctx:         context.Background(),
	}
	for _, o := range opts {
		o(saveValueConfig)
	}
	return saveValueConfig
}

type SaveValueOption func(c *saveValueCtx)

func OptionSaveValue_Database(db *auditDatabase) SaveValueOption {
	return func(c *saveValueCtx) {
		c.database = db
	}
}

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

func OptionSaveValue_IsMemoryCompile(bool2 bool) SaveValueOption {
	return func(c *saveValueCtx) {
		c.isMemoryCompile = bool2
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
	saveValueConfig := newSaveValueCtx(opts...)
	saveValueConfig.entryValue = value

	if saveValueConfig.database == nil {
		db := ssadb.GetDB()
		if db == nil {
			return utils.Error("db is nil")
		}
		database := newAuditDatabase(saveValueConfig.ctx, db)
		saveValueConfig.database = database
		defer func() {
			database.Close()
		}()
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

func (g *DBGraph) GetGraphKind() GraphKind {
	return GraphKindDump
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

	setTmpValue := func(an *ssadb.AuditNode, v *Value) {
		R := value.GetRange()
		an.TmpValue = yakunquote.TryUnquote(value.String())
		if R != nil {
			editor := R.GetEditor()
			if editor == nil {
				log.Errorf("%v: CreateOffset: rng or editor is nil", value.GetVerboseName())
				return
			}
			hash := editor.GetIrSourceHash()
			an.TmpValueFileHash = hash
			an.TmpStartOffset = R.GetStartOffset()
			an.TmpEndOffset = R.GetEndOffset()
		}
	}
	an := g.database.GetNode()
	if an == nil {
		return nil, utils.Error("get AuditNode failed")
	}

	saveIrSource := func(v *Value) {
		inst := v.getInstruction()
		if inst == nil {
			return
		}
		r := inst.GetRange()
		editor := r.GetEditor()
		if editor == nil {
			log.Errorf("%v: saveIrSource: rng or editor is nil", v.GetVerboseName())
			return
		}
		irSource := ssadb.MarshalFile(editor)
		g.database.SaveIrSource(irSource)
	}

	switch {
	case g.isMemoryCompile:
		setTmpValue(an, value)
		saveIrSource(value)
	default:
		an.AuditNodeStatus = g.AuditNodeStatus
		an.IsEntryNode = entry
		an.IRCodeID = value.GetId()
		if value.GetId() == -1 {
			setTmpValue(an, value)
		}
	}
	g.database.SaveNode(an)
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

	saveEdge := func(edge *ssadb.AuditEdge) error {
		return ssaprofile.ProfileAddWithError(true, "dbgraph_create_edge", func() error {
			g.database.SaveEdge(edge)
			return nil
		})
	}
	msg := edge.Msg
	switch edge.Kind {
	case EdgeTypeDependOn:
		edge := fromNode.CreateDependsOnEdge(g.ProgramName, toNode.ID)
		if err := saveEdge(edge); err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	case EdgeTypeEffectOn:
		edge := fromNode.CreateEffectsOnEdge(g.ProgramName, toNode.ID)
		if err := saveEdge(edge); err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	// case EdgeTypeDataflow:
	// 	edge := fromNode.CreateDataFlowEdge(g.ProgramName, toNode.ID)
	// 	if err := saveEdge(edge); err != nil {
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
		if err := saveEdge(edge); err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	}
	return nil

}

type auditDatabase struct {
	fetch      *databasex.Fetch[*ssadb.AuditNode]
	nodeSave   *databasex.Save[*ssadb.AuditNode]
	edgeSave   *databasex.Save[*ssadb.AuditEdge]
	editorSave *databasex.Save[*ssadb.IrSource]
}

func (a *auditDatabase) GetNode() *ssadb.AuditNode {
	// get node from fetch, if err retry 10 times
	for i := 0; i < 10; i++ {
		node, err := a.fetch.Fetch()
		if err == nil {
			return node
		}
		log.Warnf("fetch AuditNode failed: %v", err)
		time.Sleep(time.Millisecond * 100)
	}
	return nil
}

func (a *auditDatabase) SaveNode(node *ssadb.AuditNode) {
	if node == nil {
		return
	}
	a.nodeSave.Save(node)
}

func (a *auditDatabase) SaveEdge(edge *ssadb.AuditEdge) {
	if edge == nil {
		return
	}
	a.edgeSave.Save(edge)
}

func (a *auditDatabase) SaveIrSource(editor *ssadb.IrSource) {
	if editor == nil {
		return
	}
	a.editorSave.Save(editor)
}

func (a *auditDatabase) Close() {
	a.fetch.Close()
	a.nodeSave.Close()
	a.edgeSave.Close()
	a.editorSave.Close()
}

func newAuditDatabase(ctx context.Context, db *gorm.DB) *auditDatabase {
	ret := &auditDatabase{}
	// opt := []databasex.Option{
	// 	databasex.WithContext(ctx),
	// 	databasex.WithFetchSize(100),
	// }
	size := 1000

	ret.fetch = databasex.NewFetch[*ssadb.AuditNode](func(ctx context.Context, fetchSize int) <-chan *ssadb.AuditNode {
		out := make(chan *ssadb.AuditNode, fetchSize)
		go func() {
			defer close(out)
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				i := 0
				for i < fetchSize {
					node := &ssadb.AuditNode{}
					if ret := tx.Create(node).Error; ret != nil {
						log.Errorf("create empty AuditNode failed: %v", ret)
						continue
					}
					i++
					out <- node
				}
				return nil
			})
		}()
		return out
	}, databasex.WithContext(ctx), databasex.WithFetchSize(size), databasex.WithName("AuditNode"))

	ret.nodeSave = databasex.NewSave[*ssadb.AuditNode](func(ae []*ssadb.AuditNode) {
		if len(ae) == 0 {
			return
		}
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			for _, e := range ae {
				if ret := tx.Save(e).Error; ret != nil {
					log.Errorf("save AuditNode failed: %v", ret)
				}
			}
			return nil
		})
		return
	}, databasex.WithContext(ctx), databasex.WithSaveSize(size), databasex.WithName("AuditNode"))

	ret.edgeSave = databasex.NewSave[*ssadb.AuditEdge](func(ae []*ssadb.AuditEdge) {
		if len(ae) == 0 {
			return
		}
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			for _, e := range ae {
				if ret := tx.Save(e).Error; ret != nil {
					log.Errorf("save AuditEdge failed: %v", ret)
				}
			}
			return nil
		})
		return
	}, databasex.WithContext(ctx), databasex.WithSaveSize(size), databasex.WithName("AuditEdge"))

	ret.editorSave = databasex.NewSave[*ssadb.IrSource](func(ae []*ssadb.IrSource) {
		if len(ae) == 0 {
			return
		}
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			for _, e := range ae {
				if ret := tx.Save(e).Error; ret != nil {
					log.Errorf("save AuditEdge failed: %v", ret)
				}
			}
			return nil
		})
		return
	}, databasex.WithContext(ctx), databasex.WithSaveSize(size), databasex.WithName("SourceFile"))

	return ret
}
