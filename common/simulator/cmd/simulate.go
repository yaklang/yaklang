package main

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/simulator/cmd/sub"
	"os"
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
