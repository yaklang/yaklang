package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/utils/tcpreverse"
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
				os.Exit(0)
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		re, err := tcpreverse.NewTCPReverse(443)
		if err != nil {
			return err
		}
		re.RegisterSNIForward("example.com", &tcpreverse.TCPReverseTarget{
			Address:  "example.com:443",
			ForceTLS: true,
		})
		re.RegisterSNIForward("baidu.com", &tcpreverse.TCPReverseTarget{
			Address:  "example.com:443",
			ForceTLS: true,
		})
		re.RegisterSNIForward("www.baidu.com", &tcpreverse.TCPReverseTarget{
			Address:  "www.example.com:443",
			ForceTLS: true,
		})
		return re.Run()
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
