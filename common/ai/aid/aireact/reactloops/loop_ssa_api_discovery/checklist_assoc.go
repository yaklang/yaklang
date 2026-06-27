package loop_ssa_api_discovery

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

var dataFlowRoutePatterns = []*regexp.Regexp{
	regexp.MustCompile(`@(?:Get|Post|Put|Delete|Patch|Request)Mapping\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`@RequestMapping\s*\(\s*value\s*=\s*"([^"]+)"`),
	regexp.MustCompile(`@RequestMapping\s*\(\s*path\s*=\s*"([^"]+)"`),
	regexp.MustCompile(`(?:GET|POST|PUT|DELETE|PATCH)\s+(/[^\s"']+)`),
}

func parseRoutePathsFromDataFlowHint(hint string) []string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, re := range dataFlowRoutePatterns {
		for _, m := range re.FindAllStringSubmatch(hint, -1) {
			if len(m) < 2 {
				continue
			}
			p := normURLPath(m[1])
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func pathsMatchEndpoint(pathPattern, hintPath string) bool {
	ep := normURLPath(pathPattern)
	hp := normURLPath(hintPath)
	if ep == "" || hp == "" {
		return false
	}
	if ep == hp {
		return true
	}
	if strings.HasSuffix(ep, hp) || strings.HasSuffix(hp, ep) {
		return true
	}
	epTrim := strings.TrimSuffix(strings.TrimPrefix(ep, "/"), "/")
	hpTrim := strings.TrimSuffix(strings.TrimPrefix(hp, "/"), "/")
	return epTrim == hpTrim
}

func matchEndpointByPaths(endpoints []store.HttpEndpoint, paths []string) *store.HttpEndpoint {
	if len(paths) == 0 {
		return nil
	}
	for i := range endpoints {
		ep := &endpoints[i]
		for _, p := range paths {
			if pathsMatchEndpoint(ep.PathPattern, p) {
				return ep
			}
		}
	}
	return nil
}

func applyEndpointToChecklistItem(item *VulnChecklistItem, ep *store.HttpEndpoint, confidence string) {
	if item == nil || ep == nil {
		return
	}
	item.EndpointID = ep.ID
	item.Method = ep.Method
	item.PathPattern = ep.PathPattern
	item.HandlerClass = ep.HandlerClass
	item.AssocConfidence = confidence
}

func applyProbeTargetToChecklistItem(item *VulnChecklistItem, pt *HttpProbeTarget, confidence string) {
	if item == nil || pt == nil {
		return
	}
	item.VerifiedHttpApiID = pt.VerifiedHttpApiID
	item.EndpointID = pt.HttpEndpointID
	item.Method = pt.Method
	item.PathPattern = pt.PathPattern
	item.FullSampleURL = pt.FullSampleURL
	if pt.HandlerClass != "" {
		item.HandlerClass = pt.HandlerClass
	}
	item.AssocConfidence = confidence
}

func associateFindingToEndpoints(
	f store.DiscoverySyntaxFlowFinding,
	targets []HttpProbeTarget,
	endpoints []store.HttpEndpoint,
	epByHandler map[string]*store.HttpEndpoint,
) (VulnChecklistItem, bool) {
	item := VulnChecklistItem{
		FindingID:    f.ID,
		RuleName:     f.RuleName,
		Severity:     f.Severity,
		Title:        f.Title,
		MatchedFile:  f.MatchedFile,
		DataFlowHint: f.DataFlowHint,
		Priority:     severityToPriority(f.Severity),
	}

	if pt := MatchProbeTargetByHandler(targets, f.MatchedFile); pt != nil {
		applyProbeTargetToChecklistItem(&item, pt, "high")
		return item, true
	}

	if paths := parseRoutePathsFromDataFlowHint(f.DataFlowHint); len(paths) > 0 {
		if ep := matchEndpointByPaths(endpoints, paths); ep != nil {
			applyEndpointToChecklistItem(&item, ep, "high")
			for i := range targets {
				t := &targets[i]
				if pathsMatchEndpoint(t.PathPattern, ep.PathPattern) {
					if t.FullSampleURL != "" {
						item.FullSampleURL = t.FullSampleURL
					}
					if t.VerifiedHttpApiID > 0 {
						item.VerifiedHttpApiID = t.VerifiedHttpApiID
					}
					break
				}
			}
			return item, true
		}
	}

	if f.MatchedFile != "" {
		base := filepath.Base(f.MatchedFile)
		className := strings.TrimSuffix(base, filepath.Ext(base))
		if ep, ok := epByHandler[strings.ToLower(className)]; ok {
			applyEndpointToChecklistItem(&item, ep, "high")
			return item, true
		}
	}

	for _, ep := range endpoints {
		handler := strings.ToLower(ep.HandlerClass)
		matchedFile := strings.ToLower(f.MatchedFile)
		if handler != "" && matchedFile != "" && strings.Contains(matchedFile, handler) {
			applyEndpointToChecklistItem(&item, &ep, "medium")
			return item, true
		}
	}

	item.AssocConfidence = "none"
	return item, false
}

func vulnChecklistItemsToStore(items []VulnChecklistItem) []store.VulnChecklistItem {
	out := make([]store.VulnChecklistItem, 0, len(items))
	for _, it := range items {
		out = append(out, store.VulnChecklistItem{
			FindingID:         it.FindingID,
			EndpointID:        it.EndpointID,
			VerifiedHttpApiID: it.VerifiedHttpApiID,
			RuleName:          it.RuleName,
			Severity:          it.Severity,
			Title:             it.Title,
			MatchedFile:       it.MatchedFile,
			DataFlowHint:      it.DataFlowHint,
			Method:            it.Method,
			PathPattern:       it.PathPattern,
			FullSampleURL:     it.FullSampleURL,
			HandlerClass:      it.HandlerClass,
			Priority:          it.Priority,
			AssocConfidence:   it.AssocConfidence,
			Status:            store.VulnChecklistStatusPending,
		})
	}
	return out
}

func storeChecklistToDTO(rows []store.VulnChecklistItem) []VulnChecklistItem {
	out := make([]VulnChecklistItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, VulnChecklistItem{
			FindingID:         r.FindingID,
			RuleName:          r.RuleName,
			Severity:          r.Severity,
			Title:             r.Title,
			MatchedFile:       r.MatchedFile,
			DataFlowHint:      r.DataFlowHint,
			EndpointID:        r.EndpointID,
			VerifiedHttpApiID: r.VerifiedHttpApiID,
			Method:            r.Method,
			PathPattern:       r.PathPattern,
			FullSampleURL:     r.FullSampleURL,
			HandlerClass:      r.HandlerClass,
			Priority:          r.Priority,
			AssocConfidence:   r.AssocConfidence,
		})
	}
	return out
}
