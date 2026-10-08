package yakcmds

import sharkcli "github.com/yaklang/yaklang/common/yak/cmd/yakcmds/shark-cli"

var SharkCommand = sharkcli.Command
var SharkWorkerCommand = sharkcli.WorkerCommand

func init() {
	TrafficUtilCommands = append(TrafficUtilCommands, SharkCommand)
}
