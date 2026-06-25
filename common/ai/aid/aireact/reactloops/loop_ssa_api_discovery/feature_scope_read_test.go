package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestFilterHttpEndpointsByFeature_webOnly(t *testing.T) {
	feat := FeatureInventoryEntry{
		FeatureID:       "web_controllers",
		PackagePatterns: []string{"*.controller.web.*"},
	}
	rows := []store.HttpEndpoint{
		{ID: 1, Method: "GET", PathPattern: "/", HandlerClass: "com.publiccms.controller.web.IndexController"},
		{ID: 2, Method: "GET", PathPattern: "/admin/login", HandlerClass: "com.publiccms.controller.admin.LoginAdminController"},
		{ID: 3, Method: "GET", PathPattern: "/api/apis", HandlerClass: "com.publiccms.controller.api.ApiController"},
	}
	filtered := filterHttpEndpointsByFeature(rows, feat, nil)
	require.Len(t, filtered, 1)
	require.Equal(t, uint(1), filtered[0].ID)
}

func TestFilterVerifiedHttpApisByFeature_routeKeyFallback(t *testing.T) {
	feat := FeatureInventoryEntry{FeatureID: "web_controllers", PackagePatterns: []string{"*.controller.web.*"}}
	routeKeys := map[string]struct{}{
		routeKey("GET", "/doLogin"): {},
	}
	rows := []store.VerifiedHttpApi{
		{Method: "GET", PathPattern: "/doLogin"},
		{Method: "POST", PathPattern: "/admin/login", HandlerFile: "com/publiccms/controller/admin/LoginAdminController.java"},
	}
	filtered := filterVerifiedHttpApisByFeature(rows, feat, routeKeys)
	require.Len(t, filtered, 1)
	require.Equal(t, "/doLogin", filtered[0].PathPattern)
}

func TestParseLoopRouteKeySet(t *testing.T) {
	keys := parseLoopRouteKeySet(`["GET /","POST /doLogin"]`)
	require.Len(t, keys, 2)
	_, ok := keys["GET /"]
	require.True(t, ok)
}
