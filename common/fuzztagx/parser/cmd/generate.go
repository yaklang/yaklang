package main

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"os"
)

func main() {
	appCreate()
}

func appCreate() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "f",
			Usage: "grammar rule file",
		},
		cli.StringFlag{
			Name:  "o",
			Usage: "save path",
		},
	}
	app.Action = func(c *cli.Context) error {
		ruleFile := c.String("f")
		savePath := c.String("o")
		return GenerateGolangCode(ruleFile, savePath)
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf(`generate code error: %s`, err)
		return
	}
}
