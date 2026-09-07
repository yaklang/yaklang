package scannode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaceEnvironmentValueRemovesInheritedDuplicates(t *testing.T) {
	env := replaceEnvironmentValue(
		[]string{"KEEP=value", "OWNER=old", "OWNER=older"},
		"OWNER",
		"new",
	)
	require.Equal(t, []string{"KEEP=value", "OWNER=new"}, env)
}

func TestScriptEnvWithDebugLogLevel(t *testing.T) {
	base := []string{"FOO=1", "LOG_LEVEL=info"}
	require.Equal(t, base, scriptEnvWithDebugLogLevel(base, false))
	require.Equal(
		t,
		[]string{"FOO=1", "LOG_LEVEL=debug"},
		scriptEnvWithDebugLogLevel(base, true),
	)
}
