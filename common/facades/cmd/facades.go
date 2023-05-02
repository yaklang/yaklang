package main

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"yaklang/common/facades"
	"yaklang/common/log"
	"yaklang/common/utils"
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
			Name: "host", Value: "127.0.0.1",
		},
		cli.IntFlag{Name: "port", Value: 4434},
	}

	app.Commands = []cli.Command{
		{
			Name: "dns-server",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "domain",
				},
				cli.StringFlag{
					Name: "ip-addr",
				},
				cli.StringFlag{
					Name:  "listen-addr",
					Value: "0.0.0.0",
				},
				cli.IntFlag{
					Name:  "dns-port",
					Value: 53,
				},
			},
			Action: func(c *cli.Context) error {
				domain := c.String("domain")
				if domain == "" {
					return utils.Errorf("empty domain...")
				}

				ipAddr := c.String("ip-addr")
				if ipAddr == "" {
					ipAddr = "127.0.0.1"
				}
				s, err := facades.NewDNSServer(
					domain, ipAddr,
					c.String("listen-addr"),
					c.Int("dns-port"),
				)
				if err != nil {
					return err
				}
				return s.Serve(context.Background())
			},
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		server := facades.NewFacadeServer(c.String("host"), c.Int("port"))
		server.OnHandle(func(n *facades.Notification) {
			log.Info(n.String())
		})
		return server.ServeWithContext(context.Background())
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
