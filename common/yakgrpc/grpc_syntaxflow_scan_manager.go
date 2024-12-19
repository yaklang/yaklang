package yakgrpc

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
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowScanManager struct {
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
	// config record
	config *ypb.SyntaxFlowScanRequest
	// }}

	// config {{
	kind           schema.SyntaxflowResultKind
	ignoreLanguage bool
	// }}

	// runtime {{
	// stream
	stream ypb.Yak_SyntaxFlowScanServer
	client *yaklib.YakitClient

	// rules
	ruleChan   chan *schema.SyntaxFlowRule
	rulesCount int64

	// program
	programs []string

	// query execute
	failedQuery  int64 // query failed
	skipQuery    int64 // language not match, skip this rule
	successQuery int64
	// risk
	riskCount int64
	// query process
	totalQuery int64

	//}}
}

var syntaxFlowScanManagerMap = omap.NewEmptyOrderedMap[string, *SyntaxFlowScanManager]()

func LoadSyntaxflowTaskFromDB(taskId string, ctx context.Context, stream ypb.Yak_SyntaxFlowScanServer) (*SyntaxFlowScanManager, error) {
	if manager, ok := syntaxFlowScanManagerMap.Get(taskId); ok {
		ctx, cancel := context.WithCancel(ctx)
		manager.ctx = ctx
		manager.cancel = cancel
		if err := manager.RestoreTask(stream); err != nil {
			return nil, err
		}
		return manager, nil
	} else {
		m, err := createEmptySyntaxFlowTaskByID(taskId, ctx)
		if err != nil {
			return nil, err
		}
		// load from db
		if err := m.RestoreTask(stream); err != nil {
			return nil, err
		}
		return m, nil
	}
}

func createEmptySyntaxFlowTaskByID(
	taskId string, ctx context.Context,
) (*SyntaxFlowScanManager, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var rootctx, cancel = context.WithCancel(ctx)
	m := &SyntaxFlowScanManager{
		taskID:       taskId,
		ctx:          rootctx,
		status:       schema.SYNTAXFLOWSCAN_EXECUTING,
		resumeSignal: sync.NewCond(&sync.Mutex{}),
		isPaused:     utils.NewAtomicBool(),
		cancel:       cancel,
	}
	syntaxFlowScanManagerMap.Set(taskId, m)
	return m, nil
}

func CreateSyntaxflowTaskById(
	taskId string, ctx context.Context,
	config *ypb.SyntaxFlowScanRequest,
	stream ypb.Yak_SyntaxFlowScanServer,
) (*SyntaxFlowScanManager, error) {
	m, err := createEmptySyntaxFlowTaskByID(taskId, ctx)
	if err != nil {
		return nil, err
	}
	if err := m.initByConfig(config, stream); err != nil {
		return nil, err
	}
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

// SaveTask save task info which is from manager to database
func (m *SyntaxFlowScanManager) SaveTask() error {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}
	m.taskRecorder.Programs = strings.Join(m.programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.taskRecorder.TaskId = m.taskID
	m.taskRecorder.Status = m.status
	m.taskRecorder.SuccessQuery = m.successQuery
	m.taskRecorder.FailedQuery = m.failedQuery
	m.taskRecorder.SkipQuery = m.skipQuery
	m.taskRecorder.RiskCount = m.riskCount
	m.taskRecorder.TotalQuery = m.totalQuery
	m.taskRecorder.Kind = m.kind
	m.taskRecorder.Config, _ = json.Marshal(m.config)
	return schema.SaveSyntaxFlowScanTask(ssadb.GetDB(), m.taskRecorder)
}

func (m *SyntaxFlowScanManager) RestoreTask(stream ypb.Yak_SyntaxFlowScanServer) error {
	task, err := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), m.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume SyntaxFlow task by is failed")
	}
	m.taskRecorder = task
	m.status = task.Status
	m.status = task.Status
	m.programs = strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.successQuery = task.SuccessQuery
	m.failedQuery = task.FailedQuery
	m.skipQuery = task.SkipQuery
	m.riskCount = task.RiskCount
	m.totalQuery = task.TotalQuery
	m.kind = task.Kind
	m.config = &ypb.SyntaxFlowScanRequest{}
	if len(task.Config) == 0 {
		return utils.Errorf("Config is empty")
	}
	err = json.Unmarshal(task.Config, m.config)
	if err != nil {
		return utils.Wrapf(err, "Unmarshal SyntaxFlowScan Config: %v", task.Config)
	}
	m.initByConfig(m.config, stream)
	return nil
}

func (m *SyntaxFlowScanManager) initByConfig(config *ypb.SyntaxFlowScanRequest, stream ypb.Yak_SyntaxFlowScanServer) error {
	// init by config
	if len(config.GetProgramName()) == 0 {
		return utils.Errorf("program name is empty")
	}
	m.programs = config.GetProgramName()
	m.ignoreLanguage = config.GetIgnoreLanguage()

	// init by stream
	taskId := m.TaskId()

	m.stream = stream
	m.config = config
	yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = taskId
		return m.stream.Send(&ypb.SyntaxFlowScanResponse{
			TaskID:     taskId,
			Status:     m.status,
			ExecResult: result,
		})
	}, taskId)
	m.client = yakitClient

	if input := config.GetRuleInput(); input != nil {
		ruleCh := make(chan *schema.SyntaxFlowRule)
		go func() {
			defer close(ruleCh)
			if rule, err := ParseSyntaxFlowInput(input); err != nil {
				m.client.YakitError("compile rule failed: %s", err)
			} else {
				ruleCh <- rule
			}
		}()
		m.ruleChan = ruleCh
		m.rulesCount = 1
		m.kind = schema.SFResultKindDebug
	} else if config.GetFilter() != nil {
		m.ruleChan = sfdb.YieldSyntaxFlowRules(
			yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(), config.GetFilter()),
			m.ctx,
		)
		if rulesCount, err := yakit.QuerySyntaxFlowRuleCount(consts.GetGormProfileDatabase(), config.GetFilter()); err != nil {
			return utils.Errorf("count rules failed: %s", err)
		} else {
			m.rulesCount = rulesCount
		}
		m.kind = schema.SFResultKindScan
	}
	m.totalQuery = m.rulesCount * int64(len(m.programs))
	m.SaveTask()

	return nil
}

func (m *SyntaxFlowScanManager) TaskId() string {
	return m.taskID
}

func (m *SyntaxFlowScanManager) Stop() {
	m.cancel()
}

func (m *SyntaxFlowScanManager) IsStop() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

func (m *SyntaxFlowScanManager) Resume() {
	m.isPaused.UnSet()
}
func (m *SyntaxFlowScanManager) Pause() {
	m.isPaused.Set()
}

func (m *SyntaxFlowScanManager) IsPause() bool {
	return m.isPaused.IsSet()
}

func (m *SyntaxFlowScanManager) CurrentTaskIndex() int64 {
	return m.skipQuery + m.failedQuery + m.successQuery
}

func (m *SyntaxFlowScanManager) ScanNewTask() error {
	defer m.Stop()
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	// start task
	err := m.StartQuerySF()
	if err != nil {
		return err
	}
	return nil
}

func (m *SyntaxFlowScanManager) ResumeTask() error {
	taskIndex := m.CurrentTaskIndex()
	if taskIndex > m.totalQuery {
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

func (m *SyntaxFlowScanManager) StatusTask() error {
	m.notifyStatus("")
	return nil
}
