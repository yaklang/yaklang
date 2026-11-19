package test

import "testing"

// TODO(ssa-scala): cover extractor/case-class/tuple/typed patterns, match
// exhaustiveness, given/using, extension methods, for-comprehension guards/yield,
// class-field writes across methods, and runtime asInstanceOf checks.
// These features are not lowered yet, so this test is intentionally skipped.
func TestUnimplementedFeatures(t *testing.T) {
	t.Skip("Scala: no coverage for extractors/case classes, exhaustiveness, given/using, for guards, runtime casts")
}
