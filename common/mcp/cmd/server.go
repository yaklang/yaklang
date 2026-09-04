package main

import (
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/urfavecli"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
)

func main() {
	// This executable only hosts MCP. Re-apply argument-based stdio logging
	// before running the CLI in case it was built under a non-standard name.
	log.ConfigureMCPStdioLogging(append([]string{"mcp"}, os.Args[1:]...))

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
