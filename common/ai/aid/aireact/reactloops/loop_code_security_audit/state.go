package loop_code_security_audit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// --------- Finding（扫描阶段产出）---------

// Finding 结构化漏洞发现，与 CLAUDE security review 格式对齐
type Finding struct {
	ID              string `json:"id"`               // e.g. VULN-001
	Module          string `json:"module,omitempty"` // 所属模块
	File            string `json:"file"`
	Line            int    `json:"line"`
	Severity        string `json:"severity"` // HIGH / MEDIUM / LOW
	Category        string `json:"category"` // sql_injection / cmd_injection ...
	Title           string `json:"title"`
	Description     string `json:"description"`
	DataFlow        string `json:"data_flow"`
	ExploitScenario string `json:"exploit_scenario"`
	Recommendation  string `json:"recommendation"`
	Confidence      int    `json:"confidence"` // 1-10
}

// --------- ScanObservation（Phase2 扫描观察，包含 uncertain 线索）---------

// ScanObservation 记录 Phase2 某类别扫描的完整观察记录
// 包含 uncertain 假设（值得人工跟进的线索）和覆盖总结
type ScanObservation struct {
	CategoryID      string           `json:"category_id"`
	CategoryName    string           `json:"category_name"`
	StopReason      string           `json:"stop_reason"`
	CoverageSummary string           `json:"coverage_summary"`
	FindingsSummary string           `json:"findings_summary,omitempty"`
	UncertainLeads  []*UncertainLead `json:"uncertain_leads,omitempty"`
	SafeHypotheses  []string         `json:"safe_hypotheses,omitempty"` // 已排除的，简要列举
	ConfirmedCount  int              `json:"confirmed_count"`
	UncertainCount  int              `json:"uncertain_count"`
	SafeCount       int              `json:"safe_count"`
}

// UncertainLead 表示一个证据不足但值得关注的潜在漏洞线索
type UncertainLead struct {
	HypothesisID string `json:"hypothesis_id"`
	Title        string `json:"title"`
	SinkHint     string `json:"sink_hint"`
	SourceHint   string `json:"source_hint,omitempty"`
	Reason       string `json:"reason"`                 // 为什么标记为 uncertain
	EvidenceLog  string `json:"evidence_log,omitempty"` // 收集到的证据摘要
}

// --------- VerifyResult（验证阶段产出）---------

// VerifyStatus 验证结论
type VerifyStatus string

const (
	VerifyConfirmed VerifyStatus = "confirmed" // 确认漏洞
	VerifySafe      VerifyStatus = "safe"      // 已排除
	VerifyUncertain VerifyStatus = "uncertain" // 需人工确认
)

// VerifiedFinding 经过验证的 Finding
type VerifiedFinding struct {
	Finding    *Finding     `json:"finding"`
	Status     VerifyStatus `json:"status"`
	Confidence int          `json:"confidence"` // 验证后置信度（1-10）
	Reason     string       `json:"reason"`
	DataFlow   string       `json:"data_flow,omitempty"` // 完整数据流（验证后可能更准确）
	Exploit    string       `json:"exploit,omitempty"`   // 利用方式
	Fix        string       `json:"fix,omitempty"`       // 修复建议
}

// --------- AuditState（全局状态）---------

// AuditPhase 当前审计阶段
type AuditPhase string

const (
	AuditPhaseRecon  AuditPhase = "recon"  // Phase 1: 项目探索
	AuditPhaseScan   AuditPhase = "scan"   // Phase 2: 扫描 findings
	AuditPhaseVerify AuditPhase = "verify" // Phase 3: 逐个验证
	AuditPhaseReport AuditPhase = "report" // Phase 4: 生成报告
	AuditPhaseDone   AuditPhase = "done"
)

