package bash2ssa_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// split marker counts assert how the frontend models shell word splitting:
// only unquoted expansions in argument position get the intrinsic, and the
// intrinsic name records whether it came from a parameter or a command
// substitution.
func countSplitMarks(t *testing.T, code, callee string) int {
	t.Helper()
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError("/^@bash\\." + callee + "$/ as $split\n$split(* as $argv) as $hit")
	require.NoError(t, err)
	return len(res.GetValues("hit"))
}

func TestBashWordSplitting(t *testing.T) {
	t.Run("unquoted parameter", func(t *testing.T) {
		require.Equal(t, 1, countSplitMarks(t, "exec $foo\n", "param-fields"))
		require.Equal(t, 1, countSplitMarks(t, "exec ${foo%.bar}\n", "param-fields"))
		require.Equal(t, 1, countSplitMarks(t, "exec $1\n", "param-fields"))
		require.Equal(t, 1, countSplitMarks(t, "exec --prefix=$dir\n", "param-fields"))
	})

	t.Run("quoted parameter", func(t *testing.T) {
		require.Zero(t, countSplitMarks(t, "exec \"$foo\"\n", "param-fields"))
		require.Zero(t, countSplitMarks(t, "exec \"$foo/bar\"\n", "param-fields"))
	})

	t.Run("one-word parameters", func(t *testing.T) {
		// These cannot be split, so they must not be marked.
		require.Zero(t, countSplitMarks(t, "echo $$\n", "param-fields"))
		require.Zero(t, countSplitMarks(t, "echo $?\n", "param-fields"))
		require.Zero(t, countSplitMarks(t, "echo $#\n", "param-fields"))
		require.Zero(t, countSplitMarks(t, "echo ${#foo}\n", "param-fields"))
		// $@/$* expand to a positional vector, not a single split word.
		require.Zero(t, countSplitMarks(t, "exec $@\n", "param-fields"))
	})

	t.Run("assignments are not split", func(t *testing.T) {
		require.Zero(t, countSplitMarks(t, "x=$foo\n", "param-fields"))
		require.Zero(t, countSplitMarks(t, "PATH=$foo:$PATH bar\n", "param-fields"))
	})

	t.Run("unquoted command substitution", func(t *testing.T) {
		require.Equal(t, 1, countSplitMarks(t, "exec $(foo)\n", "cmd-fields"))
		require.Equal(t, 1, countSplitMarks(t, "exec `foo`\n", "cmd-fields"))
		require.Equal(t, 1, countSplitMarks(t, "exec bar$(foo)bar\n", "cmd-fields"))
		require.Equal(t, 1, countSplitMarks(t, "exec --opt=$(foo)\n", "cmd-fields"))
	})

	t.Run("quoted command substitution", func(t *testing.T) {
		require.Zero(t, countSplitMarks(t, "exec \"$(foo)\"\n", "cmd-fields"))
		require.Zero(t, countSplitMarks(t, "exec '$(foo)'\n", "cmd-fields"))
	})

	t.Run("arithmetic is not split", func(t *testing.T) {
		require.Zero(t, countSplitMarks(t, "echo $((2 + 2))\n", "cmd-fields"))
		require.Zero(t, countSplitMarks(t, "local elapsed=$(( $(date +%s) - start_time ))\n", "cmd-fields"))
	})
}
