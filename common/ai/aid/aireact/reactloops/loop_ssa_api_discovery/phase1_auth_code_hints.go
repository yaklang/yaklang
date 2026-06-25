package loop_ssa_api_discovery

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

//go:embed prompts/phase1_auth_login_page_playbook.txt
var phase1AuthLoginPagePlaybook string

const maxAuthCodeHintPaths = 40

// FormatAuthBackendCodeHintsForRealm lists backend source paths relevant to the current auth realm.
// The agent decides when to read them; this block only provides locations.
func FormatAuthBackendCodeHintsForRealm(rt *Runtime, authRealm string) string {
	if rt == nil {
		return ""
	}
	authRealm = NormalizeAuthRealm(authRealm)
	mountPrefix := authRealmMountPrefix(rt, authRealm)

	var sections []string
	sections = append(sections, "## Backend code locations (read when needed)")
	sections = append(sections, fmt.Sprintf(
		"Current auth_realm=%q mount_prefix=%q. Use code_reading tools on paths below when you need session/filter/security details. "+
			"For login POST body shape, prefer GET login page HTML/JS first.",
		authRealm, mountPrefix,
	))

	scope, _ := loadBackendScope(rt.WorkDir)
	if scope != nil {
		if len(scope.BackendRoots) > 0 {
			sections = append(sections, "### Backend roots\n- "+strings.Join(scope.BackendRoots, "\n- "))
		}
		if paths, notes := collectAuthHintPathsFromScope(scope, func(rel string) bool {
			return authPathMatchesRealm(rel, authRealm, mountPrefix)
		}); len(paths) > 0 {
			sections = append(sections, formatAuthHintPathList("Login/security controller candidates", paths, notes))
		}
		var routePaths []string
		for _, p := range scope.ApiRouteFiles {
			if authPathMatchesRealm(p, authRealm, mountPrefix) {
				routePaths = append(routePaths, p)
			}
		}
		if len(routePaths) > 0 {
			sections = append(sections, formatAuthHintPathList("Route/handler files (auth-related)", trimAuthHintPaths(routePaths), nil))
		}
	}

	comp, _ := loadComponentPackageMap(rt.WorkDir)
	if comp != nil {
		for _, c := range comp.Components {
			if !componentMatchesAuthRealm(c, authRealm) {
				continue
			}
			line := fmt.Sprintf("### Component %q (layer=%s)\n- package_patterns: %s",
				c.ID, c.ControllerLayer, strings.Join(c.PackagePatterns, ", "))
			if len(c.EvidenceRefs) > 0 {
				line += "\n- evidence_refs: " + strings.Join(c.EvidenceRefs, ", ")
			}
			sections = append(sections, line)
		}
	}

	profile, _ := loadProjectProfile(rt.WorkDir)
	if profile != nil {
		var configPaths []string
		for _, ep := range profile.EntryPoints {
			if !isAuthRelatedEntryPoint(ep) {
				continue
			}
			if authPathMatchesRealm(ep.RelPath, authRealm, mountPrefix) || ep.Kind == "security_config" {
				configPaths = append(configPaths, ep.RelPath)
			}
		}
		if len(configPaths) > 0 {
			sections = append(sections, formatAuthHintPathList("Security/config entry points", trimAuthHintPaths(configPaths), nil))
		}
	}

	if len(sections) <= 2 {
		sections = append(sections, fmt.Sprintf(
			"(no realm-specific paths indexed yet; search under mount_prefix %q for login/security controllers)",
			mountPrefix,
		))
	}
	return strings.Join(sections, "\n\n")
}

func authRealmMountPrefix(rt *Runtime, authRealm string) string {
	if rt != nil {
		if inv, _ := loadAuthRealmInventory(rt.WorkDir); inv != nil {
			for _, r := range inv.Realms {
				if NormalizeAuthRealm(r.AuthRealm) == authRealm {
					if mp := strings.TrimSpace(r.MountPrefix); mp != "" {
						return normURLPath(mp)
					}
				}
			}
		}
	}
	switch authRealm {
	case AuthRealmAdmin:
		return "/admin"
	case AuthRealmWeb, AuthRealmMember:
		return "/"
	default:
		return "/"
	}
}

