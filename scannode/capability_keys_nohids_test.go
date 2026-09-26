//go:build !hids

package scannode

import (
	"github.com/yaklang/yaklang/scannode/inputresolver"
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
		capabilityKeyAISkillBundleV1,
		capabilityKeyAIForgeReleaseV1,
		capabilityKeyAIForgeCustomToolsV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyAIForgeDiscoveryV1,
		capabilityKeyAIForgeDiscoveryV2,
		capabilityKeyAIForgeHTTPAssessmentV2,
		capabilityKeyPluginBundleV1,
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
		want = append(want, "")
		copy(want[index+2:], want[index+1:])
		want[index+1] = capabilityKeyAIForgeEvidenceV1
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
		capabilityKeyAISkillBundleV1,
		capabilityKeyAIForgeReleaseV1,
		capabilityKeyAIForgeCustomToolsV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyPluginBundleV1,
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
	if inputresolver.Supported() {
		for i, key := range want {
			if key == capabilityKeyAIManagedInputV1 {
				want = append(want, "")
				copy(want[i+2:], want[i+1:])
				want[i+1] = capabilityKeyAIForgeEvidenceV1
				break
			}
		}
	}
	for i, key := range want {
		if key == capabilityKeyPluginBundleV1 {
			want = append(want, "", "", "")
			copy(want[i+3:], want[i:])
			want[i] = capabilityKeyAIForgeDiscoveryV1
			want[i+1] = capabilityKeyAIForgeDiscoveryV2
			want[i+2] = capabilityKeyAIForgeHTTPAssessmentV2
			break
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capability keys: got=%#v want=%#v", got, want)
	}
}

func TestNormalizeScanNodeCapabilityKeysHidesCodeWorkspaceInStatefulRollbackMode(t *testing.T) {
	t.Parallel()

	got := normalizeScanNodeCapabilityKeysForRuntime(
		[]string{capabilityKeyAICodeWorkspaceV1, capabilityKeyAIForgeReleaseV1, "extra.capability"},
		aiSessionRuntimeModeStateful,
	)
	for _, key := range got {
		if key == capabilityKeyAICodeWorkspaceV1 || key == capabilityKeyAIForgeReleaseV1 {
			t.Fatalf("stateful rollback mode advertised unsupported capability: %#v", got)
		}
	}
}
