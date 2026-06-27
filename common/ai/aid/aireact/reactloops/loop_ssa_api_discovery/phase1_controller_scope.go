package loop_ssa_api_discovery

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const discoveryControllerScopeLoopKey = "discovery_controller_scope_json"

// ControllerVerifyScope pins a single controller file on a ReAct loop for read-session filtering.
type ControllerVerifyScope struct {
	ControllerFile  string   `json:"controller_file"`
	FeatureID       string   `json:"feature_id"`
	FeatureLabel    string   `json:"feature_label,omitempty"`
	PackagePatterns []string `json:"package_patterns,omitempty"`
	RouteKeys       []string `json:"route_keys,omitempty"`
}

func setLoopControllerScope(loop *reactloops.ReActLoop, scope ControllerVerifyScope) {
	if loop == nil {
		return
	}
	scope.ControllerFile = normalizePlanFileRef(nil, scope.ControllerFile)
	b, _ := json.Marshal(scope)
	loop.Set(discoveryControllerScopeLoopKey, string(b))
	loop.Set("discovery_controller_file", scope.ControllerFile)
}

func loopControllerScope(loop *reactloops.ReActLoop) (ControllerVerifyScope, map[string]struct{}, bool) {
	if loop == nil {
		return ControllerVerifyScope{}, nil, false
	}
	raw := strings.TrimSpace(loop.Get(discoveryControllerScopeLoopKey))
	if raw == "" {
		return ControllerVerifyScope{}, nil, false
	}
	var scope ControllerVerifyScope
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		return ControllerVerifyScope{}, nil, false
	}
	routeKeys := controllerRouteKeySet(scope.RouteKeys)
	return scope, routeKeys, strings.TrimSpace(scope.ControllerFile) != ""
}

func controllerRouteKeySet(keys []string) map[string]struct{} {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			out[k] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func handlerMatchesControllerFile(handlerClass, fileRel, controllerFile string) bool {
	controllerFile = strings.ToLower(strings.TrimSpace(normalizePlanFileRef(nil, controllerFile)))
	if controllerFile == "" {
		return false
	}
	fr := strings.ToLower(strings.TrimSpace(normalizePlanFileRef(nil, fileRel)))
	if fr != "" {
		if fr == controllerFile || strings.HasSuffix(fr, controllerFile) || strings.HasSuffix(controllerFile, fr) {
			return true
		}
		if filepath.Base(fr) == filepath.Base(controllerFile) {
			return true
		}
	}
	hc := strings.ToLower(strings.TrimSpace(handlerClass))
	if hc == "" {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(controllerFile), ".java")
	if base != "" && strings.Contains(hc, strings.TrimSuffix(base, "Controller")) {
		return true
	}
	return false
}

func httpEndpointMatchesController(ep store.HttpEndpoint, scope ControllerVerifyScope, routeKeys map[string]struct{}) bool {
	if handlerMatchesControllerFile(ep.HandlerClass, "", scope.ControllerFile) {
		return true
	}
	if len(routeKeys) > 0 {
		if _, ok := routeKeys[routeKey(ep.Method, ep.PathPattern)]; ok {
			return true
		}
	}
	return false
}

func verifiedHttpApiMatchesController(v store.VerifiedHttpApi, scope ControllerVerifyScope, routeKeys map[string]struct{}) bool {
	if handlerMatchesControllerFile("", v.HandlerFile, scope.ControllerFile) {
		return true
	}
	if len(routeKeys) > 0 {
		if _, ok := routeKeys[routeKey(v.Method, v.PathPattern)]; ok {
			return true
		}
	}
	return false
}

func filterHttpEndpointsByController(rows []store.HttpEndpoint, scope ControllerVerifyScope, routeKeys map[string]struct{}) []store.HttpEndpoint {
	if strings.TrimSpace(scope.ControllerFile) == "" {
		return rows
	}
	var out []store.HttpEndpoint
	for i := range rows {
		if httpEndpointMatchesController(rows[i], scope, routeKeys) {
			out = append(out, rows[i])
		}
	}
	return out
}

func filterVerifiedHttpApisByController(rows []store.VerifiedHttpApi, scope ControllerVerifyScope, routeKeys map[string]struct{}) []store.VerifiedHttpApi {
	if strings.TrimSpace(scope.ControllerFile) == "" {
		return rows
	}
	var out []store.VerifiedHttpApi
	for i := range rows {
		if verifiedHttpApiMatchesController(rows[i], scope, routeKeys) {
			out = append(out, rows[i])
		}
	}
	return out
}

func controllerScopeMeta(scope ControllerVerifyScope, totalBefore, totalAfter int) map[string]any {
	return map[string]any{
		"controller_scope_active": true,
		"controller_file":       scope.ControllerFile,
		"feature_id":            scope.FeatureID,
		"total_unscoped":        totalBefore,
		"total_scoped":          totalAfter,
	}
}

func attachControllerScopeToPayload(payload any, scope ControllerVerifyScope, totalBefore, totalAfter int) any {
	meta := controllerScopeMeta(scope, totalBefore, totalAfter)
	switch p := payload.(type) {
	case map[string]any:
		if p == nil {
			p = map[string]any{}
		}
		p["controller_scope"] = meta
		return p
	default:
		return map[string]any{
			"rows":              payload,
			"controller_scope": meta,
		}
	}
}
