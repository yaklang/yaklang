package test

import "testing"

// TODO(ssa-rust): cover enum/tuple/struct pattern destructuring, macro expansion,
// move/borrow and lifetime semantics, runtime `as` checks, and method receiver references.
// These features are not lowered yet, so this test is intentionally skipped.
func TestUnimplementedFeatures(t *testing.T) {
	t.Skip("Rust: no coverage for enum/tuple/struct patterns, macro expansion, ownership/refs, runtime casts")
}
