package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

var ScanCommands = []*cli.Command{
	{
		Name:  "update-nuclei-database",
		Usage: "Load Nuclei-Template into Local Yak Plugin Database",
		Action: func(c *cli.Context) error {
			var err error
			err = yak.NewScriptEngine(1).ExecuteMain(`
loglevel("info")
log.info("start to load local database"); 
die(nuclei.UpdateDatabase())`, "main")
			if err != nil {
				log.Errorf("execute nuclei.UpdateDatabase() failed: %s", err)
				return err
			}
			return nil
		},
	},
	{
		Name: "remove-nuclei-database", Usage: "Remove Nuclei-Template from Local Yak Plugin Database",
		Action: func(c *cli.Context) error {
			err := tools.RemovePoCDatabase()
			if err != nil {
				log.Errorf("remove pocs failed: %s", err)
			}
			return nil
		},
	},
	&synscanCommand,
	&servicescanCommand,
}
