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
	require.NoError(t, ApplySSA(nil, []string{"add*"}))
	require.NoError(t, ApplyLLVM(llvm.Module{}, []string{"*"}))

	err := ApplySSA(nil, []string{"missing*"})
	require.Error(t, err)
}