// AuditState 贯穿四个阶段的共享状态
type AuditState struct {
	mu sync.RWMutex

	Phase AuditPhase `json:"phase"`

	// ProjectPath/ProjectName 由 Phase1 complete_recon action 回填
	// 初始为空，AI 从用户输入中自行识别项目绝对路径
	ProjectPath string `json:"project_path,omitempty"`
	ProjectName string `json:"project_name,omitempty"`

	// WorkDir 是 AI workdir，所有审计输出文件（audit/）都写入此目录下
	WorkDir string `json:"work_dir,omitempty"`

	// Phase 1 产出：简要摘要（常驻内存，注入每轮 reactive data）
	TechStack     string `json:"tech_stack,omitempty"`
	EntryPoints   string `json:"entry_points,omitempty"`
	AuthMechanism string `json:"auth_mechanism,omitempty"`

	// Phase 1 产出：侦察报告大纲（章节列表摘要，内存常驻，注入 Phase2/3 子 loop）
	// 让后续 agent 知道报告有哪些章节，便于决策是否需要 read_recon_notes
	ReconOutline string `json:"recon_outline,omitempty"`

	// Phase 1 产出：详细侦察笔记写入磁盘文件，路径存此处
	// 由 report_generating 子 loop 按大纲写入，内容包含目录结构、路由列表、数据库访问模式等
	ReconFilePath string `json:"recon_file_path,omitempty"`

	// Phase 2 产出：原始 findings 列表
	Findings []*Finding `json:"findings,omitempty"`
	// Phase 2 产出：各类别扫描观察记录（包含 uncertain 线索、覆盖总结）
	ScanObservations []*ScanObservation `json:"scan_observations,omitempty"`
	// Phase 2 产出：findings 持久化文件路径（.audit/scan_findings.json）
	FindingsFilePath string `json:"findings_file_path,omitempty"`
	// Phase 2 产出：扫描观察记录持久化文件路径（.audit/scan_observations.md）
	ScanObservationsFilePath string `json:"scan_observations_file_path,omitempty"`

	// Phase 3 产出：验证后的漏洞列表
	VerifiedVulns []*VerifiedFinding `json:"verified_vulns,omitempty"`
	// Phase 3 产出：verified_vulns 持久化文件路径（.audit/verified_vulns.json）
	VerifiedVulnsFilePath string `json:"verified_vulns_file_path,omitempty"`

	// Phase 4 产出：最终报告
	FinalReport     string `json:"final_report,omitempty"`
	FinalReportPath string `json:"final_report_path,omitempty"`
}

func NewAuditState() *AuditState {
	return &AuditState{
		Phase:         AuditPhaseRecon,
		Findings:      make([]*Finding, 0),
		VerifiedVulns: make([]*VerifiedFinding, 0),
	}
}

// --------- Phase 1 helpers ---------

// SetProjectInfo 由 Phase1 complete_recon action 回填项目路径和名称
func (s *AuditState) SetProjectInfo(path, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path != "" {
		s.ProjectPath = path
	}
	if name != "" {
		s.ProjectName = name
	}
}

func (s *AuditState) SetReconResult(techStack, entryPoints, authMechanism string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TechStack = techStack
	s.EntryPoints = entryPoints
	s.AuthMechanism = authMechanism
	s.Phase = AuditPhaseScan
}

// SetReconOutline 记录侦察报告大纲（章节列表），内存常驻供后续 phase 注入
func (s *AuditState) SetReconOutline(outline string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ReconOutline = outline
}

// GetReconOutline 返回侦察报告大纲
func (s *AuditState) GetReconOutline() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ReconOutline
}

// SetReconFilePath 记录侦察笔记文件路径（由 report_generating 子 loop 写入磁盘后调用）
func (s *AuditState) SetReconFilePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ReconFilePath = path
}

// GetReconFilePath 返回侦察笔记文件路径（供 Phase2/3 注入提示词）
func (s *AuditState) GetReconFilePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ReconFilePath
}

