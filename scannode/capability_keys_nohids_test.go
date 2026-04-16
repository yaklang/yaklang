//go:build !hids

package scannode

import (
	"reflect"
	"testing"
)

func TestNormalizeScanNodeCapabilityKeysDefaultsToNonHIDSBuildSurface(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys(nil)
	want := []string{"yak.execute"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeScanNodeCapabilityKeysKeepsExplicitExtrasWithoutDuplicates(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys([]string{"extra.capability", "yak.execute", " ", "extra.capability"})
	want := []string{"yak.execute", "extra.capability"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}
