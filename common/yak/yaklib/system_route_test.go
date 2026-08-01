package yaklib

import (
	"testing"
	"time"
)

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

func TestMUSTPASS_LookupSystemIPWithTimeoutDoesNotUseSlowFallbacks(t *testing.T) {
	start := time.Now()
	results := LookupSystemIPWithTimeout("yak-tun-probe-must-not-exist.invalid", 0.2)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("system-only DNS lookup exceeded its bounded timeout: %v", elapsed)
	}
	if len(results) > 0 {
		t.Logf("system resolver synthesized reserved-name result (expected under Fake-IP DNS): %v", results)
	}
	if _, ok := SystemExports["LookupSystemIPWithTimeout"]; !ok {
		t.Fatal("os.LookupSystemIPWithTimeout is not registered in SystemExports")
	}
}
