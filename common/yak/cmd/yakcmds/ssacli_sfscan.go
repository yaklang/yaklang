package yakcmds

import (
	"context"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var SSACompilerSyntaxFlowCommand = &cli.Command{
	Name: "sfscan",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "program,p",
			Usage: "program name for ssa compiler in db",
		},
	},
	Action: func(c *cli.Context) error {
		program := c.String("program")
		if program == "" {
			return utils.Error("program name is required")
		}

		var opt []ssaapi.Option
		prog, err := ssaapi.FromDatabase(program, opt...)
		if err != nil {
			return err
		}

		sfdb.YieldSyntaxFlowRulesWithoutLib(context.Background())

		return nil
	},
}
