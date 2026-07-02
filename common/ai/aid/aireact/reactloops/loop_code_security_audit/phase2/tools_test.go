package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhase2WhitelistFSTools(t *testing.T) {
	require.Equal(t, []string{"grep", "read_file", "find_file"}, phase2WhitelistFSTools)
}
