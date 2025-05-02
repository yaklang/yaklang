package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/aibalance"
	"github.com/yaklang/yaklang/common/consts"
)

var (
	sigExitOnce = new(sync.Once)
)

func init() {
	go sigExitOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		defer signal.Stop(c)

		for {
			select {
			case <-c:
				fmt.Printf("Exiting due to signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
				return
			}
		}
	})
}

func main() {
	consts.InitializeYakitDatabase("", "")

	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Path to configuration file",
			Value: "config.yaml",
		},
		cli.StringFlag{
			Name:  "listen, l",
			Usage: "Address to listen on",
			Value: "127.0.0.1:8223",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		configPath := c.String("config")
		listenAddr := c.String("listen")

		b, err := aibalance.NewBalancer(configPath)
		if err != nil {
			return err
		}
		return b.RunWithAddr(listenAddr)
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("Command execution failed: [%v] error: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
