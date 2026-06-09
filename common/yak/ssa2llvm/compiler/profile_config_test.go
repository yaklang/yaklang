package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareCompileConfigDefaultsStdlibCompile(t *testing.T) {
	cfg := &CompileConfig{}

	require.NoError(t, prepareCompileConfig(cfg))
	require.True(t, cfg.StdlibCompile)
	require.False(t, cfg.StdlibCompileSet)
}

func TestPrepareCompileConfigPreservesExplicitStdlibCompileFalse(t *testing.T) {
	cfg := &CompileConfig{}
	WithCompileStdlibCompile(false)(cfg)

	require.NoError(t, prepareCompileConfig(cfg))
	require.False(t, cfg.StdlibCompile)
	require.True(t, cfg.StdlibCompileSet)
}
