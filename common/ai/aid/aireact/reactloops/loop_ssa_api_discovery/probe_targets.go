package loop_ssa_api_discovery

import (
	"fmt"
	"strings"
)

// HttpProbeTarget is a normalized HTTP probe target for Phase4 / Yak tools (not host reachability ProbeTarget()).
type HttpProbeTarget struct {
	ID                uint
	VerifiedHttpApiID uint
	HttpEndpointID    uint
	Method            string
	PathPattern       string
	FullSampleURL     string
	HandlerClass      string
	HandlerMethod     string
	HandlerFile       string
	HandlerSymbol     string
	CodeSnippet       string
	Source            string // verified_http_api
}

// ListProbeTargets returns probe-ready targets from verified_http_apis (single HTTP confirmation source).
func ListProbeTargets(rt *Runtime) ([]HttpProbeTarget, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, nil
	}

	vha, err := rt.Repo.ListVerifiedHttpApisForProbe(rt.Session.ID)
	if err != nil {
		return nil, err
	}
	out := make([]HttpProbeTarget, 0, len(vha))
	for _, row := range vha {
		out = append(out, HttpProbeTarget{
			ID:                row.ID,
			VerifiedHttpApiID: row.ID,
			Method:            row.Method,
			PathPattern:       row.PathPattern,
			FullSampleURL:     strings.TrimSpace(row.FullSampleURL),
			HandlerClass:      row.HandlerFile,
			HandlerMethod:     row.HandlerSymbol,
			HandlerFile:       row.HandlerFile,
			HandlerSymbol:     row.HandlerSymbol,
			CodeSnippet:       row.CodeSnippet,
			Source:            "verified_http_api",
		})
	}
	return out, nil
}

// CountRouteCandidatesFromWorkdir reads route_candidates.json count when present.
func CountRouteCandidatesFromWorkdir(workDir string) int {
	return countRouteCandidates(&Runtime{WorkDir: workDir})
}

func probeTargetKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

// MatchProbeTargetByHandler tries to associate a finding file with a probe target.
func MatchProbeTargetByHandler(targets []HttpProbeTarget, matchedFile string) *HttpProbeTarget {
	mf := strings.ToLower(strings.TrimSpace(matchedFile))
	if mf == "" {
		return nil
	}
	for i := range targets {
		t := &targets[i]
		for _, hint := range []string{t.HandlerFile, t.HandlerClass, t.HandlerMethod} {
			h := strings.ToLower(strings.TrimSpace(hint))
			if h != "" && (strings.Contains(mf, h) || strings.Contains(h, mf)) {
				return t
			}
		}
	}
	return nil
}

// HttpEndpointIDsFromProbeTargets returns http_endpoints.id list for Yak tools (match by method+path if needed).
func HttpEndpointIDsFromProbeTargets(rt *Runtime, targets []HttpProbeTarget) []uint {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	eps, _ := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	epIndex := map[string]uint{}
	for _, ep := range eps {
		epIndex[probeTargetKey(ep.Method, ep.PathPattern)] = ep.ID
	}
	seen := map[uint]struct{}{}
	var ids []uint
	for _, t := range targets {
		id := t.HttpEndpointID
		if id == 0 {
			id = epIndex[probeTargetKey(t.Method, t.PathPattern)]
		}
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func formatProbeTargetLabel(t HttpProbeTarget) string {
	if t.FullSampleURL != "" {
		return fmt.Sprintf("%s %s (%s)", t.Method, t.PathPattern, t.FullSampleURL)
	}
	return fmt.Sprintf("%s %s", t.Method, t.PathPattern)
}
