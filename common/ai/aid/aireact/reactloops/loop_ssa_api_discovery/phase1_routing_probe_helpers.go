package loop_ssa_api_discovery

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

type verifiedSampleHint struct {
	Method        string `json:"method"`
	PathPattern   string `json:"path_pattern"`
	FullSampleURL string `json:"full_sample_url"`
	StatusCode    int    `json:"probe_status_code,omitempty"`
	HandlerClass  string `json:"handler_class,omitempty"`
	VerdictReason string `json:"verdict_reason,omitempty"`
}

func loadRoutingProfileFromWorkDir(workDir string) (*RoutingProfileV1, error) {
	b, err := os.ReadFile(store.RoutingProfilePath(workDir))
	if err != nil {
		return nil, err
	}
	return ParseRoutingProfileJSON(string(b))
}

func isUnknownMountPrefix(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "" || s == "unknown" || s == "/unknown"
}

func fmtAnyString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return strings.TrimSpace(utils.InterfaceToString(t))
	}
}

func redactAuthEvidenceForPrompt(ev *AuthEvidenceRecord) *AuthEvidenceRecord {
	if ev == nil {
		return nil
	}
	copy := *ev
	copy.LoginEndpoints = make([]AuthLoginEndpoint, len(ev.LoginEndpoints))
	for i, le := range ev.LoginEndpoints {
		copy.LoginEndpoints[i] = le
		if copy.LoginEndpoints[i].FormFields != nil {
			ff := map[string]string{}
			for k, v := range le.FormFields {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "password") || strings.Contains(lk, "secret") || strings.Contains(lk, "token") {
					ff[k] = "<redacted>"
				} else {
					ff[k] = v
				}
			}
			copy.LoginEndpoints[i].FormFields = ff
		}
	}
	return &copy
}

func loadGapFillVerifiedSamples(rt *Runtime) []verifiedSampleHint {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	rows, err := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	if err != nil {
		return nil
	}
	var out []verifiedSampleHint
	for _, r := range rows {
		if !r.Verified {
			continue
		}
		out = append(out, verifiedSampleHint{
			Method:        r.Method,
			PathPattern:   r.PathPattern,
			FullSampleURL: r.FullSampleURL,
			StatusCode:    r.ProbeStatusCode,
			VerdictReason: r.VerdictReason,
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func collectGapFillMountPrefixes(plan *CodeReadingPlan, rp *RoutingProfileV1, stages []CodeReadingStageOutput) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = normURLPath(strings.TrimSpace(raw))
		if isUnknownMountPrefix(raw) || raw == "/" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	if plan != nil && plan.AuthEvidence != nil {
		for _, le := range plan.AuthEvidence.LoginEndpoints {
			p := strings.TrimSpace(le.Path)
			if p == "" && le.FullURL != "" {
				if u, err := parseURLPathPrefix(le.FullURL); err == nil {
					p = u
				}
			}
			if i := strings.LastIndex(p, "/"); i > 0 {
				add(p[:i])
			}
		}
	}
	for _, st := range stages {
		for _, rf := range st.RoutingFacts {
			add(rf.MountPrefix)
		}
	}
	if plan != nil {
		for _, b := range plan.EffectiveBases {
			add(b)
		}
		for _, v := range plan.URLSpaces {
			row, ok := v.(map[string]any)
			if !ok {
				continue
			}
			add(fmtAnyString(row["base_path"]))
		}
	}
	if rp != nil {
		for _, sp := range rp.URLSpaces {
			add(sp.MountPrefix)
		}
	}
	return out
}

func parseURLPathPrefix(fullURL string) (string, error) {
	fullURL = strings.TrimSpace(fullURL)
	if fullURL == "" {
		return "", utils.Error("empty url")
	}
	if i := strings.Index(fullURL, "://"); i >= 0 {
		rest := fullURL[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			path := rest[j:]
			if k := strings.LastIndex(path, "/"); k > 0 {
				return path[:k], nil
			}
		}
	}
	return "", utils.Error("no path prefix")
}
