package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

var AICommands = []*cli.Command{
	{
		Name: "ai",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "type",
				Value: "chatglm",
			},
		},
		Action: func(c *cli.Context) error {
			var t string
			switch strings.ToLower(c.String("type")) {
			case "openai":
				t = "openai"
			case "chatglm":
			default:
				return utils.Error("unsupported type: " + c.String("type"))
			}
			_ = t
			return nil
		},
	},
}
