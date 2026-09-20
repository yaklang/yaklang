//go:build irify_exclude

package sfdb

// Slim has no embedded SyntaxFlow rule corpus. Retain the lookup API for
// shared callers without embedding versions of rules that are not shipped.
func readRuleVersions() ([]byte, error) { return []byte("[]"), nil }
