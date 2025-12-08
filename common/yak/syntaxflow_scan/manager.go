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
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type scanManager struct {
	// *Config
	Config *Config

	// task info
	taskID       string
	status       string
	ctx          context.Context
	resumeSignal *sync.Cond
	cancel       context.CancelFunc

	// record {{
	// task record
	lock         sync.Mutex
	taskRecorder *schema.SyntaxFlowScanTask

	// config {{
	kind schema.SyntaxflowResultKind
	// }}

	// runtime {{
	// rules
	ruleChan   chan *schema.SyntaxFlowRule
	rulesCount int64

	// program
	// programs []string
	// concurrency uint32
	//}}

	// process
	processMonitor *processMonitor
	callback       *ScanTaskCallbacks

	// 规则级别的性能统计
	ruleProfiler *diagnostics.Recorder
}

// var syntaxFlowScanManagerMap = omap.NewEmptyOrderedMap[string, *scanManager]()

func LoadSyntaxflowTaskFromDB(ctx context.Context, runningID string, config *Config) (*scanManager, error) {
	taskId := config.GetScanResumeTaskId()
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
	config *Config,
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
		ruleProfiler: diagnostics.NewRecorder(),
	}
	m.callback.Set(runningID, config.ScanTaskCallback)
	// syntaxFlowScanManagerMap.Set(taskId, m)
	m.Config = config
	// 设置进度回调
	eventWithRule := false
	if config != nil && config.ScanTaskCallback != nil {
		eventWithRule = config.ScanTaskCallback.ProcessWithRule
	}
	m.processMonitor = newProcessMonitor(ctx, 3*time.Second, func(progress float64, info *RuleProcessInfoList) {
		m.callback.Process(m.taskID, m.status, progress, info)
	}, m.notifyResult, eventWithRule)
	return m, nil
}

func createSyntaxflowTaskById(
	ctx context.Context,
	runningID, taskId string,
	config *Config,
) (*scanManager, error) {
	m, err := createEmptySyntaxFlowTaskByID(ctx, runningID, taskId, config)
	if err != nil {
		return nil, err
	}
	m.processMonitor.StartMonitor()
	if err := m.initByConfig(); err != nil {
		return nil, err
	}
	m.setScanBatch()

	return m, nil
}

func (m *scanManager) GetConcurrency() uint32 {
	return m.Config.GetScanConcurrency()
}

// setScanBatch 设置扫描批次号
func (m *scanManager) setScanBatch() {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}

	maxBatch, err := yakit.GetMaxScanBatch(ssadb.GetDB(), m.Config.GetProgramNames())
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
	m.taskRecorder.Programs = strings.Join(m.Config.GetProgramNames(), schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.taskRecorder.TaskId = m.taskID
	m.taskRecorder.Status = m.status
	m.taskRecorder.SuccessQuery = m.GetSuccessQuery()
	m.taskRecorder.FailedQuery = m.GetFailedQuery()
	m.taskRecorder.SkipQuery = m.GetSkippedQuery()
	m.taskRecorder.RiskCount = m.GetRiskCount()
	m.taskRecorder.TotalQuery = m.GetTotalQuery()
	m.taskRecorder.Kind = m.kind

	m.taskRecorder.Config, _ = json.Marshal(m.Config)
	// m.taskRecorder.RuleNames, _ = json.Marshal(m.ruleNames)
	m.taskRecorder.ProjectId = m.Config.GetProjectID()
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
	err := schema.SaveSyntaxFlowScanTask(ssadb.GetDB(), m.taskRecorder)
	return err
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
	// m.programs = strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)

	// load config
	var config = &Config{}
	log.Errorf("task config : %v", string(task.Config))
	if len(task.Config) == 0 {
		return utils.Errorf("Config is empty")
	}
	if err = json.Unmarshal(task.Config, config); err != nil {
		return utils.Wrapf(err, "Unmarshal SyntaxFlowScan Config: %v", task.Config)
	}

	m.Config.Config = config.Config
	if err != nil {
		return utils.Wrapf(err, "NewConfig from raw config failed")
	}

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
	config := m.Config
	if config == nil {
		return utils.Errorf("config is nil")
	}

	if len(config.GetProgramNames()) > 0 {
		for _, name := range config.GetProgramNames() {
			prog, err := ssaapi.FromDatabase(name)
			if err != nil {
				log.Errorf("SyntaxFlow Scan Failed: SSA Program [%s] not found in database", name)
			}
			config.Programs = append(config.Programs, prog)
		}
	} else if config.GetProjectID() != 0 {
		// 前端如果没传programName扫描功能默认选择最新的programName进行扫描
		name, err := yakit.QueryLatestSSAProgramNameByProjectId(consts.GetGormSSAProjectDataBase(), config.GetProjectID())
		if err != nil {
			return utils.Errorf("SyntaxFlow Scan Failed: query latest SSA Program Name by Project ID failed: %s", err)
		}
		prog, err := ssaapi.FromDatabase(name)
		if err != nil {
			return utils.Errorf("SyntaxFlow Scan Failed: SSA Program [%s] not found in database", name)
		}
		config.Programs = append(config.Programs, prog)
	}

	if len(config.Programs) == 0 {
		return utils.Errorf("SyntaxFlow Scan Failed: SSA Program is empty")
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

	// log.Errorf("config: %v", config.Config.GetRuleInput())
	if input := config.GetRuleInput(); len(input) != 0 {
		// start debug mode scan task
		ruleCh := make(chan *schema.SyntaxFlowRule)
		go func() {
			defer close(ruleCh)
			for _, rinput := range input {
				if rule, err := yakit.ParseSyntaxFlowInput(rinput); err != nil {
					m.errorCallback("compile rule failed: %s", err)
				} else {
					ruleCh <- rule
				}
			}
		}()
		m.ruleChan = ruleCh
		m.rulesCount = 1
		m.kind = schema.SFResultKindDebug
	} else if config.GetRuleFilter() != nil {
		if err := setRuleChan(config.GetRuleFilter()); err != nil {
			return err
		}
	} else {
		if err := setRuleChan(nil); err != nil {
			return err
		}
	}

	programCount := len(config.Programs)
	log.Errorf("rulecount %d ; total query: %v", m.rulesCount, m.rulesCount*int64(programCount))
	m.setTotalQuery(m.rulesCount * int64(programCount))
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
	m.processMonitor.PublishResult(ret)
	m.processMonitor.EmitEvent()
}
