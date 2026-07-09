package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

// AuditStateFileName is the on-disk snapshot filename under audit/.
const AuditStateFileName = "audit_state.json"

type auditStateSnapshot struct {
	Phase                 model.AuditPhase         `json:"phase"`
	ProjectPath           string                   `json:"project_path,omitempty"`
	ProjectName           string                   `json:"project_name,omitempty"`
	WorkDir               string                   `json:"work_dir,omitempty"`
	TechStack             string                   `json:"tech_stack,omitempty"`
	EntryPoints           string                   `json:"entry_points,omitempty"`
	AuthMechanism         string                   `json:"auth_mechanism,omitempty"`
	ReconOutline          string                   `json:"recon_outline,omitempty"`
	ReconFilePath         string                   `json:"recon_file_path,omitempty"`
	ReconNoteFiles        []string                 `json:"recon_note_files,omitempty"`
	Findings              []*model.Finding         `json:"findings,omitempty"`
	ScanObservations      []*model.ScanObservation `json:"scan_observations,omitempty"`
	FindingsFilePath      string                   `json:"findings_file_path,omitempty"`
	VerifiedVulns         []*model.VerifiedFinding `json:"verified_vulns,omitempty"`
	VerifiedVulnsFilePath string                   `json:"verified_vulns_file_path,omitempty"`
	FinalReport           string                   `json:"final_report,omitempty"`
	FinalReportPath       string                   `json:"final_report_path,omitempty"`
}

func toSnapshot(s *model.AuditState) *auditStateSnapshot {
	if s == nil {
		return nil
	}
	snap := &auditStateSnapshot{
		Phase:                 s.GetPhase(),
		ProjectPath:           s.ProjectPath,
		ProjectName:           s.ProjectName,
		WorkDir:               s.WorkDir,
		TechStack:             s.TechStack,
		EntryPoints:           s.EntryPoints,
		AuthMechanism:         s.AuthMechanism,
		ReconOutline:          s.GetReconOutline(),
		ReconFilePath:         s.GetReconFilePath(),
		FindingsFilePath:      s.GetFindingsFilePath(),
		VerifiedVulnsFilePath: s.GetVerifiedVulnsFilePath(),
		FinalReport:           s.GetFinalReport(),
		FinalReportPath:       s.GetFinalReportPath(),
	}
	for _, f := range s.GetReconNoteFiles() {
		snap.ReconNoteFiles = append(snap.ReconNoteFiles, f)
	}
	for _, f := range s.GetFindings() {
		snap.Findings = append(snap.Findings, f)
	}
	for _, o := range s.GetScanObservations() {
		snap.ScanObservations = append(snap.ScanObservations, o)
	}
	for _, v := range s.GetVerifiedVulns() {
		snap.VerifiedVulns = append(snap.VerifiedVulns, v)
	}
	return snap
}

func fromSnapshot(snap *auditStateSnapshot) *model.AuditState {
	if snap == nil {
		return nil
	}
	state := model.NewAuditState()
	state.SetPhase(snap.Phase)
	state.SetProjectInfo(snap.ProjectPath, snap.ProjectName)
	state.WorkDir = snap.WorkDir
	state.SetReconResult(snap.TechStack, snap.EntryPoints, snap.AuthMechanism)
	state.SetReconOutline(snap.ReconOutline)
	state.SetReconFilePath(snap.ReconFilePath)
	state.FindingsFilePath = snap.FindingsFilePath
	state.VerifiedVulnsFilePath = snap.VerifiedVulnsFilePath
	state.SetFinalReportPath(snap.FinalReportPath)
	if snap.FinalReport != "" {
		state.SetFinalReport(snap.FinalReport)
	}
	for _, f := range snap.ReconNoteFiles {
		state.AddReconNoteFile(f)
	}
	for _, f := range snap.Findings {
		state.AddFinding(f)
	}
	for _, o := range snap.ScanObservations {
		state.AddScanObservation(o)
	}
	for _, v := range snap.VerifiedVulns {
		state.UpsertVerifiedFinding(v)
	}
	return state
}

// PersistToAuditDir writes audit state to auditDir/audit_state.json for follow-up turns.
func PersistToAuditDir(s *model.AuditState, auditDirPath string) error {
	if s == nil || strings.TrimSpace(auditDirPath) == "" {
		return nil
	}
	data, err := json.MarshalIndent(toSnapshot(s), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(auditDirPath, AuditStateFileName), data, 0o644)
}

// TryLoadAuditStateFromWorkDir restores a completed audit state from the session workdir.
func TryLoadAuditStateFromWorkDir(workDir string) (*model.AuditState, bool) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return nil, false
	}
	auditDirPath := filepath.Join(workDir, "audit")
	statePath := filepath.Join(auditDirPath, AuditStateFileName)
	if data, err := os.ReadFile(statePath); err == nil {
		var snap auditStateSnapshot
		if json.Unmarshal(data, &snap) == nil && snap.Phase == model.AuditPhaseDone {
			state := fromSnapshot(&snap)
			state.WorkDir = workDir
			hydrateStateFromArtifactFiles(state, auditDirPath)
			return state, true
		}
	}
	return reconstructCompletedStateFromArtifacts(auditDirPath, workDir)
}

func hydrateStateFromArtifactFiles(state *model.AuditState, auditDirPath string) {
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

func reconstructCompletedStateFromArtifacts(auditDirPath, workDir string) (*model.AuditState, bool) {
	reportPath := filepath.Join(auditDirPath, "security_audit_report.md")
	st, err := os.Stat(reportPath)
	if err != nil || st.IsDir() {
		return nil, false
	}
	state := model.NewAuditState()
	state.WorkDir = workDir
	state.SetFinalReportPath(reportPath)
	if content, err := os.ReadFile(reportPath); err == nil {
		state.SetFinalReport(string(content))
	} else {
		state.SetPhase(model.AuditPhaseDone)
	}
	hydrateStateFromArtifactFiles(state, auditDirPath)
	return state, state.GetPhase() == model.AuditPhaseDone
}
