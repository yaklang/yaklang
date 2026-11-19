package test

import "testing"

// TODO(ssa-lua): cover invalid tonumber (nil), upvalue mutation, _ENV lookup,
// goto/label control flow, runtime table length, and metatable/__tostring.
// These features are not lowered yet, so this test is intentionally skipped.
func TestUnimplementedFeatures(t *testing.T) {
	t.Skip("Lua: no coverage for tonumber failure, upvalue mutation, _ENV, goto, runtime length, metatables")
}
