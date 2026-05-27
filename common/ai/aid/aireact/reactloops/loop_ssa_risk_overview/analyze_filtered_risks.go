package loop_ssa_risk_overview

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func readOverviewParallelFromEnv() int {
	v := strings.TrimSpace(os.Getenv("YAK_SF_SCAN_OVERVIEW_PARALLEL"))
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

func capReviewSubLoopMaxIter(r aicommon.AIInvokeRuntime) int {
	if r == nil {
		return 8
	}
	n := int(r.GetConfig().GetMaxIterationCount())
	if n > 32 {
		n = 32
	}
	if n < 2 {
		n = 2
	}
	return n
}

func parseCommaRiskIDs(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []int64
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil || v <= 0 {
			return nil, utils.Errorf("invalid risk id token %q", p)
		}
		out = append(out, v)
	}
	return out, nil
}

func runOneRiskReviewSubLoop(
	r aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	id int64,
	mode sfu.RiskReviewMode,
) string {
	reviewLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_SSA_RISK_REVIEW,
		r,
		reactloops.WithMaxIterations(capReviewSubLoopMaxIter(r)),
	)
	if err != nil {
		return fmt.Sprintf("## Risk %d\n\n(sub-loop create failed: %v)\n", id, err)
	}
	reviewLoop.Set(sfu.LoopVarSSARiskID, fmt.Sprintf("%d", id))
	reviewLoop.Set(sfu.LoopVarSSARiskReviewMode, string(mode))
	reviewLoop.Set(sfu.LoopVarSSARiskReviewDigest, "")

	subID := fmt.Sprintf("ssa-batch-review-%d", id)
	if parentTask != nil {
		subID = fmt.Sprintf("%s-analyze-%d", parentTask.GetId(), id)
	}
	userPrompt := fmt.Sprintf("Batch SSA risk review for risk_id=%d. Summarize with reload_ssa_risk / evidence actions; end with directly_answer.", id)
	sub := aicommon.NewSubTaskBase(parentTask, subID, userPrompt, true)
	if execErr := reviewLoop.ExecuteWithExistedTask(sub); execErr != nil {
		return fmt.Sprintf("## Risk %d\n\n(execute error: %v)\n", id, execErr)
	}
	dig := strings.TrimSpace(reviewLoop.Get(sfu.LoopVarSSARiskReviewDigest))
	if dig == "" {
		dig = "(no digest produced yet — consider raising review max iterations or check SSA DB connectivity)"
	}
	return fmt.Sprintf("## Risk %d\n\n%s\n", id, dig)
}

