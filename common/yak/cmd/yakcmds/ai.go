package yakcmds

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/chatglm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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
				t = "chatglm"
				apiKey := consts.GetThirdPartyApplicationConfig("chatglm").APIKey
				verbose := apiKey
				if len(verbose) > 10 {
					verbose = verbose[:10] + "..."
					log.Infof("API Key: %s", verbose)
				}
				result, err := chatglm.NewGLMMessage("你谁？").Invoke(apiKey)
				if err != nil {
					return err
				}
				spew.Dump(result)
			default:
				return utils.Error("unsupported type: " + c.String("type"))
			}
			_ = t
			return nil
		},
	},
}
