package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

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
	// task info
	taskID       string
	status       string
	ctx          context.Context
	resumeSignal *sync.Cond
	cancel       context.CancelFunc

	memory bool
	// record {{
	// task record
	taskRecorder *schema.SyntaxFlowScanTask
	// config record
	config *ScanTaskConfig
	// }}

	// config {{
	kind           schema.SyntaxflowResultKind
	ignoreLanguage bool
	// }}

	// runtime {{
	// rules
	ruleChan   chan *schema.SyntaxFlowRule
	rulesCount int64

	// program
	programs []string

	concurrency uint32
	//}}

	// process
	processMonitor *processMonitor
}

var syntaxFlowScanManagerMap = omap.NewEmptyOrderedMap[string, *scanManager]()

func LoadSyntaxflowTaskFromDB(taskId string, ctx context.Context) (*scanManager, error) {
	if manager, ok := syntaxFlowScanManagerMap.Get(taskId); ok {
		ctx, cancel := context.WithCancel(ctx)
		manager.ctx = ctx
		manager.cancel = cancel
		if err := manager.RestoreTask(); err != nil {
			return nil, err
		}
		return manager, nil
	} else {
		m, err := createEmptySyntaxFlowTaskByID(taskId, ctx)
		if err != nil {
			return nil, err
		}
		// load from db
		if err := m.RestoreTask(); err != nil {
			return nil, err
		}
		return m, nil
	}
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
		cancel:       cancel,
	}
	syntaxFlowScanManagerMap.Set(taskId, m)
	return m, nil
}

func createSyntaxflowTaskById(
	taskId string, ctx context.Context,
	config *ScanTaskConfig,
) (*scanManager, error) {
	m, err := createEmptySyntaxFlowTaskByID(taskId, ctx)
	if err != nil {
		return nil, err
	}
	m.config = config
	// 设置进度回调
	m.processMonitor = newProcessMonitor(ctx, config.ProcessMonitorTTL, config.ProcessCallback)
	if err := m.initByConfig(); err != nil {
		return nil, err
	}
	// 设置扫描批次
	m.setScanBatch()

	return m, nil
}

func RemoveSyntaxFlowTaskByID(id string) {
	r, ok := syntaxFlowScanManagerMap.Get(id)
	if !ok {
		return
	}
	r.Stop()
	syntaxFlowScanManagerMap.Delete(id)
}

func (m *scanManager) GetConcurrency() uint32 {
	if m.concurrency == 0 {
		return 5
	}
	return m.concurrency
}

// setScanBatch 设置扫描批次号
func (m *scanManager) setScanBatch() {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}

	maxBatch, err := yakit.GetMaxScanBatch(ssadb.GetDB(), m.programs)
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
	m.taskRecorder.Programs = strings.Join(m.programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.taskRecorder.TaskId = m.taskID
	m.taskRecorder.Status = m.status
	m.taskRecorder.SuccessQuery = m.GetSuccessQuery()
	m.taskRecorder.FailedQuery = m.GetFailedQuery()
	m.taskRecorder.SkipQuery = m.GetSkippedQuery()
	m.taskRecorder.RiskCount = m.GetRiskCount()
	m.taskRecorder.TotalQuery = m.GetTotalQuery()
	m.taskRecorder.Kind = m.kind
	m.taskRecorder.Config, _ = json.Marshal(m.config)
	// m.taskRecorder.RuleNames, _ = json.Marshal(m.ruleNames)

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
				m.taskRecorder.InfoCount += c.Count
			case string(schema.SFR_SEVERITY_WARNING):
				m.taskRecorder.WarningCount += c.Count
			case string(schema.SFR_SEVERITY_CRITICAL):
				m.taskRecorder.CriticalCount += c.Count
			case string(schema.SFR_SEVERITY_HIGH):
				m.taskRecorder.HighCount += c.Count
			case string(schema.SFR_SEVERITY_LOW):
				m.taskRecorder.LowCount += c.Count
			}
		}
	}
	return schema.SaveSyntaxFlowScanTask(ssadb.GetDB(), m.taskRecorder)
}

func (m *scanManager) RestoreTask() error {
	task, err := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), m.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume SyntaxFlow task by is failed")
	}
	m.taskRecorder = task
	m.status = task.Status
	m.status = task.Status
	m.programs = strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.markRuleSuccess(task.SuccessQuery)
	m.markRuleFailed(task.FailedQuery)
	m.markRuleSkipped(task.SkipQuery)
	m.setRiskCount(task.RiskCount)
	m.setTotalQuery(task.TotalQuery)
	m.kind = task.Kind
	m.config = &ScanTaskConfig{}
	if len(task.Config) == 0 {
		return utils.Errorf("Config is empty")
	}
	if err = json.Unmarshal(task.Config, m.config); err != nil {
		return utils.Wrapf(err, "Unmarshal SyntaxFlowScan Config: %v", task.Config)
	}
	if err := m.initByConfig(); err != nil {
		return utils.Wrapf(err, "initByConfig failed")
	}
	return nil
}

