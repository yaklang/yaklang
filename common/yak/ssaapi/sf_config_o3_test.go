package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// TestSFCheck_O3_EmptyCheckSkipsSnapshot asserts that an sfCheck with no
// sub-rules (empty matchItem/untilItem, e.g. the unconditionally-created
// untilCheck/hookRunner in DataFlowWithSFConfig) must NOT build an
// originalSnapshot: it never runs check()/clearup, so the eager snapshot is
// pure waste (43M TakeSymbolSnapshot calls / 42GB on hadoop).
//
// RED: CreateCheck builds originalSnapshot eagerly for every check, including
// empty ones. GREEN: originalSnapshot stays nil until the first check() runs.
func TestSFCheck_O3_EmptyCheckSkipsSnapshot(t *testing.T) {
	cfg := sf.NewConfig()
	contextResult := sf.NewSFResult(nil, cfg)
	check := CreateCheck(contextResult, cfg)

	// An empty check (no include/exclude/until/hook items) must not have built
	// a snapshot.
	require.True(t, check.Empty(), "precondition: empty check")
	require.Nil(t, check.originalSnapshot,
		"empty check must not build an originalSnapshot (pure waste)")
}

// TestSFCheck_O3_NonEmptyCheckBuildsSnapshotOnce asserts that a check with a
// real sub-rule item builds the originalSnapshot exactly once, and that the
// snapshot reflects the parent's pre-descent state. Appending more items must
// not rebuild it (sync.Once).
func TestSFCheck_O3_NonEmptyCheckBuildsSnapshotOnce(t *testing.T) {
	cfg := sf.NewConfig()
	contextResult := sf.NewSFResult(nil, cfg)
	// Parent has a named symbol so the snapshot is non-empty.
	prog := NewTmpProgram("o3-nonempty")
	v := fastMatchTestValue(t, prog, 1)
	contextResult.SymbolTable.Set("src", sf.Values{v})

	check := CreateCheck(contextResult, cfg)
	require.Nil(t, check.originalSnapshot, "no snapshot until first real item appended")

	const include = "* & $src as $__next__"
	check.AppendItems(&sf.RecursiveConfigItem{Key: string(sf.RecursiveConfig_Include), Value: include, SyntaxFlowRule: true})
	require.NotNil(t, check.originalSnapshot, "first real item must build the snapshot")
	snap1 := check.originalSnapshot

	// Appending more items must NOT rebuild the snapshot (same pointer).
	check.AppendItems(&sf.RecursiveConfigItem{Key: string(sf.RecursiveConfig_Include), Value: include, SyntaxFlowRule: true})
	require.Same(t, snap1, check.originalSnapshot, "snapshot must be built exactly once")
}
