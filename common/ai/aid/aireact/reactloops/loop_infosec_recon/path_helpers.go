package loop_infosec_recon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	keyVerifiedJsDir         = "infosec_verified_js_dir"
	keyJsStaticPathFailCount = "infosec_js_static_path_fail_count"
	keySpinRecoveryHint      = "infosec_spin_recovery_hint"
)


func infosecTryStatLocalPath(p, wd string) (abs string, ok bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	candidates := []string{p}
	if !filepath.IsAbs(p) && wd != "" {
		candidates = append(candidates, filepath.Join(wd, p))
	}
	seen := map[string]struct{}{}
	for _, c := range candidates {
		c = filepath.Clean(c)
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		if _, err := os.Stat(c); err == nil {
			abs, aerr := filepath.Abs(c)
			if aerr != nil {
				abs = c
			}
			return abs, true
		}
	}
	return "", false
}

// infosecResolveJsStaticPaths resolves js_static_extract_ai inputs.
// Priority: dir param > paths (whole-path comma-safe) > verified_js_dir fallback > comma split.
func infosecResolveJsStaticPaths(pathsStr, dirStr, verifiedFallback, wd string) (paths []string, source string, err error) {
	dirStr = strings.TrimSpace(dirStr)
	if dirStr != "" {
		abs, rerr := infosecResolveLocalPathForExec(dirStr, wd)
		if rerr != nil {
			return nil, "", rerr
		}
		return []string{abs}, "dir parameter", nil
	}

	pathsStr = strings.TrimSpace(pathsStr)
	if pathsStr == "" {
		verifiedFallback = strings.TrimSpace(verifiedFallback)
		if verifiedFallback != "" {
			abs, rerr := infosecResolveLocalPathForExec(verifiedFallback, wd)
			if rerr != nil {
				return nil, "", utils.Errorf("no paths/dir and verified_js_dir invalid: %v", rerr)
			}
			return []string{abs}, "auto from crawl artifacts.verified_js_dir", nil
		}
		return nil, "", utils.Error("no paths in paths= parameter (use dir= for directories whose names contain commas)")
	}

	if abs, ok := infosecTryStatLocalPath(pathsStr, wd); ok {
		return []string{abs}, "single local path (comma-safe)", nil
	}

	parts := strings.Split(pathsStr, ",")
	var splitPaths []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			splitPaths = append(splitPaths, p)
		}
	}
	if len(splitPaths) == 0 {
		return nil, "", utils.Error("no paths in paths= parameter")
	}

	var normPaths []string
	for _, p := range splitPaths {
		lp := strings.ToLower(p)
		if strings.HasPrefix(lp, "http://") || strings.HasPrefix(lp, "https://") {
			if verr := infosecValidateHTTPURL(p); verr != nil {
				return nil, "", fmt.Errorf("invalid URL in paths: %w", verr)
			}
			normPaths = append(normPaths, p)
			continue
		}
		absLocal, rerr := infosecResolveLocalPathForExec(p, wd)
		if rerr != nil {
			return nil, "", fmt.Errorf("invalid local path %q: %w", p, rerr)
		}
		normPaths = append(normPaths, absLocal)
	}
	return normPaths, "comma-separated paths", nil
}
