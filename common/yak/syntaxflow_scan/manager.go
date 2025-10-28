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

// convertRulesToChannel 将规则列表转换为channel（避免重复加载）
func convertRulesToChannel(rules []*schema.SyntaxFlowRule, ctx context.Context) chan *schema.SyntaxFlowRule {
	ruleChan := make(chan *schema.SyntaxFlowRule, 10)

	go func() {
		defer close(ruleChan)
		for _, rule := range rules {
			select {
			case ruleChan <- rule:
			case <-ctx.Done():
				log.Info("context cancelled, stop sending rules")
				return
			}
		}
	}()

	return ruleChan
}

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
	}
	m.callback.Set(runningID, config.ScanTaskCallback)
	// syntaxFlowScanManagerMap.Set(taskId, m)
	m.Config = config
	// 设置进度回调
	m.processMonitor = newProcessMonitor(ctx, 30*time.Second, func(progress float64, info *RuleProcessInfoList) {
		m.callback.Process(m.taskID, m.status, progress, info)
	}, m.notifyResult)
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
	log.Errorf("save task : %v", utils.InterfaceToString(m.taskRecorder))
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

	// 步骤1: 加载项目配置（如果有项目ID）
	if err := m.loadProjectConfig(); err != nil {
		return err
	}

	log.Infof("initializing scan task with config: %+v", config.Config.GetRuleInput())

	// 步骤2: 根据不同模式设置规则channel
	if err := m.setupRuleChannel(); err != nil {
		return err
	}

	// 步骤3: 计算总任务数
	programCount := len(config.GetProgramNames())
	totalQuery := m.rulesCount * int64(programCount)
	log.Infof("ruleCount: %d, programCount: %d, totalQuery: %d", m.rulesCount, programCount, totalQuery)
	m.setTotalQuery(totalQuery)

	return nil
}

// loadProjectConfig 加载项目配置
func (m *scanManager) loadProjectConfig() error {
	config := m.Config
	projectId := config.GetProjectID()

	// 如果有项目ID且未指定程序名，从项目加载配置
	if projectId != 0 && len(config.GetProgramNames()) == 0 {
		project, err := yakit.QuerySSAProjectById(projectId)
		if err != nil || project == nil {
			return utils.Errorf("query ssa project by id failed: %s", err)
		}

		projectConfig, err := project.GetConfig()
		if projectConfig == nil || err != nil {
			return utils.Errorf("scan config error: %v", err)
		}

		m.Config.Config = projectConfig
		log.Infof("loaded config from project %d", projectId)
	}

	return nil
}

// setupRuleChannel 根据配置设置规则channel
func (m *scanManager) setupRuleChannel() error {
	config := m.Config

	// 模式1: Debug模式（单规则调试）
	if input := config.GetRuleInput(); input != nil {
		return m.setupDebugMode(input)
	}

	// 模式2: Filter模式（批量扫描）
	if filter := config.GetRuleFilter(); filter != nil {
		return m.setupScanMode(filter)
	}

	// 模式3: Project模式（项目规则）
	if projectId := config.GetProjectID(); projectId != 0 {
		return m.setupProjectMode(uint64(projectId))
	}

	// 模式4: 默认模式（全部规则）
	return m.setupScanMode(nil)
}

// setupDebugMode 设置Debug模式（单规则调试）
func (m *scanManager) setupDebugMode(input *ypb.SyntaxFlowRuleInput) error {
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
	log.Info("setup debug mode: single rule")
	return nil
}

// setupProjectMode 设置Project模式（项目规则）
func (m *scanManager) setupProjectMode(projectId uint64) error {
	project, err := yakit.QuerySSAProjectById(projectId)
	if err != nil {
		return utils.Errorf("load ssa project by id failed: %s", err)
	}

	config, err := project.GetConfig()
	if err != nil {
		return utils.Errorf("get rule filter from project config failed: %s", err)
	}

	return m.setupScanMode(config.GetRuleFilter())
}

// setupScanMode 设置扫描模式（批量规则）
func (m *scanManager) setupScanMode(filter *ypb.SyntaxFlowRuleFilter) error {
	// 步骤1: 创建规则加载器
	loader, err := m.createRuleLoader()
	if err != nil {
		return err
	}

	// 步骤2: 根据加载器类型执行不同的加载策略
	if loader.GetLoaderType() == sfdb.LoaderTypeDatabase {
		return m.loadRulesFromDatabase(loader, filter)
	}

	// OSS或其他类型加载器
	return m.loadRulesFromOSS(loader, filter)
}

