package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	defaultMaxFilesPerStage       = 30
	defaultMaxCodeReadingStages     = 50
	defaultMaxTotalCodeReadingFiles = 500
)

// APIFragment is one route extracted during a code reading stage.
type APIFragment struct {
	Method       string  `json:"method"`
	PathPattern  string  `json:"path_pattern"`
	HandlerFile  string  `json:"handler_file"`
	HandlerSymbol string `json:"handler_symbol,omitempty"`
	HandlerClass string  `json:"handler_class,omitempty"`
	CodeEvidence string  `json:"code_evidence"`
	Confidence   float64 `json:"confidence,omitempty"`
}

// RoutingFact captures mount prefix / interceptor evidence from code reading.
type RoutingFact struct {
	Kind       string  `json:"kind"`
	MountPrefix string `json:"mount_prefix,omitempty"`
	Ref        string  `json:"ref"`
	Hint       string  `json:"hint"`
	Confidence float64 `json:"confidence,omitempty"`
}

// CodeReadingStageOutput is written to code_reading_stage_<n>.json.
type CodeReadingStageOutput struct {
	Stage              int                `json:"stage"`
	ReadFilesCompleted []string           `json:"read_files_completed"`
	APIFragments       []APIFragment      `json:"api_fragments"`
	RoutingFacts       []RoutingFact      `json:"routing_facts,omitempty"`
	NextWorklist       []WorklistSeedItem `json:"next_worklist"`
	AuthNotes          string             `json:"auth_notes,omitempty"`
	AuthEvidence       *AuthEvidenceRecord `json:"auth_evidence,omitempty"`
}

// CodeReadingWorklist manages pending files for staged reading.
type CodeReadingWorklist struct {
	items   []WorklistSeedItem
	seen    map[string]struct{}
	readTotal int
}

func newCodeReadingWorklist(seed []WorklistSeedItem) *CodeReadingWorklist {
	w := &CodeReadingWorklist{seen: map[string]struct{}{}}
	for _, s := range seed {
		w.enqueueItem(s)
	}
	return w
}

func (w *CodeReadingWorklist) enqueueItem(item WorklistSeedItem) {
	rel := filepath.ToSlash(strings.TrimSpace(item.RelPath))
	if rel == "" {
		return
	}
	if _, ok := w.seen[rel]; ok {
		return
	}
	w.seen[rel] = struct{}{}
	if item.Category == "" {
		item.Category = worklistCategoryAPIHandler
	}
	if item.Priority == 0 {
		item.Priority = 3
	}
	w.items = append(w.items, item)
}

func (w *CodeReadingWorklist) enqueue(rel, reason string) {
	w.enqueueItem(WorklistSeedItem{RelPath: rel, Reason: reason})
}

func (w *CodeReadingWorklist) Len() int { return len(w.items) }

// PopBatchWithAuthGate defers priority>=3 batches until auth_entry stage completes.
func (w *CodeReadingWorklist) PopBatchWithAuthGate(max int, authRequired, authResolved bool) []WorklistSeedItem {
	if authRequired && !authResolved && len(w.items) > 0 {
		p := w.items[0].Priority
		if p == 0 {
			p = 99
		}
		if p >= 3 {
			return nil
		}
	}
	return w.PopBatch(max)
}

func (w *CodeReadingWorklist) PopBatch(max int) []WorklistSeedItem {
	if max <= 0 {
		max = defaultMaxFilesPerStage
	}
	if len(w.items) == 0 {
		return nil
	}
	// Keep batches within the same priority tier when possible.
	batchPriority := w.items[0].Priority
	if batchPriority == 0 {
		batchPriority = 99
	}
	limit := max
	for i := 1; i < len(w.items) && i < max; i++ {
		p := w.items[i].Priority
		if p == 0 {
			p = 99
		}
		if p != batchPriority {
			limit = i
			break
		}
	}
	if len(w.items) <= limit {
		batch := w.items
		w.items = nil
		w.readTotal += len(batch)
		return batch
	}
	batch := w.items[:limit]
	w.items = w.items[limit:]
	w.readTotal += len(batch)
	return batch
}