func authPathMatchesRealm(relPath, authRealm, mountPrefix string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(relPath)))
	if lower == "" {
		return false
	}
	if !isAuthEntryPath(relPath) && !strings.Contains(lower, "security") && !strings.Contains(lower, "interceptor") {
		mp := strings.ToLower(strings.Trim(strings.TrimSpace(mountPrefix), "/"))
		if mp != "" && mp != "/" && strings.Contains(lower, mp) {
			return true
		}
		return false
	}
	switch authRealm {
	case AuthRealmAdmin:
		return strings.Contains(lower, "/admin") || strings.Contains(lower, ".admin.") ||
			strings.Contains(lower, "admin/") || strings.Contains(lower, "manage")
	case AuthRealmWeb, AuthRealmMember:
		if strings.Contains(lower, "/admin") || strings.Contains(lower, ".admin.") {
			return false
		}
		return strings.Contains(lower, ".web.") || strings.Contains(lower, "/web/") ||
			strings.Contains(lower, "login") || strings.Contains(lower, "member") ||
			strings.Contains(lower, "signin") || strings.Contains(lower, "front")
	case AuthRealmAPI:
		return strings.Contains(lower, "/api") || strings.Contains(lower, ".api.") ||
			strings.Contains(lower, "oauth") || strings.Contains(lower, "token")
	case AuthRealmOAuth:
		return strings.Contains(lower, "oauth") || strings.Contains(lower, "openid")
	default:
		return true
	}
}

func componentMatchesAuthRealm(c ComponentPackageEntry, authRealm string) bool {
	layer := strings.ToLower(strings.TrimSpace(c.ControllerLayer))
	switch authRealm {
	case AuthRealmAdmin:
		if layer == "admin" || layer == "backend" {
			return true
		}
	case AuthRealmWeb:
		if layer == "web" || layer == "frontend" || layer == "member" {
			return true
		}
	case AuthRealmAPI:
		if layer == "api" || layer == "internal" {
			return true
		}
	default:
		if layer == authRealm {
			return true
		}
	}
	for _, p := range c.PackagePatterns {
		pl := strings.ToLower(p)
		if strings.Contains(pl, "."+authRealm+".") {
			return true
		}
	}
	return layer == authRealm
}

func isAuthRelatedEntryPoint(ep ProjectEntryPoint) bool {
	kind := strings.ToLower(strings.TrimSpace(ep.Kind))
	if kind == "security_config" || kind == "web_mvc_config" {
		return true
	}
	return isAuthEntryPath(ep.RelPath) || strings.Contains(strings.ToLower(ep.RelPath), "security")
}

func collectAuthHintPathsFromScope(scope *BackendScopeReport, match func(string) bool) ([]string, map[string]string) {
	if scope == nil {
		return nil, nil
	}
	var paths []string
	notes := map[string]string{}
	for _, c := range scope.ControllerFileCandidates {
		if !match(c.RelPath) {
			continue
		}
		paths = append(paths, c.RelPath)
		if c.Reason != "" {
			notes[c.RelPath] = c.Reason
		}
	}
	return trimAuthHintPaths(paths), notes
}

func trimAuthHintPaths(paths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range paths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
		if len(out) >= maxAuthCodeHintPaths {
			break
		}
	}
	return out
}

func formatAuthHintPathList(title string, paths []string, notes map[string]string) string {
	if len(paths) == 0 {
		return ""
	}
	var lines []string
	for _, p := range paths {
		line := "- `" + p + "`"
		if notes != nil {
			if note, ok := notes[p]; ok && note != "" {
				line += " (" + note + ")"
			}
		}
		lines = append(lines, line)
	}
	return "### " + title + "\n" + strings.Join(lines, "\n")
}

func buildAuthMechanismExtraContext(rt *Runtime, authRealm string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Current auth_realm\n%s\n\n", authRealm))
	b.WriteString(strings.TrimSpace(phase1AuthLoginPagePlaybook))
	b.WriteString("\n\n")
	b.WriteString(FormatAuthBackendCodeHintsForRealm(rt, authRealm))
	b.WriteString("\n\n")
	b.WriteString(FormatPostLoginRedirectHint(rt, authRealm))
	b.WriteString("\n\n")
	b.WriteString(embeddedArtifactsForAgent(rt,
		store.AuthRealmInventoryPath(rt.WorkDir),
		store.AuthMechanismMapPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
		store.ComponentPackageMapPath(rt.WorkDir),
	))
	return b.String()
}

func buildAuthCalibrationExtraContext(rt *Runtime, authRealm string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Current auth_realm\n%s\n\n", authRealm))
	b.WriteString(formatCredentialGroupHintForRealm(rt, authRealm))
	b.WriteString("\n\n")
	b.WriteString(strings.TrimSpace(phase1AuthLoginPagePlaybook))
	b.WriteString("\n\n")
	b.WriteString(FormatAuthBackendCodeHintsForRealm(rt, authRealm))
	b.WriteString("\n\n")
	b.WriteString(FormatPostLoginRedirectHint(rt, authRealm))
	b.WriteString("\n\n")
	b.WriteString(embeddedArtifactsForAgent(rt,
		store.AuthSurfaceMapPath(rt.WorkDir),
		store.AuthMechanismMapPath(rt.WorkDir),
		store.FailureSemanticsPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
		store.BackendScopePath(rt.WorkDir),
	))
	return b.String()
}
