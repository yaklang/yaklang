package loop_ssa_api_discovery

import (
	"sync"
)

// PipelineState 贯穿编排器各阶段的轻量元数据（详细数据在 SQLite）。
// Deprecated JSON path fields (VulnChecklistPath 等) 仅作导出镜像路径缓存；阶段间读取请用 Repo / discovery_read_session_data。
type PipelineState struct {
	mu sync.RWMutex

	WorkDir     string
	SQLitePath  string
	SessionUUID string

	DiscoveryReportPath string
	SyntaxFlowJSONPath  string
	FinalReportPath     string

	// Phase5 sub-step report paths
	// Deprecated: 导出镜像路径；真源为 vuln_checklist_items 表。
	VulnChecklistPath     string
	Step0ReportPath       string
	Step1AuthReportPath   string
	Step2VerifyReportPath string
	Step3GreyboxReportPath string

	GreyboxExecuted bool

	Phase4ModeRaw string
	// DeepMiningDoneAPIs tracks verified_http_api ids that passed finalize_endpoint_deep_mining.
	DeepMiningDoneAPIs map[uint]struct{}

	SummaryLog []string
}

func (p *PipelineState) SetGreyboxExecuted(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.GreyboxExecuted = v
}

func (p *PipelineState) GetGreyboxExecuted() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.GreyboxExecuted
}

func NewPipelineState(workDir, sqlitePath, sessionUUID string) *PipelineState {
	return &PipelineState{
		WorkDir:            workDir,
		SQLitePath:         sqlitePath,
		SessionUUID:        sessionUUID,
		DeepMiningDoneAPIs: map[uint]struct{}{},
	}
}

func (p *PipelineState) SetPhase4Mode(mode string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Phase4ModeRaw = NormalizePhase4Mode(mode)
}

func (p *PipelineState) MarkDeepMiningDone(apiID uint) {
	if apiID == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.DeepMiningDoneAPIs == nil {
		p.DeepMiningDoneAPIs = map[uint]struct{}{}
	}
	p.DeepMiningDoneAPIs[apiID] = struct{}{}
}

func (p *PipelineState) IsDeepMiningDone(apiID uint) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.DeepMiningDoneAPIs == nil {
		return false
	}
	_, ok := p.DeepMiningDoneAPIs[apiID]
	return ok
}

func (p *PipelineState) CountDeepMiningDone() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.DeepMiningDoneAPIs)
}

func (p *PipelineState) SetDiscoveryReportPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.DiscoveryReportPath = path
}

func (p *PipelineState) GetDiscoveryReportPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.DiscoveryReportPath
}

func (p *PipelineState) SetSyntaxFlowJSONPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.SyntaxFlowJSONPath = path
}

func (p *PipelineState) GetSyntaxFlowJSONPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.SyntaxFlowJSONPath
}

func (p *PipelineState) SetFinalReportPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.FinalReportPath = path
}

func (p *PipelineState) GetFinalReportPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.FinalReportPath
}

func (p *PipelineState) SetVulnChecklistPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.VulnChecklistPath = path
}

func (p *PipelineState) GetVulnChecklistPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.VulnChecklistPath
}

func (p *PipelineState) SetStep0ReportPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Step0ReportPath = path
}

func (p *PipelineState) GetStep0ReportPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Step0ReportPath
}

func (p *PipelineState) SetStep1AuthReportPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Step1AuthReportPath = path
}

func (p *PipelineState) GetStep1AuthReportPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Step1AuthReportPath
}

func (p *PipelineState) SetStep2VerifyReportPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Step2VerifyReportPath = path
}

func (p *PipelineState) GetStep2VerifyReportPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Step2VerifyReportPath
}

func (p *PipelineState) SetStep3GreyboxReportPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Step3GreyboxReportPath = path
}

func (p *PipelineState) GetStep3GreyboxReportPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Step3GreyboxReportPath
}

func (p *PipelineState) AppendSummaryLog(entry string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.SummaryLog = append(p.SummaryLog, entry)
}

func (p *PipelineState) GetSummaryLog() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.SummaryLog))
	copy(out, p.SummaryLog)
	return out
}