// GetReconFileContent 读取侦察笔记文件内容（供 read_recon_notes action 使用）
func (s *AuditState) GetReconFileContent() (string, error) {
	s.mu.RLock()
	path := s.ReconFilePath
	s.mu.RUnlock()
	if path == "" {
		return "", fmt.Errorf("侦察笔记文件尚未生成（Phase 1 尚未完成或未写入）")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取侦察笔记失败 %s: %w", path, err)
	}
	return string(data), nil
}

// --------- Phase 2 helpers ---------

// AddScanObservation 追加一条类别扫描观察记录
func (s *AuditState) AddScanObservation(obs *ScanObservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ScanObservations = append(s.ScanObservations, obs)
}

// GetScanObservations 返回所有扫描观察记录的只读副本
func (s *AuditState) GetScanObservations() []*ScanObservation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ScanObservation, len(s.ScanObservations))
	copy(result, s.ScanObservations)
	return result
}

// GetScanObservationsFilePath 返回扫描观察持久化文件路径
func (s *AuditState) GetScanObservationsFilePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ScanObservationsFilePath
}

// AddFinding 添加一个扫描发现，自动生成 ID
func (s *AuditState) AddFinding(f *Finding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.ID == "" {
		f.ID = fmt.Sprintf("VULN-%03d", len(s.Findings)+1)
	}
	s.Findings = append(s.Findings, f)
}

// GetFindings 返回所有 findings 的只读副本
func (s *AuditState) GetFindings() []*Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Finding, len(s.Findings))
	copy(result, s.Findings)
	return result
}

// GetFindingByID 按 ID 获取 Finding
func (s *AuditState) GetFindingByID(id string) *Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.Findings {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// --------- Phase 3 helpers ---------

// AddVerifiedFinding 添加验证结果
func (s *AuditState) AddVerifiedFinding(vf *VerifiedFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VerifiedVulns = append(s.VerifiedVulns, vf)
}

// GetVerifiedVulns 返回已验证的漏洞（只读副本）
func (s *AuditState) GetVerifiedVulns() []*VerifiedFinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*VerifiedFinding, len(s.VerifiedVulns))
	copy(result, s.VerifiedVulns)
	return result
}

// GetConfirmedVulns 只返回 confirmed 状态的漏洞
func (s *AuditState) GetConfirmedVulns() []*VerifiedFinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*VerifiedFinding
	for _, v := range s.VerifiedVulns {
		if v.Status == VerifyConfirmed {
			result = append(result, v)
		}
	}
	return result
}

// --------- Phase 4 helpers ---------

func (s *AuditState) SetFinalReport(report string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FinalReport = report
	s.Phase = AuditPhaseDone
}

func (s *AuditState) GetFinalReport() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FinalReport
}

// --------- 统计 ---------

type AuditStats struct {
	TotalFindings  int
	HighCount      int
	MediumCount    int
	LowCount       int
	ConfirmedCount int
	UncertainCount int
	SafeCount      int
}

func (s *AuditState) GetStats() AuditStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := AuditStats{TotalFindings: len(s.Findings)}
	for _, v := range s.VerifiedVulns {
		switch v.Status {
		case VerifyConfirmed:
			stats.ConfirmedCount++
			switch strings.ToUpper(v.Finding.Severity) {
			case "HIGH":
				stats.HighCount++
			case "MEDIUM":
				stats.MediumCount++
			case "LOW":
				stats.LowCount++
			}
		case VerifyUncertain:
			stats.UncertainCount++
		case VerifySafe:
			stats.SafeCount++
		}
	}
	return stats
}

// --------- 持久化 helpers ---------

