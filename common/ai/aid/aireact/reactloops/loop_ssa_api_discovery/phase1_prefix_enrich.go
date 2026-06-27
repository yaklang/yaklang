package loop_ssa_api_discovery

import (
	"strings"
)

// joinMountAndRelativePath prepends mount prefix when relative path does not already include it.
func joinMountAndRelativePath(mountPrefix, relativePath string) string {
	mountPrefix = normURLPath(strings.TrimSpace(mountPrefix))
	relativePath = normURLPath(strings.TrimSpace(relativePath))
	if relativePath == "" {
		return mountPrefix
	}
	if mountPrefix == "" || mountPrefix == "/" {
		return relativePath
	}
	if relativePath == mountPrefix || strings.HasPrefix(relativePath, mountPrefix+"/") {
		return relativePath
	}
	return joinURLPath(mountPrefix, strings.TrimPrefix(relativePath, "/"))
}

func resolvePathPrefixForHandler(rt *Runtime, handlerClass, fileRelPath string, packagePatterns []string) string {
	if rt == nil {
		return ""
	}
	if m, err := loadServletRoutingMap(rt.WorkDir); err == nil && m != nil {
		if p := resolveURLPrefixFromServletMap(m, fileRelPath, handlerClass, packagePatterns); p != "" {
			return p
		}
	}
	if surface, err := loadAuthSurfaceMap(rt.WorkDir); err == nil && surface != nil {
		rel := strings.ToLower(strings.ReplaceAll(fileRelPath, "\\", "/"))
		for _, s := range surface.Surfaces {
			for _, prefix := range s.PathPrefixes {
				p := normURLPath(prefix)
				if p == "" || p == "/" {
					continue
				}
				if rel != "" && strings.Contains(rel, strings.TrimPrefix(p, "/")) {
					return p
				}
			}
			for _, pat := range s.PackagePatterns {
				if authPackagePatternMatches(handlerClass, []string{pat}) {
					if len(s.PathPrefixes) > 0 {
						return normURLPath(s.PathPrefixes[0])
					}
					if mp := normURLPath(s.MountPrefix); mp != "" && mp != "/" {
						return mp
					}
				}
			}
		}
	}
	for _, pat := range packagePatterns {
		pat = strings.ToLower(strings.TrimSpace(pat))
		switch {
		case strings.Contains(pat, ".controller.admin"):
			return "/admin"
		case strings.Contains(pat, ".controller.api"):
			return "/api"
		}
	}
	rel := strings.ToLower(strings.ReplaceAll(fileRelPath, "\\", "/"))
	switch {
	case strings.Contains(rel, "/controller/admin/"), strings.Contains(rel, ".controller.admin."):
		return "/admin"
	case strings.Contains(rel, "/controller/api/"), strings.Contains(rel, ".controller.api."):
		return "/api"
	}
	return ""
}

func enrichStaticRouteHintPath(rt *Runtime, hint StaticRouteHint, job FeatureWorkJob) StaticRouteHint {
	if isWildcardRoutePattern(hint.PathPattern) {
		return hint
	}
	prefix := resolvePathPrefixForHandler(rt, hint.HandlerClass, hint.FileRelPath, job.PackagePatterns)
	if prefix == "" {
		prefix = resolvePathPrefixForHandler(rt, "", job.EntryFile, job.PackagePatterns)
	}
	if prefix == "" {
		return hint
	}
	out := hint
	out.PathPattern = joinMountAndRelativePath(prefix, hint.PathPattern)
	return out
}

func enrichStaticRouteHintsForJob(rt *Runtime, job FeatureWorkJob, hints []StaticRouteHint) []StaticRouteHint {
	if len(hints) == 0 {
		return hints
	}
	out := make([]StaticRouteHint, len(hints))
	for i, h := range hints {
		out[i] = enrichStaticRouteHintPath(rt, h, job)
	}
	return out
}

func buildPrefixCandidatesBlock(rt *Runtime, job FeatureWorkJob) string {
	prefix := resolvePathPrefixForHandler(rt, "", job.EntryFile, job.PackagePatterns)
	if prefix == "" {
		return ""
	}
	realm := InferAuthRealmForFeatureJob(rt, job)
	return "## prefix_candidates (engine)\n" +
		"- auth_realm=" + realm + "\n" +
		"- suggested_mount_prefix=" + prefix + "\n" +
		"- rule: full_path = context_path + servlet url_prefix + class @RequestMapping + method mapping\n" +
		"- prefer servlet_routing_map over routing_profile.url_spaces for mount\n" +
		"- static_hints.path_pattern may be relative; do not use as final URL without prefix\n" +
		"- on 404 retry with servlet url_prefix prepended before changing method/body\n" +
		"- on 403 with @Csrf: route exists; add _csrf and retry POST\n"
}
