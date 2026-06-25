package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	failureKindWrongPath    = "wrong_path"
	failureKindWrongMethod  = "wrong_method"
	failureKindWrongParam   = "wrong_param"
	failureKindUnauthorized = "unauthorized"
	failureKindSuccess      = "success"
)

// DefaultFailureSemantics returns built-in patterns when artifact is missing.
func DefaultFailureSemantics() *FailureSemanticsV1 {
	return &FailureSemanticsV1{
		SchemaVersion: artifactV2SchemaVersion,
		Categories: []FailureSemanticsCategory{
			{
				Kind:         failureKindWrongPath,
				Description:  "Route not matched; generic API fallback",
				BodyPatterns: []string{`"interfaceNotFound"`, `interfaceNotFound`, `interface not found`, `No handler found`},
				RouteVerdict: "wrong_route",
			},
			{
				Kind:            failureKindUnauthorized,
				Description:     "Auth required or session expired",
				StatusCodes:     []int{401, 403},
				BodyPatterns:    []string{`<title>.*[Ll]ogin`, `Username does not exist`, `Wrong password`, `未登录`, `请登录`},
				ContentTypeHint: "text/html",
				RouteVerdict:    "auth_required",
			},
			{
				Kind:         failureKindWrongMethod,
				Description:  "HTTP method not allowed",
				StatusCodes:  []int{405},
				BodyPatterns: []string{`Method Not Allowed`, `HTTP Status 405`},
				RouteVerdict: "wrong_method",
			},
			{
				Kind:         failureKindWrongParam,
				Description:  "Missing or invalid parameters",
				StatusCodes:  []int{400, 422},
				BodyPatterns: []string{`"statusCode":"400"`, `validation`, `required`, `参数`},
				RouteVerdict: "wrong_param",
			},
			{
				Kind:         failureKindSuccess,
				Description:  "Business JSON response",
				BodyPatterns: []string{`"statusCode":"20`, `"statusCode":"30`, `"code":0`, `"success":true`},
				RouteVerdict: "hit",
			},
		},
	}
}

func loadFailureSemanticsOrDefault(workDir string) *FailureSemanticsV1 {
	fs, err := loadFailureSemantics(workDir)
	if err != nil || fs == nil || len(fs.Categories) == 0 {
		return DefaultFailureSemantics()
	}
	return fs
}

func failureSemanticsJSONForProbe(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	fs := loadFailureSemanticsOrDefault(rt.WorkDir)
	b, _ := json.Marshal(fs)
	return string(b)
}