// PersistScanObservations 将扫描观察记录序列化为 Markdown 写入 filePath，并记录路径。
// 由 orchestrator 在 Phase2 结束后调用。
func (s *AuditState) PersistScanObservations(filePath string) error {
	s.mu.RLock()
	obs := make([]*ScanObservation, len(s.ScanObservations))
	copy(obs, s.ScanObservations)
	s.mu.RUnlock()

	if len(obs) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Phase 2 扫描观察记录\n\n")
	sb.WriteString("> 本文件记录各漏洞类别扫描的完整观察，包括 uncertain 线索（值得人工跟进）、覆盖总结和已排除的假设。\n\n")

	for _, o := range obs {
		sb.WriteString(fmt.Sprintf("---\n\n## 类别：%s（%s）\n\n", o.CategoryName, o.CategoryID))
		sb.WriteString(fmt.Sprintf("- **停止原因**: %s\n", o.StopReason))
		sb.WriteString(fmt.Sprintf("- **假设统计**: confirmed=%d uncertain=%d safe=%d\n", o.ConfirmedCount, o.UncertainCount, o.SafeCount))
		sb.WriteString(fmt.Sprintf("- **覆盖总结**: %s\n", o.CoverageSummary))
		if o.FindingsSummary != "" {
			sb.WriteString(fmt.Sprintf("- **发现总结**: %s\n", o.FindingsSummary))
		}
		sb.WriteString("\n")

		if len(o.UncertainLeads) > 0 {
			sb.WriteString("### Uncertain 线索（证据不足，值得人工跟进）\n\n")
			for _, lead := range o.UncertainLeads {
				sb.WriteString(fmt.Sprintf("#### [%s] %s\n\n", lead.HypothesisID, lead.Title))
				sb.WriteString(fmt.Sprintf("- **Sink**: `%s`\n", lead.SinkHint))
				if lead.SourceHint != "" {
					sb.WriteString(fmt.Sprintf("- **Source**: `%s`\n", lead.SourceHint))
				}
				sb.WriteString(fmt.Sprintf("- **Uncertain 原因**: %s\n", lead.Reason))
				if lead.EvidenceLog != "" {
					sb.WriteString(fmt.Sprintf("- **已收集证据**: %s\n", lead.EvidenceLog))
				}
				sb.WriteString("\n")
			}
		}

		if len(o.SafeHypotheses) > 0 {
			sb.WriteString("### 已排除假设\n\n")
			for _, s := range o.SafeHypotheses {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
			sb.WriteString("\n")
		}
	}

	if err := os.WriteFile(filePath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write scan_observations file: %w", err)
	}
	s.mu.Lock()
	s.ScanObservationsFilePath = filePath
	s.mu.Unlock()
	return nil
}

// PersistFindings 将当前 findings 序列化为 JSON 写入 filePath，并记录路径。
// 由 orchestrator 在 Phase2 结束后调用。
func (s *AuditState) PersistFindings(filePath string) error {
	s.mu.RLock()
	findings := make([]*Finding, len(s.Findings))
	copy(findings, s.Findings)
	s.mu.RUnlock()

	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal findings: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("write findings file: %w", err)
	}
	s.mu.Lock()
	s.FindingsFilePath = filePath
	s.mu.Unlock()
	return nil
}

// PersistVerifiedVulns 将当前 verified_vulns 序列化写入 filePath，并记录路径。
// 由 orchestrator 在 Phase3 结束后调用。
func (s *AuditState) PersistVerifiedVulns(filePath string) error {
	s.mu.RLock()
	vulns := make([]*VerifiedFinding, len(s.VerifiedVulns))
	copy(vulns, s.VerifiedVulns)
	s.mu.RUnlock()

	data, err := json.MarshalIndent(vulns, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal verified_vulns: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("write verified_vulns file: %w", err)
	}
	s.mu.Lock()
	s.VerifiedVulnsFilePath = filePath
	s.mu.Unlock()
	return nil
}

func (s *AuditState) GetFindingsFilePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FindingsFilePath
}

func (s *AuditState) GetVerifiedVulnsFilePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.VerifiedVulnsFilePath
}

func (s *AuditState) SetFinalReportPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FinalReportPath = path
}

func (s *AuditState) GetFinalReportPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FinalReportPath
}

// --------- 模板渲染用 ---------

func (s *AuditState) GetPhase() AuditPhase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Phase
}

func (s *AuditState) SetPhase(p AuditPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Phase = p
}
