package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestFilterVerifiedHttpApisByController(t *testing.T) {
	scope := ControllerVerifyScope{
		ControllerFile: "mod/FooController.java",
		FeatureID:      "api_core",
	}
	rows := []store.VerifiedHttpApi{
		{Method: "GET", PathPattern: "/api/foo", HandlerFile: "mod/FooController.java"},
		{Method: "GET", PathPattern: "/admin/bar", HandlerFile: "mod/BarController.java"},
	}
	out := filterVerifiedHttpApisByController(rows, scope, nil)
	require.Len(t, out, 1)
	require.Equal(t, "/api/foo", out[0].PathPattern)
}

func TestControllerRouteKeySet(t *testing.T) {
	m := controllerRouteKeySet([]string{routeKey("GET", "/a"), routeKey("POST", "/b")})
	require.Len(t, m, 2)
	require.Contains(t, m, routeKey("GET", "/a"))
}
