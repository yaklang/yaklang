package yakcmds

import (
	"testing"

	"github.com/stretchr/testify/require"
	sharkcli "github.com/yaklang/yaklang/common/yak/cmd/yakcmds/shark-cli"
)

func TestSharkCommandRegistered(t *testing.T) {
	require.Same(t, sharkcli.Command, SharkCommand)
	require.Same(t, sharkcli.WorkerCommand, SharkWorkerCommand)
	require.True(t, SharkWorkerCommand.Hidden)
	for _, command := range TrafficUtilCommands {
		if command == SharkCommand {
			return
		}
	}
	t.Fatal("shark must be registered in Traffic Utils")
}
