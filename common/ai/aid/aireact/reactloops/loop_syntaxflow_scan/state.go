package loop_syntaxflow_scan

import (
	"sync"
)

// SyntaxFlowPhase tracks orchestration progress for the multi-stage scan pipeline.
type SyntaxFlowPhase string

const (
	SyntaxFlowPhaseIntake      SyntaxFlowPhase = "intake"
	SyntaxFlowPhaseCompileScan SyntaxFlowPhase = "compile_scan"
	SyntaxFlowPhaseInterpret   SyntaxFlowPhase = "interpret"
	SyntaxFlowPhaseReport      SyntaxFlowPhase = "report"
	SyntaxFlowPhaseDone        SyntaxFlowPhase = "done"
)

// SyntaxFlowState is shared by the orchestrator init and the interpret sub-loop (via loop vars + emitters).
type SyntaxFlowState struct {
	mu sync.RWMutex

	Phase SyntaxFlowPhase

	// TaskID is set when running in attach mode (existing SyntaxFlow scan task in SSA DB).
	TaskID string

	// ResolvedSFScanConfigJSON is a full code-scan JSON for fresh compile+scan (when not using TaskID).
	ResolvedSFScanConfigJSON string

	// ConfigInferred is "1" when project_path was used to build JSON; "0" when JSON was explicit.
	ConfigInferred string
}

func NewSyntaxFlowState() *SyntaxFlowState {
	return &SyntaxFlowState{
		Phase:          SyntaxFlowPhaseIntake,
		ConfigInferred: "0",
	}
}

func (s *SyntaxFlowState) SetPhase(p SyntaxFlowPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Phase = p
}

func (s *SyntaxFlowState) GetPhase() SyntaxFlowPhase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Phase
}

func (s *SyntaxFlowState) SetTaskID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TaskID = id
}

func (s *SyntaxFlowState) GetTaskID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TaskID
}

func (s *SyntaxFlowState) SetResolvedSFScanConfigJSON(j string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResolvedSFScanConfigJSON = j
}

func (s *SyntaxFlowState) GetResolvedSFScanConfigJSON() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ResolvedSFScanConfigJSON
}

func (s *SyntaxFlowState) SetConfigInferred(inferred string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inferred == "1" {
		s.ConfigInferred = "1"
	} else {
		s.ConfigInferred = "0"
	}
}

func (s *SyntaxFlowState) GetConfigInferred() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ConfigInferred
}
