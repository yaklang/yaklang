package loop_ssa_api_discovery

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const discoveryFeatureScopeLoopKey = "discovery_feature_scope_json"
const discoveryFeatureRouteKeysLoopKey = "discovery_feature_route_keys_json"

// setLoopFeatureScope pins the active feature inventory entry on a ReAct loop so
// discovery_read_session_data returns only rows belonging to that feature's packages.
func setLoopFeatureScope(loop *reactloops.ReActLoop, rt *Runtime, feat FeatureInventoryEntry) {
	if loop == nil {
		return
	}
	b, _ := json.Marshal(feat)
	loop.Set(discoveryFeatureScopeLoopKey, string(b))
	if rt != nil {
		if keys := featureRouteKeySet(rt, feat); len(keys) > 0 {
			kb, _ := json.Marshal(keys)
			loop.Set(discoveryFeatureRouteKeysLoopKey, string(kb))
		}
	}
}

func loopFeatureScope(loop *reactloops.ReActLoop) (FeatureInventoryEntry, map[string]struct{}, bool) {
	if loop == nil {
		return FeatureInventoryEntry{}, nil, false
	}
	raw := strings.TrimSpace(loop.Get(discoveryFeatureScopeLoopKey))
	if raw == "" {
		return FeatureInventoryEntry{}, nil, false
	}
	var feat FeatureInventoryEntry
	if err := json.Unmarshal([]byte(raw), &feat); err != nil {
		return FeatureInventoryEntry{}, nil, false
	}
	routeKeys := parseLoopRouteKeySet(loop.Get(discoveryFeatureRouteKeysLoopKey))
	return feat, routeKeys, feat.FeatureID != ""
}

func parseLoopRouteKeySet(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil
	}
	out := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			out[k] = struct{}{}
		}
	}
	return out
}

func featureRouteKeySet(rt *Runtime, feat FeatureInventoryEntry) []string {
	if rt == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(method, path string) {
		k := routeKey(method, path)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if apiMap, err := loadFeatureApiMap(rt.WorkDir); err == nil && apiMap != nil {
		for _, f := range apiMap.Features {
			if f.FeatureID != feat.FeatureID {
				continue
			}
			for _, a := range f.Apis {
				add(a.Method, a.PathPattern)
			}
		}
	}
	return out
}

func httpEndpointMatchesFeature(ep store.HttpEndpoint, feat FeatureInventoryEntry, routeKeys map[string]struct{}) bool {
	if handlerMatchesFeature(ep.HandlerClass, "", feat) {
		return true
	}
	if len(routeKeys) > 0 {
		if _, ok := routeKeys[routeKey(ep.Method, ep.PathPattern)]; ok {
			return true
		}
	}
	return false
}

func verifiedHttpApiMatchesFeature(v store.VerifiedHttpApi, feat FeatureInventoryEntry, routeKeys map[string]struct{}) bool {
	if handlerMatchesFeature("", v.HandlerFile, feat) {
		return true
	}
	if len(routeKeys) > 0 {
		if _, ok := routeKeys[routeKey(v.Method, v.PathPattern)]; ok {
			return true
		}
	}
	return false
}

func filterHttpEndpointsByFeature(rows []store.HttpEndpoint, feat FeatureInventoryEntry, routeKeys map[string]struct{}) []store.HttpEndpoint {
	if feat.FeatureID == "" {
		return rows
	}
	out := make([]store.HttpEndpoint, 0, len(rows))
	for i := range rows {
		if httpEndpointMatchesFeature(rows[i], feat, routeKeys) {
			out = append(out, rows[i])
		}
	}
	return out
}

func filterVerifiedHttpApisByFeature(rows []store.VerifiedHttpApi, feat FeatureInventoryEntry, routeKeys map[string]struct{}) []store.VerifiedHttpApi {
	if feat.FeatureID == "" {
		return rows
	}
	out := make([]store.VerifiedHttpApi, 0, len(rows))
	for i := range rows {
		if verifiedHttpApiMatchesFeature(rows[i], feat, routeKeys) {
			out = append(out, rows[i])
		}
	}
	return out
}

func filterVerifiedEndpointsByFeature(rows []store.VerifiedEndpoint, allowedEndpointIDs map[uint]struct{}, feat FeatureInventoryEntry, routeKeys map[string]struct{}) []store.VerifiedEndpoint {
	if feat.FeatureID == "" {
		return rows
	}
	out := make([]store.VerifiedEndpoint, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if row.HttpEndpointID != 0 {
			if _, ok := allowedEndpointIDs[row.HttpEndpointID]; ok {
				out = append(out, row)
				continue
			}
		}
		if len(routeKeys) > 0 {
			if _, ok := routeKeys[routeKey(row.Method, row.PathPattern)]; ok {
				out = append(out, row)
			}
		}
	}
	return out
}

func filterEndpointValidationAttemptsByFeature(rows []store.EndpointValidationAttempt, allowedEndpointIDs map[uint]struct{}, feat FeatureInventoryEntry) []store.EndpointValidationAttempt {
	if feat.FeatureID == "" {
		return rows
	}
	out := make([]store.EndpointValidationAttempt, 0, len(rows))
	for i := range rows {
		if _, ok := allowedEndpointIDs[rows[i].HttpEndpointID]; ok {
			out = append(out, rows[i])
		}
	}
	return out
}

func filterCoverageWorkItemsByFeature(rows []store.CoverageWorkItem, allowedEndpointIDs map[uint]struct{}, feat FeatureInventoryEntry) []store.CoverageWorkItem {
	if feat.FeatureID == "" {
		return rows
	}
	out := make([]store.CoverageWorkItem, 0, len(rows))
	for i := range rows {
		if _, ok := allowedEndpointIDs[rows[i].RefID]; ok {
			out = append(out, rows[i])
		}
	}
	return out
}

func featureScopedEndpointIDs(endpoints []store.HttpEndpoint, feat FeatureInventoryEntry, routeKeys map[string]struct{}) map[uint]struct{} {
	out := make(map[uint]struct{})
	for i := range endpoints {
		if httpEndpointMatchesFeature(endpoints[i], feat, routeKeys) {
			out[endpoints[i].ID] = struct{}{}
		}
	}
	return out
}

func featureScopeMeta(feat FeatureInventoryEntry, totalBefore, totalAfter int) map[string]any {
	return map[string]any{
		"feature_id":       feat.FeatureID,
		"label":            feat.Label,
		"package_patterns": feat.PackagePatterns,
		"total_unscoped":   totalBefore,
		"total_scoped":     totalAfter,
	}
}

func attachFeatureScopeToPayload(payload any, feat FeatureInventoryEntry, totalBefore, totalAfter int) any {
	m, ok := payload.(map[string]any)
	if !ok || m == nil {
		return payload
	}
	m["feature_scope"] = featureScopeMeta(feat, totalBefore, totalAfter)
	return m
}