// createRuleLoader 创建规则加载器
func (m *scanManager) createRuleLoader() (sfdb.RuleLoader, error) {
	db := consts.GetGormProfileDatabase()

	// 检查是否配置了OSS客户端
	var ossClient sfdb.OSSClient
	if clientInterface := m.Config.GetOSSRuleClient(); clientInterface != nil {
		if client, ok := clientInterface.(sfdb.OSSClient); ok {
			ossClient = client
		}
	}

	// 根据配置创建相应的加载器
	if ossClient != nil {
		log.Info("creating OSS rule loader")
		return sfdb.CreateRuleLoader(sfdb.RuleSourceTypeOSS, ossClient, db), nil
	}

	log.Info("creating database rule loader")
	return sfdb.CreateDefaultRuleLoader(db), nil
}

// loadRulesFromDatabase 从数据库加载规则（轻量计数 + 流式加载）
func (m *scanManager) loadRulesFromDatabase(loader sfdb.RuleLoader, filter *ypb.SyntaxFlowRuleFilter) error {
	db := consts.GetGormProfileDatabase()
	db = yakit.FilterSyntaxFlowRule(db, filter)

	// 轻量计数：只查询规则名称
	var ruleNames []string
	if err := db.Pluck("rule_name", &ruleNames).Error; err != nil {
		return utils.Errorf("count rules failed: %s", err)
	}

	m.rulesCount = int64(len(ruleNames))
	log.Infof("database: counted %d rules", m.rulesCount)

	// 流式加载：使用稳定的 YieldSyntaxFlowRules（避免嵌套 goroutine 和资源竞争）
	// 注意：不使用 convertRuleLoaderToChannel，因为它会引入额外的 goroutine 层
	// 导致 context 取消时出现竞争条件（特别是在循环测试时）
	m.ruleChan = sfdb.YieldSyntaxFlowRules(db, m.ctx)
	m.kind = schema.SFResultKindScan
	return nil
}

// loadRulesFromOSS 从OSS加载规则（一次性加载 + 缓存）
// 同时也会从数据库加载自定义规则，实现 OSS + Database 规则合并
func (m *scanManager) loadRulesFromOSS(loader sfdb.RuleLoader, filter *ypb.SyntaxFlowRuleFilter) error {
	// 一次性加载所有 OSS 规则
	ossRules, err := loader.LoadRules(m.ctx, filter)
	if err != nil {
		log.Warnf("load rules from %s failed: %v, fallback to database", loader.GetLoaderType(), err)
		// 失败则回退到数据库
		dbLoader := sfdb.CreateDefaultRuleLoader(consts.GetGormProfileDatabase())
		return m.loadRulesFromDatabase(dbLoader, filter)
	}

	log.Infof("OSS: loaded %d rules", len(ossRules))

	// 同时从数据库加载自定义规则
	dbLoader := sfdb.CreateDefaultRuleLoader(consts.GetGormProfileDatabase())
	dbRules, err := dbLoader.LoadRules(m.ctx, filter)
	if err != nil {
		log.Warnf("load rules from database failed: %v, using OSS rules only", err)
		// 数据库失败不影响 OSS 规则使用
		dbRules = nil
	} else {
		log.Infof("Database: loaded %d custom rules", len(dbRules))
	}

	// 合并 OSS 规则和数据库规则
	allRules := make([]*schema.SyntaxFlowRule, 0, len(ossRules)+len(dbRules))
	allRules = append(allRules, ossRules...)
	allRules = append(allRules, dbRules...)

	m.rulesCount = int64(len(allRules))
	log.Infof("Total: %d rules (OSS: %d, Database: %d)", len(allRules), len(ossRules), len(dbRules))

	// 使用合并后的规则创建channel（避免重复加载）
	m.ruleChan = convertRulesToChannel(allRules, m.ctx)
	m.kind = schema.SFResultKindScan

	// 关闭加载器
	if err := loader.Close(); err != nil {
		log.Errorf("close rule loader failed: %v", err)
	}

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
