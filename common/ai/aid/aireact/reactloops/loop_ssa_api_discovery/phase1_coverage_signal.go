package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// CoverageSignal is the programmatic input fed to CoverageSignalReAct.
// ReAct reads this after each batch to decide whether to continue reading or finish.
// All fields are objective; ReAct owns the interpretation and final decision.
type CoverageSignal struct {
	GeneratedAt string `json:"generated_at"`

	// Route coverage: static harvest vs. AI-discovered routes
	StaticHarvestRoutes []string `json:"static_harvest_routes"`  // from static_route_hints.json
	DiscoveredRoutes    []string `json:"discovered_routes"`       // from feature_api_map (processed=true)
	UndiscoveredRoutes  []string `json:"undiscovered_routes"`     // StaticHarvestRoutes \ DiscoveredRoutes
	RouteCoveragePct   float64  `json:"route_coverage_pct"`      // 0-100

	// HTTP-entry unit coverage: registry http_entry vs. processed in feature_api_map
	TotalHttpEntries int      `json:"total_http_entries"`    // registry kind_hint == http_entry
	AnalyzedEntries  int      `json:"analyzed_entries"`      // in feature_api_map with processed=true
	PendingEntries   []string `json:"pending_entries"`        // rel_paths not yet processed
	EntryCoveragePct float64  `json:"entry_coverage_pct"`    // 0-100

	// Priority tiers for read ordering (P0=must-read, P1=should-read, P2=skip)
	PriorityTiers PriorityTiers `json:"priority_tiers"`

	// Suggested next batch from program (for ReAct reference only)
	ReadQueueHint []string `json:"read_queue_hint"`

	// Program's subjective confidence in current state
	ConfidenceLevel string `json:"confidence"` // "low" | "medium" | "high"

	// Packages that are low-value for HTTP API discovery
	LowValuePackages []string `json:"low_value_packages"`

	// Signal version for schema evolution
	SchemaVersion int `json:"schema_version"`
}

// PriorityTiers groups pending http_entry rel_paths by analysis priority.
type PriorityTiers struct {
	// P0: Controller classes (kind_hint=http_entry)
	P0 []string `json:"p0"`
	// P1: Service, interceptor, auth-related
	P1 []string `json:"p1"`
	// P2: Config, initializer
	P2 []string `json:"p2"`
}

const coverageSignalSchemaVersion = 1

