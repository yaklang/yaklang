package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestCppParseLocalProject parses every .cpp file of a local checkout named by
// CPP_PROJECT_TARGET. It is local-only: the corpus is not committed.
func TestCppParseLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project parse test")
	}
	target := os.Getenv("CPP_PROJECT_TARGET")
	if target == "" {
		t.Skip("set CPP_PROJECT_TARGET to a C++ project directory to run")
	}
	stat, err := os.Stat(target)
	require.NoError(t, err)
	require.True(t, stat.IsDir(), "CPP_PROJECT_TARGET must be a directory")

	progs, err := ssaapi.ParseProjectFromPath(
		target,
		ssaapi.WithLanguage(ssaconfig.CPP),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs, "project parse returned no program")

	functions := 0
	for _, prog := range progs {
		functions += prog.Program.Funcs.Len()
	}
	require.Positive(t, functions, "project parse lowered no function")
}