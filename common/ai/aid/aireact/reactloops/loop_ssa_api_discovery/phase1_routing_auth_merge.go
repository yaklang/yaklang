package loop_ssa_api_discovery

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// MergeAuthSurfaceIntoRoutingProfile adds url_spaces from auth_surface path_prefixes into routing_profile.
func MergeAuthSurfaceIntoRoutingProfile(rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	servletMap, _ := loadServletRoutingMap(rt.WorkDir)
	surface, err := loadAuthSurfaceMap(rt.WorkDir)
	if err != nil || surface == nil {
		return err
	}
	rp, err := loadRoutingProfileFromWorkDir(rt.WorkDir)
	if err != nil || rp == nil {
		rp = &RoutingProfileV1{
			SchemaVersion:    routingProfileSchemaVersion,
			ValidationStatus: "provisional",
			Target: RoutingProfileTarget{
				Raw:             rt.Session.TargetRaw,
				EffectiveOrigin: EffectiveTargetBaseURL(rt.Session),
				ContextPath:     "/",
			},
		}
	}
	base := strings.TrimSuffix(EffectiveTargetBaseURL(rt.Session), "/")
	seen := map[string]struct{}{}
	for _, sp := range rp.URLSpaces {
		mp := normURLPath(sp.MountPrefix)
		if mp != "" {
			seen[mp] = struct{}{}
		}
	}
	for _, s := range surface.Surfaces {
		prefixes := s.PathPrefixes
		if len(prefixes) == 0 {
			mp := normURLPath(s.MountPrefix)
			if mp != "" && mp != "/" {
				prefixes = []string{mp}
			}
		}
		for _, p := range prefixes {
			mp := normURLPath(p)
			if mp == "" || mp == "/" {
				continue
			}
			if _, ok := seen[mp]; ok {
				continue
			}
			seen[mp] = struct{}{}
			id := strings.TrimPrefix(mp, "/")
			if id == "" {
				id = "root"
			}
			rp.URLSpaces = append(rp.URLSpaces, RoutingURLSpace{
				ID:          id,
				Label:       s.AuthRealm,
				MountPrefix: mp,
				Confidence:  0.85,
				Evidence: []RoutingEvidence{{
					Kind: "auth_surface_path_prefix",
					Ref:  store.AuthSurfaceMapPath(rt.WorkDir),
					Hint: "path_prefixes from auth_surface_map",
				}},
			})
		}
	}
	if len(rp.URLSpaces) == 0 {
		rp.URLSpaces = append(rp.URLSpaces, RoutingURLSpace{
			ID: "default", MountPrefix: "/", Confidence: 0.4,
		})
	}
	rp.EffectiveBases = nil
	for _, sp := range rp.URLSpaces {
		rp.EffectiveBases = append(rp.EffectiveBases, RoutingEffectiveBase{
			SpaceID: sp.ID,
			BaseURL: base + sp.MountPrefix,
		})
	}
	if err := normalizeRoutingProfileV1(rp, rt); err != nil {
		return err
	}
	SanitizeRoutingProfileURLSpaces(rp, servletMap)
	if err := normalizeRoutingProfileV1(rp, rt); err != nil {
		return err
	}
	canonical, err := CanonicalRoutingProfileJSON(rp)
	if err != nil {
		return err
	}
	if rt.Repo != nil && strings.TrimSpace(rt.Session.UUID) != "" {
		_ = rt.Repo.UpdateSessionFields(rt.Session.UUID, map[string]interface{}{
			"routing_profile_json": canonical,
		})
	}
	if err := WriteRoutingProfileFile(rt.WorkDir, canonical); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: merged auth_surface into routing_profile url_spaces=%d", len(rp.URLSpaces))
	return nil
}
