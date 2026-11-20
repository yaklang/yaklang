package preprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestEvalPreprocessorCondition_Comparison(t *testing.T) {
	defs := map[string]string{"OPENSSL_CONFIGURED_API": "30400"}
	require.True(t, EvalPreprocessorCondition("OPENSSL_CONFIGURED_API > 0", nil, nil, defs))
	require.True(t, EvalPreprocessorCondition("30400 >= 30000", nil, nil, defs))
	require.False(t, EvalPreprocessorCondition("30400 < 30000", nil, nil, defs))
}

func TestEvalPreprocessorCondition_Defined(t *testing.T) {
	defs := map[string]string{"FOO": "1"}
	require.True(t, EvalPreprocessorCondition("defined(FOO)", nil, nil, defs))
	require.True(t, EvalPreprocessorCondition("defined FOO", nil, nil, defs))
	require.False(t, EvalPreprocessorCondition("defined(BAR)", nil, nil, defs))
}

func TestEvalPreprocessorCondition_Arithmetic(t *testing.T) {
	env := NewMacroEnvironment(nil)
	env.ApplyDefineLine("#define MAJOR 3")
	env.ApplyDefineLine("#define MINOR 4")
	require.Equal(t, "3", env.tables.Object["MAJOR"])
	require.True(t, EvalPreprocessorCondition("MAJOR * 10000 + MINOR * 100 >= 30400", env, nil, nil))
}

func TestCond_IfComparison(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte(`
#if VERSION > 1
int v = 1;
#else
int v = 0;
#endif
`), 0o644))
	cfg := DefaultConfig()
	cfg.Defines["VERSION"] = "2"
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.NoError(t, err)
	require.Contains(t, out, "v = 1")
	require.NotContains(t, out, "v = 0")
}