// ComputeCoverageSignal returns the current programmatic coverage snapshot.
// It reads static_route_hints.json, feature_api_map.json, code_unit_registry.json,
// feature_work_progress.json, and feature_inventory.json to produce an objective signal
// for CoverageSignalReAct.
//
// CRITICAL: All "coverage" metrics are relative to F1 feature_inventory jobs,
// NOT to the full code_unit_registry. The registry may contain thousands of unrelated
// files (third-party libraries, generated code); only the F1-assigned jobs are
// the authoritative scope for Phase1 coverage.
func ComputeCoverageSignal(rt *Runtime) (*CoverageSignal, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}

	sig := &CoverageSignal{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		SchemaVersion:  coverageSignalSchemaVersion,
		ConfidenceLevel: "low",
	}

	// 1. F1-assigned job scope (authoritative for Phase1 coverage)
	//    Denominator = jobs from feature_inventory, NOT registry units.
	f1Jobs, _ := loadJobsFromInventoryOrRegistry(rt)
	f1Total := len(f1Jobs)
	sig.TotalHttpEntries = f1Total // Override registry count with F1 scope

	// 2. Static harvest routes
	hints := loadStaticHarvestHints(rt)
	sig.StaticHarvestRoutes = uniqueRouteKeys(hints)

	// 2. Discovered routes from feature_api_map
	sig.DiscoveredRoutes = loadDiscoveredRouteKeys(rt)

	// 3. Undiscovered routes
	discoveredSet := strSet(sig.DiscoveredRoutes)
	for _, r := range sig.StaticHarvestRoutes {
		if !discoveredSet[r] {
			sig.UndiscoveredRoutes = append(sig.UndiscoveredRoutes, r)
		}
	}

	// 4. Route coverage percentage
	if len(sig.StaticHarvestRoutes) > 0 {
		sig.RouteCoveragePct = float64(len(sig.DiscoveredRoutes)) / float64(len(sig.StaticHarvestRoutes)) * 100
	} else {
		sig.RouteCoveragePct = 0
	}

	// 5. HTTP-entry unit coverage — based on F1 feature_inventory jobs, NOT registry.
	//    sig.TotalHttpEntries was already set to f1Total above.
	//    Build pending set from F1 jobs not yet in analyzedSet.
	analyzedSet := loadAnalyzedEntrySet(rt)
	sig.AnalyzedEntries = len(analyzedSet)

	// Use F1 job entry files as the authoritative pending set.
	f1Pending := loadF1PendingEntryFiles(rt, f1Jobs, analyzedSet)
	sig.PendingEntries = f1Pending

	if sig.TotalHttpEntries > 0 {
		sig.EntryCoveragePct = float64(sig.AnalyzedEntries) / float64(sig.TotalHttpEntries) * 100
	}

	// 6. Priority tiers
	sig.PriorityTiers = tierPendingEntries(sig.PendingEntries)

	// 7. Suggested read queue (P0 first, capped at batch size hint)
	sig.ReadQueueHint = suggestedReadQueue(sig.PriorityTiers)

	// 8. Low-value packages
	sig.LowValuePackages = lowValuePackages()

	// 9. Confidence level (programmatic hint, not a gate)
	sig.ConfidenceLevel = computeConfidenceLevel(sig)

	sort.Strings(sig.StaticHarvestRoutes)
	sort.Strings(sig.DiscoveredRoutes)
	sort.Strings(sig.UndiscoveredRoutes)
	sort.Strings(sig.PendingEntries)

	return sig, nil
}

// PersistCoverageSignal writes the signal to coverage_signal.json and phase artifact.
func PersistCoverageSignal(rt *Runtime, sig *CoverageSignal) error {
	if rt == nil || sig == nil {
		return nil
	}
	b, err := json.MarshalIndent(sig, "", "  ")
	if err != nil {
		return err
	}
	path := store.CoverageSignalPath(rt.WorkDir)
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactCoverageSignal, string(b))
	}
	return nil
}

// LoadCoverageSignal reads a previously persisted signal.
func LoadCoverageSignal(workDir string) (*CoverageSignal, error) {
	var sig CoverageSignal
	return loadJSONArtifact(store.CoverageSignalPath(workDir), &sig)
}

// --- helpers ---

func coverageRouteKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimLeft(strings.TrimSpace(path), "/")
}

