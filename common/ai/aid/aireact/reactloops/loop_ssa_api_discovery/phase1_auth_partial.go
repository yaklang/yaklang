package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const rejectReasonAuthRealmUnavailable = "auth_realm_unavailable"

// authPartialAuthEnabled reports whether partial auth is allowed (some realms verified, others skipped).
// Enable via user input `partial_auth: yes` / Runtime.AllowPartialAuth, or env YAK_SSA_AUTH_PARTIAL_OK=1.
func authPartialAuthEnabled(rt *Runtime) bool {
	if rt != nil && rt.AllowPartialAuth {
		return true
	}
	return authPartialOkEnabled()
}

func authStateIsPartial(rt *Runtime) bool {
	if rt == nil {
		return false
	}
	b, err := os.ReadFile(store.AuthStatePath(rt.WorkDir))
	if err != nil {
		return false
	}
	var rec authStateRecord
	if json.Unmarshal(b, &rec) != nil {
		return false
	}
	return rec.State == authStatePartial
}

// authReadyForDownstream is true when full auth succeeded or partial auth is enabled with at least one verified credential.
func authReadyForDownstream(rt *Runtime) bool {
	if authVerifiedFromRuntime(rt) {
		return true
	}
	if !authPartialAuthEnabled(rt) {
		return false
	}
	return authStateIsPartial(rt) && hasVerifiedAuthCredential(rt)
}

// VerifiedAuthRealmsList returns auth realms that currently have verified DB credentials.
func VerifiedAuthRealmsList(rt *Runtime, ev *AuthEvidenceRecord) []string {
	if ev == nil && rt != nil {
		ev, _ = loadAuthEvidenceFromWorkDir(rt.WorkDir)
	}
	realms := requiredAuthRealmsForPartial(rt, ev)
	var out []string
	for _, r := range realms {
		if realmHasVerifiedCredential(rt, r) {
			out = append(out, r)
		}
	}
	if len(out) == 0 && hasVerifiedAuthCredential(rt) {
		out = append(out, "default")
	}
	return out
}

// AuthGateSatisfied is the Phase1 auth gate: all realms when strict; at least one verified realm when partial enabled.
func AuthGateSatisfied(rt *Runtime, ev *AuthEvidenceRecord) bool {
	if HasAuthCredentialsSatisfied(rt, ev) {
		return true
	}
	if authPartialAuthEnabled(rt) && len(VerifiedAuthRealmsList(rt, ev)) > 0 {
		return true
	}
	return false
}

func writeAuthStateAfterCalibration(rt *Runtime, ev *AuthEvidenceRecord, cal *AuthCalibrationV1) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	if HasAuthCredentialsSatisfied(rt, ev) {
		detail := "auth calibration ready"
		if cal != nil && cal.AllCalibrated {
			detail = "all auth realms calibrated"
		}
		_, err := writeAuthState(rt, authStateSuccess, detail)
		return err
	}
	if !authPartialAuthEnabled(rt) {
		return utils.Error("verified credentials missing for required realms")
	}
	verified := VerifiedAuthRealmsList(rt, ev)
	missing := missingAuthRealms(rt, ev, requiredAuthRealmsForPartial(rt, ev))
	detail := fmt.Sprintf("partial auth: verified realm(s)=%s; missing=%s; API probe limited to verified realms",
		strings.Join(verified, ","), strings.Join(missing, ","))
	log.Warnf("ssa_api_discovery: %s", detail)
	_, err := writeAuthState(rt, authStatePartial, detail)
	return err
}

// InferAuthRealmForFeatureJob maps a feature work unit to an auth realm using surface map and entry path heuristics.
func InferAuthRealmForFeatureJob(rt *Runtime, job FeatureWorkJob) string {
	if surface, err := loadAuthSurfaceMap(rt.WorkDir); err == nil && surface != nil {
		for _, s := range surface.Surfaces {
			for _, pat := range job.PackagePatterns {
				if authPackagePatternMatches(pat, s.PackagePatterns) {
					return NormalizeAuthRealm(s.AuthRealm)
				}
			}
			rel := strings.ToLower(strings.ReplaceAll(job.EntryFile, "\\", "/"))
			for _, prefix := range s.PathPrefixes {
				p := strings.ToLower(strings.TrimSpace(prefix))
				if p != "" && strings.Contains(rel, p) {
					return NormalizeAuthRealm(s.AuthRealm)
				}
			}
		}
	}
	rel := strings.ToLower(strings.ReplaceAll(job.EntryFile, "\\", "/"))
	switch {
	case strings.Contains(rel, "/controller/admin/"), strings.Contains(rel, ".controller.admin."):
		return AuthRealmAdmin
	case strings.Contains(rel, "/controller/api/"), strings.Contains(rel, ".controller.api."):
		return AuthRealmAPI
	case strings.Contains(rel, "/controller/web/"), strings.Contains(rel, ".controller.web."):
		return AuthRealmWeb
	}
	return NormalizeAuthRealm(InferAuthRealmFromHandlerClass(job.EntryFile))
}

