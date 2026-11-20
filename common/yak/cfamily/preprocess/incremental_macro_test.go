package preprocess

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// applyDefineLineIncremental exists so a TU does not rebuild the whole macro
// table for every #define/#undef. The expander must still see exactly what a
// rebuild would produce, including cache invalidation for redefined macros.
func TestIncrementalMacroRedefinitionInvalidatesTokenCache(t *testing.T) {
	tables := NewMacroTables()
	env := newMacroEnvFromTables(tables)

	require.True(t, applyDefineLineIncremental("#define FOO 2", &tables, env))
	require.Equal(t, "2", strings.TrimSpace(env.expandSource("FOO")))

	require.True(t, applyDefineLineIncremental("#define FOO 3", &tables, env))
	require.Equal(t, "3", strings.TrimSpace(env.expandSource("FOO")),
		"a redefinition must drop the cached body tokens")

	undefineMacroIncremental("FOO", &tables, env)
	require.Equal(t, "FOO", strings.TrimSpace(env.expandSource("FOO")))
}

func TestIncrementalMacroSwitchesFunctionAndObjectForms(t *testing.T) {
	tables := NewMacroTables()
	env := newMacroEnvFromTables(tables)

	require.True(t, applyDefineLineIncremental("#define DUP 1", &tables, env))
	require.Contains(t, tables.Object, "DUP")

	// A function-like define replaces the object-like one, as ApplyDefineLine does.
	require.True(t, applyDefineLineIncremental("#define DUP(x) (x)", &tables, env))
	require.NotContains(t, tables.Object, "DUP")
	require.Contains(t, tables.Function, "DUP")
	require.Equal(t, "(7)", strings.TrimSpace(env.expandSource("DUP(7)")))

	// ... and the other way round.
	require.True(t, applyDefineLineIncremental("#define DUP 9", &tables, env))
	require.NotContains(t, tables.Function, "DUP")
	require.Contains(t, tables.Object, "DUP")
	require.Equal(t, "9", strings.TrimSpace(env.expandSource("DUP")))
}

// The exported tables are shared with the conditional stack (`#ifdef`); the
// incremental path must keep that view in sync too.
func TestIncrementalMacroKeepsExportedTablesAndEnvInSync(t *testing.T) {
	tables := NewMacroTables()
	env := newMacroEnvFromTables(tables)

	require.True(t, applyDefineLineIncremental("#define FLAG 1", &tables, env))
	require.Equal(t, "1", tables.Object["FLAG"])

	undefineMacroIncremental("FLAG", &tables, env)
	require.NotContains(t, tables.Object, "FLAG")
	require.Equal(t, "FLAG", strings.TrimSpace(env.expandSource("FLAG")))

	// An expander rebuilt from the mutated exported tables is equivalent to the
	// incrementally updated one for every macro form.
	require.True(t, applyDefineLineIncremental("#define OBJ 5", &tables, env))
	require.True(t, applyDefineLineIncremental("#define FN(a) (a)*2", &tables, env))
	rebuilt := newMacroEnvFromTables(tables)
	require.Equal(t, env.expandSource("OBJ FN(3)"), rebuilt.expandSource("OBJ FN(3)"))
}

func TestIncrementalMacroIgnoresNonDefineLines(t *testing.T) {
	tables := NewMacroTables()
	env := newMacroEnvFromTables(tables)
	require.False(t, applyDefineLineIncremental("int x;", &tables, env))
	require.False(t, applyDefineLineIncremental("#undef X", &tables, env))
	require.Empty(t, tables.Object)
}
