//go:build hids

package scannode

import (
	"github.com/yaklang/yaklang/scannode/inputresolver"
	"reflect"
	"testing"
)

func TestNormalizeScanNodeCapabilityKeysAddsHIDSCapabilityWhenCompiled(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys(nil)
	want := []string{
		"yak.execute",
		"hids",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAICodeWorkspaceV1,
	}
	if inputresolver.Supported() {
		index := len(want)
		for i, k := range want {
			if k == capabilityKeyAICodeWorkspaceV1 {
				index = i + 1
				break
			}
		}
		want = append(want, "")
		copy(want[index+1:], want[index:])
		want[index] = capabilityKeyAIManagedInputV1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeScanNodeCapabilityKeysDeduplicatesCompiledHIDSCapability(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys([]string{
		"hids",
		capabilityKeySSARuleSyncExport,
		"extra.capability",
		"yak.execute",
		"extra.capability",
	})
	want := []string{
		"yak.execute",
		"hids",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAICodeWorkspaceV1,
		"extra.capability",
	}
	if inputresolver.Supported() {
		index := len(want)
		for i, k := range want {
			if k == capabilityKeyAICodeWorkspaceV1 {
				index = i + 1
				break
			}
		}
		want = append(want, "")
		copy(want[index+1:], want[index:])
		want[index] = capabilityKeyAIManagedInputV1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}
