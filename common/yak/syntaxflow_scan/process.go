package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type RuleProcessCallback func(progress float64, info *RuleProcessInfoList)

type processMonitor struct {
	ctx    context.Context
	Status *omap.OrderedMap[string, *RuleProcessInfo] `json:"status"`

	// query execute
	FailedQuery   atomic.Int64 // query failed
	SkippedQuery  atomic.Int64 // language not match, skip this rule
	SuccessQuery  atomic.Int64
	FinishedQuery atomic.Int64 // total finished queries (success + failed + skip)
	RiskCount     atomic.Int64 // total risk found
	// query process
	TotalQuery atomic.Int64

	processCallBack RuleProcessCallback
	resultCallback  RuleResultCallback
	monitorTTL      time.Duration
	eventCh         chan struct{}
	resultCh        chan *ssaapi.SyntaxFlowResult
	waitGroup       sync.WaitGroup
	closed          atomic.Bool
}

type RuleResultCallback func(*ssaapi.SyntaxFlowResult)

type RuleProcessInfoList struct {
	Progress      float64            `json:"progress"`
	Time          int64              `json:"time"`
	Rules         []*RuleProcessInfo `json:"rules"`
	FailedQuery   int64              `json:"failed_query"`
	SkippedQuery  int64              `json:"skipped_query"`
	SuccessQuery  int64              `json:"success_query"`
	FinishedQuery int64              `json:"finished_query"`
	TotalQuery    int64              `json:"total_query"`
	RiskCount     int64              `json:"risk_count"`
}

type RuleProcessInfo struct {
	// rule running identity
	RuleName    string `json:"rule_name"`
	ProgramName string `json:"program_name"`

	// time info `json:"rule_name"`
	StartTime  int64 `json:"start_time"`
	UpdateTime int64 `json:"update_time"`
	EndTime    int64 `json:"end_time"`

	// rule Progress `json:""`
	Progress float64 `json:"progress"`
	Info     string  `json:"info"`

	// running status `json:""`
	Finished  bool  `json:"finished"`
	Error     error `json:"error"`
	RiskCount int64 `json:"risk_count"`

	Report bool `json:"-"`
}

func (r *RuleProcessInfo) String() string {
	// json  marshal
	raw, _ := json.Marshal(r)
	return string(raw)
}

func (r RuleProcessInfoList) String() string {
	// json  marshal
	raw, _ := json.MarshalIndent(r, "", "  ")
	return string(raw)
}

func (r *RuleProcessInfo) Key() string {
	return r.RuleName + "@" + r.ProgramName
}

func (pm *processMonitor) Close() {
	if !pm.closed.CompareAndSwap(false, true) {
		// log.Errorf("process monitor closed wait swap fail!!!!")
		pm.waitGroup.Wait()
		return
	}

	log.Infof("process monitor closed wait !!!!")
	close(pm.eventCh)
	close(pm.resultCh)
	pm.waitGroup.Wait()
}

func newProcessMonitor(ctx context.Context, ttl time.Duration, callback RuleProcessCallback, resultCallback RuleResultCallback) *processMonitor {
	pm := &processMonitor{
		ctx:             ctx,
		Status:          omap.NewEmptyOrderedMap[string, *RuleProcessInfo](),
		processCallBack: callback,
		resultCallback:  resultCallback,
		monitorTTL:      ttl,
		eventCh:         make(chan struct{}, 128),
		resultCh:        make(chan *ssaapi.SyntaxFlowResult, 128),
	}
	return pm
}
func (pm *processMonitor) StartMonitor() {
	pm.Status = omap.NewEmptyOrderedMap[string, *RuleProcessInfo]()
	pm.eventCh = make(chan struct{}, 128)
	pm.resultCh = make(chan *ssaapi.SyntaxFlowResult, 128)
	pm.closed.Store(false)

	pm.waitGroup = sync.WaitGroup{}
	pm.waitGroup.Add(1)
	go func() {
		defer pm.waitGroup.Done()
		ticker := time.NewTicker(pm.monitorTTL)
		defer ticker.Stop()

		defer pm.reportProcess() // final event
		defer pm.drainResults()

		for {
			select {
			case <-pm.ctx.Done():
				return
			case _, ok := <-pm.eventCh:
				if !ok {
					return
				}
				pm.reportProcess()
				ticker.Reset(pm.monitorTTL)
			case res, ok := <-pm.resultCh:
				if !ok {
					return
				}
				pm.handleResult(res)
			case <-ticker.C:
				pm.reportProcess()
			}
		}
	}()
}

func (pm *processMonitor) handleResult(res *ssaapi.SyntaxFlowResult) {
	if res == nil {
		return
	}
	if pm.resultCallback != nil {
		// log.Errorf("call resultCallback")
		pm.resultCallback(res)
	}
}

func (pm *processMonitor) drainResults() {
	for pm.resultCh != nil {
		select {
		case res, ok := <-pm.resultCh:
			if !ok {
				pm.resultCh = nil
				return
			}
			pm.handleResult(res)
		default:
			return
		}
	}
}

func (p *processMonitor) reportProcess() {
	if p.processCallBack != nil {
		info := p.snapshotInfoList()
		// log.Errorf("process report process: %v ", info.Progress)
		p.processCallBack(info.Progress, info)
	}
}

func (p *processMonitor) EmitEvent() {
	if p.closed.Load() {
		return
	}
	defer func() {
		if e := recover(); e != nil {
			log.Errorf("err: %v", e)
		}
	}()
	// Build a consistent snapshot at emit time
	// log.Infof("write to eventCh")
	select {
	case p.eventCh <- struct{}{}:
	default:
		// channel full, drop event to avoid blocking
	}
}

