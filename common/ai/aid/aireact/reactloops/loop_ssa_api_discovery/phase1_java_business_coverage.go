package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const businessFunctionMapSchemaVersion = 1

// BusinessFunctionEntry is one classified business domain.
type BusinessFunctionEntry struct {
	Description     string   `json:"description,omitempty"`
	BusinessSurface string   `json:"business_surface,omitempty"`
	Modules         []string `json:"modules,omitempty"`
	ScopePaths      []string `json:"scope_paths"`
}

// BusinessCoverageResult is embedded in business_function_map.json.
type BusinessCoverageResult struct {
	Policy         string          `json:"policy"`
	TotalRequired  int             `json:"total_required"`
	Covered        int             `json:"covered"`
	Complete       bool            `json:"complete"`
	UncoveredUnits []JavaScopeUnit `json:"uncovered_units,omitempty"`
}

// BusinessFunctionMap is persisted to business_function_map.json.
type BusinessFunctionMap struct {
	SchemaVersion          int                              `json:"schema_version"`
	GeneratedAt            string                           `json:"generated_at,omitempty"`
	Language               string                           `json:"language,omitempty"`
	Layout                 string                           `json:"layout,omitempty"`
	ClassificationStrategy string                           `json:"classification_strategy,omitempty"`
	Functions              map[string]BusinessFunctionEntry `json:"functions"`
	Coverage               BusinessCoverageResult           `json:"coverage"`
}

// JavaBusinessCoverageReport is the outcome of a coverage check.
type JavaBusinessCoverageReport struct {
	Complete       bool
	TotalRequired  int
	Covered        int
	UncoveredUnits []JavaScopeUnit
	Feedback       string
}

// Phase1BusinessCoverageError stops Phase1 when business scope coverage fails after max rounds.
type Phase1BusinessCoverageError struct {
	Reason string
}

func (e *Phase1BusinessCoverageError) Error() string {
	if e == nil {
		return "phase1 business coverage failed"
	}
	return "Phase1 业务域覆盖未完成: " + e.Reason
}

// IsPhase1BusinessCoverageFailed reports deliberate business coverage stop.
func IsPhase1BusinessCoverageFailed(err error) bool {
	_, ok := err.(*Phase1BusinessCoverageError)
	return ok
}

// WritePhase1BusinessCoverageFailureReport writes a markdown summary when F1 feature inventory coverage fails.
func WritePhase1BusinessCoverageFailureReport(rt *Runtime, reason string) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	path := store.Phase1DiscoveryReportPath(rt.WorkDir)
	body := fmt.Sprintf("# Phase1 业务域覆盖失败\n\n- session: %s\n- reason: %s\n\n请检查 feature_inventory / java_business_scope_inventory 覆盖率后再重试。\n",
		rt.Session.UUID, reason)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: phase1 business coverage failure report written %s", path)
	return nil
}

func evaluateJavaBusinessCoverage(inv *JavaBusinessScopeInventory, scopePaths []string) JavaBusinessCoverageReport {
	report := JavaBusinessCoverageReport{}
	if inv == nil {
		report.Feedback = "java_business_scope_inventory missing"
		return report
	}
	required := collectRequiredScopeUnits(inv)
	report.TotalRequired = len(required)
	if report.TotalRequired == 0 {
		report.Complete = true
		return report
	}
	assigned := normalizeAssignedScopePaths(scopePaths)
	for _, u := range required {
		if unitCoveredByAssigned(u.Path, assigned) {
			report.Covered++
			continue
		}
		report.UncoveredUnits = append(report.UncoveredUnits, u)
	}
	sort.Slice(report.UncoveredUnits, func(i, j int) bool {
		return report.UncoveredUnits[i].Path < report.UncoveredUnits[j].Path
	})
	report.Complete = len(report.UncoveredUnits) == 0
	if !report.Complete {
		report.Feedback = formatBusinessCoverageFeedback(report.UncoveredUnits, report.TotalRequired, report.Covered)
	}
	return report
}

func collectRequiredScopeUnits(inv *JavaBusinessScopeInventory) []JavaScopeUnit {
	requiredKinds := map[string]struct{}{}
	for _, k := range inv.CoveragePolicy.RequiredKinds {
		requiredKinds[k] = struct{}{}
	}
	optionalKinds := map[string]struct{}{}
	for _, k := range inv.CoveragePolicy.OptionalKinds {
		optionalKinds[k] = struct{}{}
	}
	var out []JavaScopeUnit
	for _, mod := range inv.Modules {
		for _, u := range mod.ScopeUnits {
			if _, ok := requiredKinds[u.Kind]; ok {
				out = append(out, u)
				continue
			}
			if _, ok := optionalKinds[u.Kind]; ok {
				out = append(out, u)
			}
		}
	}
	return out
}