func authCalibrationJSONForProbe(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	c, err := loadAuthCalibration(rt.WorkDir)
	if err != nil || c == nil {
		return ""
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ClassifyProbeResponse maps status/body to a failure semantics kind.
func ClassifyProbeResponse(fs *FailureSemanticsV1, statusCode int, contentType, body string) (kind string, routeVerdict string) {
	if fs == nil {
		fs = DefaultFailureSemantics()
	}
	bodyLower := strings.ToLower(body)
	ctLower := strings.ToLower(contentType)
	for _, cat := range fs.Categories {
		if len(cat.StatusCodes) > 0 {
			matched := false
			for _, sc := range cat.StatusCodes {
				if sc == statusCode {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if cat.ContentTypeHint != "" && !strings.Contains(ctLower, strings.ToLower(cat.ContentTypeHint)) {
			continue
		}
		if matchBodyPatterns(bodyLower, cat.BodyPatterns) {
			return cat.Kind, firstNonEmpty(cat.RouteVerdict, kindToVerdict(cat.Kind))
		}
	}
	if statusCode == 404 {
		return failureKindWrongPath, "wrong_route"
	}
	if statusCode == 405 {
		return failureKindWrongMethod, "wrong_method"
	}
	if statusCode == 401 || statusCode == 403 {
		return failureKindUnauthorized, "auth_required"
	}
	if statusCode >= 200 && statusCode < 300 && strings.Contains(bodyLower, "interfaceNotFound") {
		return failureKindWrongPath, "wrong_route"
	}
	if statusCode >= 200 && statusCode < 400 && strings.Contains(ctLower, "text/html") && strings.Contains(bodyLower, "login") {
		return failureKindUnauthorized, "auth_required"
	}
	return "", "inconclusive"
}

func kindToVerdict(kind string) string {
	switch kind {
	case failureKindWrongPath:
		return "wrong_route"
	case failureKindWrongMethod:
		return "wrong_method"
	case failureKindWrongParam:
		return "wrong_param"
	case failureKindUnauthorized:
		return "auth_required"
	case failureKindSuccess:
		return "hit"
	default:
		return "inconclusive"
	}
}

func matchBodyPatterns(bodyLower string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "(?") || strings.Contains(p, `.*`) {
			if re, err := regexp.Compile("(?i)" + p); err == nil && re.MatchString(bodyLower) {
				return true
			}
			continue
		}
		if strings.Contains(bodyLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// ApplyFailureSemanticsToProbeResult overrides AI verdict when response matches negative semantics.
func ApplyFailureSemanticsToProbeResult(rt *Runtime, pr *ProbeResult) {
	if pr == nil || rt == nil {
		return
	}
	fs := loadFailureSemanticsOrDefault(rt.WorkDir)
	kind, verdict := ClassifyProbeResponse(fs, pr.ProbeStatusCode, pr.ContentType, pr.ResponseExcerpt)
	if kind == "" {
		return
	}
	switch kind {
	case failureKindWrongPath, failureKindWrongMethod:
		if pr.Verified {
			pr.Verified = false
			pr.RejectReason = "failure_semantics_override: " + kind
			if pr.VerdictReason == "" {
				pr.VerdictReason = pr.RejectReason
			} else {
				pr.VerdictReason = pr.RejectReason + "; was: " + pr.VerdictReason
			}
		}
	case failureKindUnauthorized:
		// Route may exist but needs auth — without credentials, treat as skipped analysis.
		if pr.Verified && strings.Contains(strings.ToLower(pr.ResponseExcerpt), "login") {
			pr.Verified = false
			pr.RejectReason = "failure_semantics_override: unauthorized/login page"
		}
		if !pr.Verified && pr.RejectReason == "" && strings.TrimSpace(pr.AuthHeadersJSON) == "" {
			pr.RejectReason = "auth_required_skipped"
			if pr.VerdictReason == "" {
				pr.VerdictReason = "unauthenticated probe returned auth wall (401/403); skip analysis"
			}
		}
	case failureKindSuccess:
		if !pr.Verified && verdict == "hit" {
			pr.Verified = true
			if pr.VerdictReason == "" {
				pr.VerdictReason = "failure_semantics: success pattern matched"
			}
		}
	}
	_ = verdict
}

func validateFailureSemantics(fs *FailureSemanticsV1) error {
	if fs == nil {
		return utils.Error("failure_semantics required")
	}
	required := []string{failureKindWrongPath, failureKindUnauthorized, failureKindSuccess}
	found := map[string]bool{}
	for _, c := range fs.Categories {
		k := strings.TrimSpace(c.Kind)
		if k != "" {
			found[k] = true
		}
		if k == failureKindWrongPath && len(c.BodyPatterns) == 0 && len(c.StatusCodes) == 0 {
			return utils.Error("wrong_path category needs body_patterns or status_codes")
		}
	}
	for _, r := range required {
		if !found[r] {
			return utils.Errorf("failure_semantics missing required category: %s", r)
		}
	}
	return nil
}

func validateAuthSurfaceMap(m *AuthSurfaceMapV1) error {
	if m == nil {
		return utils.Error("auth_surface_map required")
	}
	if len(m.Surfaces) == 0 {
		return utils.Error("auth_surface_map.surfaces required")
	}
	for i, s := range m.Surfaces {
		if strings.TrimSpace(s.AuthRealm) == "" {
			return utils.Errorf("surfaces[%d].auth_realm required", i)
		}
		if len(s.PackagePatterns) == 0 && len(s.PathPrefixes) == 0 {
			return utils.Errorf("surfaces[%d] needs package_patterns or path_prefixes", i)
		}
	}
	return nil
}

func validateAuthCalibration(c *AuthCalibrationV1, surface *AuthSurfaceMapV1) error {
	if c == nil {
		return utils.Error("auth_calibration required")
	}
	requiredRealms := map[string]struct{}{}
	if surface != nil {
		for _, s := range surface.Surfaces {
			requiredRealms[s.AuthRealm] = struct{}{}
		}
	}
	if len(requiredRealms) == 0 {
		return nil
	}
	calibratedCount := 0
	for _, r := range c.Realms {
		if !r.Calibrated {
			continue
		}
		if len(r.Probes) < 2 {
			return utils.Errorf("auth_calibration realm %s needs at least 2 calibration probes", r.AuthRealm)
		}
		passed := 0
		for _, p := range r.Probes {
			if p.Passed {
				passed++
			}
		}
		if passed < 2 {
			return utils.Errorf("auth_calibration realm %s: calibrated=true but only %d probes passed", r.AuthRealm, passed)
		}
		calibratedCount++
	}
	if calibratedCount == 0 {
		return utils.Error("auth_calibration: no realm calibrated (need at least one successful auth mechanism)")
	}
	c.AllCalibrated = calibratedCount >= len(requiredRealms)
	return nil
}

func validateFeatureInventory(inv *FeatureInventoryV1, rt *Runtime) error {
	if inv == nil {
		return utils.Error("feature_inventory required")
	}
	if len(inv.Features) == 0 {
		return utils.Error("feature_inventory.features required")
	}
	if rt == nil {
		return utils.Error("runtime required for feature_inventory validation")
	}
	reg, err := loadCodeUnitRegistry(rt.WorkDir)
	if err != nil || reg == nil || len(reg.Units) == 0 {
		return utils.Error("code_unit_registry required; run BuildCodeUnitRegistry in prep")
	}
	registrySet := registryRelPathSet(reg)
	assignedOwner := map[string]string{}
	for i, f := range inv.Features {
		if strings.TrimSpace(f.FeatureID) == "" {
			return utils.Errorf("features[%d].feature_id required", i)
		}
		sk := strings.TrimSpace(f.SurfaceKind)
		if sk != SurfaceKindHTTPAPI && sk != SurfaceKindCodeOnly {
			return utils.Errorf("features[%d].surface_kind required (http_api|code_only)", i)
		}
		entries := EntryFilesForFeature(f)
		if len(entries) == 0 {
			return utils.Errorf("features[%d].entry_files required", i)
		}
		if sk == SurfaceKindCodeOnly && strings.TrimSpace(f.NoHttpReason) == "" {
			return utils.Errorf("features[%d].no_http_reason required when surface_kind=code_only", i)
		}
		for _, ef := range entries {
			rel := normEntryFileRef(ef)
			if rel == "" {
				return utils.Errorf("features[%d].entry_files contains empty path", i)
			}
			if _, ok := registrySet[rel]; !ok {
				return utils.Errorf("features[%d].entry_files %q not in code_unit_registry", i, rel)
			}
			if prev, dup := assignedOwner[rel]; dup {
				return utils.Errorf("entry_file %q assigned to both %s and %s", rel, prev, f.FeatureID)
			}
			assignedOwner[rel] = f.FeatureID
		}
	}
	inv.Coverage = evaluateFeatureEntryFilesCoverage(reg, inv)
	if !inv.Coverage.Complete {
		log.Warnf("feature_inventory coverage incomplete (soft gate): %s", formatFeatureCoverageFeedback(inv.Coverage))
	}
	return nil
}

func validateRoutingProfileMountPrefixes(p *RoutingProfileV1) error {
	if p == nil {
		return utils.Error("routing_profile required")
	}
	if strings.TrimSpace(strings.ToLower(p.ValidationStatus)) == "failed" {
		return utils.Error("routing_profile validation_status=failed")
	}
	hasMount := false
	for _, sp := range p.URLSpaces {
		mp := strings.TrimSpace(sp.MountPrefix)
		if mp != "" && mp != "/" {
			hasMount = true
		}
		if mp == "/" {
			hasMount = true
		}
	}
	if !hasMount && len(p.EffectiveBases) > 0 {
		hasMount = true
	}
	if !hasMount {
		return utils.Error("routing_profile: mount_prefixes empty; url_spaces[].mount_prefix or effective_bases required")
	}
	return ValidateRoutingProfileForCommit(p)
}

func ResolveAuthRealmForHandler(surface *AuthSurfaceMapV1, handlerClass string) string {
	if surface == nil {
		return ""
	}
	hc := strings.ToLower(handlerClass)
	for _, s := range surface.Surfaces {
		for _, pat := range s.PackagePatterns {
			pat = strings.ToLower(strings.TrimSpace(pat))
			if pat == "" {
				continue
			}
			pat = strings.TrimPrefix(pat, "*.")
			pat = strings.ReplaceAll(pat, "*", "")
			if pat != "" && strings.Contains(hc, pat) {
				return s.AuthRealm
			}
		}
	}
	return ""
}

func loadAuthSurfaceMapOrEmpty(workDir string) *AuthSurfaceMapV1 {
	m, _ := loadAuthSurfaceMap(workDir)
	if m == nil {
		return &AuthSurfaceMapV1{Surfaces: []AuthSurfaceEntry{}}
	}
	return m
}

func failureSemanticsExists(workDir string) bool {
	_, err := os.Stat(store.FailureSemanticsPath(workDir))
	return err == nil
}
