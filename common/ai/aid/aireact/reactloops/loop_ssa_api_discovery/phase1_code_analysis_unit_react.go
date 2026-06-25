package loop_ssa_api_discovery

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_code_analysis_unit_playbook.txt
var phase1CodeAnalysisUnitPlaybook string

const codeAnalysisUnitCommittedLoopKey = "code_analysis_unit_committed"

func runPhase1CodeAnalysisUnitReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, job FeatureWorkJob) (CodeAnalysisUnitResult, error) {
	extra := buildCodeAnalysisUnitExtra(rt, job)
	loop, err := buildPhase1CodeAnalysisUnitLoop(r, rt, job, extra)
	if err != nil {
		return CodeAnalysisUnitResult{}, err
	}
	scope := ControllerVerifyScope{
		ControllerFile:  job.EntryFile,
		FeatureID:       job.FeatureID,
		FeatureLabel:    job.FeatureLabel,
		PackagePatterns: job.PackagePatterns,
	}
	setLoopControllerScope(loop, scope)
	subName := "phase1_code_unit_" + controllerVerifySubSlug(job.EntryFile)
	if err := runPhase1ReActLoop(task, subName, loop); err != nil {
		return CodeAnalysisUnitResult{}, err
	}
	raw := strings.TrimSpace(loop.Get(codeAnalysisUnitCommittedLoopKey))
	if raw == "" {
		return CodeAnalysisUnitResult{}, utils.Error("code analysis unit not committed")
	}
	var unit CodeAnalysisUnitResult
	if err := json.Unmarshal([]byte(raw), &unit); err != nil {
		return CodeAnalysisUnitResult{}, err
	}
	if err := validateCodeAnalysisUnitResult(&unit, job); err != nil {
		return CodeAnalysisUnitResult{}, err
	}
	codeMap, err := ensureFeatureCodeAnalysisMap(rt)
	if err != nil {
		return CodeAnalysisUnitResult{}, err
	}
	mergeFeatureCodeAnalysisUnit(codeMap, unit)
	if err := persistFeatureCodeAnalysisMap(rt, codeMap); err != nil {
		return CodeAnalysisUnitResult{}, err
	}
	return unit, nil
}

func buildPhase1CodeAnalysisUnitLoop(r aicommon.AIInvokeRuntime, rt *Runtime, job FeatureWorkJob, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1CodeAnalysisAgentBaseOptions(r, rt, job, extra)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildFinalizeCodeAnalysisUnit(rt, job),
		buildBlockedDirectlyAnswer("finalize_code_analysis_unit"),
		buildCodeAnalysisUnitFinishOverride(job),
	)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_FEATURE_VERIFY, r, preset...)
}

func phase1CodeAnalysisAgentBaseOptions(r aicommon.AIInvokeRuntime, rt *Runtime, job FeatureWorkJob, extra string) []reactloops.ReActLoopOption {
	persistent := strings.TrimSpace(phase1CodeAnalysisUnitPlaybook)
	if extra != "" {
		persistent += "\n\n" + strings.TrimSpace(extra)
	}
	persistent += "\n\n" + strings.TrimSpace(ssaDiscoveryFSBuiltinToolParamsHint)
	return []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(true),
		reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentInstruction(persistent),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			base := ""
			if rt != nil && rt.Session != nil {
				base = EffectiveTargetBaseURL(rt.Session)
			}
			return fmt.Sprintf(`<|PHASE1_CODE_UNIT_%s|>
session: %s
target_base: %s
code_root: %s
entry_file: %s
feature_id: %s
surface_kind: code_only
no_http_reason: %s
feedback:
%s
<|END_%s|>`,
				nonce,
				loop.Get("discovery_session_uuid"),
				base,
				loopGetCodeRoot(rt),
				job.EntryFile,
				job.FeatureID,
				job.NoHttpReason,
				feedbacker.String(),
				nonce,
			), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			if rt != nil && rt.Session != nil {
				loop.Set("discovery_session_uuid", rt.Session.UUID)
				loop.Set("discovery_sqlite_path", rt.SQLitePath)
				loop.Set("discovery_code_root", rt.Session.CodeRootPath)
			}
			op.NextAction("read_file")
		}),
		buildDiscoveryGetStatus(),
		buildDiscoveryReadSessionData(),
		buildCodeReadingReadFileAudit(rt),
	}
}