// requiredAuthRealmsForPartial returns realms that need credentials (from evidence or surface map).
func requiredAuthRealmsForPartial(rt *Runtime, ev *AuthEvidenceRecord) []string {
	realms := RequiredAuthRealms(rt, ev)
	if len(realms) > 0 {
		return realms
	}
	if rt == nil {
		return nil
	}
	surface, err := loadAuthSurfaceMap(rt.WorkDir)
	if err != nil || surface == nil || !DetectMultiAuth(rt, ev) {
		return realms
	}
	seen := map[string]struct{}{}
	for _, s := range surface.Surfaces {
		r := NormalizeAuthRealm(s.AuthRealm)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		realms = append(realms, r)
	}
	return realms
}

func authPackagePatternMatches(jobPat string, surfacePatterns []string) bool {
	jobPat = strings.ToLower(strings.TrimSpace(jobPat))
	if jobPat == "" {
		return false
	}
	for _, sp := range surfacePatterns {
		sp = strings.ToLower(strings.TrimSpace(sp))
		if sp == "" {
			continue
		}
		if jobPat == sp {
			return true
		}
		core := strings.TrimPrefix(strings.TrimSuffix(sp, ".*"), "*.")
		core = strings.TrimPrefix(core, "*")
		if core != "" && strings.Contains(jobPat, core) {
			return true
		}
	}
	return false
}

// ShouldSkipFeatureWorkForPartialAuth returns true when partial auth is on and the job's realm has no verified credential.
func ShouldSkipFeatureWorkForPartialAuth(rt *Runtime, job FeatureWorkJob) (bool, string) {
	if !authPartialAuthEnabled(rt) {
		return false, ""
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if !DetectMultiAuth(rt, ev) {
		return false, ""
	}
	realm := InferAuthRealmForFeatureJob(rt, job)
	if realm == "" {
		return false, ""
	}
	required := requiredAuthRealmsForPartial(rt, ev)
	isRequired := false
	for _, r := range required {
		if r == realm {
			isRequired = true
			break
		}
	}
	if !isRequired {
		return false, ""
	}
	if realmHasVerifiedCredential(rt, realm) {
		return false, ""
	}
	return true, fmt.Sprintf("%s:%s (partial_auth: no verified credential for this realm)", rejectReasonAuthRealmUnavailable, realm)
}

// ProbeAuthRealmAvailable is true when strict mode has a credential, or partial mode has one for this realm.
func ProbeAuthRealmAvailable(rt *Runtime, authRealm string) bool {
	authRealm = NormalizeAuthRealm(authRealm)
	if authRealm == "" {
		return hasVerifiedAuthCredential(rt)
	}
	if realmHasVerifiedCredential(rt, authRealm) {
		return true
	}
	if authPartialAuthEnabled(rt) {
		return false
	}
	return realmHasVerifiedCredential(rt, authRealm)
}

func formatPartialAuthProbeScope(rt *Runtime, job FeatureWorkJob) string {
	if !authPartialAuthEnabled(rt) {
		return ""
	}
	ev, _ := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	verified := VerifiedAuthRealmsList(rt, ev)
	missing := missingAuthRealms(rt, ev, requiredAuthRealmsForPartial(rt, ev))
	realm := InferAuthRealmForFeatureJob(rt, job)
	var b strings.Builder
	b.WriteString("## partial_auth_scope\n")
	b.WriteString("- partial_auth: **enabled** — only probe APIs whose auth_realm has verified credentials.\n")
	if len(verified) > 0 {
		b.WriteString("- verified_realms: " + strings.Join(verified, ", ") + "\n")
	}
	if len(missing) > 0 {
		b.WriteString("- skipped_realms (no API analysis): " + strings.Join(missing, ", ") + "\n")
	}
	if realm != "" {
		b.WriteString(fmt.Sprintf("- this_entry auth_realm=%s available=%v\n", realm, ProbeAuthRealmAvailable(rt, realm)))
	}
	return b.String()
}

func commitSkippedFeatureWorkForPartialAuth(rt *Runtime, job FeatureWorkJob, skipReason string) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	entry := HttpApiUnitResult{
		EntryFile:    job.EntryFile,
		FeatureID:    job.FeatureID,
		FeatureLabel: job.FeatureLabel,
		NoApiReason:  skipReason,
	}
	normalizeHttpApiUnitResult(&entry, job)
	if err := validateHttpApiUnitResult(rt, &entry, job); err != nil {
		return err
	}
	inv, _ := loadFeatureInventory(rt.WorkDir)
	return mergeAndPersistHttpApiUnitResult(rt, inv, entry)
}
