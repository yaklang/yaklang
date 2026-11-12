package yakcmds

import (
	"testing"

	"github.com/stretchr/testify/require"
	memfitcli "github.com/yaklang/yaklang/common/yak/cmd/yakcmds/memfit-cli"
)

func TestMemfitSubpackageCommandsAreRegistered(t *testing.T) {
	require.Same(t, memfitcli.Command, MemfitCommand)
	require.Same(t, memfitcli.CodingCommand, MemfitCodingCommand)
	require.Same(t, memfitcli.WorkerCommand, MemfitWorkerCommand)
	require.True(t, MemfitWorkerCommand.Hidden)

	foundGeneral := false
	foundCoding := false
	for _, command := range AICommands {
		if command == MemfitCommand {
			foundGeneral = true
		}
		if command == MemfitCodingCommand {
			foundCoding = true
		}
	}
	require.True(t, foundGeneral, "memfit must remain in the AI command group")
	require.True(t, foundCoding, "memfit-code must be in the AI command group")
}
