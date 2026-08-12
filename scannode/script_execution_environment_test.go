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
