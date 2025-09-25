package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type scanManager struct {
	*Config

	// task info
	taskID       string
	status       string
	ctx          context.Context
	resumeSignal *sync.Cond
	isPaused     *utils.AtomicBool
	cancel       context.CancelFunc

	// record {{
	// task record
	taskRecorder *schema.SyntaxFlowScanTask
	// ssaConfig record
	ssaConfig *ssaconfig.Config
	// }}

	// config {{
	kind schema.SyntaxflowResultKind
	// }}

	// rules
	ruleChan   chan *schema.SyntaxFlowRule
	rulesCount int64

	// query execute
	failedQuery   int64 // query failed
	skipQuery     int64 // language not match, skip this rule
	successQuery  int64
	finishedQuery int64 // total finished queries (success + failed + skip)
	// risk
	riskCount    int64
	riskCountMap *utils.SafeMap[int64]
	// query process
	totalQuery int64
	//}}
	client *yaklib.YakitClient // TODO NO NEED client
}

var syntaxFlowScanManagerMap = omap.NewEmptyOrderedMap[string, *scanManager]()

func loadSyntaxFlowTaskFromDB(taskId string, ctx context.Context) (*scanManager, error) {
	// load from cache
	// TODO: 不需要缓存
	if manager, ok := syntaxFlowScanManagerMap.Get(taskId); ok {
		ctx, cancel := context.WithCancel(ctx)
		manager.ctx = ctx
		manager.cancel = cancel
		if err := manager.restoreTask(); err != nil {
			return nil, err
		}
		return manager, nil
	} else {
		// load from db
		m, err := createEmptySyntaxFlowTaskByID(taskId, ctx)
		if err != nil {
			return nil, err
		}
		if err := m.restoreTask(); err != nil {
			return nil, err
		}
		return m, nil
	}
}

func removeSyntaxFlowTaskByID(id string) {
	r, ok := syntaxFlowScanManagerMap.Get(id)
	if !ok {
		return
	}
	r.Stop()
	syntaxFlowScanManagerMap.Delete(id)
}

func createEmptySyntaxFlowTaskByID(
	taskId string, ctx context.Context,
) (*scanManager, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var rootctx, cancel = context.WithCancel(ctx)
	m := &scanManager{
		taskID:       taskId,
		ctx:          rootctx,
		status:       schema.SYNTAXFLOWSCAN_EXECUTING,
		resumeSignal: sync.NewCond(&sync.Mutex{}),
		isPaused:     utils.NewAtomicBool(),
		riskCountMap: utils.NewSafeMap[int64](),
		cancel:       cancel,
	}
	syntaxFlowScanManagerMap.Set(taskId, m)
	return m, nil
}

func createSyntaxFlowTaskByConfig(ctx context.Context, c *Config, taskIds ...string) (*scanManager, error) {
	taskId := ""
	if len(taskId) > 0 {
		taskId = taskIds[0]
	} else {
		taskId = uuid.NewString()
	}
	m, err := createEmptySyntaxFlowTaskByID(taskId, ctx)
	if err != nil {
		return nil, err
	}
	m.Config = c
	err = m.initScanRules()
	if err != nil {
		return nil, err
	}
	m.setScanBatch()
	return m, nil
}

func (m *scanManager) initScanRules() error {
	if m == nil || m.ssaConfig == nil {
		return utils.Error("Valid SyntaxFlow Scan Failed: config is nil")
	}
	if input := m.ssaConfig.GetRuleInput(); input != nil {
		ruleCh := make(chan *schema.SyntaxFlowRule)
		go func() {
			defer close(ruleCh)
			if rule, err := yakit.ParseSyntaxFlowInput(input); err != nil {
				m.client.YakitError("compile rule failed: %s", err)
			} else {
				ruleCh <- rule
			}
		}()
		m.ruleChan = ruleCh
		m.rulesCount = 1
		m.kind = schema.SFResultKindDebug
	} else if ruleNames := m.ssaConfig.GetRuleNames(); len(ruleNames) > 0 {
		// resume task, use ruleNames
		m.ruleChan = sfdb.YieldSyntaxFlowRules(
			yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(),
				nil, yakit.WithSyntaxFlowRuleName(ruleNames...),
			),
			m.ctx,
		)
		m.rulesCount = int64(len(ruleNames))
		m.kind = schema.SFResultKindScan
	} else if filter := m.ssaConfig.GetRuleFilter(); filter != nil {
		db := consts.GetGormProfileDatabase()
		db = yakit.FilterSyntaxFlowRule(db, filter)
		// get all rule name
		var ruleNames []string
		err := db.Pluck("rule_name", &ruleNames).Error
		m.ssaConfig.SetRuleNames(ruleNames)
		if err != nil {
			return utils.Errorf("count rules failed: %s", err)
		}
		m.ruleChan = sfdb.YieldSyntaxFlowRules(
			yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(),
				m.ssaConfig.GetRuleFilter(), yakit.WithSyntaxFlowRuleName(m.ssaConfig.GetRuleNames()...),
			),
			m.ctx,
		)
		m.rulesCount = int64(len(m.ssaConfig.GetRuleNames()))
		m.kind = schema.SFResultKindScan
	}
	m.totalQuery = m.rulesCount * int64(len(m.ProgramNames))
	_ = m.SaveTask()
	return nil
}