func buildCodeAnalysisUnitExtra(rt *Runtime, job FeatureWorkJob) string {
	return buildEmbeddedContextBlock("code_analysis_task",
		rt.WorkDir,
		func(workDir string) (string, error) {
			return readArtifactExcerpt(store.FeatureInventoryPath(workDir), 8000)
		},
	)
}

func buildFinalizeCodeAnalysisUnit(rt *Runtime, job FeatureWorkJob) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_code_analysis_unit",
		"Commit code analysis unit result and exit.",
		[]aitool.ToolOption{aitool.WithStringParam("result_json", aitool.WithParam_Required(true))},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("result_json"))
			var unit CodeAnalysisUnitResult
			if err := json.Unmarshal([]byte(raw), &unit); err != nil {
				op.Feedback("invalid result_json: " + err.Error())
				op.Continue()
				return
			}
			if strings.TrimSpace(unit.EntryFile) == "" {
				unit.EntryFile = job.EntryFile
			}
			if strings.TrimSpace(unit.FeatureID) == "" {
				unit.FeatureID = job.FeatureID
			}
			if err := validateCodeAnalysisUnitResult(&unit, job); err != nil {
				op.Feedback("validation: " + err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(unit, "", "  ")
			loop.Set(codeAnalysisUnitCommittedLoopKey, string(b))
			op.Exit()
		},
	)
}

func buildCodeAnalysisUnitFinishOverride(job FeatureWorkJob) reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "finish",
		Description: "Blocked until finalize_code_analysis_unit succeeds.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if strings.TrimSpace(loop.Get(codeAnalysisUnitCommittedLoopKey)) != "" {
				op.Exit()
				return
			}
			op.Feedback("use finalize_code_analysis_unit to commit code analysis result; do not finish early")
			op.Continue()
		},
	})
}

func validateCodeAnalysisUnitResult(unit *CodeAnalysisUnitResult, job FeatureWorkJob) error {
	if unit == nil {
		return utils.Error("nil code analysis unit")
	}
	if strings.TrimSpace(unit.EntryFile) == "" {
		return utils.Error("entry_file required")
	}
	if strings.TrimSpace(unit.FeatureID) == "" {
		return utils.Error("feature_id required")
	}
	if len(unit.Functions) == 0 && strings.TrimSpace(unit.NoCallableReason) == "" {
		return utils.Error("functions required or no_callable_reason when no callable methods")
	}
	_ = filepath.Base(unit.EntryFile)
	_ = job
	return nil
}

func codeAnalysisUnitForEntry(workDir, entryFile string) (*CodeAnalysisUnitResult, bool) {
	m, err := loadFeatureCodeAnalysisMap(workDir)
	if err != nil || m == nil {
		return nil, false
	}
	entryFile = normEntryFileRef(entryFile)
	for i := range m.Units {
		if normEntryFileRef(m.Units[i].EntryFile) == entryFile {
			return &m.Units[i], true
		}
	}
	return nil, false
}

func allCodeOnlyUnitsPresent(rt *Runtime, inv *FeatureInventoryV1) (bool, string) {
	if rt == nil || inv == nil {
		return false, "nil runtime or inventory"
	}
	var missing []string
	for _, feat := range inv.Features {
		if strings.TrimSpace(feat.SurfaceKind) != SurfaceKindCodeOnly {
			continue
		}
		for _, ef := range EntryFilesForFeature(feat) {
			rel := normEntryFileRef(ef)
			if _, ok := codeAnalysisUnitForEntry(rt.WorkDir, rel); !ok {
				missing = append(missing, rel)
			}
		}
	}
	if len(missing) > 0 {
		return false, fmt.Sprintf("%d code_only units missing from feature_code_analysis_map", len(missing))
	}
	return true, ""
}
