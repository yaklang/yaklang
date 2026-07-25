package schema

import (
	"testing"
)

func TestShouldSave_DebugOnlyEvents(t *testing.T) {
	// Save and restore the global flag.
	old := debugEventPersistence.Load()
	defer debugEventPersistence.Store(old)

	// By default (debug off), prompt_profile and reference_material should NOT save.
	debugEventPersistence.Store(false)

	pp := &AiOutputEvent{Type: EVENT_TYPE_PROMPT_PROFILE, NodeId: "system"}
	if pp.ShouldSave() {
		t.Error("prompt_profile should NOT save when debug persistence is off")
	}

	rm := &AiOutputEvent{Type: EVENT_TYPE_REFERENCE_MATERIAL, NodeId: "reference_material"}
	if rm.ShouldSave() {
		t.Error("reference_material should NOT save when debug persistence is off")
	}

	// Other event types should still save normally.
	tc := &AiOutputEvent{Type: EVENT_TOOL_CALL_START, NodeId: "tc"}
	if !tc.ShouldSave() {
		t.Error("tool_call_start should save regardless of debug mode")
	}

	// With debug on, prompt_profile and reference_material SHOULD save.
	debugEventPersistence.Store(true)

	if !pp.ShouldSave() {
		t.Error("prompt_profile should save when debug persistence is on")
	}
	if !rm.ShouldSave() {
		t.Error("reference_material should save when debug persistence is on")
	}
}
