package yakcmds

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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
		cli.BoolFlag{
			Name:  "code,show-code",
			Usage: "show code",
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

		var results []*ssaapi.SyntaxFlowResult

		for rule := range sfdb.YieldSyntaxFlowRulesWithoutLib(context.Background()) {
			rule := rule
			ScanWithSFRule(prog, rule, func(result *ssaapi.SyntaxFlowResult) {
				if ret := result.GetAlertValues(); ret.Len() > 0 {
					results = append(results, result)
				}
			})
		}

		for _, result := range results {
			fmt.Println("-----------------------------------------")
			fmt.Println(result.Dump(c.Bool("code")))
		}

		return nil
	},
}

func ScanWithSFRule(prog *ssaapi.Program, i *schema.SyntaxFlowRule, callback func(result *ssaapi.SyntaxFlowResult)) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("execute: %v failed(recover): %s", i.Title, err)
		}
	}()
	result, err := prog.SyntaxFlowRule(i)
	if err != nil {
		log.Debugf("execute: %v failed: %s", i.Title, err)
		return
	}
	callback(result)
}