func (w *CodeReadingWorklist) MergeNext(items []WorklistSeedItem) int {
	added := 0
	for _, it := range items {
		if !isBackendCodeRelPath(it.RelPath) && !isLoginTemplateRelPath(it.RelPath) {
			continue
		}
		before := len(w.seen)
		w.enqueueItem(it)
		if len(w.seen) > before {
			added++
		}
	}
	return added
}

func worklistSeedFromRuntime(rt *Runtime) []WorklistSeedItem {
	if rt == nil {
		return nil
	}
	if recon, err := loadPhase1ReconOutput(rt.WorkDir); err == nil && len(recon.NextWorklist) > 0 {
		return recon.NextWorklist
	}
	if p, err := loadProjectProfile(rt.WorkDir); err == nil && len(p.WorklistSeed) > 0 {
		return p.WorklistSeed
	}
	if payload, err := GetPhaseArtifactPayload(rt, store.ArtifactApiPreanalysisFull, store.ApiPreanalysisReportPath(rt.WorkDir)); err == nil {
		var pre struct {
			ControllerFileCandidates []struct {
				RelPath string `json:"rel_path"`
			} `json:"controller_file_candidates"`
			RouteFileCandidates []struct {
				RelPath string `json:"rel_path"`
			} `json:"route_file_candidates"`
		}
		if json.Unmarshal([]byte(payload), &pre) == nil {
			var seed []WorklistSeedItem
			for _, c := range pre.ControllerFileCandidates {
				seed = append(seed, WorklistSeedItem{RelPath: c.RelPath, Reason: "preanalysis controller"})
			}
			for _, c := range pre.RouteFileCandidates {
				seed = append(seed, WorklistSeedItem{RelPath: c.RelPath, Reason: "preanalysis route file"})
			}
			if len(seed) > 0 {
				return seed
			}
		}
	}
	return nil
}

func validateCodeReadingStageOutput(stage int, out *CodeReadingStageOutput, batch []WorklistSeedItem, rt *Runtime) error {
	if out == nil {
		return utils.Error("nil stage output")
	}
	if out.Stage != stage {
		return utils.Errorf("stage mismatch: expected %d got %d", stage, out.Stage)
	}
	if len(out.ReadFilesCompleted) == 0 {
		return utils.Error("read_files_completed required")
	}
	for i, frag := range out.APIFragments {
		if strings.TrimSpace(frag.Method) == "" {
			return utils.Errorf("api_fragments[%d].method required", i)
		}
		if strings.TrimSpace(frag.PathPattern) == "" {
			return utils.Errorf("api_fragments[%d].path_pattern required", i)
		}
		if strings.TrimSpace(frag.HandlerFile) == "" && strings.TrimSpace(frag.CodeEvidence) == "" {
			return utils.Errorf("api_fragments[%d] requires handler_file or code_evidence", i)
		}
	}
	if err := validateConfigStageMountRequired(stage, out, batch); err != nil {
		return err
	}
	if batchHasAuthEntry(batch) {
		if err := validateAuthStageOutput(out, batch, rt); err != nil {
			return err
		}
	}
	if len(batch) > 0 && len(out.NextWorklist) == 0 && len(out.APIFragments) == 0 &&
		!batchHasRoutingConfig(batch) && !batchHasAuthEntry(batch) {
		return utils.Error("stage produced no api_fragments and no next_worklist")
	}
	return nil
}

func loadAllCodeReadingStages(workDir string) ([]CodeReadingStageOutput, error) {
	var stages []CodeReadingStageOutput
	appendStage := func(b []byte, path string) error {
		var out CodeReadingStageOutput
		if err := json.Unmarshal(b, &out); err != nil {
			return utils.Wrapf(err, "parse %s", path)
		}
		stages = append(stages, out)
		return nil
	}
	if b, err := os.ReadFile(store.CodeReadingStagePath(workDir, 0)); err == nil {
		if err := appendStage(b, store.CodeReadingStagePath(workDir, 0)); err != nil {
			return stages, err
		}
	}
	for n := 1; n <= defaultMaxCodeReadingStages; n++ {
		path := store.CodeReadingStagePath(workDir, n)
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return stages, err
		}
		if err := appendStage(b, path); err != nil {
			return stages, err
		}
	}
	return stages, nil
}

