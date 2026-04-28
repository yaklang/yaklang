package yakcmds

import (
	"strings"

	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
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

func init() {
	AICommands = append(AICommands, LocalModelCommands...)
	AICommands = append(AICommands, TieredAIConfigCommands...)
	AICommands = append(AICommands, AIFocusCommand)
}
