package tests

import (
	"embed"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/scala/scala2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed all:code
var codeFS embed.FS

func TestScalaFrontendBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.scala")
	require.NoError(t, err)

	ast, err := scala2ssa.Frontend(string(raw))
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestScalaFrontendVersionSamples(t *testing.T) {
	for _, name := range []string{"code/legacy2.scala", "code/modern3.scala"} {
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile(name)
			require.NoError(t, err)
			ast, err := scala2ssa.Frontend(string(raw))
			require.NoError(t, err)
			require.NotNil(t, ast)
		})
	}
}

func TestScalaProjectCompileBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.scala")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("main.scala", string(raw))

	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.SCALA),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	prog := progs[0]
	prog.Show()
	require.NotEmpty(t, prog.Ref("add"))
	require.NotEmpty(t, prog.Ref("main"))
}

func TestAllSyntaxForScala_G4(t *testing.T) {
	budget := 10 * time.Second
	if raw := os.Getenv("YAK_SCALA_FIXTURE_PARSE_BUDGET_SEC"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		require.NoError(t, err, "invalid YAK_SCALA_FIXTURE_PARSE_BUDGET_SEC")
		require.Positive(t, seconds, "invalid YAK_SCALA_FIXTURE_PARSE_BUDGET_SEC")
		budget = time.Duration(seconds) * time.Second
	}

	entries, err := codeFS.ReadDir("code")
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".scala" {
			continue
		}
		count++
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile("code/" + name)
			require.NoError(t, err)

			start := time.Now()
			ast, parseErr := scala2ssa.Frontend(string(raw))
			elapsed := time.Since(start)
			if parseErr != nil {
				t.Fatalf("parse %s failed after %s: %v", name, elapsed, parseErr)
			}
			require.NotNil(t, ast)
			if elapsed > budget {
				t.Fatalf("parse %s took %s, over budget %s", name, elapsed, budget)
			}
		})
	}
	require.Positive(t, count, "no .scala fixtures found in code/")
}
