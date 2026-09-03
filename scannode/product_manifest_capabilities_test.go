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
