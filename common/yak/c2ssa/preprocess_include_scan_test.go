package c2ssa

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectStubIncludePaths(t *testing.T) {
	t.Parallel()

	t.Run("angle_and_quoted_dedup", func(t *testing.T) {
		src := `
#include <stdio.h>
#include "local.h"
#include <stdio.h>
`
		got := collectStubIncludePaths(src)
		require.ElementsMatch(t, []string{"stdio.h", "local.h"}, got)
	})

	t.Run("line_comment_hides_include", func(t *testing.T) {
		src := "int x; // #include <fake.h>\n#include <real.h>\n"
		got := collectStubIncludePaths(src)
		require.Equal(t, []string{"real.h"}, got)
	})

	t.Run("block_comment_multiline", func(t *testing.T) {
		src := "/*\n#include <in_block.h>\n*/\n#include <out.h>\n"
		got := collectStubIncludePaths(src)
		require.Equal(t, []string{"out.h"}, got)
	})

	t.Run("two_includes_one_line", func(t *testing.T) {
		src := "#include <a.h> #include \"b.h\"\n"
		got := collectStubIncludePaths(src)
		require.ElementsMatch(t, []string{"a.h", "b.h"}, got)
	})
}

func TestIsAngleBracketIncludeLineForFilter(t *testing.T) {
	t.Parallel()
	require.True(t, isAngleBracketIncludeLineForFilter("  #include <stdio.h>  "))
	require.True(t, isAngleBracketIncludeLineForFilter("#include <stdio.h> // tail"))
	require.False(t, isAngleBracketIncludeLineForFilter(`#include "x.h"`))
	require.False(t, isAngleBracketIncludeLineForFilter("int x;"))
}

func TestFilterSystemIncludes(t *testing.T) {
	t.Parallel()

	t.Run("drops_angle_keeps_quoted", func(t *testing.T) {
		src := strings.Join([]string{
			"#include <sys/types.h>",
			`#include "project.h"`,
			"int x;",
		}, "\n")
		out := filterSystemIncludes(src)
		require.Contains(t, out, `#include "project.h"`)
		require.NotContains(t, out, "<sys/types.h>")
		require.Contains(t, out, "int x;")
	})

	t.Run("line_comment_then_real_angle_include_dropped", func(t *testing.T) {
		src := "// #include <not_real.h>\n#include <real.h>\n"
		out := filterSystemIncludes(src)
		require.Contains(t, out, "// #include <not_real.h>", "comment line kept verbatim")
		require.NotContains(t, out, "#include <real.h>", "active angle include line removed")
	})

	t.Run("angle_inside_block_comment_not_dropped", func(t *testing.T) {
		src := "/*\n#include <inner.h>\n*/\n"
		out := filterSystemIncludes(src)
		require.Contains(t, out, "inner.h", "include inside comment remains in mirrored text")
	})
}

func TestStripCCommentsFromPhysicalLine(t *testing.T) {
	t.Parallel()
	inBlock := false
	require.Equal(t, "before  after", stripCCommentsFromPhysicalLine("before /*middle*/ after", &inBlock))
	require.False(t, inBlock)

	inBlock = false
	require.Equal(t, "  ", stripCCommentsFromPhysicalLine("  /*", &inBlock))
	require.True(t, inBlock)
	require.Equal(t, " after ", stripCCommentsFromPhysicalLine(" */ after ", &inBlock))
	require.False(t, inBlock)
}
