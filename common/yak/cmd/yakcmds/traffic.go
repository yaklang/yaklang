package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

var TrafficUtilCommands = []cli.Command{
	// chaosmaker
	{
		Name: "import-chaosmaker-json",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "file,f"},
		},
		Action: func(c *cli.Context) error {
			file := utils.GetFirstExistedFile(c.String("file"))
			if file == "" {
				return utils.Errorf("file not found: %v", c.String("file"))
			}

			return rule.ImportRulesFromFile(consts.GetGormProfileDatabase(), file)
		},
	},
	{
		Name: "export-chaosmaker-json",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "file,f"},
		},
		Action: func(c *cli.Context) error {
			return rule.ExportRulesToFile(consts.GetGormProfileDatabase(), c.String("file"))
		},
	},
	ChaosMakerCommand,
	SuricataLoaderCommand,
	PcapCommand,
}

func init() {
	for _, i := range TrafficUtilCommands {
		i.Category = "Traffic Processing Utils"
	}
}
