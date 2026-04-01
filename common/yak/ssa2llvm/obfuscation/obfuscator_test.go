package obfuscation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/go-llvm"
)

func TestNormalizeNames(t *testing.T) {
	names := NormalizeNames([]string{"addsub, custom", "  LLVM  "})
	require.Equal(t, []string{"addsub", "custom", "llvm"}, names)
}

func TestGlobPatterns(t *testing.T) {
	require.NoError(t, Apply(&Context{Stage: StageSSAPre}, []string{"add*"}))
	require.NoError(t, Apply(&Context{Stage: StageLLVM, LLVM: llvm.Module{}}, []string{"*"}))

	err := Apply(&Context{Stage: StageSSAPre}, []string{"missing*"})
	require.Error(t, err)
}
