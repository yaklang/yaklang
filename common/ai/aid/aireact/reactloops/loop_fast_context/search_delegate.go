package loop_fast_context

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
		"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SearchInput configures an isolated FastContext run for parent loops (e.g. code audit phase2).
type SearchInput struct {
	Query             string
	WorkDir           string
	ReferenceMaterial string
}

// SearchResult is returned to callers after an isolated run.
type SearchResult struct {
	Report    *ExplorationReport
	Markdown  string
	FilePaths []string
	Error     error
}

// RunFastContextSearch runs fast_context as a sub-agent with a CLEAN timeline
// (no inheritance of the parent's timeline snapshot). The parent passes any
// context the sub-agent needs (query, work dir, reference material) explicitly
// via SearchInput / job.UserInput / ConfigureSubLoop; the sub-agent's timeline
// is fully isolated and never merged back into the parent.
//
// parentLoop is the enclosing ReActLoop (used to register the fast_context
// sub-agent into the parent's ProgressRegistry so the stall-heartbeat sub-agent
// bypass treats the parent's blocking wait as "still progressing"). May be nil
// when called outside a ReActLoop context (registry registration is skipped).
func RunFastContextSearch(
	invoker aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
	input SearchInput,
) SearchResult {
	if invoker == nil {
		return SearchResult{Error: utils.Error("invoker is nil")}
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return SearchResult{Error: utils.Error("query is required")}
	}

	if parentTask == nil {
		return SearchResult{Error: utils.Error("parent task is nil")}
	}

	// 显式上下文：fast_context 使用 Clean timeline，不继承父 timeline 条目；
	// 必要上下文通过 UserInput 显式传入（查询目标 + 工作目录 + 参考资料）。
	userInput := strings.TrimSpace(query)
	if ref := strings.TrimSpace(input.ReferenceMaterial); ref != "" {
		userInput += "\n\n## Reference Material\n\n" + ref
	}
	if wd := strings.TrimSpace(input.WorkDir); wd != "" {
		userInput += "\n\n## Working Directory\n\n" + wd
	}

	results := reactloops.DispatchSubAgents(
		invoker, parentTask,
		[]reactloops.SubAgentJob{{
			Order:     1,
			Identifier: "fast-context",
			TaskName:   "fast-context",
			LoopName:   schema.AI_REACT_LOOP_NAME_FAST_CONTEXT,
			UserInput:  userInput,
		}},
		reactloops.SubAgentOptions{
			ParentLoop:    parentLoop,
			TimelineMode:  reactloops.SubAgentTimelineClean,
			InheritEmitter: true,
			ConfigureLoop: func(loop *reactloops.ReActLoop) {
				ConfigureSubLoop(loop, input)
			},
			ExtraLoopOpts: []reactloops.ReActLoopOption{withFastContextToolPool(invoker)},
		},
	)
	if len(results) == 0 || results[0] == nil {
		return SearchResult{Error: utils.Error("fast_context sub-agent returned no result")}
	}
	result := results[0]
	if result.ExecErr != nil {
		return SearchResult{Error: utils.Wrap(result.ExecErr, "fast_context sub-agent run")}
	}
	subLoop := result.SubLoop

	searchResult := ExtractSearchResult(subLoop)
	log.Infof("[FastContext] sub-agent search done paths=%d err=%v", len(searchResult.FilePaths), searchResult.Error)
	return searchResult
}

// ConfigureSubLoop sets loop variables before execution.
func ConfigureSubLoop(subLoop *reactloops.ReActLoop, input SearchInput) {
	if subLoop == nil {
		return
	}
	subLoop.Set(loopVarUserQuery, strings.TrimSpace(input.Query))
	subLoop.Set(loopVarWorkDir, strings.TrimSpace(input.WorkDir))
	subLoop.Set(loopVarReferenceMaterial, strings.TrimSpace(input.ReferenceMaterial))
	subLoop.Set(loopVarFileIndex, "")
	subLoop.Set(loopVarSearchRounds, "0")
	subLoop.Set(loopVarReport, "")
}

func withFastContextToolPool(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithToolsGetter(func() []*aitool.Tool {
		if invoker == nil || invoker.GetConfig() == nil || invoker.GetConfig().GetAiToolManager() == nil {
			return nil
		}
		enabled, err := invoker.GetConfig().GetAiToolManager().GetEnableTools()
		if err != nil {
			return nil
		}
		allow := map[string]struct{}{
			"grep": {}, "find_file": {}, "read_file": {},
		}
		var out []*aitool.Tool
		for _, t := range enabled {
			if t == nil {
				continue
			}
			if _, ok := allow[t.GetName()]; ok {
				out = append(out, t)
			}
		}
		return out
	})
}

