package yakcmds

import (
	"strings"

	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var ProjectCommands = []*cli.Command{
	{
		Name:  "profile-export",
		Usage: "Export Yakit Profile Database to File",
		Action: func(c *cli.Context) {
			f := c.String("output")
			if utils.GetFirstExistedPath(f) != "" {
				log.Errorf("path[%s] is existed", f)
				return
			}

			if c.String("type") == "" {
				log.Error("export type cannot be emtpy")
				return
			}
			switch ret := strings.ToLower(c.String("type")); ret {
			case "plugin", "plugins":
				err := yakit.ExportYakScript(consts.GetGormProfileDatabase(), f)
				if err != nil {
					log.Errorf("output failed: %s", err)
				}
			default:
				log.Error("unsupported resource type: " + ret)
				return
			}
		}, Flags: []cli.Flag{
			cli.StringFlag{Name: "output"},
			cli.StringFlag{Name: "type"},
		}},
}
