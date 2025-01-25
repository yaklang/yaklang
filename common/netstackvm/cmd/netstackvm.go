package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/netstackvm"
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
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "",
		},
	}

	app.Action = func(c *cli.Context) error {
		vm, err := netstackvm.NewNetStackVirtualMachine(
			netstackvm.WithPcapDevice("en0"),
			netstackvm.WithPcapPromisc(true),
		)
		if err != nil {
			return err
		}
		vm.StartDHCP(nil)
		vm.Wait()
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
