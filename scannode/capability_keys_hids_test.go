//go:build hids

package scannode

import (
	"reflect"
	"testing"
)

func TestNormalizeScanNodeCapabilityKeysAddsHIDSCapabilityWhenCompiled(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys(nil)
	want := []string{"yak.execute", "hids"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeScanNodeCapabilityKeysDeduplicatesCompiledHIDSCapability(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys([]string{"hids", "extra.capability", "yak.execute", "extra.capability"})
	want := []string{"yak.execute", "hids", "extra.capability"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}
