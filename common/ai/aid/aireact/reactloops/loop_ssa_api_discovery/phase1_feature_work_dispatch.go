package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	featureWorkJobHTTPAPI   = "http_api"
	featureWorkJobCodeOnly  = "code_only"
	featureWorkStatusDone   = "completed"
	featureWorkStatusFailed = "failed"
)

// coverageBatchSize is the number of jobs to run before asking CoverageSignalReAct.
const defaultCoverageBatchSize = 8

func coverageBatchSize() int {
	n := defaultCoverageBatchSize
	s := strings.TrimSpace(os.Getenv("YAK_SSA_API_DISCOVERY_FEATURE_BATCH_SIZE"))
	if s == "" {
		return n
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return n
	}
	if v > 32 {
		return 32
	}
	return v
}

// FeatureWorkJob is one concurrent analysis unit (single entry file).
type FeatureWorkJob struct {
	EntryFile       string
	FeatureID       string
	FeatureLabel    string
	SurfaceKind     string
	PackagePatterns []string
	NoHttpReason    string
	StaticHints     []StaticRouteHint
}

type featureWorkProgressEntry struct {
	EntryFile string `json:"entry_file"`
	JobKind   string `json:"job_kind"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

type featureWorkProgress struct {
	Entries []featureWorkProgressEntry `json:"entries"`
}

func featureWorkStepSafeName(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func runPhase1FeatureWorkChain(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	chainStart := time.Now()
	rt.execStepStart("phase2.feature_work", "ai+programmatic")
	defer func() {
		rt.execStepEnd("phase2.feature_work", "ai+programmatic", chainStart, []string{
			store.FeatureWorkProgressPath(rt.WorkDir),
			store.FeatureApiMapPath(rt.WorkDir),
			store.CoverageSignalPath(rt.WorkDir),
		})
	}()

	// Load feature inventory and collect all jobs. If inventory is missing or empty,
	// fall back to a programmatic job list derived from the registry's http_entry units.
	jobs, invErr := loadJobsFromInventoryOrRegistry(rt)
	if invErr != nil {
		return invErr
	}
	if len(jobs) == 0 {
		log.Infof("ssa_api_discovery: feature work no pending jobs")
		return nil
	}

	// Filter out already-completed jobs.
	progress, _ := loadFeatureWorkProgress(rt.WorkDir)
	done := progressCompletedSet(progress)
	pending := filterPendingJobs(rt, jobs, done)
	if len(pending) == 0 {
		log.Infof("ssa_api_discovery: feature work all %d entry_files already completed", len(jobs))
		return nil
	}

	// Sort by tier: P0 (controller) first, then P1, P2.
	pending = sortByPriorityTier(pending)

	if r != nil {
		r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("phase1_feature_work: start total=%d pending=%d concurrent=%d batch=%d",
			len(jobs), len(pending), featureWorkConcurrent(), coverageBatchSize()))
	}

	// Batch loop: run N jobs → compute signal → ask ReAct → decide next.
	batchIdx := 0
	for len(pending) > 0 {
		batchIdx++
		batch := pending
		batchLimit := coverageBatchSize()
		if len(batch) > batchLimit {
			batch = pending[:batchLimit]
			pending = pending[batchLimit:]
		} else {
			pending = nil
		}

		batchStep := fmt.Sprintf("phase2.feature_work.batch_%d", batchIdx)
		batchStart := time.Now()
		rt.execStepStart(batchStep, "ai+programmatic")
		rt.execInfo(batchStep, "ai+programmatic", fmt.Sprintf("jobs=%d pending_after=%d", len(batch), len(pending)))

		// Run this batch.
		batchErr := runFeatureWorkBatch(r, task, rt, batch)
		if batchErr != nil {
			log.Warnf("ssa_api_discovery: batch error: %v", batchErr)
			rt.execStepError(batchStep, "ai+programmatic", batchStart, batchErr, []string{store.FeatureWorkProgressPath(rt.WorkDir)})
		} else {
			rt.execStepEnd(batchStep, "ai+programmatic", batchStart, []string{store.FeatureWorkProgressPath(rt.WorkDir)})
		}

		// Still have more to process? Ask CoverageSignalReAct for the verdict.
		if len(pending) > 0 {
			sig, sigErr := ComputeCoverageSignal(rt)
			if sigErr != nil {
				log.Warnf("ssa_api_discovery: compute coverage signal failed: %v", sigErr)
				// Continue processing on error — don't block on signal compute failure.
				continue
			}
			_ = PersistCoverageSignal(rt, sig)

			decision, decErr := RunCoverageSignalReAct(context.Background(), r, task, rt)
			if decErr != nil {
				log.Warnf("ssa_api_discovery: CoverageSignalReAct failed: %v", decErr)
				// Continue — treat as implicit "continue".
				continue
			}

			log.Infof("ssa_api_discovery: CoverageSignalReAct verdict=%s pct=%.1f%% pending=%d reasoning=%s",
				decision.Verdict, sig.RouteCoveragePct, len(pending), decision.Reasoning)

			switch decision.Verdict {
			case VerdictFinish:
				log.Infof("ssa_api_discovery: CoverageSignalReAct voted finish; ending feature work.")
				pending = nil

			case VerdictReprioritize:
				if len(decision.NextQueue) > 0 {
					pending = applyReActQueueUpdate(rt, pending, decision.NextQueue)
					log.Infof("ssa_api_discovery: queue reprioritized to %d items", len(pending))
				}
			// VerdictContinue: loop continues with next batch.
			}
		}
	}

	// IMPORTANT: Always run CoverageSignalReAct once at the end to persist the final verdict,
	// even when the last batch exhausted all pending jobs (pending == nil) or when
	// the loop exited due to parent context cancellation. This ensures the gate check
	// (verifyCoverageSignalVerdict) always finds a verdict and doesn't falsely fail.
	runFinalCoverageSignalVerdict(r, rt)

	// Final sync to verified_http_apis before exiting.
	_ = SyncFeatureApiMapToVerifiedHttpApis(rt)

	if r != nil {
		r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("phase1_feature_work: done"))
	}
	return nil
}

// loadJobsFromInventoryOrRegistry returns jobs from feature_inventory if available,
// otherwise falls back to programmatic http_entry jobs from the registry.
func loadJobsFromInventoryOrRegistry(rt *Runtime) ([]FeatureWorkJob, error) {
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err == nil && inv != nil && len(inv.Features) > 0 {
		jobs, err := collectFeatureWorkJobs(rt, inv)
		if err == nil && len(jobs) > 0 {
			return jobs, nil
		}
	}

	// Fallback: build jobs directly from registry's http_entry units.
	return buildJobsFromHttpEntryRegistry(rt)
}

// buildJobsFromHttpEntryRegistry creates FeatureWorkJobs for all http_entry units in the registry.
func buildJobsFromHttpEntryRegistry(rt *Runtime) ([]FeatureWorkJob, error) {
	reg, err := loadCodeUnitRegistry(rt.WorkDir)
	if err != nil || reg == nil {
		return nil, utils.Error("code_unit_registry missing; cannot build fallback jobs")
	}
	hintByFile := staticRouteHintsByFile(rt)
	seen := map[string]struct{}{}
	var jobs []FeatureWorkJob
	for _, u := range reg.Units {
		if u.KindHint != codeUnitKindHintHTTPEntry {
			continue
		}
		rel := normEntryFileRef(u.RelPath)
		if rel == "" {
			continue
		}
		if _, dup := seen[rel]; dup {
			continue
		}
		seen[rel] = struct{}{}
		jobs = append(jobs, FeatureWorkJob{
			EntryFile:   rel,
			FeatureID:   "http_entry_" + rel,
			SurfaceKind: SurfaceKindHTTPAPI,
			StaticHints: enrichStaticRouteHintsForJob(rt, FeatureWorkJob{EntryFile: rel}, hintByFile[rel]),
		})
	}
	return jobs, nil
}

func collectFeatureWorkJobs(rt *Runtime, inv *FeatureInventoryV1) ([]FeatureWorkJob, error) {
	if inv == nil {
		return nil, utils.Error("nil feature inventory")
	}
	hintByFile := staticRouteHintsByFile(rt)
	seen := map[string]struct{}{}
	var jobs []FeatureWorkJob
	for _, feat := range inv.Features {
		sk := strings.TrimSpace(feat.SurfaceKind)
		if sk == "" {
			sk = SurfaceKindHTTPAPI
		}
		for _, ef := range EntryFilesForFeature(feat) {
			rel := normalizePlanFileRef(rt, ef)
			if rel == "" {
				continue
			}
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			job := FeatureWorkJob{
				EntryFile:       rel,
				FeatureID:       feat.FeatureID,
				FeatureLabel:    feat.Label,
				SurfaceKind:     sk,
				PackagePatterns: append([]string(nil), feat.PackagePatterns...),
				NoHttpReason:    feat.NoHttpReason,
			}
			job.StaticHints = enrichStaticRouteHintsForJob(rt, job, hintByFile[rel])
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].FeatureID == jobs[j].FeatureID {
			return jobs[i].EntryFile < jobs[j].EntryFile
		}
		return jobs[i].FeatureID < jobs[j].FeatureID
	})
	return jobs, nil
}

func staticRouteHintsByFile(rt *Runtime) map[string][]StaticRouteHint {
	out := map[string][]StaticRouteHint{}
	if rt == nil {
		return out
	}
	rep, err := readStaticRouteHintsReport(rt.WorkDir)
	if err != nil || rep == nil {
		return out
	}
	for _, h := range rep.Hints {
		rel := normalizePlanFileRef(rt, h.FileRelPath)
		if rel == "" {
			continue
		}
		out[rel] = append(out[rel], h)
	}
	return out
}

func featureWorkConcurrent() int {
	return controllerVerifyConcurrent()
}

func progressCompletedSet(p featureWorkProgress) map[string]struct{} {
	out := map[string]struct{}{}
	for _, e := range p.Entries {
		if e.Status != featureWorkStatusDone {
			continue
		}
		if rel := strings.TrimSpace(e.EntryFile); rel != "" {
			out[rel] = struct{}{}
		}
	}
	return out
}

func appendFeatureWorkProgress(entries []featureWorkProgressEntry, entryFile, jobKind, status, reason string) []featureWorkProgressEntry {
	entryFile = strings.TrimSpace(entryFile)
	for i := range entries {
		if entries[i].EntryFile == entryFile {
			entries[i].JobKind = jobKind
			entries[i].Status = status
			entries[i].Reason = reason
			return entries
		}
	}
	return append(entries, featureWorkProgressEntry{
		EntryFile: entryFile,
		JobKind:   jobKind,
		Status:    status,
		Reason:    reason,
	})
}

func loadFeatureWorkProgress(workDir string) (featureWorkProgress, error) {
	b, err := os.ReadFile(store.FeatureWorkProgressPath(workDir))
	if err != nil {
		if os.IsNotExist(err) {
			return featureWorkProgress{}, nil
		}
		return featureWorkProgress{}, err
	}
	var p featureWorkProgress
	if err := json.Unmarshal(b, &p); err != nil {
		return featureWorkProgress{}, err
	}
	return p, nil
}

func saveFeatureWorkProgress(workDir string, p featureWorkProgress) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(store.FeatureWorkProgressPath(workDir), b)
}

func allRegistryUnitsCompleted(rt *Runtime) (bool, string) {
	if rt == nil {
		return false, "nil runtime"
	}
	reg, err := loadCodeUnitRegistry(rt.WorkDir)
	if err != nil || reg == nil || len(reg.Units) == 0 {
		return false, "code_unit_registry missing"
	}
	progress, _ := loadFeatureWorkProgress(rt.WorkDir)
	done := progressCompletedSet(progress)
	var missing []string
	for _, u := range reg.Units {
		rel := normEntryFileRef(u.RelPath)
		if _, ok := done[rel]; !ok {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		limit := len(missing)
		if limit > 5 {
			limit = 5
		}
		return false, fmt.Sprintf("%d registry units not completed (e.g. %v)", len(missing), missing[:limit])
	}
	return true, ""
}

// runFeatureWorkBatch runs a slice of FeatureWorkJobs concurrently, updating progress on completion or failure.
func runFeatureWorkBatch(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, batch []FeatureWorkJob) error {
	if len(batch) == 0 {
		return nil
	}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, featureWorkConcurrent())
	var wg sync.WaitGroup

	for _, j := range batch {
		wg.Add(1)
		go func(job FeatureWorkJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			entryRel := normalizePlanFileRef(rt, job.EntryFile)
			jobStep := fmt.Sprintf("phase2.feature_work.job.%s", featureWorkStepSafeName(entryRel))
			jobStart := time.Now()

			if skip, skipReason := ShouldSkipFeatureWorkForPartialAuth(rt, job); skip {
				rt.execStepStart(jobStep, "programmatic")
				runErr := commitSkippedFeatureWorkForPartialAuth(rt, job, skipReason)
				if runErr != nil {
					rt.execStepError(jobStep, "programmatic", jobStart, runErr, nil)
				} else {
					rt.execStepEnd(jobStep, "programmatic", jobStart, []string{store.FeatureWorkProgressPath(rt.WorkDir)})
				}
				mu.Lock()
				status := featureWorkStatusDone
				reason := skipReason
				if runErr != nil {
					status = featureWorkStatusFailed
					reason = runErr.Error()
					if firstErr == nil {
						firstErr = runErr
					}
				}
				progress, _ := loadFeatureWorkProgress(rt.WorkDir)
				progress.Entries = appendFeatureWorkProgress(progress.Entries, entryRel, job.SurfaceKind, status, reason)
				_ = saveFeatureWorkProgress(rt.WorkDir, progress)
				mu.Unlock()
				return
			}

			rt.execStepStart(jobStep, "ai")
			log.Infof("ssa_api_discovery: feature work %s kind=%s feature=%s", entryRel, job.SurfaceKind, job.FeatureID)
			var runErr error
			switch strings.TrimSpace(job.SurfaceKind) {
			case SurfaceKindHTTPAPI:
				_, runErr = runPhase1HttpApiUnitReAct(r, task, rt, job)
			case SurfaceKindCodeOnly:
				_, runErr = runPhase1CodeAnalysisUnitReAct(r, task, rt, job)
			default:
				runErr = utils.Errorf("unknown surface_kind %q for %s", job.SurfaceKind, entryRel)
			}
			if runErr != nil {
				rt.execStepError(jobStep, "ai", jobStart, runErr, nil)
			} else {
				rt.execStepEnd(jobStep, "ai", jobStart, []string{store.FeatureWorkProgressPath(rt.WorkDir)})
			}

			mu.Lock()
			defer mu.Unlock()
			status := featureWorkStatusDone
			reason := ""
			if runErr != nil {
				log.Warnf("ssa_api_discovery: feature work %s failed: %v", entryRel, runErr)
				status = featureWorkStatusFailed
				reason = runErr.Error()
				if firstErr == nil {
					firstErr = utils.Errorf("feature work failed for %s: %w", entryRel, runErr)
				}
			}
			progress, _ := loadFeatureWorkProgress(rt.WorkDir)
			progress.Entries = appendFeatureWorkProgress(progress.Entries, entryRel, job.SurfaceKind, status, reason)
			_ = saveFeatureWorkProgress(rt.WorkDir, progress)
		}(j)
	}
	wg.Wait()
	return firstErr
}

// filterPendingJobs removes jobs whose EntryFile is in the done set.
func filterPendingJobs(rt *Runtime, jobs []FeatureWorkJob, done map[string]struct{}) []FeatureWorkJob {
	var pending []FeatureWorkJob
	for _, j := range jobs {
		rel := normalizePlanFileRef(rt, j.EntryFile)
		if rel == "" {
			continue
		}
		if _, ok := done[rel]; ok {
			continue
		}
		pending = append(pending, j)
	}
	return pending
}

// sortByPriorityTier sorts jobs: P0 (controller) first, then P1 (service/auth), then P2 (config/other).
func sortByPriorityTier(jobs []FeatureWorkJob) []FeatureWorkJob {
	type tieredJob struct {
		job  FeatureWorkJob
		tier int // 0=highest priority
	}
	var tiered []tieredJob
	for _, j := range jobs {
		rel := j.EntryFile
		lower := strings.ToLower(strings.ReplaceAll(rel, "\\", "/"))
		tier := 2
		switch {
		case strings.Contains(lower, "/controller/"):
			tier = 0
		case strings.Contains(lower, "/service/") || strings.Contains(lower, "/interceptor/") ||
			strings.Contains(lower, "/security/") || strings.Contains(lower, "/auth/"):
			tier = 1
		}
		tiered = append(tiered, tieredJob{job: j, tier: tier})
	}
	sort.Slice(tiered, func(i, j int) bool {
		if tiered[i].tier != tiered[j].tier {
			return tiered[i].tier < tiered[j].tier
		}
		return tiered[i].job.EntryFile < tiered[j].job.EntryFile
	})
	out := make([]FeatureWorkJob, len(tiered))
	for i, t := range tiered {
		out[i] = t.job
	}
	return out
}

// applyReActQueueUpdate reorders pending jobs to match the ReAct-supplied queue,
// appending any jobs not mentioned in the queue at the end.
func applyReActQueueUpdate(rt *Runtime, pending []FeatureWorkJob, queueUpdate []string) []FeatureWorkJob {
	if len(queueUpdate) == 0 {
		return pending
	}
	// Build a map of rel_path -> job for fast lookup.
	jobMap := map[string]FeatureWorkJob{}
	for _, j := range pending {
		rel := normalizePlanFileRef(rt, j.EntryFile)
		if rel != "" {
			jobMap[rel] = j
		}
	}
	var reordered []FeatureWorkJob
	seen := map[string]bool{}
	for _, rel := range queueUpdate {
		if job, ok := jobMap[rel]; ok {
			reordered = append(reordered, job)
			seen[rel] = true
		}
	}
	// Append remaining jobs not in the queue, preserving order.
	for _, j := range pending {
		rel := normalizePlanFileRef(rt, j.EntryFile)
		if rel != "" && !seen[rel] {
			reordered = append(reordered, j)
		}
	}
	return reordered
}

// runFinalCoverageSignalVerdict computes and persists the final coverage verdict at the
// end of the feature work chain. This runs even when pending is exhausted or the loop
// was interrupted, ensuring the gate check always finds a verdict.
func runFinalCoverageSignalVerdict(r aicommon.AIInvokeRuntime, rt *Runtime) {
	if rt == nil {
		return
	}
	step := "phase2.coverage_signal.final"
	started := time.Now()
	rt.execStepStart(step, "ai")
	sig, sigErr := ComputeCoverageSignal(rt)
	if sigErr != nil {
		log.Warnf("ssa_api_discovery: final compute coverage signal failed: %v", sigErr)
		// Still persist a fallback decision so the gate doesn't falsely fail.
		_ = persistFallbackCoverageSignalDecision(rt, "coverage signal compute failed")
		rt.execStepError(step, "ai", started, sigErr, []string{store.CoverageSignalPath(rt.WorkDir)})
		return
	}
	_ = PersistCoverageSignal(rt, sig)

	// Run ReAct with enough context to make a judgment.
	decision, decErr := RunCoverageSignalReAct(context.Background(), r, nil, rt)
	if decErr != nil {
		log.Warnf("ssa_api_discovery: final CoverageSignalReAct failed: %v; persisting fallback", decErr)
		_ = persistFallbackCoverageSignalDecision(rt, fmt.Sprintf("CoverageSignalReAct error: %v", decErr))
		rt.execStepError(step, "ai", started, decErr, []string{store.CoverageSignalPath(rt.WorkDir)})
		return
	}

	log.Infof("ssa_api_discovery: final CoverageSignalReAct verdict=%s route_pct=%.1f%% entry_pct=%.1f%% reasoning=%s",
		decision.Verdict, sig.RouteCoveragePct, sig.EntryCoveragePct, decision.Reasoning)
	rt.execStepEnd(step, "ai", started, []string{store.CoverageSignalPath(rt.WorkDir)})
}

// persistFallbackCoverageSignalDecision writes a fallback verdict when ReAct couldn't run.
// This prevents the gate from failing with "verdict empty" when the pipeline was interrupted.
func persistFallbackCoverageSignalDecision(rt *Runtime, reason string) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	// If coverage is at 100%, treat as finish; otherwise treat as continue (interrupted).
	sig, _ := ComputeCoverageSignal(rt)
	var verdict CoverageSignalVerdict
	if sig != nil && sig.EntryCoveragePct >= 100 {
		verdict = VerdictFinish
	} else {
		verdict = VerdictContinue
	}
	decision := &CoverageSignalDecision{
		Verdict:    verdict,
		Reasoning:  reason,
		SignalJSON: "",
	}
	b, err := json.MarshalIndent(decision, "", "  ")
	if err != nil {
		return err
	}
	_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, "coverage_signal_decision", string(b))
	log.Infof("ssa_api_discovery: persisted fallback verdict=%s reason=%s", verdict, reason)
	return nil
}
