package scannode

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestProductNodeManifestCapabilitiesMatchCompiledSurface(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../.github/scripts/legion-product-node-capabilities.json")
	if err != nil {
		t.Fatalf("read product-node capability contract: %v", err)
	}
	var advertised []string
	if err := json.Unmarshal(data, &advertised); err != nil {
		t.Fatalf("decode product-node capability contract: %v", err)
	}

	want := append(compiledScanNodeCapabilityKeys(), AIRuntimeHostCapabilityKey)
	if !slices.Contains(want, "hids") {
		want = append(want, "hids")
	}
	slices.Sort(advertised)
	slices.Sort(want)
	if !slices.Equal(advertised, want) {
		t.Fatalf("product-node manifest capabilities drifted from compiled surface: got=%#v want=%#v", advertised, want)
	}
}

func TestProductManifestApplicationCapabilitiesDoNotOverrideStatefulRuntime(t *testing.T) {
	raw, err := os.ReadFile("../.github/scripts/legion-product-node-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var advertised []string
	if err := json.Unmarshal(raw, &advertised); err != nil {
		t.Fatal(err)
	}
	actual := normalizeScanNodeCapabilityKeysForRuntime(advertised, aiSessionRuntimeModeStateful)
	for _, key := range []string{capabilityKeyAISkillBundleV1, capabilityKeyAIForgeReleaseV1, capabilityKeyAIForgeCustomToolsV1, capabilityKeyAIToolsBuiltinV1} {
		if slices.Contains(actual, key) {
			t.Fatalf("stateful runtime inherited unsupported application capability %s", key)
		}
	}
}

func TestNormalizeScanNodeCapabilityKeysDropsLegacyForgeProfiles(t *testing.T) {
	legacy := []string{"ai.forge.evidence.v1", "ai.forge.discovery.v1", "ai.forge.discovery.v2", "ai.forge.http_assessment.v2"}
	for _, mode := range []string{aiSessionRuntimeModeStateful, aiSessionRuntimeModeStateless} {
		actual := normalizeScanNodeCapabilityKeysForRuntime(legacy, mode)
		for _, key := range legacy {
			if slices.Contains(actual, key) {
				t.Fatalf("legacy key advertised: %s", key)
			}
		}
		if slices.Contains(actual, capabilityKeyAIToolsBuiltinV1) != (mode == aiSessionRuntimeModeStateless) {
			t.Fatalf("built-in tool capability does not match runtime mode: %s", mode)
		}
	}
}
