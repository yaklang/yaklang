package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/pcapx"
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
		log.Info("lowtun start")
		ifc, err := lowtun.New()
		if err != nil {
			return err
		}
		defer ifc.Close()

		log.Infof("lowtun start success: %v", ifc.Name())

		var buf = make([]byte, 1500)
		for {
			n, err := ifc.Read(buf)
			if err != nil {
				return err
			}

			spew.Dump(buf[:n])

			_, network, transport, payload, err := pcapx.ParseAuto(buf[:n])
			if err != nil {
				log.Error(err)
				// clean buffer
				for i := 0; i < n; i++ {
					buf[i] = 0
				}
				continue
			}

			// copy network to modify source

			// pcapx.InjectRaw()

			_ = network
			_ = transport
			_ = payload
			fmt.Println(network)
			// handle buf[:n] to next hop, iface is en0
			// send to en0
			ifc.Write(buf[:n])
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
