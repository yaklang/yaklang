package main

import (
	"github.com/urfave/cli"
	"os"
	"yaklang/common/log"
	"yaklang/common/simulator/cmd/sub"
)

func main() {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		sub.Example,
		sub.TestModule,
		sub.Simple,
	}
	app.Flags = []cli.Flag{}
	app.Action = func(c *cli.Context) error {
		return nil
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("simulator cmd running error: %s", err)
		return
	}
}
