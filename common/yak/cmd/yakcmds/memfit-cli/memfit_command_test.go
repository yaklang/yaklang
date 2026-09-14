package memfitcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/yaklang/yaklang/common/urfavecli"
)

func TestMemfitCommandContract(t *testing.T) {
	require.Equal(t, "memfit", Command.Name)
	require.Equal(t, "memfit-worker", WorkerCommand.Name)
	require.True(t, WorkerCommand.Hidden)

	flags := make(map[string]cli.Flag)
	for _, flag := range Command.Flags {
		flags[strings.Split(flag.GetName(), ",")[0]] = flag
	}
	require.Contains(t, flags, "ai-type")
	require.Contains(t, flags, "api-key")
	require.Contains(t, flags, "model")
	require.Contains(t, flags, "base-url")
	require.Contains(t, flags, "print")
	require.Contains(t, flags, "plain")

	review, ok := flags["review"].(cli.StringFlag)
	require.True(t, ok)
	require.Equal(t, "yolo", review.Value)
}
