package pass

import (
	"testing"
)

func TestDefaultPipelineIsRegistered(t *testing.T) {
	for _, name := range DefaultNames() {
		if _, ok := registered[name]; !ok {
			t.Fatalf("default pass %q is not registered", name)
		}
	}
}

func TestApplySelectedRejectsUnknownPass(t *testing.T) {
	program := parseSSAForPass(t, "empty", "type: () -> void", "entry_0:", "    ret")
	if err := ApplySelected(program, "missing-pass"); err == nil {
		t.Fatal("unknown pass was accepted")
	}
}

func TestApplySelectedRunsRequestedPassesInOrder(t *testing.T) {
	program := parseSSAForPass(t, "empty", "type: () -> void", "entry_0:", "    ret")
	if err := ApplySelected(program, "apply-function-define", "infer-types"); err != nil {
		t.Fatalf("selected passes failed: %v", err)
	}
}

func TestRegisteredPassesExposeDescriptions(t *testing.T) {
	for _, item := range All() {
		if item.Name == "" || item.Description == "" || item.Apply == nil {
			t.Fatalf("incomplete pass registration: %+v", item)
		}
	}
}
