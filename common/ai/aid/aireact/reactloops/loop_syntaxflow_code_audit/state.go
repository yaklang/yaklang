package loop_syntaxflow_code_audit

// SFCodeAuditState holds cross-phase context for SyntaxFlow-driven code audit orchestration.
type SFCodeAuditState struct {
	WorkDir string

	ProjectPath   string
	ProjectName   string
	TechStack     string
	EntryPoints   string
	ReconFilePath string

	RuleFilePath string

	// ScanReviewSummary is set when optional syntaxflow_scan sub-loop runs (user gave task_id).
	ScanReviewSummary string

	FinalReportPath string
}

func (s *SFCodeAuditState) SetProjectFromExplore(projectPath, projectName, tech, entry, reconFile string) {
	if projectPath != "" {
		s.ProjectPath = projectPath
	}
	if projectName != "" {
		s.ProjectName = projectName
	}
	s.TechStack = tech
	s.EntryPoints = entry
	if reconFile != "" {
		s.ReconFilePath = reconFile
	}
}

func (s *SFCodeAuditState) SetRulePath(p string) {
	s.RuleFilePath = p
}

func (s *SFCodeAuditState) SetScanReviewSummary(summary string) {
	s.ScanReviewSummary = summary
}
