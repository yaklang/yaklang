package yakcmds

import memfitcli "github.com/yaklang/yaklang/common/yak/cmd/yakcmds/memfit-cli"

// Keep registration in yakcmds while the complete Memfit command, isolated
// worker, local client, protocol, and TUI live in their own package.
var (
	MemfitCommand       = memfitcli.Command
	MemfitWorkerCommand = memfitcli.WorkerCommand
)

func init() {
	AICommands = append(AICommands, MemfitCommand)
}
