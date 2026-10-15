package ssaapi

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

const (
	MAXTime         = time.Millisecond * 500
	MaxPathElements = 10
)

// sanitizeAuditNodeString makes source-derived text safe for UTF-8 databases.
// Some repositories contain truncated multibyte sequences or non-UTF-8 bytes;
// PostgreSQL rejects those values and dbcache stops persisting later batches.
func sanitizeAuditNodeString(value string) string {
	return strings.ToValidUTF8(value, "�")
}

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

	isMemoryCompile     bool
	diagnosticsRecorder *diagnostics.Recorder
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

func OptionSaveValue_ResultRiskHash(hash string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.RiskHash = hash
	}
}

func OptionSaveValue_ResultRiskFeatureHash(hash string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.RiskFeatureHash = hash
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

var defaultSize = 100

func SaveValue(value *Value, opts ...SaveValueOption) error {
	saveValueConfig := newSaveValueCtx(opts...)
	saveValueConfig.entryValue = value

	if saveValueConfig.database == nil {
		db := ssadb.GetDB()
		if db == nil {
			return utils.Error("db is nil")
		}
		database := newAuditDatabase(saveValueConfig.ctx, db, defaultSize)
		saveValueConfig.database = database
		defer func() {
			if err := database.Close(); err != nil {
				log.Errorf("audit database close failed: %v", err)
			}
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
		an.TmpValue = sanitizeAuditNodeString(yakunquote.TryUnquote(value.String()))
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
	an := ssadb.NewAuditNode()
	an.AuditNodeStatus = g.AuditNodeStatus
	an.IsEntryNode = entry

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
		an.IRCodeID = -1
		setTmpValue(an, value)
		saveIrSource(value)
	default:
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
		err := g.DiagnosticsTrack("dbgraph_create_edge", func() error {
			g.database.SaveEdge(edge)
			return nil
		})
		return err
	}
	msg := edge.Msg
	switch edge.Kind {
	case EdgeTypeDependOn:
		edge := fromNode.CreateDependsOnEdge(g.ProgramName, toNode)
		if err := saveEdge(edge); err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	case EdgeTypeEffectOn:
		edge := fromNode.CreateEffectsOnEdge(g.ProgramName, toNode)
		if err := saveEdge(edge); err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
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
		edge := fromNode.CreatePredecessorEdge(g.ProgramName, toNode, step, label)
		if err := saveEdge(edge); err != nil {
			log.Errorf("save AuditEdge failed: %v", err)
		}
	}
	return nil

}

type auditDatabase struct {
	nodeSave   *dbcache.Save[*ssadb.AuditNode]
	edgeSave   *dbcache.Save[*ssadb.AuditEdge]
	editorSave *dbcache.Save[*ssadb.IrSource]
}

const (
	// Keep batches bounded for SQLite and for the wide AuditNode model.
	auditNodeWriteBatchSize = 50
	auditEdgeWriteBatchSize = 90
)

func batchSaveAuditNodes(db *gorm.DB, items []*ssadb.AuditNode) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	// dbcache.Save drops nil entries before they reach the batch callback, and
	// CreateInBatches wraps the inserts in its own transaction, so neither a
	// nil filter nor an outer transaction is needed here.
	if result := db.CreateInBatches(items, auditNodeWriteBatchSize); result.Error != nil {
		return utils.Errorf("save AuditNode failed: %w", result.Error)
	}
	return nil
}

func batchSaveAuditEdges(db *gorm.DB, items []*ssadb.AuditEdge) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if result := db.CreateInBatches(items, auditEdgeWriteBatchSize); result.Error != nil {
		return utils.Errorf("save AuditEdge failed: %w", result.Error)
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

func (a *auditDatabase) Close() error {
	var errs []error
	if a.nodeSave != nil {
		if err := a.nodeSave.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.edgeSave != nil {
		if err := a.edgeSave.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.editorSave != nil {
		if err := a.editorSave.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return utils.JoinErrors(errs...)
}

// captureAuditGraph walks value now and keeps the rows in memory. The walk
// has to happen while this rule's analysis is still on the value:
// ResetInterRuleState nils those caches when the rule returns, and a later
// GetPredecessors call does not rebuild them. The returned rows are what the
// database saver inserts.
func captureAuditGraph(ctx context.Context, value *Value, opts ...SaveValueOption) ([]*ssadb.AuditNode, []*ssadb.AuditEdge, []*ssadb.IrSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var mu sync.Mutex
	var nodes []*ssadb.AuditNode
	var edges []*ssadb.AuditEdge
	var sources []*ssadb.IrSource
	db := &auditDatabase{}
	queue := dbcache.WithInitialBufferSize(64)
	saveSize := dbcache.WithSaveSize(defaultSize)
	db.nodeSave = dbcache.NewSave(func(items []*ssadb.AuditNode) error {
		mu.Lock()
		nodes = append(nodes, items...)
		mu.Unlock()
		return nil
	}, dbcache.WithContext(ctx), saveSize, queue, dbcache.WithName("AuditNodeCapture"))
	db.edgeSave = dbcache.NewSave(func(items []*ssadb.AuditEdge) error {
		mu.Lock()
		edges = append(edges, items...)
		mu.Unlock()
		return nil
	}, dbcache.WithContext(ctx), saveSize, queue, dbcache.WithName("AuditEdgeCapture"))
	db.editorSave = dbcache.NewSave(func(items []*ssadb.IrSource) error {
		mu.Lock()
		sources = append(sources, items...)
		mu.Unlock()
		return nil
	}, dbcache.WithContext(ctx), saveSize, queue, dbcache.WithName("SourceFileCapture"))

	call := make([]SaveValueOption, len(opts), len(opts)+2)
	copy(call, opts)
	call = append(call, OptionSaveValue_Database(db), OptionSaveValue_Context(ctx))
	err := SaveValue(value, call...)
	err = utils.JoinErrors(err, db.Close())
	return nodes, edges, sources, err
}

func saveAuditSources(db *gorm.DB, sources []*ssadb.IrSource) error {
	if db == nil || len(sources) == 0 {
		return nil
	}
	rows := make([]*ssadb.IrSource, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			rows = append(rows, source)
		}
	}
	// IrSource has no primary or unique key. Save also inserts these rows;
	// CreateInBatches preserves that behavior without requiring a conflict key.
	if err := db.CreateInBatches(rows, riskBatchSize).Error; err != nil {
		return utils.Errorf("save IrSource failed: %w", err)
	}
	return nil
}

func newAuditDatabase(ctx context.Context, db *gorm.DB, size int) *auditDatabase {
	ret := &auditDatabase{}

	saveSize := size * 2
	// Most result graphs are small. Avoid allocating three large queues for
	// every result; large graphs can grow the queues without changing batches.
	queueSize := dbcache.WithInitialBufferSize(64)

	ret.nodeSave = dbcache.NewSave[*ssadb.AuditNode](func(ae []*ssadb.AuditNode) error {
		if len(ae) == 0 {
			return nil
		}
		return batchSaveAuditNodes(db, ae)
	}, dbcache.WithContext(ctx), dbcache.WithSaveSize(saveSize), queueSize, dbcache.WithName("AuditNode"))

	ret.edgeSave = dbcache.NewSave[*ssadb.AuditEdge](func(ae []*ssadb.AuditEdge) error {
		if len(ae) == 0 {
			return nil
		}
		return batchSaveAuditEdges(db, ae)
	}, dbcache.WithContext(ctx), dbcache.WithSaveSize(saveSize), queueSize, dbcache.WithName("AuditEdge"))

	ret.editorSave = dbcache.NewSave[*ssadb.IrSource](func(ae []*ssadb.IrSource) error {
		return saveAuditSources(db, ae)
	}, dbcache.WithContext(ctx), dbcache.WithSaveSize(size), queueSize, dbcache.WithName("SourceFile"))

	return ret
}
