package ssaapi

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCompileUnitSourcePersistenceAcrossBatches(t *testing.T) {
	t.Setenv(compileUnitBatchMinFilesEnv, "1")
	t.Setenv(compileUnitBatchMinBytesEnv, "0")
	t.Setenv(compileUnitBatchMaxFilesEnv, "1")
	for _, test := range []struct {
		language ssaconfig.Language
		files    map[string]string
		query    string
	}{
		{ssaconfig.GO, map[string]string{
			"go.mod":     "module source.local\n",
			"first/a.go": "package first\nfunc Run() { println(\"source-first\") }",
			"last/b.go":  "package last\nfunc Run() { println(\"source-last\") }",
		}, `println(* as $source) as $call`},
		{ssaconfig.PHP, map[string]string{
			"first/a.php": "<?php function first() { echo 'source-first'; }",
			"last/b.php":  "<?php function last() { echo 'source-last'; }",
		}, `echo(* as $source) as $call`},
		{ssaconfig.PYTHON, map[string]string{
			"first/a.py": "def first():\n    print('source-first')\n",
			"last/b.py":  "def last():\n    print('source-last')\n",
		}, `print(* as $source) as $call`},
	} {
		t.Run(string(test.language), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			for path, code := range test.files {
				vf.AddFile(path, code)
			}
			name := uuid.NewString()
			t.Cleanup(func() {
				ProgramCache.Remove(name)
				ssadb.DeleteProgram(ssadb.GetDB(), name)
			})
			programs, err := ParseProjectWithFS(vf, WithLanguage(test.language), WithProgramName(name),
				ssaconfig.WithCompileConcurrency(4), ssaconfig.WithCompileIrCacheTTL(time.Millisecond),
				ssaconfig.WithCompileIrCacheMax(1))
			require.NoError(t, err)
			require.Len(t, programs, 1)
			ProgramCache.Remove(name)
			loaded, err := FromDatabase(name)
			require.NoError(t, err)
			result, err := loaded.SyntaxFlowWithError(test.query)
			require.NoError(t, err)
			require.Len(t, result.GetValues("call"), 2)
			require.Contains(t, result.GetValues("source").String(), "source-first")
			require.Contains(t, result.GetValues("source").String(), "source-last")
			for _, call := range result.GetValues("call") {
				require.NotNil(t, call.GetRange(), "source ranges must survive asynchronous batch persistence")
				require.NotNil(t, call.GetRange().GetEditor())
				require.Contains(t, call.GetRange().GetEditor().GetSourceCode(), "source-")
			}
		})
	}
}