// BuildCodeReadingPlanFromStageFiles merges code_reading_stage_*.json into a plan.
func BuildCodeReadingPlanFromStageFiles(rt *Runtime) (*CodeReadingPlan, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	stages, err := loadAllCodeReadingStages(rt.WorkDir)
	if err != nil {
		return nil, err
	}
	if len(stages) == 0 {
		return nil, utils.Error("no code reading stage files")
	}
	profile, _ := loadProjectProfile(rt.WorkDir)
	plan := mergeStagesToCodeReadingPlan(stages, profile)
	if len(plan.DiscoveredAPIs) == 0 {
		plan = mergeStaticHintsIntoPlan(rt, plan)
	}
	if len(plan.DiscoveredAPIs) == 0 {
		return nil, utils.Error("stages produced no discovered_apis")
	}
	return plan, nil
}

func mergeStagesToCodeReadingPlan(stages []CodeReadingStageOutput, profile *ProjectProfileV1) *CodeReadingPlan {
	seenRoute := map[string]struct{}{}
	seenFile := map[string]struct{}{}
	urlSpaces := map[string]any{}
	var apis []DiscoveredAPI
	var completed []string
	var bases []string
	authNotes := ""
	var authEvidence *AuthEvidenceRecord

	for _, st := range stages {
		if strings.TrimSpace(st.AuthNotes) != "" {
			authNotes = st.AuthNotes
		}
		if st.AuthEvidence != nil {
			authEvidence = st.AuthEvidence
		}
		for _, f := range st.ReadFilesCompleted {
			if _, ok := seenFile[f]; !ok {
				seenFile[f] = struct{}{}
				completed = append(completed, f)
			}
		}
		for _, frag := range st.APIFragments {
			key := routeKey(frag.Method, frag.PathPattern)
			if _, ok := seenRoute[key]; ok {
				continue
			}
			seenRoute[key] = struct{}{}
			apis = append(apis, DiscoveredAPI{
				Method:        strings.ToUpper(strings.TrimSpace(frag.Method)),
				PathPattern:   normURLPath(frag.PathPattern),
				HandlerFile:   frag.HandlerFile,
				HandlerSymbol: frag.HandlerSymbol,
				HandlerClass:  frag.HandlerClass,
				CodeEvidence:  frag.CodeEvidence,
			})
		}
		for _, rf := range st.RoutingFacts {
			if mp := strings.TrimSpace(rf.MountPrefix); mp != "" {
				id := fmt.Sprintf("fact_%s", strings.TrimPrefix(mp, "/"))
				if id == "fact_" {
					id = "default"
				}
				urlSpaces[id] = map[string]any{
					"base_path": mp,
					"evidence":  rf.Ref,
				}
				bases = append(bases, mp)
			}
		}
	}

	if len(urlSpaces) == 0 {
		ctx := "/"
		if profile != nil && profile.ContextPath != "" && profile.ContextPath != "unknown" {
			ctx = normURLPath(profile.ContextPath)
		}
		urlSpaces["default"] = map[string]any{"base_path": ctx, "evidence": "project_profile.context_path"}
		bases = append(bases, ctx)
	}

	return &CodeReadingPlan{
		DiscoveredAPIs:     apis,
		ReadFilesCompleted: completed,
		ReadQueue:          completed,
		EffectiveBases:     dedupeStrings(bases),
		URLSpaces:          urlSpaces,
		HintDiff:           "staged code reading (evidence-first)",
		AuthNotes:          authNotes,
		AuthEvidence:       authEvidence,
	}
}