func normalizeAssignedScopePaths(scopePaths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range scopePaths {
		p = normScopePath(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func unitCoveredByAssigned(unitPath string, assigned []string) bool {
	for _, a := range assigned {
		if isScopePathDescendantOrEqual(unitPath, a) {
			return true
		}
	}
	return false
}

func formatBusinessCoverageFeedback(uncovered []JavaScopeUnit, total, covered int) string {
	return formatBusinessCoverageFeedbackIncremental(uncovered, total, covered, 0)
}

func formatBusinessCoverageFeedbackIncremental(uncovered []JavaScopeUnit, total, covered, newlyCovered int) string {
	missing := total - covered
	var b strings.Builder
	if newlyCovered > 0 {
		b.WriteString(fmt.Sprintf("本轮新增覆盖 %d 个 scope unit（累计 %d/%d）。\n", newlyCovered, covered, total))
	} else {
		b.WriteString(fmt.Sprintf("业务域覆盖未完成：还需覆盖 %d 个 scope unit（累计 %d/%d）。\n", missing, covered, total))
	}
	if missing <= 0 {
		return b.String()
	}
	b.WriteString("仍缺失（下一 round 只需补齐这些 path，已覆盖的会自动保留）：\n")
	limit := len(uncovered)
	if limit > 40 {
		limit = 40
	}
	for i := 0; i < limit; i++ {
		u := uncovered[i]
		hints := strings.Join(u.Hints, ",")
		if hints != "" {
			hints = " hints=" + hints
		}
		b.WriteString(fmt.Sprintf("- [%s] %s domain=%s%s\n", u.Kind, u.Path, u.DomainSegment, hints))
	}
	if len(uncovered) > limit {
		b.WriteString(fmt.Sprintf("… 另有 %d 条未列出\n", len(uncovered)-limit))
	}
	b.WriteString("下一 round 可只提交**增量** function 块（或向已有块追加 scope_paths），系统会自动与历史覆盖合并。")
	return b.String()
}

// Phase1BusinessFunctionResult is the finalize payload from the business ReAct loop.
type Phase1BusinessFunctionResult struct {
	Language               string                           `json:"language,omitempty"`
	Layout                 string                           `json:"layout,omitempty"`
	ClassificationStrategy string                           `json:"classification_strategy,omitempty"`
	Functions              map[string]BusinessFunctionEntry `json:"functions"`
}

func buildBusinessFunctionMap(inv *JavaBusinessScopeInventory, result *Phase1BusinessFunctionResult) (*BusinessFunctionMap, error) {
	if result == nil {
		return nil, utils.Error("nil business function result")
	}
	if len(result.Functions) == 0 {
		return nil, utils.Error("functions required")
	}
	scopePaths := collectScopePathsFromFunctionMapPayload(result.Functions)
	cov := evaluateJavaBusinessCoverage(inv, scopePaths)
	m := &BusinessFunctionMap{
		SchemaVersion:          businessFunctionMapSchemaVersion,
		Language:               result.Language,
		Layout:                 result.Layout,
		ClassificationStrategy: result.ClassificationStrategy,
		Functions:              result.Functions,
		Coverage: BusinessCoverageResult{
			Policy:         "java_package_units_must_be_covered",
			TotalRequired:  cov.TotalRequired,
			Covered:        cov.Covered,
			Complete:       cov.Complete,
			UncoveredUnits: cov.UncoveredUnits,
		},
	}
	if inv != nil {
		if m.Language == "" {
			m.Language = inv.Language
		}
		if m.Layout == "" {
			m.Layout = inv.Layout
		}
	}
	return m, nil
}

func collectScopePathsFromFunctionMapPayload(functions map[string]BusinessFunctionEntry) []string {
	if len(functions) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, fn := range functions {
		for _, p := range fn.ScopePaths {
			p = normScopePath(p)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// mergeBusinessFunctionResults overlays delta onto accum; scope_paths and modules union per function name.
func mergeBusinessFunctionResults(accum, delta *Phase1BusinessFunctionResult) *Phase1BusinessFunctionResult {
	if delta == nil || len(delta.Functions) == 0 {
		if accum == nil {
			return &Phase1BusinessFunctionResult{Functions: map[string]BusinessFunctionEntry{}}
		}
		out := *accum
		if out.Functions == nil {
			out.Functions = map[string]BusinessFunctionEntry{}
		} else {
			out.Functions = cloneBusinessFunctionMap(out.Functions)
		}
		return &out
	}
	out := &Phase1BusinessFunctionResult{Functions: map[string]BusinessFunctionEntry{}}
	if accum != nil {
		out.Language = accum.Language
		out.Layout = accum.Layout
		out.ClassificationStrategy = accum.ClassificationStrategy
		out.Functions = cloneBusinessFunctionMap(accum.Functions)
	}
	if delta.Language != "" {
		out.Language = delta.Language
	}
	if delta.Layout != "" {
		out.Layout = delta.Layout
	}
	if delta.ClassificationStrategy != "" {
		out.ClassificationStrategy = delta.ClassificationStrategy
	}
	for name, fn := range delta.Functions {
		if prev, ok := out.Functions[name]; ok {
			out.Functions[name] = mergeBusinessFunctionEntry(prev, fn)
		} else {
			out.Functions[name] = fn
		}
	}
	return out
}

func cloneBusinessFunctionMap(in map[string]BusinessFunctionEntry) map[string]BusinessFunctionEntry {
	if len(in) == 0 {
		return map[string]BusinessFunctionEntry{}
	}
	out := make(map[string]BusinessFunctionEntry, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeBusinessFunctionEntry(a, b BusinessFunctionEntry) BusinessFunctionEntry {
	out := a
	if strings.TrimSpace(b.Description) != "" {
		out.Description = b.Description
	}
	if strings.TrimSpace(b.BusinessSurface) != "" {
		out.BusinessSurface = b.BusinessSurface
	}
	out.Modules = unionStrings(a.Modules, b.Modules)
	out.ScopePaths = unionScopePaths(a.ScopePaths, b.ScopePaths)
	return out
}

func unionStrings(a, b []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range append(a, b...) {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func unionScopePaths(a, b []string) []string {
	return normalizeAssignedScopePaths(append(a, b...))
}

func countNewlyCoveredUnits(inv *JavaBusinessScopeInventory, prevPaths, mergedPaths []string) int {
	if inv == nil {
		return 0
	}
	prev := evaluateJavaBusinessCoverage(inv, prevPaths)
	merged := evaluateJavaBusinessCoverage(inv, mergedPaths)
	if merged.Covered <= prev.Covered {
		return 0
	}
	return merged.Covered - prev.Covered
}

func persistBusinessFunctionMap(rt *Runtime, m *BusinessFunctionMap) error {
	if rt == nil || m == nil {
		return utils.Error("nil map")
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(store.BusinessFunctionMapPath(rt.WorkDir), b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactBusinessFunctionMap, string(b))
	}
	return syncBusinessFunctionMapToDB(rt, m)
}

func syncBusinessFunctionMapToDB(rt *Runtime, m *BusinessFunctionMap) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil || m == nil {
		return nil
	}
	existing, _ := rt.Repo.ListBusinessCapabilities(rt.Session.ID)
	byName := map[string]store.BusinessCapability{}
	for _, row := range existing {
		byName[row.Name] = row
	}
	for name, fn := range m.Functions {
		scopeJSON, _ := json.Marshal(fn.ScopePaths)
		modJSON, _ := json.Marshal(fn.Modules)
		row := &store.BusinessCapability{
			SessionID:       rt.Session.ID,
			Name:            name,
			Description:     fn.Description,
			LayerHint:       fn.BusinessSurface,
			ScopePathsJSON:  string(scopeJSON),
			ModuleHintsJSON: string(modJSON),
		}
		if ex, ok := byName[name]; ok {
			row.ID = ex.ID
			row.CreatedAt = ex.CreatedAt
			if err := rt.Repo.UpdateBusinessCapability(row); err != nil {
				return err
			}
			continue
		}
		if err := rt.Repo.CreateBusinessCapability(row); err != nil {
			return err
		}
	}
	log.Infof("ssa_api_discovery: synced business_function_map functions=%d", len(m.Functions))
	return nil
}

func loadBusinessFunctionMap(workDir string) (*BusinessFunctionMap, error) {
	b, err := os.ReadFile(store.BusinessFunctionMapPath(workDir))
	if err != nil {
		return nil, err
	}
	var m BusinessFunctionMap
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func summarizeJavaScopeInventory(inv *JavaBusinessScopeInventory) string {
	if inv == nil {
		return "inventory: missing"
	}
	return fmt.Sprintf("java_layout: %s (%d modules, %d java_package units)",
		inv.Layout, len(inv.Modules), inv.Stats.JavaPackageUnits)
}

func domainSegmentHints(inv *JavaBusinessScopeInventory) []string {
	if inv == nil {
		return nil
	}
	seen := map[string]int{}
	for _, mod := range inv.Modules {
		for _, u := range mod.ScopeUnits {
			if u.Kind != scopeUnitKindJavaPackage || u.DomainSegment == "" {
				continue
			}
			seen[u.DomainSegment]++
		}
	}
	type pair struct {
		seg string
		n   int
	}
	var pairs []pair
	for seg, n := range seen {
		pairs = append(pairs, pair{seg, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n == pairs[j].n {
			return pairs[i].seg < pairs[j].seg
		}
		return pairs[i].n > pairs[j].n
	})
	var out []string
	for i, p := range pairs {
		if i >= 20 {
			break
		}
		out = append(out, fmt.Sprintf("%s(%d)", p.seg, p.n))
	}
	return out
}
