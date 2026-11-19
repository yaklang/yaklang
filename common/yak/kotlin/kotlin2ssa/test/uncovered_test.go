package test

import "testing"

// TODO(ssa-kotlin): cover `as?` failure/null, smart casts, sealed/enum
// exhaustiveness, class-field writes across methods, and user-defined/generic
// type mapping. These features are not lowered yet, so this test is intentionally skipped.
func TestUnimplementedFeatures(t *testing.T) {
	t.Skip("Kotlin: no coverage for as?/smart cast, sealed exhaustiveness, class-field sync, generic types")
}
