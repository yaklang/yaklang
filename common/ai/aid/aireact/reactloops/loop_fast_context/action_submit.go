package loop_fast_context

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func submitFastContextResultAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"submit_fast_context_result",
		"Submit the final FastContext exploration report (structured locations + short summary). "+
			"This is the only legal exit. An internal quality gate runs before delivery.",
		[]aitool.ToolOption{
			aitool.WithStringParam("summary",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Human-readable conclusion, max ~50 words")),
			aitool.WithArrayParam("locations", "object",
				[]aitool.PropertyOption{aitool.WithParam_Required(true)},
				aitool.WithParam_Description("path"),
				aitool.WithParam_Description("start_line"),
				aitool.WithParam_Description("end_line"),
				aitool.WithParam_Description("reason"),
			),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("summary")) == "" {
				return utils.Error("summary is required")
			}
			locParams := action.GetInvokeParamsArray("locations")
			indexed := listFileIndex(loop)
			if len(locParams) == 0 && len(indexed) == 0 {
				return utils.Error("locations is required (or run grep_files / grep_files_batch to build file index first)")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			report, err := parseExplorationReport(loop, action)
			if err != nil {
				op.Feedback("结果格式无效: " + err.Error() + "\n请修正 locations 后重试。")
				op.Continue()
				return
			}

			// Step 2+ will add a real internal evaluator; for now enforce minimal quality gates.
			if issues := validateReportQuality(report); len(issues) > 0 {
				op.Feedback("内部质量检查未通过:\n- " + strings.Join(issues, "\n- ") +
					"\n请补充搜索或修正 locations 后再次提交。")
				op.Continue()
				return
			}

			md := report.FormatUserMarkdown()
			loop.Set(loopVarReport, utils.Jsonify(report))
			loop.Set("fastcontext_result_md", md)

			deliverExplorationReport(loop, invoker, md)
			invoker.AddToTimeline("[FASTCONTEXT_COMPLETE]", utils.ShrinkString(md, 1024))
			log.Infof("[FastContext] complete locations=%d", len(report.Locations))
			op.Exit()
		},
	)
}

func parseExplorationReport(loop *reactloops.ReActLoop, action *aicommon.Action) (*ExplorationReport, error) {
	summary := strings.TrimSpace(action.GetString("summary"))
	locParams := action.GetInvokeParamsArray("locations")

	var hits []LocationHit
	for _, p := range locParams {
		hits = append(hits, LocationHit{
			Path:       p.GetString("path"),
			StartLine:  int(p.GetInt("start_line")),
			EndLine:    int(p.GetInt("end_line")),
			Reason:     p.GetString("reason"),
			Confidence: p.GetString("confidence"),
		})
	}

	rounds := utils.InterfaceToInt(loop.Get(loopVarSearchRounds))
	indexed := locationsFromFileIndex(loop)
	hits = mergeLocationHits(hits, indexed)

	return &ExplorationReport{
		Query:     strings.TrimSpace(loop.Get(loopVarUserQuery)),
		Summary:   summary,
		Locations: normalizeLocationHits(hits),
		SearchStats: SearchStats{
			Rounds:      rounds,
			ToolCalls:   rounds,
			UniqueFiles: len(hits),
		},
	}, nil
}

func mergeLocationHits(primary []LocationHit, extra []LocationHit) []LocationHit {
	return normalizeLocationHits(append(append([]LocationHit{}, primary...), extra...))
}

func normalizeLocationHits(hits []LocationHit) []LocationHit {
	seen := make(map[string]struct{}, len(hits))
	out := make([]LocationHit, 0, len(hits))
	for _, h := range hits {
		path := strings.TrimSpace(h.Path)
		if path == "" {
			continue
		}
		key := path
		if h.StartLine > 0 {
			key += fmt.Sprintf(":%d-%d", h.StartLine, h.EndLine)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		h.Path = path
		out = append(out, h)
	}
	return out
}

func validateReportQuality(report *ExplorationReport) []string {
	if report == nil {
		return []string{"report is nil"}
	}
	var issues []string
	if len(report.Locations) == 0 {
		issues = append(issues, "至少需要一个带绝对路径的 location（先运行 grep_files / grep_files_batch 建立索引）")
	}
	for i, loc := range report.Locations {
		if !strings.HasPrefix(loc.Path, "/") && !strings.Contains(loc.Path, ":") {
			// allow Windows drive letters loosely
			if len(loc.Path) < 2 || loc.Path[1] != ':' {
				issues = append(issues, "location["+itoa(i)+"].path 应使用绝对路径")
			}
		}
	}
	words := len(strings.Fields(report.Summary))
	if words > 60 {
		issues = append(issues, "summary 过长，请控制在约 50 词以内")
	}
	return issues
}

func itoa(i int) string {
	return utils.InterfaceToString(i)
}

func deliverExplorationReport(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, markdown string) {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" || invoker == nil {
		return
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		taskID := ""
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
		if _, err := emitter.EmitTextMarkdownStreamEvent(
			"fastcontext-explore-result",
			strings.NewReader(markdown),
			taskID,
			func() {},
		); err != nil {
			log.Warnf("[FastContext] emit markdown stream failed: %v", err)
		}
	}
	invoker.EmitResultAfterStream(markdown)
}