// WithAnalyzeFilteredRisksAction fans out Go-driven per-risk review sub-loops for all IDs matching the filter or explicit risk_ids.
func WithAnalyzeFilteredRisksAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"analyze_filtered_risks",
		"Deterministically run ssa_risk_review once per SSA risk id matching the current filter (or explicit risk_ids CSV). Go enumerates all ids up to limit — the model does not choose subsets. Default mode=analyze (read-only); use analyze_dispose only when intentional.",
		[]aitool.ToolOption{
			aitool.WithStringParam("risk_ids", aitool.WithParam_Description("Optional comma-separated SSA risk ids; when set, bypasses filter resolution.")),
			aitool.WithStringParam("mode", aitool.WithParam_Description("analyze (default) | analyze_dispose")),
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Max risks to analyze (default 50).")),
			aitool.WithIntegerParam("parallel", aitool.WithParam_Description("Concurrent review sub-loops (default 1 or env YAK_SF_SCAN_OVERVIEW_PARALLEL).")),
			aitool.WithStringParam("search", aitool.WithParam_Description("Fuzzy search; merged with loop/attachment base filter when risk_ids empty.")),
			aitool.WithStringParam("runtime_id", aitool.WithParam_Description("Comma-separated runtime ids; merged with base filter.")),
			aitool.WithStringParam("program_name", aitool.WithParam_Description("Comma-separated program names; merged with base filter.")),
			aitool.WithStringParam("severity", aitool.WithParam_Description("Comma-separated severities; merged with base filter.")),
			aitool.WithStringParam("risk_type", aitool.WithParam_Description("Comma-separated risk types; merged with base filter.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if loop == nil {
				return utils.Error("loop is nil")
			}
			mode := sfu.ParseRiskReviewMode(action.GetString("mode"))
			_ = mode // validated downstream via RiskEvidence on disposal only
			rawIDs := strings.TrimSpace(action.GetString("risk_ids"))
			if rawIDs == "" {
				hasFilter := strings.TrimSpace(loop.Get(sfu.LoopVarSSAOverviewFilterJSON)) != "" ||
					strings.TrimSpace(loop.Get(sfu.LoopVarSSARisksFilterJSON)) != "" ||
					strings.TrimSpace(action.GetString("search")) != "" ||
					strings.TrimSpace(action.GetString("runtime_id")) != "" ||
					strings.TrimSpace(action.GetString("program_name")) != "" ||
					strings.TrimSpace(action.GetString("severity")) != "" ||
					strings.TrimSpace(action.GetString("risk_type")) != ""
				if !hasFilter {
					return utils.Error("analyze_filtered_risks requires risk_ids or a filter (call query_ssa_risk_overview first, or pass search/runtime_id/program_name/severity/risk_type)")
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			task := operator.GetTask()
			db := sfu.GetSSADB()
			if db == nil && r.GetConfig() != nil {
				db = r.GetConfig().GetDB()
			}
			if db == nil {
				operator.Feedback("analyze_filtered_risks: SSA database not available")
				operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
				operator.Continue()
				return
			}

			mode := sfu.ParseRiskReviewMode(action.GetString("mode"))
			limit := int64(action.GetInt("limit"))
			if limit <= 0 {
				limit = 50
			}

			ev := sfu.NewRiskEvidence(db)

			var ids []int64
			var total int64

			rawIDs := strings.TrimSpace(action.GetString("risk_ids"))
			if rawIDs != "" {
				parsed, err := parseCommaRiskIDs(rawIDs)
				if err != nil {
					operator.Feedback(err.Error())
					operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
					operator.Continue()
					return
				}
				ids = parsed
				total = int64(len(ids))
				if int64(len(ids)) > limit {
					ids = ids[:limit]
				}
			} else {
				filter := sfu.MergeQuerySSARiskOverviewFilter(loop, task, action)
				var err error
				ids, total, err = ev.ResolveRiskIDs(filter, limit)
				if err != nil {
					operator.Feedback(fmt.Sprintf("analyze_filtered_risks: resolve ids: %v", err))
					operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
					operator.Continue()
					return
				}
			}

			if len(ids) == 0 {
				operator.Feedback("analyze_filtered_risks: no matching risk ids")
				operator.Continue()
				return
			}

			parallel := int(action.GetInt("parallel"))
			if parallel <= 0 {
				parallel = readOverviewParallelFromEnv()
			}
			if parallel < 1 {
				parallel = 1
			}
			if parallel > 8 {
				parallel = 8
			}

			sem := make(chan struct{}, parallel)
			var wg sync.WaitGroup
			var mu sync.Mutex
			sections := make([]string, len(ids))
			for i, id := range ids {
				i, id := i, id
				wg.Add(1)
				go func() {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					sec := runOneRiskReviewSubLoop(r, task, id, mode)
					mu.Lock()
					sections[i] = sec
					mu.Unlock()
				}()
			}
			wg.Wait()

			md := strings.Join(sections, "\n\n---\n\n")
			md = "# SSA Risk batch analyze\n\n" + fmt.Sprintf("mode=%s | matched_total≈%d | analyzed=%d\n\n", mode, total, len(ids)) + md

			artifactName := fmt.Sprintf("ssa_risk_analyze_%s", utils.RandStringBytes(8))
			path := loop.GetInvoker().EmitFileArtifactWithExt(artifactName, ".md", md)
			short := utils.ShrinkTextBlock(md, 6000)
			loop.Set(sfu.LoopVarSSARiskOverviewAnalysisSummary, short)
			r.AddToTimeline("ssa_risk_overview", fmt.Sprintf("[analyze_filtered_risks] analyzed %d risk(s), artifact=%s", len(ids), path))

			recordAction(loop, "analyze_filtered_risks",
				fmt.Sprintf("mode=%s limit=%d parallel=%d", mode, limit, parallel),
				fmt.Sprintf("analyzed=%d total_hint=%d artifact=%s", len(ids), total, path))
			r.AddToTimeline("ssa_risk_overview_analyze",
				fmt.Sprintf("[analyze_filtered_risks] mode=%s analyzed %d/%d risks, artifact=%s",
					mode, len(ids), total, path))

			fb := fmt.Sprintf("[analyze_filtered_risks] mode=%s analyzed %d risk(s) (total_hint≈%d). Short summary below; full report: %s\n\n%s",
				mode, len(ids), total, path, short)
			operator.Feedback(fb)
			operator.Continue()
		},
	)
}
