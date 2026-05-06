package loop_syntaxflow_scan

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
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

// EmitSyntaxFlowScanProgress 在引擎内同步阶段：顶栏 loading（EmitStatus）+ 结构化事件（EVENT_TYPE_STRUCTURED, nodeId=syntaxflow_scan_progress）。
func EmitSyntaxFlowScanProgress(loop *reactloops.ReActLoop, phase, message, taskID, err string) {
	EmitSyntaxFlowScanPhase(loop, 0, "", phase, message, taskID, err, nil)
}

// EmitSyntaxFlowScanPhase 三阶段编排：step=1 编译、2 扫描、3 风险轮询/解读；stage=start|end|tick。
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
