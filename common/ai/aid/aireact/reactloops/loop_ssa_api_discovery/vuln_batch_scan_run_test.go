package loop_ssa_api_discovery

import "testing"

func TestVulnScanAuthMode(t *testing.T) {
	if got := vulnScanAuthMode(false, 0); got != "none" {
		t.Fatalf("want none, got %q", got)
	}
	if got := vulnScanAuthMode(true, 3); got != "credential_3" {
		t.Fatalf("want credential_3, got %q", got)
	}
}

func TestResolveVulnScanCredential_NilRuntime(t *testing.T) {
	id, ok := resolveVulnScanCredential(nil, nil, 0)
	if ok || id != 0 {
		t.Fatalf("expected no credential, got id=%d ok=%v", id, ok)
	}
}
