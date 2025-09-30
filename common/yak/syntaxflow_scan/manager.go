package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
	lock         sync.Mutex
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
	callback       *ScanTaskCallbacks
}

// var syntaxFlowScanManagerMap = omap.NewEmptyOrderedMap[string, *scanManager]()

func LoadSyntaxflowTaskFromDB(ctx context.Context, runningID string, config *ScanTaskConfig) (*scanManager, error) {
	taskId := config.GetResumeTaskId()
	var manager *scanManager
	// var ok bool
	// if manager, ok = syntaxFlowScanManagerMap.Get(taskId); ok {
	// 	ctx, cancel := context.WithCancel(ctx)
	// 	manager.ctx = ctx
	// 	manager.cancel = cancel
	// } else {
	var err error
	manager, err = createEmptySyntaxFlowTaskByID(ctx, runningID, taskId, config)
	if err != nil {
		return nil, err
	}
	// }
	manager.callback.Set(runningID, config.ScanTaskCallback)
	manager.processMonitor.StartMonitor()
	if err := manager.RestoreTask(); err != nil {
		return nil, err
	}
	return manager, nil
}
func RemoveSyntaxFlowTaskByID(id string) {
	// _, ok := syntaxFlowScanManagerMap.Get(id)
	// if !ok {
	// 	return
	// }
	// syntaxFlowScanManagerMap.Delete(id)
}

func createEmptySyntaxFlowTaskByID(
	ctx context.Context,
	runningID, taskId string,
	config *ScanTaskConfig,
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
		callback:     NewScanTaskCallbacks(),
	}
	m.callback.Set(runningID, config.ScanTaskCallback)
	// syntaxFlowScanManagerMap.Set(taskId, m)
	m.config = config
	// 设置进度回调
	m.processMonitor = newProcessMonitor(ctx, 30*time.Second, func(progress float64, info *RuleProcessInfoList) {
		m.callback.Process(m.taskID, m.status, progress, info)
	})
	return m, nil
}

func createSyntaxflowTaskById(
	ctx context.Context,
	runningID, taskId string,
	config *ScanTaskConfig,
) (*scanManager, error) {
	m, err := createEmptySyntaxFlowTaskByID(ctx, runningID, taskId, config)
	if err != nil {
		return nil, err
	}
	m.processMonitor.StartMonitor()
	if err := m.initByConfig(); err != nil {
		return nil, err
	}
	// 设置扫描批次
	m.setScanBatch()

	return m, nil
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
	if m == nil {
		// return utils.Errorf("manager is nil ")
		return nil
	}
	m.lock.Lock()
	defer m.lock.Unlock()

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
	log.Errorf("save task, success: %d; failed: %d; skip: %d", m.taskRecorder.SuccessQuery, m.taskRecorder.FailedQuery, m.taskRecorder.SkipQuery)
	m.taskRecorder.Kind = m.kind

	m.taskRecorder.Config, _ = json.Marshal(m.config)
	// m.taskRecorder.RuleNames, _ = json.Marshal(m.ruleNames)

	if m.status == schema.SYNTAXFLOWSCAN_DONE || m.status == schema.SYNTAXFLOWSCAN_PAUSED {
		levelCounts, _ := yakit.GetSSARiskLevelCount(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{m.TaskId()},
		})
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

func (m *scanManager) RestoreTask() error {
	// load info
	task, err := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), m.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume SyntaxFlow task by [%s] is failed", m.TaskId())
	}
	m.taskRecorder = task
	m.status = task.Status
	m.status = task.Status
	m.programs = strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)

	// load config
	var config = &ScanTaskConfig{}
	if len(task.Config) == 0 {
		return utils.Errorf("Config is empty")
	}
	if err = json.Unmarshal(task.Config, config); err != nil {
		return utils.Wrapf(err, "Unmarshal SyntaxFlowScan Config: %v", task.Config)
	}
	m.config.SyntaxFlowScanRequest = config.SyntaxFlowScanRequest

	// init rule
	if err := m.initByConfig(); err != nil {
		return utils.Wrapf(err, "initByConfig failed")
	}

	// restore process
	m.SetSuccessQuery(task.SuccessQuery)
	m.SetFailedQuery(task.FailedQuery)
	m.SetSkippedQuery(task.SkipQuery)
	log.Errorf("restore task, success: %d; failed: %d; skip: %d", task.SuccessQuery, task.FailedQuery, task.SkipQuery)
	m.SetFinishedQuery(task.SuccessQuery + task.FailedQuery + task.SkipQuery)
	m.setRiskCount(task.RiskCount)
	m.setTotalQuery(task.TotalQuery)
	m.kind = task.Kind
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
		scanConfig := project.GetScanConfig()
		if scanConfig == nil {
			return utils.Errorf("scan config is nil")
		}

		m.programs = []string{project.ProjectName}
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
		m.concurrency = config.GetConcurrency()
	}

	setRuleChan := func(filter *ypb.SyntaxFlowRuleFilter) error {
		db := consts.GetGormProfileDatabase()
		db = yakit.FilterSyntaxFlowRule(db, filter)
		// get all rule name
		var ruleNames []string
		err := db.Pluck("rule_name", &ruleNames).Error
		if err != nil {
			return utils.Errorf("count rules failed: %s", err)
		}
		m.ruleChan = sfdb.YieldSyntaxFlowRules(db, m.ctx)
		m.rulesCount = int64(len(ruleNames))
		m.kind = schema.SFResultKindScan
		return nil
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
	} else if config.GetFilter() != nil {
		if err := setRuleChan(config.GetFilter()); err != nil {
			return err
		}
	} else if projectId != 0 {
		project, err := yakit.LoadSSAProjectBuilderByID(uint(projectId))
		if err != nil {
			return utils.Errorf("load ssa project by id failed: %s", err)
		}
		if err := setRuleChan(project.GetRuleFilter()); err != nil {
			return err
		}
	} else {
		if err := setRuleChan(nil); err != nil {
			return err
		}
	}

	log.Errorf("rulecount %d ; total query: %v", m.rulesCount, m.rulesCount*int64(len(m.programs)))
	m.setTotalQuery(m.rulesCount * int64(len(m.programs)))
	return nil
}

func (m *scanManager) TaskId() string {
	return m.taskID
}

func (m *scanManager) Stop(runningID string) {
	if m == nil {
		return
	}
	m.cancel()
	m.processMonitor.Close()
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
	pause := m.callback.Pause()
	if pause {
		m.status = schema.SYNTAXFLOWSCAN_PAUSED
	}
	return pause
}

func (m *scanManager) CurrentTaskIndex() int64 {
	return m.GetFinishedQuery()
}

func (m *scanManager) ScanNewTask() error {
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
	log.Errorf("resume task %s from index %d", m.taskID, taskIndex)
	log.Errorf("total query: %d; finish query: %d", m.GetTotalQuery(), m.GetFinishedQuery())
	if taskIndex > m.GetTotalQuery() {
		m.status = schema.SYNTAXFLOWSCAN_DONE
		return nil
	}
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	err := m.StartQuerySF(taskIndex)
	if err != nil {
		return err
	}
	return nil
}

func (m *scanManager) StatusTask(res ...*ssaapi.SyntaxFlowResult) {
	if m == nil {
		return
	}
	var ret *ssaapi.SyntaxFlowResult = nil
	if len(res) > 0 {
		ret = res[0]
	}
	m.notifyResult(ret)
	m.processMonitor.EmitEvent()
}
