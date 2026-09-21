//go:build irify_exclude

package sfdb

import "testing"

func TestSlimGate_NoEmbeddedRuleVersions(t *testing.T) {
	if len(getVersionMap()) != 0 {
		t.Fatal("slim must not retain the full edition's embedded rule versions")
	}
	if _, err := GetVersionFromEmbed("missing-rule"); err == nil {
		t.Fatal("unavailable embedded versions must remain a lookup error")
	}
}