func uniqueRouteKeys(hints []StaticRouteHint) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, h := range hints {
		k := coverageRouteKey(h.Method, h.PathPattern)
		if k == " " || k == "" {
			continue
		}
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func loadStaticHarvestHints(rt *Runtime) []StaticRouteHint {
	rep, err := readStaticRouteHintsReport(rt.WorkDir)
	if err != nil || rep == nil {
		return nil
	}
	return rep.Hints
}

func loadDiscoveredRouteKeys(rt *Runtime) []string {
	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	if err != nil || apiMap == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, f := range apiMap.Features {
		if !f.Processed {
			continue
		}
		for _, a := range f.Apis {
			k := coverageRouteKey(a.Method, a.PathPattern)
			if k == " " || k == "" {
				continue
			}
			if _, dup := seen[k]; !dup {
				seen[k] = struct{}{}
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func loadHttpEntryPaths(rt *Runtime) []string {
	reg, err := loadCodeUnitRegistry(rt.WorkDir)
	if err != nil || reg == nil {
		return nil
	}
	var out []string
	for _, u := range reg.Units {
		if u.KindHint == codeUnitKindHintHTTPEntry {
			out = append(out, u.RelPath)
		}
	}
	sort.Strings(out)
	return out
}

func loadAnalyzedEntrySet(rt *Runtime) map[string]bool {
	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	out := map[string]bool{}
	if err != nil || apiMap == nil {
		return out
	}
	inv, _ := loadFeatureInventory(rt.WorkDir)
	entryByFeature := map[string]string{}
	if inv != nil {
		for _, f := range inv.Features {
			for _, ef := range EntryFilesForFeature(f) {
				if rel := normEntryFileRef(ef); rel != "" {
					entryByFeature[f.FeatureID] = rel
				}
			}
		}
	}
	seen := map[string]struct{}{}
	for _, f := range apiMap.Features {
		if !f.Processed {
			continue
		}
		if rel := entryByFeature[f.FeatureID]; rel != "" {
			if _, dup := seen[rel]; !dup {
				seen[rel] = struct{}{}
				out[rel] = true
			}
		}
		for _, a := range f.Apis {
			rel := normalizePlanFileRef(rt, a.HandlerFile)
			if rel == "" {
				rel = normEntryFileRef(a.HandlerFile)
			}
			if rel == "" {
				continue
			}
			if _, dup := seen[rel]; !dup {
				seen[rel] = struct{}{}
				out[rel] = true
			}
		}
	}
	return out
}

func tierPendingEntries(pending []string) PriorityTiers {
	var t PriorityTiers
	for _, rel := range pending {
		lower := strings.ToLower(strings.ReplaceAll(rel, "\\", "/"))
		switch {
		case strings.Contains(lower, "/controller/") || strings.Contains(lower, "controller.java"):
			t.P0 = append(t.P0, rel)
		case strings.Contains(lower, "/service/") || strings.Contains(lower, "/interceptor/") ||
			strings.Contains(lower, "/security/") || strings.Contains(lower, "/auth/"):
			t.P1 = append(t.P1, rel)
		default:
			t.P2 = append(t.P2, rel)
		}
	}
	sort.Strings(t.P0)
	sort.Strings(t.P1)
	sort.Strings(t.P2)
	return t
}

func suggestedReadQueue(t PriorityTiers) []string {
	const batchHint = 8
	var queue []string
	for _, rel := range t.P0 {
		if len(queue) >= batchHint {
			break
		}
		queue = append(queue, rel)
	}
	if len(queue) < batchHint {
		for _, rel := range t.P1 {
			if len(queue) >= batchHint {
				break
			}
			queue = append(queue, rel)
		}
	}
	return queue
}

func lowValuePackages() []string {
	return []string{
		"com/publiccms/views/directive/",
		"com/publiccms/entities/",
		"com/publiccms/logic/dao/",
		"com/publiccms/views/pojo/",
		"com/publiccms/views/method/",
		"com/google/typography/",
	}
}

func computeConfidenceLevel(sig *CoverageSignal) string {
	highRoutes := sig.RouteCoveragePct >= 85 && len(sig.UndiscoveredRoutes) <= 5
	highEntries := sig.EntryCoveragePct >= 90

	if highRoutes && highEntries {
		return "high"
	}
	if sig.RouteCoveragePct >= 50 || sig.EntryCoveragePct >= 50 {
		return "medium"
	}
	return "low"
}

func strSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}

// CoverageSignalSummary returns a human-readable summary of the signal for logging.
func CoverageSignalSummary(sig *CoverageSignal) string {
	if sig == nil {
		return "coverage_signal: (nil)"
	}
	return fmt.Sprintf(
		"coverage_signal: route_pct=%.1f%%(%d/%d) entry_pct=%.1f%%(%d/%d) pending=%d confidence=%s",
		sig.RouteCoveragePct, len(sig.DiscoveredRoutes), len(sig.StaticHarvestRoutes),
		sig.EntryCoveragePct, sig.AnalyzedEntries, sig.TotalHttpEntries,
		len(sig.PendingEntries), sig.ConfidenceLevel,
	)
}

// SummarizeCoverageSignalForReAct returns a compact markdown table of the signal for prompt injection.
func SummarizeCoverageSignalForReAct(sig *CoverageSignal) string {
	if sig == nil {
		return "_No coverage signal available._"
	}
	var b strings.Builder
	b.WriteString("## Current Coverage Signal\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	b.WriteString(fmt.Sprintf("| static harvest routes | %d |\n", len(sig.StaticHarvestRoutes)))
	b.WriteString(fmt.Sprintf("| discovered routes (AI) | %d |\n", len(sig.DiscoveredRoutes)))
	b.WriteString(fmt.Sprintf("| **route coverage** | **%.1f%%** |\n", sig.RouteCoveragePct))
	b.WriteString(fmt.Sprintf("| undiscovered routes | %d |\n", len(sig.UndiscoveredRoutes)))
	b.WriteString(fmt.Sprintf("| http_entry units total | %d |\n", sig.TotalHttpEntries))
	b.WriteString(fmt.Sprintf("| http_entry analyzed | %d |\n", sig.AnalyzedEntries))
	b.WriteString(fmt.Sprintf("| **entry coverage** | **%.1f%%** |\n", sig.EntryCoveragePct))
	b.WriteString(fmt.Sprintf("| pending entries | %d |\n", len(sig.PendingEntries)))
	b.WriteString(fmt.Sprintf("| confidence | %s |\n", sig.ConfidenceLevel))
	if len(sig.UndiscoveredRoutes) > 0 {
		b.WriteString("| top undiscovered | ")
		if len(sig.UndiscoveredRoutes) > 5 {
			b.WriteString(fmt.Sprintf("%v ... (%d more)", sig.UndiscoveredRoutes[:5], len(sig.UndiscoveredRoutes)-5))
		} else {
			b.WriteString(fmt.Sprintf("%v", sig.UndiscoveredRoutes))
		}
		b.WriteString(" |\n")
	}
	b.WriteString("| tier P0 (controller) | ")
	if len(sig.PriorityTiers.P0) > 0 {
		b.WriteString(fmt.Sprintf("%d files", len(sig.PriorityTiers.P0)))
	} else {
		b.WriteString("-")
	}
	b.WriteString(" |\n")
	b.WriteString("| tier P1 (service/auth) | ")
	if len(sig.PriorityTiers.P1) > 0 {
		b.WriteString(fmt.Sprintf("%d files", len(sig.PriorityTiers.P1)))
	} else {
		b.WriteString("-")
	}
	b.WriteString(" |\n")
	b.WriteString("| tier P2 (config/other) | ")
	if len(sig.PriorityTiers.P2) > 0 {
		b.WriteString(fmt.Sprintf("%d files", len(sig.PriorityTiers.P2)))
	} else {
		b.WriteString("-")
	}
	b.WriteString(" |\n")
	return b.String()
}

// CoverageSignalForPrompt returns the signal as a JSON string for prompt injection.
func CoverageSignalForPrompt(sig *CoverageSignal) string {
	if sig == nil {
		return "{}"
	}
	b, err := json.MarshalIndent(sig, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// UndiscoveredRouteLabels returns a slice of short human-readable labels for undiscovered routes.
func UndiscoveredRouteLabels(sig *CoverageSignal) []string {
	var out []string
	for _, r := range sig.UndiscoveredRoutes {
		out = append(out, r)
	}
	if len(out) > 10 {
		return out[:10]
	}
	return out
}

// loadF1PendingEntryFiles returns the list of F1-assigned entry files that have not
// yet been analyzed (not in analyzedSet). Uses feature_work_progress for done-set
// as the source of truth, augmented with feature_api_map processed entries.
func loadF1PendingEntryFiles(rt *Runtime, f1Jobs []FeatureWorkJob, analyzedSet map[string]bool) []string {
	// Build the full set of F1-assigned entry files.
	f1EntrySet := map[string]struct{}{}
	for _, job := range f1Jobs {
		rel := normalizePlanFileRef(rt, job.EntryFile)
		if rel != "" {
			f1EntrySet[rel] = struct{}{}
		}
	}

	// Start from F1 entries, remove already-analyzed.
	var pending []string
	for rel := range f1EntrySet {
		if !analyzedSet[rel] {
			pending = append(pending, rel)
		}
	}
	sort.Strings(pending)
	return pending
}