// ExtractSearchResult reads deliverable fields from a finished sub-loop.
func ExtractSearchResult(subLoop *reactloops.ReActLoop) SearchResult {
	if subLoop == nil {
		return SearchResult{Error: utils.Error("sub-loop is nil")}
	}

	report := loadReportFromLoop(subLoop)
	paths := uniquePaths(report, listFileIndex(subLoop))
	if len(paths) == 0 {
		return SearchResult{Error: utils.Error("fast_context finished without file paths")}
	}

	md := strings.TrimSpace(subLoop.Get("fastcontext_result_md"))
	if md == "" && report != nil {
		md = report.FormatUserMarkdown()
	}

	return SearchResult{
		Report:    report,
		Markdown:  md,
		FilePaths: paths,
		Error:     nil,
	}
}

func loadReportFromLoop(loop *reactloops.ReActLoop) *ExplorationReport {
	raw := strings.TrimSpace(loop.Get(loopVarReport))
	if raw == "" {
		if v := loop.GetVariable(loopVarReport); v != nil {
			raw = strings.TrimSpace(utils.InterfaceToString(v))
		}
	}
	if raw == "" {
		// Fallback: build from file index only
		paths := listFileIndex(loop)
		if len(paths) == 0 {
			return nil
		}
		return &ExplorationReport{
			Query:     loop.Get(loopVarUserQuery),
			Summary:   "Indexed candidate files from parallel search.",
			Locations: locationsFromFileIndex(loop),
			SearchStats: SearchStats{
				Rounds:      utils.InterfaceToInt(loop.Get(loopVarSearchRounds)),
				UniqueFiles: len(paths),
			},
		}
	}
	var report ExplorationReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return nil
	}
	return &report
}

func uniquePaths(report *ExplorationReport, indexed []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range indexed {
		add(p)
	}
	if report != nil {
		for _, loc := range report.Locations {
			add(loc.Path)
		}
	}
	return out
}

// FilterAuditCandidatePaths drops obvious noise paths for security audit targets.
func FilterAuditCandidatePaths(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p == "" || isNoiseAuditPath(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// PrioritizeAuditCandidatePaths keeps the highest-priority audit targets up to maxKeep.
// Paths under vulnerability modules and source files rank above helpers, docs, and tests.
func PrioritizeAuditCandidatePaths(paths []string, maxKeep int) []string {
	if maxKeep <= 0 || len(paths) <= maxKeep {
		return paths
	}
	type scored struct {
		path  string
		score int
	}
	scoredPaths := make([]scored, 0, len(paths))
	for _, p := range paths {
		scoredPaths = append(scoredPaths, scored{path: p, score: auditCandidatePriorityScore(p)})
	}
	for i := 0; i < len(scoredPaths); i++ {
		for j := i + 1; j < len(scoredPaths); j++ {
			if scoredPaths[j].score > scoredPaths[i].score {
				scoredPaths[i], scoredPaths[j] = scoredPaths[j], scoredPaths[i]
			}
		}
	}
	out := make([]string, 0, maxKeep)
	for i := 0; i < maxKeep && i < len(scoredPaths); i++ {
		out = append(out, scoredPaths[i].path)
	}
	return out
}

func auditCandidatePriorityScore(p string) int {
	lower := strings.ToLower(filepathClean(p))
	score := 0
	if strings.Contains(lower, "/vulnerabilities/") {
		score += 40
	}
	if strings.Contains(lower, "/source/") {
		score += 30
	}
	if strings.HasSuffix(lower, "/index.php") {
		score += 15
	}
	if strings.Contains(lower, "/includes/") || strings.Contains(lower, "/login.php") {
		score += 10
	}
	if strings.Contains(lower, "impossible.") {
		score -= 25
	}
	if strings.Contains(lower, "/help") || strings.Contains(lower, "view_help") {
		score -= 15
	}
	if strings.Contains(lower, "/external/") || strings.Contains(lower, "/docs/") {
		score -= 20
	}
	if strings.HasSuffix(lower, ".js") {
		score -= 5
	}
	return score
}

func isNoiseAuditPath(p string) bool {
	lower := strings.ToLower(filepathClean(p))
	noisy := []string{
		"/vendor/", "/node_modules/", "/.git/", "/testdata/", "/test/fixtures/",
		"/__tests__/", "/mocks/", "/dist/", "/build/", "/.idea/", "/.vscode/",
	}
	for _, n := range noisy {
		if strings.Contains(lower, n) {
			return true
		}
	}
	if strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".test.js") ||
		strings.HasSuffix(lower, ".spec.ts") || strings.HasSuffix(lower, ".spec.js") {
		return true
	}
	return false
}

func filepathClean(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
