package loop_ai_skill_audit

import (
	"sync"
)

// SkillAuditState holds shared state across all phases of the AI Skill audit.
type SkillAuditState struct {
	mu sync.RWMutex

	WorkDir   string
	SkillPath string

	// Populated after Phase 1 (dir_explore)
	SkillName      string
	TechStack      string
	EntryPoints    string
	ReconFilePath  string
	ReconNoteFiles []string

	// Populated after Phase 2 (static analysis)
	// RiskLevel is one of: "Clean", "Medium", "High", "Critical"
	RiskLevel       string
	FindingsSummary string
	FindingsXML     string // raw <vuln>...</vuln> XML blocks
	AuditNoteFiles  []string

	// Populated after Phase 3 (report_generating)
	FinalReport     string
	FinalReportPath string
}

// NewSkillAuditState creates an empty SkillAuditState.
func NewSkillAuditState() *SkillAuditState {
	return &SkillAuditState{}
}

func (s *SkillAuditState) SetProjectInfo(skillPath, skillName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SkillPath = skillPath
	s.SkillName = skillName
}

func (s *SkillAuditState) SetReconResult(techStack, entryPoints string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TechStack = techStack
	s.EntryPoints = entryPoints
}

func (s *SkillAuditState) SetReconFilePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ReconFilePath = path
}

func (s *SkillAuditState) AddReconNoteFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.ReconNoteFiles {
		if f == path {
			return
		}
	}
	s.ReconNoteFiles = append(s.ReconNoteFiles, path)
}

func (s *SkillAuditState) AddAuditNoteFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.AuditNoteFiles {
		if f == path {
			return
		}
	}
	s.AuditNoteFiles = append(s.AuditNoteFiles, path)
}

// SetAuditResult stores the outcome of Phase 2 static analysis.
func (s *SkillAuditState) SetAuditResult(riskLevel, findingsSummary, findingsXML string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RiskLevel = riskLevel
	s.FindingsSummary = findingsSummary
	s.FindingsXML = findingsXML
}

func (s *SkillAuditState) SetFinalReport(report string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FinalReport = report
}

func (s *SkillAuditState) SetFinalReportPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FinalReportPath = path
}

func (s *SkillAuditState) GetFinalReport() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FinalReport
}

func (s *SkillAuditState) GetAuditNoteFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.AuditNoteFiles))
	copy(result, s.AuditNoteFiles)
	return result
}

func (s *SkillAuditState) GetReconNoteFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.ReconNoteFiles))
	copy(result, s.ReconNoteFiles)
	return result
}
