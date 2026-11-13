package pass

import "testing"

func TestDefaultPassesAreRegistered(t *testing.T) {
	for _, name := range DefaultNames() {
		pass, ok := Get(name)
		if !ok || pass.Apply == nil {
			t.Fatalf("default pass %q is not registered", name)
		}
	}
}
