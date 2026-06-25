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

const authStatePartial = "partial"

func authPartialOkEnabled() bool {
	return os.Getenv("YAK_SSA_AUTH_PARTIAL_OK") == "1"
}

// RefreshAuthEvidenceFromDB merges DB auth_credentials into auth_evidence.json.
func RefreshAuthEvidenceFromDB(rt *Runtime) error {
	if rt == nil || strings.TrimSpace(rt.WorkDir) == "" {
		return utils.Error("nil runtime or workdir")
	}
	ev, err := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if err != nil {
		surface, serr := loadAuthSurfaceMap(rt.WorkDir)
		if serr != nil {
			return err
		}
		ev = authEvidenceFromSurfaceMap(surface)
	}
	if ev == nil {
		ev = &AuthEvidenceRecord{}
	}
	preserveVerified := ev.Verified
	preserveDetail := ev.VerificationDetail
	SyncCredentialBindingsFromDB(rt, ev)
	if n := EnsureSharedLoginCredentialAliases(rt, ev); n > 0 {
		SyncCredentialBindingsFromDB(rt, ev)
	}
	if rt.Repo != nil && rt.Session != nil {
		if creds, err := rt.Repo.ListAuthCredentials(rt.Session.ID); err == nil {
			propagateSharedLoginEndpointCredentialIDs(ev, creds)
		}
	}
	ev.MultiAuth = DetectMultiAuth(rt, ev)
	realms := RequiredAuthRealms(rt, ev)
	if len(realms) == 0 {
		ev.Verified = hasVerifiedAuthCredential(rt)
	} else {
		ev.Verified = allRequiredRealmsHaveVerifiedCredentials(rt, realms)
	}
	if ev.Verified {
		if len(realms) > 0 {
			ev.VerificationDetail = fmt.Sprintf("all %d required auth realm(s) have verified credentials", len(realms))
		} else {
			ev.VerificationDetail = "verified auth credentials present"
		}
	} else if len(ev.CredentialBindings) > 0 {
		missing := missingAuthRealms(rt, ev, realms)
		ev.VerificationDetail = fmt.Sprintf("partial: missing verified credentials for realm(s): %s", strings.Join(missing, ","))
	} else if preserveDetail != "" && !ev.Verified {
		ev.VerificationDetail = preserveDetail
	} else if preserveVerified && !ev.Verified {
		ev.VerificationDetail = "credentials in DB but realm coverage incomplete"
	}
	raw, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(store.AuthEvidencePath(rt.WorkDir), raw); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactAuthEvidence, string(raw))
	log.Infof("ssa_api_discovery: auth_evidence refreshed verified=%v bindings=%d", ev.Verified, len(ev.CredentialBindings))
	return nil
}

func allRequiredRealmsHaveVerifiedCredentials(rt *Runtime, realms []string) bool {
	if rt == nil || len(realms) == 0 {
		return hasVerifiedAuthCredential(rt)
	}
	for _, r := range realms {
		if !realmHasVerifiedCredential(rt, r) {
			return false
		}
	}
	return true
}

func realmHasVerifiedCredential(rt *Runtime, authRealm string) bool {
	if hasDirectVerifiedCredentialForRealm(rt, authRealm) {
		return true
	}
	return realmHasVerifiedCredentialWithEvidence(rt, nil, authRealm)
}

func missingAuthRealms(rt *Runtime, ev *AuthEvidenceRecord, realms []string) []string {
	if len(realms) == 0 {
		if !hasVerifiedAuthCredential(rt) {
			return []string{"default"}
		}
		return nil
	}
	var out []string
	for _, r := range realms {
		if !realmHasVerifiedCredential(rt, r) {
			out = append(out, r)
		}
	}
	return out
}
