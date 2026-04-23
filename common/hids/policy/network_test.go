//go:build hids

package policy

import "testing"

func TestProcessRolesClassifiesWithoutDuplicateCandidates(t *testing.T) {
	t.Parallel()

	roles := ProcessRoles("", "/usr/bin/curl", "/usr/bin/curl https://example.com")
	if len(roles) != 1 || roles[0] != "network_tool" {
		t.Fatalf("unexpected roles: %#v", roles)
	}
	if !HasProcessRole("", "/usr/sbin/uWSGI", "", "web") {
		t.Fatal("expected mixed-case image basename to match web role")
	}
	if !HasAnyProcessRole("bash", "", "", "interpreter", "shell") {
		t.Fatal("expected shell role match")
	}
}
