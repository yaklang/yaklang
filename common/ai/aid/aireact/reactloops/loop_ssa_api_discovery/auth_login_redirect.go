package loop_ssa_api_discovery

import (
	"fmt"
	"strings"
)

// SuggestPostLoginVerifyPaths derives session-verify paths after login returns 302+Set-Cookie.
// loginPostPath is the POST target (e.g. /admin/login); location is the Location header value (e.g. index.html).
func SuggestPostLoginVerifyPaths(loginPostPath, location, mountPrefix string) []string {
	loginPostPath = normURLPath(strings.TrimSpace(loginPostPath))
	mountPrefix = normURLPath(strings.TrimSpace(mountPrefix))
	location = strings.TrimSpace(location)

	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = normURLPath(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	if strings.HasPrefix(location, "/") {
		add(location)
		return out
	}
	if strings.Contains(location, "://") {
		if i := strings.Index(location, "://"); i >= 0 {
			rest := location[i+3:]
			if slash := strings.Index(rest, "/"); slash >= 0 {
				add(rest[slash:])
			}
		}
		return out
	}

	rel := strings.TrimPrefix(location, "./")
	if rel == "" {
		rel = "index.html"
	}

	if loginPostPath != "" {
		parent := parentURLPath(loginPostPath)
		add(joinURLPath(parent, rel))
		if parent != loginPostPath {
			add(parent)
		}
	}

	if mountPrefix != "" && mountPrefix != "/" {
		add(joinURLPath(mountPrefix, rel))
		add(mountPrefix)
		add(joinURLPath(mountPrefix, "index.html"))
	}

	return out
}

func parentURLPath(p string) string {
	p = normURLPath(p)
	if p == "" || p == "/" {
		return "/"
	}
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "/"
}

func joinURLPath(base, rel string) string {
	base = normURLPath(base)
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "/")
	if rel == "" {
		return base
	}
	if base == "" || base == "/" {
		return "/" + rel
	}
	return strings.TrimRight(base, "/") + "/" + rel
}

// FormatPostLoginRedirectHint guides the agent after login 302+Set-Cookie when redirect follow may 404.
func FormatPostLoginRedirectHint(rt *Runtime, authRealm string) string {
	mount := authRealmMountPrefix(rt, authRealm)
	loginPost := ""
	if rt != nil {
		if m, _ := loadAuthMechanismMap(rt.WorkDir); m != nil {
			if d, ok := m.Realms[authRealm]; ok {
				if p := strings.TrimSpace(d.LoginPostPath); p != "" {
					loginPost = p
				} else if p := strings.TrimSpace(d.LoginPath); p != "" {
					loginPost = p
				}
			}
		}
		if loginPost == "" {
			if surface, _ := loadAuthSurfaceMap(rt.WorkDir); surface != nil {
				for _, s := range surface.Surfaces {
					if NormalizeAuthRealm(s.AuthRealm) == NormalizeAuthRealm(authRealm) {
						loginPost = strings.TrimSpace(s.LoginPostPath)
						if loginPost == "" {
							loginPost = strings.TrimSpace(s.LoginPath)
						}
						if mp := strings.TrimSpace(s.MountPrefix); mp != "" {
							mount = normURLPath(mp)
						}
						break
					}
				}
			}
		}
	}

	var b strings.Builder
	b.WriteString("## Post-login redirect resolution\n")
	b.WriteString(fmt.Sprintf("auth_realm=%q mount_prefix=%q login_post_path=%q\n\n", authRealm, mount, loginPost))
	b.WriteString("After login POST returns **302+Set-Cookie**, treat login as **success** even if auto-follow redirect 404s.\n")
	b.WriteString("Relative `Location` (e.g. `index.html`) resolves against the **parent directory of login_post_path**, not by appending to the full login URL.\n")
	b.WriteString("- Wrong (common HTTP client bug): `{login_post_path}/index.html`\n")
	b.WriteString("- Try instead: `{parent(login_post_path)}/{Location}` and `{mount_prefix}/{Location}`\n")
	b.WriteString("- After programmatic_auto_save: `discovery_select_auth_credential`, then GET verify paths with **`auth_credential_id`** only (no manual Cookie).\n")
	b.WriteString("- If redirect follow 404 but login POST had 302+session: session is still valid; probe with auth_credential_id.\n")

	candidates := SuggestPostLoginVerifyPaths(loginPost, "index.html", mount)
	if len(candidates) > 0 {
		b.WriteString("\n### Suggested verify paths (relative Location=index.html)\n")
		for _, p := range candidates {
			b.WriteString("- `" + p + "`\n")
		}
	}
	if loginPost != "" {
		wrong := joinURLPath(loginPost, "index.html")
		b.WriteString("\n### Avoid as primary verify URL\n")
		b.WriteString("- `" + wrong + "` (usually wrong resolution of relative redirect)\n")
	}
	return b.String()
}

// postLoginVerifyURLHint returns actionable feedback when verify_url looks like a mis-resolved relative redirect.
func postLoginVerifyURLHint(loginPostPath, verifyURL, mountPrefix string) string {
	loginPostPath = normURLPath(strings.TrimSpace(loginPostPath))
	mountPrefix = normURLPath(strings.TrimSpace(mountPrefix))
	verifyURL = strings.TrimSpace(verifyURL)
	if loginPostPath == "" {
		return ""
	}
	wrong := joinURLPath(loginPostPath, "index.html")
	if verifyURL != "" && !strings.Contains(verifyURL, "/login/index.html") && !strings.Contains(verifyURL, "/login/index.") {
		return ""
	}
	candidates := SuggestPostLoginVerifyPaths(loginPostPath, "index.html", mountPrefix)
	if len(candidates) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("post_login_redirect_hint: verify_url may be a mis-resolved relative Location.")
	b.WriteString(" Try GET with saved Cookie:")
	for _, p := range candidates {
		b.WriteString("\n- " + p)
	}
	if wrong != "" {
		b.WriteString("\nAvoid: " + wrong)
	}
	return b.String()
}
