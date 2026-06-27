package loop_ssa_api_discovery

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func writeFrameworkToolkitGateArtifacts(rt *Runtime, catalog *CombinedAPICatalog, report *ToolkitVerifyReport, frameworkID string) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	if err := ensureFeatureInventoryFromRegistry(rt); err != nil {
		return err
	}
	apiMap, err := buildFeatureApiMapFromCatalog(rt, catalog, report)
	if err != nil {
		return err
	}
	if err := persistFeatureApiMap(rt, apiMap); err != nil {
		return err
	}
	if err := writeToolkitFeatureWorkProgress(rt); err != nil {
		return err
	}
	if err := writeToolkitCoverageSignalDecision(rt, frameworkID); err != nil {
		return err
	}
	if _, err := writeRouteCandidatesFromCatalog(rt, catalog); err != nil {
		return err
	}
	if err := writeMinimalPhase1PrepBundle(rt, "framework_toolkit:"+frameworkID); err != nil {
		return err
	}
	fs := &FailureSemanticsV1{
		SchemaVersion: artifactV2SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Categories: []FailureSemanticsCategory{
			{Kind: "unauthorized", StatusCodes: []int{401, 403}, Description: "toolkit default"},
			{Kind: "wrong_path", StatusCodes: []int{404}, Description: "toolkit default"},
		},
	}
	if err := persistFailureSemantics(rt, fs); err != nil {
		log.Warnf("ssa_api_discovery: failure_semantics bootstrap: %v", err)
	}
	if err := SyncFeatureApiMapToVerifiedHttpApis(rt); err != nil {
		return err
	}
	if err := BackfillAllVerifiedHttpApisCatalog(rt); err != nil {
		log.Warnf("ssa_api_discovery: verified backfill: %v", err)
	}
	return nil
}

func ensureFeatureInventoryFromRegistry(rt *Runtime) error {
	if _, err := loadFeatureInventory(rt.WorkDir); err == nil {
		return nil
	}
	if _, err := loadCodeUnitRegistry(rt.WorkDir); err != nil {
		if _, berr := BuildCodeUnitRegistry(rt); berr != nil {
			return berr
		}
	}
	return BackfillFeatureInventoryFromRegistry(rt)
}

func buildFeatureApiMapFromCatalog(rt *Runtime, catalog *CombinedAPICatalog, report *ToolkitVerifyReport) (*FeatureApiMapV1, error) {
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err != nil {
		return nil, err
	}
	byFeature := map[string]*FeatureApiMapEntry{}
	invByID := map[string]FeatureInventoryEntry{}
	for _, feat := range inv.Features {
		invByID[feat.FeatureID] = feat
		if strings.TrimSpace(feat.SurfaceKind) != SurfaceKindHTTPAPI {
			continue
		}
		entry := &FeatureApiMapEntry{
			FeatureID: feat.FeatureID,
			Label:     feat.Label,
			Processed: true,
		}
		byFeature[feat.FeatureID] = entry
	}
	if catalog != nil {
		verifiedKeys := map[string]struct{}{}
		if rt.Repo != nil && rt.Session != nil {
			vha, _ := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
			for i := range vha {
				v := &vha[i]
				if store.VerifiedHttpApiHasProbeEvidence(v) && v.Verified {
					verifiedKeys[routeKey(v.Method, v.PathPattern)] = struct{}{}
				}
			}
		}
		for _, rec := range catalog.Records {
			featID := featureIDForCombinedRecord(rt, inv, rec)
			entry, ok := byFeature[featID]
			if !ok {
				entry = &FeatureApiMapEntry{
					FeatureID: featID,
					Label:     filepath.Base(rec.BackendFile),
					Processed: true,
				}
				byFeature[featID] = entry
			}
			api := combinedRecordToFeatureAPIEntry(rt, rec)
			if _, ok := verifiedKeys[routeKey(api.Method, api.PathPattern)]; ok {
				api.Verified = true
				api.VerdictReason = "framework_toolkit bulk verify"
			}
			entry.Apis = append(entry.Apis, api)
		}
	}
	out := &FeatureApiMapV1{
		SchemaVersion: artifactV2SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Features:      make([]FeatureApiMapEntry, 0, len(byFeature)),
	}
	for _, e := range byFeature {
		e.ApiCount = len(e.Apis)
		if e.ApiCount == 0 {
			if report != nil && !rt.Session.TargetReachable {
				e.NoApiReason = "target unreachable; static catalog only"
			} else {
				e.NoApiReason = inferToolkitNoApiReason(invByID[e.FeatureID])
			}
		}
		out.Features = append(out.Features, *e)
	}
	return out, nil
}

