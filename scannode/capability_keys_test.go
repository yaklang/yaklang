package scannode

import "testing"

func TestNormalizeScanNodeCapabilityKeysSkillBundleByRuntimeMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{aiSessionRuntimeModeStateless, aiSessionRuntimeModeStateful} {
		for _, explicit := range []bool{false, true} {
			name := mode + "/compiled"
			var input []string
			if explicit {
				name = mode + "/explicit"
				input = []string{" ai.skill_bundle.v1 ", "ai.skill_bundle.v1"}
			}
			t.Run(name, func(t *testing.T) {
				keys := normalizeScanNodeCapabilityKeysForRuntime(input, mode)
				count := 0
				for _, key := range keys {
					if key == "ai.skill_bundle.v1" {
						count++
					}
				}
				want := 0
				if mode == aiSessionRuntimeModeStateless {
					want = 1
				}
				if count != want {
					t.Fatalf("Skill capability count = %d, want %d: %v", count, want, keys)
				}
			})
		}
	}
}