func (m *scanManager) initByConfig() error {
	config := m.config
	if config == nil {
		return utils.Errorf("config is nil")
	}

	projectId := config.GetSSAProjectId()
	if projectId != 0 && len(config.GetProgramName()) == 0 {
		// init by project info in db
		project, err := yakit.LoadSSAProjectBuilderByID(uint(projectId))
		if err != nil || project == nil {
			return utils.Errorf("query ssa project by id failed: %s", err)
		}
		m.programs = []string{project.ProjectName}
		scanConfig := project.GetScanConfig()
		if scanConfig == nil {
			return utils.Errorf("scan config is nil")
		}
		m.ignoreLanguage = scanConfig.IgnoreLanguage
		m.memory = scanConfig.Memory
		m.concurrency = scanConfig.Concurrency
	} else {
		// init by stream config
		if len(config.GetProgramName()) == 0 {
			return utils.Errorf("program name is empty")
		}
		m.programs = config.GetProgramName()
		m.ignoreLanguage = config.GetIgnoreLanguage()
		m.memory = config.GetMemory()
		if config.GetConcurrency() != 0 {
			m.concurrency = config.GetConcurrency()
		}
	}

	if input := config.GetRuleInput(); input != nil {
		// start debug mode scan task
		ruleCh := make(chan *schema.SyntaxFlowRule)
		go func() {
			defer close(ruleCh)
			if rule, err := yakit.ParseSyntaxFlowInput(input); err != nil {
				m.errorCallback("compile rule failed: %s", err)
			} else {
				ruleCh <- rule
			}
		}()
		m.ruleChan = ruleCh
		m.rulesCount = 1
		m.kind = schema.SFResultKindDebug
	} else if len(config.RuleNames) != 0 {
		// resume task, use ruleNames
		m.ruleChan = sfdb.YieldSyntaxFlowRules(
			yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(),
				nil, yakit.WithSyntaxFlowRuleName(config.RuleNames...),
			),
			m.ctx,
		)
		m.rulesCount = int64(len(config.RuleNames))
		m.kind = schema.SFResultKindScan
	} else if config.GetFilter() != nil {
		db := consts.GetGormProfileDatabase()
		db = yakit.FilterSyntaxFlowRule(db, config.GetFilter())
		// get all rule name
		var ruleNames []string
		err := db.Pluck("rule_name", &ruleNames).Error
		config.RuleNames = ruleNames
		if err != nil {
			return utils.Errorf("count rules failed: %s", err)
		}
		m.ruleChan = sfdb.YieldSyntaxFlowRules(
			yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(),
				config.GetFilter(), yakit.WithSyntaxFlowRuleName(config.RuleNames...),
			),
			m.ctx,
		)
		m.rulesCount = int64(len(config.RuleNames))
		m.kind = schema.SFResultKindScan
	} else if projectId != 0 {
		db := consts.GetGormProfileDatabase()
		count, err := yakit.GetRuleCountBySSAProjectId(db, uint(config.GetSSAProjectId()))
		if err != nil {
			return utils.Errorf("get rule count by ssa project id failed: %s", err)
		}
		m.rulesCount = count

		m.ruleChan = yakit.YieldSyntaxFlowRulesBySSAProjectId(
			db,
			m.ctx,
			uint(config.GetSSAProjectId()),
		)
		m.rulesCount = count
		m.kind = schema.SFResultKindScan
	} else {
		return utils.Errorf("config is invalid")
	}

	m.setTotalQuery(m.rulesCount * int64(len(m.programs)))
	m.SaveTask()
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

func (m *scanManager) IsPause() bool {
	if m.config.pauseCheck == nil {
		return false
	}
	pause := m.config.pauseCheck()
	if pause {
		m.status = schema.SYNTAXFLOWSCAN_PAUSED
	}
	return pause
}

func (m *scanManager) CurrentTaskIndex() int64 {
	return m.GetFinishedQuery()
}

func (m *scanManager) ScanNewTask() error {
	defer m.Stop()
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	// start task
	err := m.StartQuerySF()
	if err != nil {
		return err
	}
	return nil
}

func (m *scanManager) ResumeTask() error {
	taskIndex := m.CurrentTaskIndex()
	if taskIndex > m.processMonitor.TotalQuery.Load() {
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.SaveTask()
		return nil
	}
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	err := m.StartQuerySF(taskIndex)
	if err != nil {
		return err
	}
	return nil
}

func (m *scanManager) StatusTask() {
	m.notifyResult(nil)
	m.processMonitor.Callback()
}
