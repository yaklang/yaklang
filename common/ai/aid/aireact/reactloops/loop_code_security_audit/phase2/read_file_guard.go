// Package loop_code_security_audit — phase2_read_file_guard.go
//
// ToolInvokeParamsMutator for read_file: phase-A spot-check clamping and per-file read counters
// that feed buildPhase2PhaseASpotReadGuard / buildPhase2PhaseBReadSpinGuard.
//
// Phase B does not clamp read_file lines (deep audit). Reads of caller files discovered via
// trace grep are allowed; only reads on locked, not-yet-marked targets increment PhaseBReadCounts.
package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const phase2SearchReadFileMaxLines = 80

// clampPhase2ReadFileParams enforces spot-check reads in phase A (search).
// Full-file reads (lines=0 / mode=auto) are rewritten to a small window.
func clampPhase2ReadFileParams(phase ScanPhase, params aitool.InvokeParams) (aitool.InvokeParams, string) {
	if phase != ScanPhaseSearch || len(params) == 0 {
		return params, ""
	}

	out := make(aitool.InvokeParams, len(params))
	for k, v := range params {
		out[k] = v
	}

	mode := strings.ToLower(strings.TrimSpace(utils.InterfaceToString(out["mode"])))
	lines := int(utils.InterfaceToInt(out["lines"]))
	offset := int(utils.InterfaceToInt(out["offset"]))

	isFullRead := lines <= 0 || mode == "auto" || mode == ""
	if !isFullRead && lines <= phase2SearchReadFileMaxLines {
		return params, ""
	}

	if !isFullRead && lines > phase2SearchReadFileMaxLines {
		out["lines"] = phase2SearchReadFileMaxLines
		return out, fmt.Sprintf("阶段A已将 read_file lines 从 %d 限制为 %d（抽查模式）", lines, phase2SearchReadFileMaxLines)
	}

	if offset <= 0 {
		out["offset"] = 1
	}
	out["lines"] = phase2SearchReadFileMaxLines
	out["mode"] = "lines"
	delete(out, "line-size")
	delete(out, "chunk-size")
	return out, fmt.Sprintf("阶段A禁止全文件 read_file，已改为 offset=%v lines=%d mode=lines（深读请 lock_target_files(done=true) 后进入阶段B）",
		out["offset"], phase2SearchReadFileMaxLines)
}

// buildPhase2ReadFileParamsMutator registers read_file counters used by spin guards.
func buildPhase2ReadFileParamsMutator(
	r aicommon.AIInvokeRuntime,
	scan *ScanState,
	category model.VulnCategory,
) reactloops.ToolInvokeParamsMutator {
	return func(toolName string, params aitool.InvokeParams) aitool.InvokeParams {
		if toolName != "read_file" {
			return params
		}
		scan.mu.Lock()
		phase := scan.Phase
		scan.mu.Unlock()

		clamped, note := clampPhase2ReadFileParams(phase, params)
		if phase == ScanPhaseSearch {
			file := strings.TrimSpace(utils.InterfaceToString(params["file"]))
			if file != "" {
				scan.MarkSpotChecked(file)
			}
			count := scan.BumpPhaseASpotReads()
			if note == "" && count >= 2 {
				log.Infof("[CodeAudit/Phase2/%s] phase A spot read #%d since last lock", category.ID, count)
			}
		} else if phase == ScanPhaseAudit {
			file := strings.TrimSpace(utils.InterfaceToString(params["file"]))
			if file != "" && scan.IsTargetFile(file) && !scan.IsFileAudited(file) {
				cnt := scan.BumpPhaseBRead(file)
				if cnt >= 2 {
					log.Infof("[CodeAudit/Phase2/%s] phase B read #%d on %s (awaiting mark_file_done)", category.ID, cnt, file)
				}
			}
		}
		if note == "" {
			return params
		}
		log.Infof("[CodeAudit/Phase2/%s] %s", category.ID, note)
		if r != nil {
			r.AddToTimeline("[READ_FILE_CLAMP]", fmt.Sprintf("[Phase2/%s] %s", category.ID, note))
		}
		return clamped
	}
}

func emitPhase2CategoryBanner(loop *reactloops.ReActLoop, category model.VulnCategory, index, total int, subPhase string) {
	if loop == nil {
		return
	}
	title := fmt.Sprintf("[%d/%d] %s (%s)", index, total, category.Name, category.ID)
	if subPhase != "" {
		title += " — " + subPhase
	}
	bilingual := fmt.Sprintf("%s / Category %s (%s)", title, category.Name, category.ID)
	reactloops.EmitActionLog(loop, util.ScanNodeID, bilingual)
	reactloops.EmitStatus(loop, fmt.Sprintf("审计类别：%s (%s) / Category: %s", category.Name, category.ID, category.ID))

	emitter := loop.GetEmitter()
	if emitter == nil {
		return
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	_, _ = emitter.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "code_audit_scan_category", map[string]any{
		"category_id":   category.ID,
		"category_name": category.Name,
		"index":         index,
		"total":         total,
		"sub_phase":     subPhase,
		"task_id":       taskID,
	})
}