func (p *processMonitor) PublishResult(res *ssaapi.SyntaxFlowResult) {
	if p.closed.Load() {
		return
	}
	defer func() {
		if e := recover(); e != nil {
			log.Errorf("err: %v", e)
		}
	}()
	// log.Errorf("write to resultCh")
	select {
	case <-p.ctx.Done():
	case p.resultCh <- res:
	}
}

func (p *processMonitor) snapshotInfoList() *RuleProcessInfoList {
	ret := &RuleProcessInfoList{
		Time:          time.Now().UnixMicro(),
		FailedQuery:   p.FailedQuery.Load(),
		SkippedQuery:  p.SkippedQuery.Load(),
		SuccessQuery:  p.SuccessQuery.Load(),
		FinishedQuery: p.FinishedQuery.Load(),
		TotalQuery:    p.TotalQuery.Load(),
		RiskCount:     p.RiskCount.Load(),
	}
	// progress
	if total := ret.TotalQuery; total == 0 {
		ret.Progress = 0
	} else {
		ret.Progress = float64(ret.FinishedQuery) / float64(total)
	}
	// rule
	ret.Rules = make([]*RuleProcessInfo, 0, p.Status.Len())
	p.Status.ForEach(func(i string, rpi *RuleProcessInfo) bool {
		if rpi.Finished && rpi.Report {
			// already reported, skip
			return true
		}

		if rpi.Finished {
			rpi.Report = true // mark as reported
		}
		ret.Rules = append(ret.Rules, rpi)
		return true
	})
	return ret
}

func (p *processMonitor) UpdateRuleError(program, rule string, err error) {
	key := rule + "@" + program
	ruleInfo, ok := p.Status.Get(key)

	if !ok {
		return
	}

	ruleInfo.Progress = 1
	ruleInfo.Error = err
	ruleInfo.EndTime = time.Now().Unix()
	p.Status.Set(key, ruleInfo)
}

func (p *processMonitor) UpdateRuleStatus(program, rule string, progress float64, info string) {
	key := rule + "@" + program
	statusMap := p.Status

	ruleInfo, ok := statusMap.Get(key)
	if !ok {
		ruleInfo = &RuleProcessInfo{
			RuleName:    rule,
			ProgramName: program,
			StartTime:   time.Now().Unix(),
		}
	}

	ruleInfo.UpdateTime = time.Now().Unix()
	ruleInfo.Progress = progress
	ruleInfo.Info = info

	if progress >= 1.0 {
		ruleInfo.Finished = true
		ruleInfo.EndTime = time.Now().Unix()
	}
	p.Status.Set(key, ruleInfo)
}

// 规则执行成功
func (m *scanManager) markRuleSuccess(num ...int64) {
	count := int64(1)
	if len(num) > 0 {
		count = (num[0])
	}
	m.processMonitor.SuccessQuery.Add(count)
	m.AddFinishedQuery(count)
}

func (m *scanManager) SetSuccessQuery(num int64) {
	m.processMonitor.SuccessQuery.Store(num)
}

func (m *scanManager) GetSuccessQuery() int64 {
	return m.processMonitor.SuccessQuery.Load()
}

// 规则执行失败
func (m *scanManager) markRuleFailed(num ...int64) {
	count := int64(1)
	if len(num) > 0 {
		count = num[0]
	}
	m.processMonitor.FailedQuery.Add(count)
	m.AddFinishedQuery(count)
}

func (m *scanManager) SetFailedQuery(num int64) {
	m.processMonitor.FailedQuery.Store(num)
}

func (m *scanManager) GetFailedQuery() int64 {
	return m.processMonitor.FailedQuery.Load()
}

// 规则跳过
func (m *scanManager) markRuleSkipped(num ...int64) {
	count := int64(1)
	if len(num) > 0 {
		count = num[0]
	}
	m.processMonitor.SkippedQuery.Add(count)
	m.AddFinishedQuery(count)
}

func (m *scanManager) SetSkippedQuery(num int64) {
	m.processMonitor.SkippedQuery.Store(num)
}

func (m *scanManager) GetSkippedQuery() int64 {
	return m.processMonitor.SkippedQuery.Load()
}

func (m *scanManager) setTotalQuery(num int64) {
	m.processMonitor.TotalQuery.Store(num)
}
func (m *scanManager) GetTotalQuery() int64 {
	return m.processMonitor.TotalQuery.Load()
}

func (m *scanManager) setRiskCount(num int64) {
	m.processMonitor.RiskCount.Store(num)
}

func (m *scanManager) GetRiskCount() int64 {
	return m.processMonitor.RiskCount.Load()
}

func (m *scanManager) AddFinishedQuery(num int64) {
	m.processMonitor.FinishedQuery.Add(num)
	if m.processMonitor.FinishedQuery.Load() >= m.processMonitor.TotalQuery.Load() {
		m.status = schema.SYNTAXFLOWSCAN_DONE
	}
}

func (m *scanManager) SetFinishedQuery(num int64) {
	m.processMonitor.FinishedQuery.Store(num)
	if m.processMonitor.FinishedQuery.Load() >= m.processMonitor.TotalQuery.Load() {
		m.status = schema.SYNTAXFLOWSCAN_DONE
	}
}

func (m *scanManager) GetFinishedQuery() int64 {
	return m.processMonitor.FinishedQuery.Load()
}
