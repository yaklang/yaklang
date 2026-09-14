package yakcmds

import (
	"testing"

	"github.com/stretchr/testify/require"
	memfitcli "github.com/yaklang/yaklang/common/yak/cmd/yakcmds/memfit-cli"
)

func TestMemfitSubpackageCommandsAreRegistered(t *testing.T) {
	require.Same(t, memfitcli.Command, MemfitCommand)
	require.Same(t, memfitcli.WorkerCommand, MemfitWorkerCommand)
	require.True(t, MemfitWorkerCommand.Hidden)

	found := false
	for _, command := range AICommands {
		if command == MemfitCommand {
			found = true
			break
		}
	}
	require.True(t, found, "memfit must remain in the AI command group")
}
