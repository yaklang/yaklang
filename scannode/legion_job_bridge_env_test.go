package scannode

import (
	"fmt"
	"testing"
)

// TestSelectAISessionRuntimeDriverStateless verifies that LEGION_AI_RUNTIME=stateless
// selects the stateless driver (S3c). Uses %T to identify the concrete type
// because the driver types are unexported and reflect.TypeOf().Name() is
// unreliable for them.
func TestSelectAISessionRuntimeDriverStateless(t *testing.T) {
	t.Setenv("LEGION_AI_RUNTIME", "stateless")
	d := selectAISessionRuntimeDriver()
	got := fmt.Sprintf("%T", d)
	if got != "scannode.statelessAIEngineRuntimeDriver" {
		t.Fatalf("LEGION_AI_RUNTIME=stateless: expected statelessAIEngineRuntimeDriver, got %s", got)
	}
}

func TestSelectAISessionRuntimeDriverDefaultStateless(t *testing.T) {
	cases := map[string]string{
		"unset":    "",
		"empty":    "   ",
		"explicit": "STATELESS",
		"invalid":  "something-else",
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("LEGION_AI_RUNTIME", val)
			d := selectAISessionRuntimeDriver()
			got := fmt.Sprintf("%T", d)
			if got != "scannode.statelessAIEngineRuntimeDriver" {
				t.Fatalf("LEGION_AI_RUNTIME=%q: expected statelessAIEngineRuntimeDriver, got %s", val, got)
			}
		})
	}
}

func TestSelectAISessionRuntimeDriverExplicitStatefulRollback(t *testing.T) {
	t.Setenv("LEGION_AI_RUNTIME", " STATEFUL ")
	d := selectAISessionRuntimeDriver()
	got := fmt.Sprintf("%T", d)
	if got != "scannode.yakAIEngineRuntimeDriver" {
		t.Fatalf("explicit stateful rollback: expected yakAIEngineRuntimeDriver, got %s", got)
	}
}
