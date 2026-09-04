package sfvm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNativeCallGuardSurvivesNestedConfigCopies(t *testing.T) {
	guard := func(name string) error { return fmt.Errorf("query-local denial: %s", name) }
	parent := NewConfig(WithNativeCallGuard(guard))
	for name, config := range map[string]*Config{
		"parent":     parent,
		"WithConfig": NewConfig(WithConfig(parent)),
		"Copy":       parent.Copy(),
		"nested":     NewConfig(WithConfig(parent.Copy())).Copy(),
	} {
		t.Run(name, func(t *testing.T) {
			frame := &SFFrame{config: config}
			// A denied call must fail before touching even the input stack or
			// looking up a native, including unknown future native extensions.
			_, err := frame.execNativeCall(&SFI{OpCode: OpNativeCall, UnaryStr: "future-native"})
			if !errors.Is(err, CriticalError) || !strings.Contains(err.Error(), "query-local denial: future-native") {
				t.Fatalf("native authority guard was lost: %v", err)
			}
		})
	}
	if NewConfig().nativeCallGuard != nil {
		t.Fatal("query-local guard leaked into ordinary engine config")
	}
}
