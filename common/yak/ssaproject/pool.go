package ssaproject

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func normalizeSSADBPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// SSADBPathCandidates returns normalized path variants for SSA sqlite path matching.
func SSADBPathCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 3)
	for _, p := range []string{raw, normalizeSSADBPath(raw), filepath.Clean(raw)} {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func defaultSSADBPathCandidates() []string {
	return SSADBPathCandidates(consts.GetCanonicalDefaultSSADatabasePath())
}

// IsSharedPoolProject reports whether the SSA project row belongs to the shared default/temp IR pool.
func IsSharedPoolProject(p *schema.SSAProject) bool {
	if p == nil {
		return true
	}
	path := strings.TrimSpace(p.DatabasePath)
	if path == "" {
		return true
	}
	norm := normalizeSSADBPath(path)
	for _, c := range defaultSSADBPathCandidates() {
		if norm == c {
			return true
		}
	}
	return false
}

func inferBindModeFromSchema(p *schema.SSAProject) ypb.SSAProjectDatabaseBindMode {
	if IsSharedPoolProject(p) {
		return ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED
	}
	return ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_DEDICATED
}

// MatchesBindMode reports whether schema row belongs to the requested create/list pool.
func MatchesBindMode(p *schema.SSAProject, mode ypb.SSAProjectDatabaseBindMode) bool {
	if p == nil {
		return false
	}
	switch mode {
	case ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED:
		return IsSharedPoolProject(p)
	case ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_DEDICATED:
		return !IsSharedPoolProject(p)
	default:
		return true
	}
}

func hashSuffixForBindMode(mode ypb.SSAProjectDatabaseBindMode) string {
	switch mode {
	case ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_SHARED:
		return "pool:shared"
	case ypb.SSAProjectDatabaseBindMode_SSA_PROJECT_BIND_DEDICATED:
		return "pool:dedicated"
	default:
		return "pool:legacy"
	}
}

// CalcProjectHash scopes (url, name) by bind pool so shared and dedicated rows may coexist.
func CalcProjectHash(url, projectName string, mode ypb.SSAProjectDatabaseBindMode) string {
	return utils.CalcMd5(url, projectName, hashSuffixForBindMode(mode))
}

// RefreshProjectHash updates Hash from URL, name, and current database_path bind state.
func RefreshProjectHash(p *schema.SSAProject) {
	if p == nil {
		return
	}
	mode := inferBindModeFromSchema(p)
	p.Hash = CalcProjectHash(p.URL, p.ProjectName, mode)
}
