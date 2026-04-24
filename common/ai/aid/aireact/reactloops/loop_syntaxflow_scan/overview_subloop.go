package loop_syntaxflow_scan

import (
	"fmt"
	"os"
	"strings"
	"sync"

	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	// envSSARiskOverviewSubLoop 不为 "0"/"false"/"off" 时，终局/轮询总览走 ssa_risk_overview 子环 + copy（失败则回退 Apply）。
	envSSARiskOverviewSubLoop = "YAK_SSA_RISK_OVERVIEW_SUBLOOP"
	// envSSARiskOverviewInScanSubLoop 为 "1"/"true"/"on" 时，长扫中周期总览也跑短子环（默认关，仅 emit）。
	envSSARiskOverviewInScanSubLoop = "YAK_SSA_RISK_OVERVIEW_IN_SCAN_SUBLOOP"
)

var interpretSSAVarMu sync.Mutex

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func envFalsy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off", "no", "":
		return true
	default:
		return false
	}
}

// UseSSARiskOverviewSubLoop returns true unless YAK_SSA_RISK_OVERVIEW_SUBLOOP is 0/false/off.
func UseSSARiskOverviewSubLoop() bool {
	v := os.Getenv(envSSARiskOverviewSubLoop)
	if v == "" {
		return true
	}
	return !envFalsy(v)
}

// UseInScanSSARiskOverviewSubLoop 长扫中周期子环（成本高；默认关）。
func UseInScanSSARiskOverviewSubLoop() bool {
	return envTruthy(os.Getenv(envSSARiskOverviewInScanSubLoop))
}

// WithInterpretSSAVarLock serializes writers that Set ssa_risk_* on the interpret loop from
// the poll goroutine vs. other code paths. reload_ssa_risk_overview 仍走模型同线程，不在此包一层。
func WithInterpretSSAVarLock(fn func()) {
	interpretSSAVarMu.Lock()
	defer interpretSSAVarMu.Unlock()
	fn()
}

// NewSyntaxflowSubTask 与 loop_syntaxflow_scan.newSubTask 同形，供子环独立子任务 id。
func NewSyntaxflowSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	if parent == nil {
		return nil
	}
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

// CopyOverviewOutputsToParent copies overview loop vars needed by syntaxflow_scan_interpret reactive.
func CopyOverviewOutputsToParent(overview, interpret *reactloops.ReActLoop) {
	if overview == nil || interpret == nil {
		return
	}
	keys := []string{
		"ssa_risk_overview_preface",
		"ssa_risk_list_summary",
		"ssa_risk_total_hint",
		sfutil.LoopVarSSAOverviewFilterJSON,
		sfutil.LoopVarSSARisksFilterJSON,
	}
	for _, k := range keys {
		interpret.Set(k, overview.Get(k))
	}
}

func capOverviewSubLoopMaxIter(r aicommon.AIInvokeRuntime) int {
	if r == nil {
		return 3
	}
	n := int(r.GetConfig().GetMaxIterationCount())
	if n > 5 {
		n = 5
	}
	if n < 1 {
		n = 1
	}
	return n
}

// runSSARiskOverviewSubLoopWithChild creates an overview ReAct loop, runs Execute, then
// ApplySSARiskOverviewDB on the same child to reach listLimit rows, for copy to interpret.
func runSSARiskOverviewSubLoopWithChild(
	r aicommon.AIInvokeRuntime,
	interpret *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
	riskDB *gorm.DB,
	taskID string,
	filter *ypb.SSARisksFilter,
	listLimit int64,
) (overview *reactloops.ReActLoop, err error) {
	if r == nil || interpret == nil || parentTask == nil {
		return nil, fmt.Errorf("nil invoker, interpret or task")
	}
	if riskDB == nil {
		return nil, fmt.Errorf("nil risk db")
	}
	if filter == nil {
		if taskID == "" {
			return nil, fmt.Errorf("empty taskID and filter")
		}
		filter = &ypb.SSARisksFilter{RuntimeID: []string{taskID}}
	}
	overview, err = reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_SSA_RISK_OVERVIEW,
		r,
		reactloops.WithMaxIterations(capOverviewSubLoopMaxIter(r)),
	)
	if err != nil {
		return nil, err
	}
	if tid := strings.TrimSpace(interpret.Get(sfutil.LoopVarSyntaxFlowTaskID)); tid != "" {
		overview.Set(sfutil.LoopVarSyntaxFlowTaskID, tid)
	} else if taskID != "" {
		overview.Set(sfutil.LoopVarSyntaxFlowTaskID, taskID)
	}
	for _, k := range []string{sfutil.LoopVarSSARisksFilterJSON, sfutil.LoopVarSSAOverviewFilterJSON, sfutil.LoopVarSyntaxFlowScanSessionMode, "sf_scan_task_id"} {
		if s := interpret.Get(k); s != "" {
			overview.Set(k, s)
		}
	}
	PersistEffectiveOverviewFilter(overview, filter)
	sub := NewSyntaxflowSubTask(parentTask, "ssa_risk_overview_subloop")
	if sub == nil {
		return nil, fmt.Errorf("subtask")
	}
	if err := overview.ExecuteWithExistedTask(sub); err != nil {
		return overview, err
	}
	_ = ApplySSARiskOverviewDB(overview, r, riskDB, sub, filter, listLimit)
	return overview, nil
}

// ApplySSARiskOverviewToInterpret 终局/轮询入口：优先子环 + copy，失败或 env 关则 Apply 直写 interpret。
func ApplySSARiskOverviewToInterpret(
	interpret *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	parentTask aicommon.AIStatefulTask,
	taskID string,
	filter *ypb.SSARisksFilter,
	listLimit int64,
) {
	if interpret == nil || r == nil {
		return
	}
	riskDB := sfutil.GetSSADB()
	if riskDB == nil {
		riskDB = db
	}
	if riskDB == nil {
		_ = ApplySSARiskOverviewDB(interpret, r, nil, parentTask, filter, listLimit)
		return
	}
	if !UseSSARiskOverviewSubLoop() {
		WithInterpretSSAVarLock(func() {
			_ = ApplySSARiskOverviewDB(interpret, r, riskDB, parentTask, filter, listLimit)
		})
		return
	}
	WithInterpretSSAVarLock(func() {
		ov, err := runSSARiskOverviewSubLoopWithChild(r, interpret, parentTask, riskDB, taskID, filter, listLimit)
		if err != nil {
			log.Warnf("[ssa_risk_overview] subloop: %v, fallback Apply", err)
			_ = ApplySSARiskOverviewDB(interpret, r, riskDB, parentTask, filter, listLimit)
			return
		}
		if ov == nil {
			_ = ApplySSARiskOverviewDB(interpret, r, riskDB, parentTask, filter, listLimit)
			return
		}
		CopyOverviewOutputsToParent(ov, interpret)
		hint := interpret.Get("ssa_risk_total_hint")
		AppendSFPipelineLine(interpret, fmt.Sprintf("【3.x·子环 ssa_risk_overview】已完成，approx count=%s", hint))
	})
}
