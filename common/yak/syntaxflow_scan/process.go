package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type RuleProcessCallback func(progress float64, status ProcessStatus, info *RuleProcessInfoList)

type processMonitor struct {
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
}

type ProcessStatus string

const (
	ProcessStatusProgress ProcessStatus = "progress"
	ProcessStatusRuleInfo ProcessStatus = "rule_info"
)

type RuleProcessInfoList struct {
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

func newProcessMonitor(ctx context.Context, ttl time.Duration, callback RuleProcessCallback) *processMonitor {
	ret := &processMonitor{
		Status:          omap.NewEmptyOrderedMap[string, *RuleProcessInfo](),
		processCallBack: callback,
	}

	go func() {
		ticker := time.NewTicker(ttl)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ret.Callback()
			}
		}
	}()

	return ret
}

func (p *processMonitor) Process() float64 {
	log.Infof("process: finished %d / total %d", p.FinishedQuery.Load(), p.TotalQuery.Load())
	return float64(p.FinishedQuery.Load()) / float64(p.TotalQuery.Load())
}

func (p *processMonitor) Callback(infos ...*RuleProcessInfo) {
	process := p.Process()
	if p.processCallBack != nil {
		// foreach status, if update time > ttl, callback it
		var status ProcessStatus
		var tmp = &RuleProcessInfoList{
			Time:          time.Now().UnixMicro(),
			FailedQuery:   p.FailedQuery.Load(),
			SkippedQuery:  p.SkippedQuery.Load(),
			SuccessQuery:  p.SuccessQuery.Load(),
			FinishedQuery: p.FinishedQuery.Load(),
			TotalQuery:    p.TotalQuery.Load(),
			RiskCount:     p.RiskCount.Load(),
		}
		if len(infos) != 0 {
			status = ProcessStatusRuleInfo
			tmp.Rules = infos
		} else {
			status = ProcessStatusProgress
			tmp.Rules = p.Status.Filter(func(s string, rpi *RuleProcessInfo) (bool, error) {
				return !rpi.Finished, nil
			}).Values()
		}
		p.processCallBack(process, status, tmp)
	}
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
		defer p.Callback(ruleInfo)
	}

	ruleInfo.UpdateTime = time.Now().Unix()
	ruleInfo.Progress = progress
	ruleInfo.Info = info

	if progress >= 1.0 {
		ruleInfo.Finished = true
		ruleInfo.EndTime = time.Now().Unix()
		defer p.Callback(ruleInfo)
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

func (m *scanManager) GetFailedQuery() int64 {
	return m.processMonitor.FailedQuery.Load()
}

// 规则跳过
func (m *scanManager) markRuleSkipped(num ...int64) {
	count := int64(1)
	if len(num) > 0 {
		count = num[0]
	}
	log.Infof("add skip query: %d", count)
	m.processMonitor.SkippedQuery.Add(count)
	m.AddFinishedQuery(count)
}

func (m *scanManager) GetSkippedQuery() int64 {
	return m.processMonitor.SkippedQuery.Load()
}

func (m *scanManager) setTotalQuery(num int64) {
	log.Errorf("set total query: %d", num)
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
	log.Infof("add finished query: %d", num)
	m.processMonitor.FinishedQuery.Add(num)
	if m.processMonitor.FinishedQuery.Load() >= m.processMonitor.TotalQuery.Load() {
		m.status = schema.SYNTAXFLOWSCAN_DONE
	}
}

func (m *scanManager) GetFinishedQuery() int64 {
	return m.processMonitor.FinishedQuery.Load()
}
