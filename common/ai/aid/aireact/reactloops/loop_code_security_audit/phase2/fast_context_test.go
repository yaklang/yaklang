package phase2

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatFastContextCandidatePaths(t *testing.T) {
	require.Contains(t, formatFastContextCandidatePaths(nil), "无候选")
	require.Contains(t, formatFastContextCandidatePaths([]string{}), "无候选")

	out := formatFastContextCandidatePaths([]string{"/a.go", "/b.go"})
	require.Contains(t, out, "/a.go")
	require.Contains(t, out, "/b.go")

	many := make([]string, 50)
	for i := range many {
		many[i] = "/file.go"
	}
	out = formatFastContextCandidatePaths(many)
	require.Contains(t, out, "另有 10 个")
	require.Equal(t, 40, strings.Count(out, "/file.go"))
}
