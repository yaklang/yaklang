package main

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/crep"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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
				fmt.Printf("exit by signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		// not implemented
		return crep.NewWebsocketController("a", 8881).Run()
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