// inferToolkitNoApiReason explains http_api features with zero catalog routes so Phase1 gate passes.
func inferToolkitNoApiReason(feat FeatureInventoryEntry) string {
	if strings.TrimSpace(feat.SurfaceKind) == SurfaceKindCodeOnly {
		return "code_only feature"
	}
	label := strings.TrimSpace(feat.Label)
	base := strings.ToLower(filepath.Base(label))
	if base == "package-info.java" {
		return "package metadata; no HTTP endpoints"
	}
	if strings.HasPrefix(label, "Abstract") && strings.HasSuffix(label, "Controller.java") {
		return "abstract base class; routes belong to concrete subclasses"
	}
	if strings.HasSuffix(label, "Advice.java") || strings.Contains(label, "ExceptionAdvice") {
		return "@ControllerAdvice; no direct HTTP routes"
	}
	for _, ef := range EntryFilesForFeature(feat) {
		if strings.ToLower(filepath.Base(ef)) == "package-info.java" {
			return "package metadata; no HTTP endpoints"
		}
	}
	return "no routes in static catalog after framework_toolkit extract"
}

func featureIDForCombinedRecord(rt *Runtime, inv *FeatureInventoryV1, rec CombinedAPIRecord) string {
	if inv != nil {
		for _, feat := range inv.Features {
			for _, ef := range EntryFilesForFeature(feat) {
				if pathsReferSameController(ef, rec.BackendFile) {
					return feat.FeatureID
				}
			}
		}
	}
	base := strings.TrimSuffix(filepath.Base(rec.BackendFile), ".java")
	if base == "" {
		base = "catalog"
	}
	return "http_entry_" + normEntryFileRef(base)
}

func pathsReferSameController(a, b string) bool {
	a = strings.ToLower(filepath.Base(a))
	b = strings.ToLower(filepath.Base(b))
	return a != "" && a == b
}

func combinedRecordToFeatureAPIEntry(rt *Runtime, rec CombinedAPIRecord) FeatureApiEntry {
	path := normURLPath(rec.Path)
	fullURL := buildBulkVerifyURL(rt, path)
	verified := false
	if rt != nil && rt.Session != nil && !rt.Session.TargetReachable {
		verified = false
	}
	return FeatureApiEntry{
		Method:        rec.Method,
		PathPattern:   path,
		HandlerFile:   rec.BackendFile,
		HandlerClass:  rec.HandlerClass,
		HandlerSymbol: rec.HandlerMethod,
		AuthRealm:     rec.Auth.Realm,
		Verified:      verified,
		FullSampleURL: fullURL,
		VerdictReason: "framework_toolkit catalog entry",
	}
}

func writeToolkitFeatureWorkProgress(rt *Runtime) error {
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err != nil {
		return err
	}
	var entries []featureWorkProgressEntry
	for _, feat := range inv.Features {
		for _, ef := range EntryFilesForFeature(feat) {
			entries = append(entries, featureWorkProgressEntry{
				EntryFile: ef,
				JobKind:   feat.SurfaceKind,
				Status:    featureWorkStatusDone,
				Reason:    "skipped_by_toolkit",
			})
		}
	}
	return saveFeatureWorkProgress(rt.WorkDir, featureWorkProgress{Entries: entries})
}

func writeToolkitCoverageSignalDecision(rt *Runtime, frameworkID string) error {
	decision := &CoverageSignalDecision{
		Verdict:    "finish",
		Reasoning:  "framework_toolkit fast path completed for " + frameworkID,
		SignalJSON: `{"source":"framework_toolkit"}`,
	}
	persistCoverageSignalDecision(rt, decision)
	return nil
}

func writeRouteCandidatesFromCatalog(rt *Runtime, catalog *CombinedAPICatalog) (int, error) {
	if catalog == nil {
		return 0, nil
	}
	var candidates []RouteCandidate
	for _, rec := range catalog.Records {
		candidates = append(candidates, RouteCandidate{
			Method:     rec.Method,
			Path:       rec.Path,
			ClassName:  rec.HandlerClass,
			MethodName: rec.HandlerMethod,
			Source:     "framework_toolkit",
			Framework:  rt.SelectedFrameworkID,
			AuthHint:   strings.Join(rec.Auth.Mechanisms, ","),
		})
	}
	path := store.RouteCandidatesPath(rt.WorkDir)
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := writeJSONFile(path, data); err != nil {
		return 0, err
	}
	return len(candidates), nil
}

func shouldSkipVPhaseForToolkit(rt *Runtime) bool {
	if rt == nil {
		return false
	}
	return rt.FrameworkToolkitEnabled && rt.FrameworkToolkitMode == FrameworkToolkitModeFast
}
