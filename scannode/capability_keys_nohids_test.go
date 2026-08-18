//go:build !hids

package scannode

import (
	"reflect"
	"testing"
)

func TestNormalizeScanNodeCapabilityKeysDefaultsToNonHIDSBuildSurface(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys(nil)
	want := []string{
		"yak.execute",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAICodeWorkspaceV1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeScanNodeCapabilityKeysKeepsExplicitExtrasWithoutDuplicates(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeys([]string{
		"extra.capability",
		"yak.execute",
		capabilityKeySSARuleSyncExport,
		" ",
		"extra.capability",
	})
	want := []string{
		"yak.execute",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAICodeWorkspaceV1,
		"extra.capability",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeScanNodeCapabilityKeysHidesCodeWorkspaceInStatefulRollbackMode(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeysForRuntime(
		[]string{capabilityKeyAICodeWorkspaceV1, "extra.capability"},
		aiSessionRuntimeModeStateful,
	)
	for _, key := range got {
		if key == capabilityKeyAICodeWorkspaceV1 {
			t.Fatalf("stateful rollback mode advertised unsupported capability: %#v", got)
		}
	}
}
