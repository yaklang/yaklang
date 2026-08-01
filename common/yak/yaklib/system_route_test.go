package yaklib

import "testing"

func TestMUSTPASS_GetRouteInfoLoopbackWithoutPrivilege(t *testing.T) {
	info := GetRouteInfo("127.0.0.1")
	if info == nil {
		t.Fatal("GetRouteInfo returned nil")
	}
	if info["error"] != "" {
		t.Fatalf("GetRouteInfo loopback failed: %s", info["error"])
	}
	if info["interface"] == "" {
		t.Fatalf("GetRouteInfo returned no interface: %#v", info)
	}
	if _, ok := SystemExports["GetRouteInfo"]; !ok {
		t.Fatal("os.GetRouteInfo is not registered in SystemExports")
	}
}
