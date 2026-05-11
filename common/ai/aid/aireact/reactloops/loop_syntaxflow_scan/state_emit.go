package loop_syntaxflow_scan

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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

// SyntaxFlowScanSessionMode is the orchestrator intake mode after P1.
type SyntaxFlowScanSessionMode uint8

const (
	// SyntaxFlowSessionModeNone means no intake committed yet (initial state or unparsed signals).
	// After successful P1 it must be Attach or Start — never None.
	SyntaxFlowSessionModeNone SyntaxFlowScanSessionMode = iota
	SyntaxFlowSessionModeAttach
	SyntaxFlowSessionModeStart
)

func (m SyntaxFlowScanSessionMode) String() string {
	switch m {
	case SyntaxFlowSessionModeAttach:
		return "attach"
	case SyntaxFlowSessionModeStart:
		return "start"
	default:
		return "none"
	}
}

// WireValue returns tokens for LoopVarSyntaxFlowScanSessionMode (attach|start), or empty for none.
func (m SyntaxFlowScanSessionMode) WireValue() string {
	switch m {
	case SyntaxFlowSessionModeAttach:
		return sfu.SessionModeAttach
	case SyntaxFlowSessionModeStart:
		return sfu.SessionModeStart
	default:
		return ""
	}
}

// ParseSyntaxFlowScanSessionMode parses irify_session_mode values (attach | start).
func ParseSyntaxFlowScanSessionMode(raw string) SyntaxFlowScanSessionMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case sfu.SessionModeAttach:
		return SyntaxFlowSessionModeAttach
	case sfu.SessionModeStart:
		return SyntaxFlowSessionModeStart
	default:
		return SyntaxFlowSessionModeNone
	}
}

// SyntaxFlowScanIntakeSignals is parsed intake/config snapshot (attachments + optional LiteForge).
// Parse/fetch leaves Mode as none until commitScanFromIntakeSignals commits Attach or Start.
type SyntaxFlowScanIntakeSignals struct {
	Mode             SyntaxFlowScanSessionMode
	TaskID           string
	ProjectPath      string
	SFScanConfigJSON string // resolved code-scan JSON body after intake when applicable
	// ConfigInferred is "1" when SFScanConfigJSON was inferred from local path; otherwise "0". Wired as loop var sf_scan_config_inferred.
	ConfigInferred string
	Reason         string // LiteForge justification when attachments lacked payload
}

// SyntaxFlowState is shared by the orchestrator init task and phase emitters (loop vars + emitter).
type SyntaxFlowState struct {
	mu sync.RWMutex

	Phase SyntaxFlowPhase

	// Intake snapshot after successful P1 (mode, ids, config, inferred flag, reason).
	SyntaxFlowScanIntakeSignals
}

func NewSyntaxFlowState() *SyntaxFlowState {
	return &SyntaxFlowState{
		Phase: SyntaxFlowPhaseIntake,
		SyntaxFlowScanIntakeSignals: SyntaxFlowScanIntakeSignals{
			Mode:           SyntaxFlowSessionModeNone,
			ConfigInferred: "0",
		},
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

func (s *SyntaxFlowState) SetSessionMode(m SyntaxFlowScanSessionMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SyntaxFlowScanIntakeSignals.Mode = m
}

func (s *SyntaxFlowState) GetSessionMode() SyntaxFlowScanSessionMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SyntaxFlowScanIntakeSignals.Mode
}

func (s *SyntaxFlowState) SetTaskID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SyntaxFlowScanIntakeSignals.TaskID = strings.TrimSpace(id)
}

func (s *SyntaxFlowState) GetTaskID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SyntaxFlowScanIntakeSignals.TaskID
}

func (s *SyntaxFlowState) SetSFScanConfigJSON(j string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SyntaxFlowScanIntakeSignals.SFScanConfigJSON = strings.TrimSpace(j)
}

func (s *SyntaxFlowState) GetSFScanConfigJSON() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SyntaxFlowScanIntakeSignals.SFScanConfigJSON
}

func (s *SyntaxFlowState) SetProjectPath(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SyntaxFlowScanIntakeSignals.ProjectPath = strings.TrimSpace(p)
}

func (s *SyntaxFlowState) GetProjectPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SyntaxFlowScanIntakeSignals.ProjectPath
}

func (s *SyntaxFlowState) SetConfigInferred(inferred string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inferred == "1" {
		s.SyntaxFlowScanIntakeSignals.ConfigInferred = "1"
	} else {
		s.SyntaxFlowScanIntakeSignals.ConfigInferred = "0"
	}
}

func (s *SyntaxFlowState) GetConfigInferred() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.SyntaxFlowScanIntakeSignals.ConfigInferred
	if v == "" {
		return "0"
	}
	return v
}

func (s *SyntaxFlowState) SetIntakeReason(r string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SyntaxFlowScanIntakeSignals.Reason = r
}

// EmitSyntaxFlowScanProgress 在引擎内同步阶段：顶栏 loading（EmitStatus）+ 结构化事件（EVENT_TYPE_STRUCTURED, nodeId=syntaxflow_scan_progress）。
func EmitSyntaxFlowScanProgress(loop *reactloops.ReActLoop, phase, message, taskID, err string) {
	EmitSyntaxFlowScanPhase(loop, 0, "", phase, message, taskID, err, nil)
}

// EmitSyntaxFlowScanPhase 三阶段编排：step=1 编译、2 扫描、3 风险轮询/总览；stage=start|end|tick。
// extra 可带 program_name、risk_count、status 等结构化附字段。
func EmitSyntaxFlowScanPhase(loop *reactloops.ReActLoop, step int, stage, phase, message, taskID, err string, extra map[string]any) {
	if loop == nil {
		return
	}
	statusLine := message
	if statusLine == "" {
		statusLine = phase
	}
	loop.LoadingStatus("SyntaxFlow: " + statusLine)

	em := loop.GetEmitter()
	if em == nil {
		return
	}
	m := map[string]any{
		"loop":    "syntaxflow_scan",
		"phase":   phase,
		"message": message,
	}
	if step > 0 {
		m["step"] = step
	}
	if stage != "" {
		m["stage"] = stage
	}
	if taskID != "" {
		m["task_id"] = taskID
	}
	if err != "" {
		m["error"] = err
	}
	for k, v := range extra {
		m[k] = v
	}
	if _, e := em.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "syntaxflow_scan_progress", m); e != nil {
		log.Debugf("syntaxflow_scan_progress emit: %v", e)
	}
}
