package tiers

import (
	"slices"
	"testing"
)

// TestLadderIsNested guards the invariant the rest of the package is built on:
// every tier contains the one below it. Select returns the first covering tier
// and the compiler falls back to any later one, so a ladder with a gap would
// silently link a script against an archive missing one of its modules.
func TestLadderIsNested(t *testing.T) {
	for i := 1; i < len(All); i++ {
		lower, upper := All[i-1], All[i]
		if !upper.Covers(lower.Modules) {
			t.Errorf("tier %q does not contain %q: %v vs %v",
				upper.Name, lower.Name, upper.Modules, lower.Modules)
		}
		if len(upper.Modules) <= len(lower.Modules) {
			t.Errorf("tier %q is not larger than %q", upper.Name, lower.Name)
		}
	}
}

// TestModulesSorted keeps the module lists canonical: build_yaklib.sh names a
// build by comparing its sorted module list against these, so an unsorted entry
// would make that tier unnameable and every archive built from it "custom".
func TestModulesSorted(t *testing.T) {
	for _, tier := range All {
		if !slices.IsSorted(tier.Modules) {
			t.Errorf("tier %q modules are not sorted: %v", tier.Name, tier.Modules)
		}
	}
}

func TestSelectPicksSmallestCoveringTier(t *testing.T) {
	for _, tc := range []struct {
		name    string
		modules []string
		want    string
	}{
		{"no modules", nil, "core"},
		{"pure computation", []string{"codec"}, "core"},
		{"codec and yakit", []string{"codec", "yakit"}, "core"},
		{"network", []string{"http"}, "net"},
		{"core plus network", []string{"codec", "poc"}, "net"},
		{"ssa", []string{"ssa"}, "full"},
		{"ssa with codec", []string{"codec", "ssa"}, "full"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Select(tc.modules)
			if err != nil {
				t.Fatalf("Select(%v): %v", tc.modules, err)
			}
			if got.Name != tc.want {
				t.Errorf("Select(%v) = %q, want %q", tc.modules, got.Name, tc.want)
			}
		})
	}
}

// TestSelectRejectsUnknownModule keeps an unshimmed module from quietly
// selecting a tier that cannot register it. The caller treats the error as
// "use the embedded archive", which either has the module or reports it by
// name before the linker runs.
func TestSelectRejectsUnknownModule(t *testing.T) {
	if _, err := Select([]string{"codec", "mitm"}); err == nil {
		t.Fatal("Select accepted a module no tier provides")
	}
}

func TestAtLeastReturnsTierAndLarger(t *testing.T) {
	got := AtLeast("net")
	want := []string{"net", "full"}
	if len(got) != len(want) {
		t.Fatalf("AtLeast(net) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("AtLeast(net)[%d] = %q, want %q", i, got[i].Name, want[i])
		}
	}
	if AtLeast("nonexistent") != nil {
		t.Error("AtLeast returned tiers for an unknown name")
	}
}

// TestLargestCoversEveryTier is what makes the embedded archive a safe
// fallback: it is built as the largest tier, so no script can need a module it
// lacks.
func TestLargestCoversEveryTier(t *testing.T) {
	for _, tier := range All {
		if !Largest().Covers(tier.Modules) {
			t.Errorf("largest tier %q does not cover %q", Largest().Name, tier.Name)
		}
	}
}