// setScanBatch 设置扫描批次号
func (m *scanManager) setScanBatch() {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}

	maxBatch, err := yakit.GetMaxScanBatch(ssadb.GetDB(), m.ProgramNames)
	if err != nil {
		m.taskRecorder.ScanBatch = 1
	} else {
		m.taskRecorder.ScanBatch = maxBatch + 1
	}
}

// SaveTask save task info which is from manager to database
func (m *scanManager) SaveTask() error {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}
	m.taskRecorder.Programs = strings.Join(m.ProgramNames, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.taskRecorder.TaskId = m.taskID
	m.taskRecorder.Status = m.status
	m.taskRecorder.SuccessQuery = atomic.LoadInt64(&m.successQuery)
	m.taskRecorder.FailedQuery = atomic.LoadInt64(&m.failedQuery)
	m.taskRecorder.SkipQuery = atomic.LoadInt64(&m.skipQuery)
	m.taskRecorder.RiskCount = atomic.LoadInt64(&m.riskCount)
	m.taskRecorder.TotalQuery = m.totalQuery
	m.taskRecorder.Kind = m.kind
	m.taskRecorder.Config, _ = json.Marshal(m.ssaConfig)

	if m.status == schema.SYNTAXFLOWSCAN_DONE || m.status == schema.SYNTAXFLOWSCAN_PAUSED {
		levelCounts, err := yakit.GetSSARiskLevelCount(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{m.TaskId()},
		})
		if err != nil {
			return err
		}
		for _, c := range levelCounts {
			switch c.Severity {
			case string(schema.SFR_SEVERITY_INFO):
				m.taskRecorder.InfoCount = c.Count
			case string(schema.SFR_SEVERITY_WARNING):
				m.taskRecorder.WarningCount = c.Count
			case string(schema.SFR_SEVERITY_CRITICAL):
				m.taskRecorder.CriticalCount = c.Count
			case string(schema.SFR_SEVERITY_HIGH):
				m.taskRecorder.HighCount = c.Count
			case string(schema.SFR_SEVERITY_LOW):
				m.taskRecorder.LowCount = c.Count
			}
		}
	}
	return schema.SaveSyntaxFlowScanTask(ssadb.GetDB(), m.taskRecorder)
}

func (m *scanManager) restoreTask() error {
	task, err := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), m.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume SyntaxFlow task by is failed")
	}
	m.taskRecorder = task
	m.status = task.Status
	m.status = task.Status
	m.ProgramNames = strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.successQuery = task.SuccessQuery
	m.failedQuery = task.FailedQuery
	m.skipQuery = task.SkipQuery
	m.finishedQuery = task.SuccessQuery + task.FailedQuery + task.SkipQuery // 计算已完成的查询数
	m.riskCount = task.RiskCount
	m.totalQuery = task.TotalQuery
	m.kind = task.Kind
	m.ssaConfig = &ssaconfig.Config{}
	if len(task.Config) == 0 {
		return utils.Errorf("Config is empty")
	}
	if err = json.Unmarshal(task.Config, m.ssaConfig); err != nil {
		return utils.Wrapf(err, "Unmarshal SyntaxFlowScan Config: %v", task.Config)
	}
	err = m.initScanRules()
	if err != nil {
		return err
	}
	return nil
}

func (m *scanManager) TaskId() string {
	return m.taskID
}

func (m *scanManager) Stop() {
	m.cancel()
}

func (m *scanManager) IsStop() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

// isFinish判断任务是否完成
func (m *scanManager) isFinish() bool {
	return m.finishedQuery >= m.totalQuery
}

func (m *scanManager) Resume() {
	m.isPaused.UnSet()
}
func (m *scanManager) Pause() {
	m.isPaused.Set()
}

func (m *scanManager) IsPause() bool {
	return m.isPaused.IsSet()
}

func (m *scanManager) CurrentTaskIndex() int64 {
	return atomic.LoadInt64(&m.finishedQuery)
}

func (m *scanManager) startScan() error {
	defer m.Stop()
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	// start task
	err := m.startQuerySF()
	if err != nil {
		return err
	}
	return nil
}

func (m *scanManager) resumeTask() error {
	taskIndex := m.CurrentTaskIndex()
	if taskIndex > m.totalQuery {
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.SaveTask()
		return nil
	}
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	err := m.startQuerySF(taskIndex)
	if err != nil {
		return err
	}
	return nil
}

// 规则执行成功
func (m *scanManager) markRuleSuccess() {
	atomic.AddInt64(&m.successQuery, 1)
	atomic.AddInt64(&m.finishedQuery, 1)
}

// 规则执行失败
func (m *scanManager) markRuleFailed() {
	atomic.AddInt64(&m.failedQuery, 1)
	atomic.AddInt64(&m.finishedQuery, 1)
}

// 规则跳过
func (m *scanManager) markRuleSkipped() {
	atomic.AddInt64(&m.skipQuery, 1)
	atomic.AddInt64(&m.finishedQuery, 1)
}

// 添加风险计数（不影响完成计数）
func (m *scanManager) addRiskCount(count int64) {
	atomic.AddInt64(&m.riskCount, count)
}

// 获取当前完成进度（用于调试）
func (m *scanManager) getProgress() (finished, total int64) {
	return atomic.LoadInt64(&m.finishedQuery), m.totalQuery
}

func (m *scanManager) GetRiskCountMap() *utils.SafeMap[int64] {
	return m.riskCountMap
}
