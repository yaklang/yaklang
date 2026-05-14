package loop_syntaxflow_scan

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const LoopVarInterpretLog = sfu.LoopVarSFInterpretLog

func AppendSfScanInterpretLog(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, taskID, line string) {
	sfu.AppendSfScanInterpretLog(loop, r, taskID, line)
}

var (
	PersistEffectiveOverviewFilter   = sfu.PersistEffectiveOverviewFilter
	MergeReloadSSARiskOverviewFilter = sfu.MergeReloadSSARiskOverviewFilter
	ApplySSARiskOverviewDB           = sfu.ApplySSARiskOverviewDB
)

var interpretSSAVarMu sync.Mutex

func WithInterpretSSAVarLock(fn func()) {
	interpretSSAVarMu.Lock()
	defer interpretSSAVarMu.Unlock()
	fn()
}

func NewSyntaxflowSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	if parent == nil {
		return nil
	}
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
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

func StartScanTaskStatusPoll(
	loop *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	runtimeID string,
	dispatcher *riskDispatcher,
) {
	if runtimeID == "" {
		return
	}
	db := sfu.GetSSADB()
	if db == nil {
		return
	}
	ctx := task.GetContext()

	ticker := time.NewTicker(45 * time.Second)
	go func() {
		defer ticker.Stop()
		var riskGateLast int64 = -1
		var riskGateSame int

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st, err := schema.GetSyntaxFlowScanTaskById(db, runtimeID)
				if err != nil {
					log.Debugf("[syntaxflow_scan] poll task: %v", err)
					continue
				}
				if st == nil {
					continue
				}

				if st.Status == schema.SYNTAXFLOWSCAN_EXECUTING {
					riskGateLast = -1
					riskGateSame = 0
					AppendSfScanInterpretLog(loop, r, runtimeID,
						"poll: 扫描进行中 "+sfu.FormatScanTaskProgressLine(st)+fmtRiskLine(st))
					continue
				}

				if riskGateLast < 0 {
					riskGateLast = st.RiskCount
					riskGateSame = 1
				} else if st.RiskCount == riskGateLast {
					riskGateSame++
				} else {
					riskGateLast = st.RiskCount
					riskGateSame = 1
				}

				if riskGateSame < 2 {
					continue
				}

				endText := FormatSyntaxFlowScanEndReport(st)
				loop.Set(sfu.LoopVarSFScanEndSummary, endText)
				AppendSFPipelineLine(loop, "【2·结束】"+endText)

				parentT := OrchestratorParentTaskID(loop, task.GetId())
				EmitSyntaxFlowUserStageMarkdown(loop, parentT, "p2_scan_finished_user",
					BuildScanStagePhase2ScanFinishedTable(st))

				r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock("扫描已结束(终态): "+endText, 4000))

				if dispatcher != nil {
					dispatcher.NotifyScanTerminal()
				} else {
					loop.Set(sfu.LoopVarSFRiskConverged, "1")
				}

				return
			}
		}
	}()
}

func fmtRiskLine(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	return fmt.Sprintf(" risk_count=%d crit=%d high=%d warn=%d low=%d info=%d",
		st.RiskCount, st.CriticalCount, st.HighCount, st.WarningCount, st.LowCount, st.InfoCount)
}
