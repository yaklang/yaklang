package main

import (
	"os"

	"github.com/yaklang/yaklang/common/mcp"
	cli "github.com/yaklang/yaklang/common/urfavecli"
)

func main() {
	mcpCommand := mcp.MCPCommand
	app := &cli.App{
		Name:     mcpCommand.Name,
		HelpName: mcpCommand.Name,
		Usage:    mcpCommand.Usage,
		Writer:   os.Stdout,
		Flags:    mcpCommand.Flags,
		Action:   mcpCommand.Action,
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
