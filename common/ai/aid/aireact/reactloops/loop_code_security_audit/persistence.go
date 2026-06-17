package loop_code_security_audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const auditStateFileName = "audit_state.json"

// auditStateSnapshot is the on-disk JSON representation of AuditState (no mutex).
type auditStateSnapshot struct {
	Phase                    AuditPhase         `json:"phase"`
	ProjectPath              string             `json:"project_path,omitempty"`
	ProjectName              string             `json:"project_name,omitempty"`
	WorkDir                  string             `json:"work_dir,omitempty"`
	TechStack                string             `json:"tech_stack,omitempty"`
	EntryPoints              string             `json:"entry_points,omitempty"`
	AuthMechanism            string             `json:"auth_mechanism,omitempty"`
	ReconOutline             string             `json:"recon_outline,omitempty"`
	ReconFilePath            string             `json:"recon_file_path,omitempty"`
	ReconNoteFiles           []string           `json:"recon_note_files,omitempty"`
	Findings                 []*Finding         `json:"findings,omitempty"`
	ScanObservations         []*ScanObservation `json:"scan_observations,omitempty"`
	FindingsFilePath         string             `json:"findings_file_path,omitempty"`
	ScanObservationsFilePath string             `json:"scan_observations_file_path,omitempty"`
	VerifiedVulns            []*VerifiedFinding `json:"verified_vulns,omitempty"`
	VerifiedVulnsFilePath    string             `json:"verified_vulns_file_path,omitempty"`
	FinalReport              string             `json:"final_report,omitempty"`
	FinalReportPath          string             `json:"final_report_path,omitempty"`
}

func (s *AuditState) toSnapshot() *auditStateSnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := &auditStateSnapshot{
		Phase:                    s.Phase,
		ProjectPath:              s.ProjectPath,
		ProjectName:              s.ProjectName,
		WorkDir:                  s.WorkDir,
		TechStack:                s.TechStack,
		EntryPoints:              s.EntryPoints,
		AuthMechanism:            s.AuthMechanism,
		ReconOutline:             s.ReconOutline,
		ReconFilePath:            s.ReconFilePath,
		FindingsFilePath:         s.FindingsFilePath,
		ScanObservationsFilePath: s.ScanObservationsFilePath,
		VerifiedVulnsFilePath:    s.VerifiedVulnsFilePath,
		FinalReport:              s.FinalReport,
		FinalReportPath:          s.FinalReportPath,
	}
	if len(s.ReconNoteFiles) > 0 {
		snap.ReconNoteFiles = append([]string(nil), s.ReconNoteFiles...)
	}
	if len(s.Findings) > 0 {
		snap.Findings = append([]*Finding(nil), s.Findings...)
	}
	if len(s.ScanObservations) > 0 {
		snap.ScanObservations = append([]*ScanObservation(nil), s.ScanObservations...)
	}
	if len(s.VerifiedVulns) > 0 {
		snap.VerifiedVulns = append([]*VerifiedFinding(nil), s.VerifiedVulns...)
	}
	return snap
}

func auditStateFromSnapshot(snap *auditStateSnapshot) *AuditState {
	if snap == nil {
		return nil
	}
	state := NewAuditState()
	state.Phase = snap.Phase
	state.ProjectPath = snap.ProjectPath
	state.ProjectName = snap.ProjectName
	state.WorkDir = snap.WorkDir
	state.TechStack = snap.TechStack
	state.EntryPoints = snap.EntryPoints
	state.AuthMechanism = snap.AuthMechanism
	state.ReconOutline = snap.ReconOutline
	state.ReconFilePath = snap.ReconFilePath
	state.FindingsFilePath = snap.FindingsFilePath
	state.ScanObservationsFilePath = snap.ScanObservationsFilePath
	state.VerifiedVulnsFilePath = snap.VerifiedVulnsFilePath
	state.FinalReport = snap.FinalReport
	state.FinalReportPath = snap.FinalReportPath
	if len(snap.ReconNoteFiles) > 0 {
		state.ReconNoteFiles = append([]string(nil), snap.ReconNoteFiles...)
	}
	if len(snap.Findings) > 0 {
		state.Findings = append([]*Finding(nil), snap.Findings...)
	}
	if len(snap.ScanObservations) > 0 {
		state.ScanObservations = append([]*ScanObservation(nil), snap.ScanObservations...)
	}
	if len(snap.VerifiedVulns) > 0 {
		state.VerifiedVulns = append([]*VerifiedFinding(nil), snap.VerifiedVulns...)
	}
	return state
}

// PersistToAuditDir writes audit state to auditDir/audit_state.json for follow-up turns.
func (s *AuditState) PersistToAuditDir(auditDirPath string) error {
	if s == nil || strings.TrimSpace(auditDirPath) == "" {
		return nil
	}
	snap := s.toSnapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(auditDirPath, auditStateFileName), data, 0o644)
}

// TryLoadAuditStateFromWorkDir restores a completed audit state from the session workdir.
// Returns false when no completed audit artifacts are found.
func TryLoadAuditStateFromWorkDir(workDir string) (*AuditState, bool) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return nil, false
	}
	auditDirPath := filepath.Join(workDir, "audit")
	statePath := filepath.Join(auditDirPath, auditStateFileName)
	if data, err := os.ReadFile(statePath); err == nil {
		var snap auditStateSnapshot
		if json.Unmarshal(data, &snap) == nil && snap.Phase == AuditPhaseDone {
			state := auditStateFromSnapshot(&snap)
			state.WorkDir = workDir
			hydrateStateFromArtifactFiles(state, auditDirPath)
			return state, true
		}
	}
	return reconstructCompletedStateFromArtifacts(auditDirPath, workDir)
}

func hydrateStateFromArtifactFiles(state *AuditState, auditDirPath string) {
	if state == nil {
		return
	}
	if state.GetFinalReportPath() == "" {
		p := filepath.Join(auditDirPath, "security_audit_report.md")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			state.SetFinalReportPath(p)
			if state.GetFinalReport() == "" {
				if content, err := os.ReadFile(p); err == nil {
					state.SetFinalReport(string(content))
				}
			}
		}
	}
	if state.GetVerifiedVulnsFilePath() == "" {
		p := filepath.Join(auditDirPath, "verified_vulns.json")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			state.VerifiedVulnsFilePath = p
		}
	}
	if state.GetFindingsFilePath() == "" {
		p := filepath.Join(auditDirPath, "scan_findings.json")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			state.FindingsFilePath = p
		}
	}
	if state.GetReconFilePath() == "" {
		p := filepath.Join(auditDirPath, "recon_notes.md")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			state.SetReconFilePath(p)
		}
	}
}

func reconstructCompletedStateFromArtifacts(auditDirPath, workDir string) (*AuditState, bool) {
	reportPath := filepath.Join(auditDirPath, "security_audit_report.md")
	st, err := os.Stat(reportPath)
	if err != nil || st.IsDir() {
		return nil, false
	}
	state := NewAuditState()
	state.WorkDir = workDir
	state.SetFinalReportPath(reportPath)
	if content, err := os.ReadFile(reportPath); err == nil {
		state.SetFinalReport(string(content))
	} else {
		state.SetPhase(AuditPhaseDone)
	}
	hydrateStateFromArtifactFiles(state, auditDirPath)
	return state, state.GetPhase() == AuditPhaseDone
}
