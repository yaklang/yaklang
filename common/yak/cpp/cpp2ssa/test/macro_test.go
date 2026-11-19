package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/c2ssa/preprocess"
	"github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
)

func readMacroFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := codeFS.ReadFile("code/macros/" + name)
	require.NoError(t, err)
	return string(raw)
}

// TestCppPreprocessMacroFeatures pins the preprocessing features that real test
// frameworks rely on: token pasting, stringification, variadic arguments,
// multi-line calls and bodies, conditional defines, and a body line comment.
func TestCppPreprocessMacroFeatures(t *testing.T) {
	header := readMacroFixture(t, "framework.hpp")
	entry := readMacroFixture(t, "macros.cpp")

	fs := filesys.NewVirtualFs()
	fs.AddFile("framework.hpp", header)
	fs.AddFile("macros.cpp", entry)

	project := preprocess.BuildProject(fs, cpp2ssa.DefaultPreprocessConfig())
	defer project.Close()
	out, err := project.PreprocessTU("macros.cpp", entry)
	require.NoError(t, err)

	require.NotContains(t, out, "CF_CHECK", "CF_CHECK must be expanded")
	require.NotContains(t, out, "// NOLINT", "macro body comments must be removed")
	require.Contains(t, out, "while (false)", "do/while body must survive")
	require.Contains(t, out, `cf_check("compute(\"abc\") == 3"`, "# must escape quotes")
	require.Contains(t, out, `"1 + 2_tail"`, "# plus phase-6 concatenation")
	require.Contains(t, out, `"hello"_mark`, "#x##suffix must paste")
	require.Regexp(t, `\(\(1\)\s*\+\s*\(2\)\)`, out, "multi-line invocation must expand")
	require.Contains(t, out, `"default"`, "#elif/#else must select the else branch")

	_, parseErr := cpp2ssa.Frontend(out)
	require.NoError(t, parseErr)
}

// TestCppMacroFixtureCompiles runs the same fixture through the frontend and the
// SSA builder, so the expanded text is not only parseable but lowerable.
func TestCppMacroFixtureCompiles(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"framework.hpp": readMacroFixture(t, "framework.hpp"),
		"macros.cpp":    readMacroFixture(t, "macros.cpp"),
	})
	prog := progs[0]
	for _, ref := range []string{"macro_features", "test_compute", "foobar", "checks"} {
		require.NotEmpty(t, prog.Ref(ref), "missing SSA symbol %q", ref)
	}
}
