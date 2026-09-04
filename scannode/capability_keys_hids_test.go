//go:build hids

package scannode

import (
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
	if legionSyntaxFlowRuntimeAvailable() {
		want = append(want, capabilityKeyAISyntaxFlowRuleV1)
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
	}
	if legionSyntaxFlowRuntimeAvailable() {
		want = append(want, capabilityKeyAISyntaxFlowRuleV1)
	}
	want = append(want, "extra.capability")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}
