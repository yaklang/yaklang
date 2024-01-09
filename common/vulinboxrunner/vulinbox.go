package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
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

	// aes rsa - http://116.214.131.28/wui/index.html#/?logintype=1&_key=g2jsh9

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port,p",
			Value: 8080,
		},
		cli.BoolFlag{
			Name: "safe",
		},
		cli.BoolFlag{
			Name: "nohttps",
		},
		cli.StringFlag{
			Name:  "host,t",
			Value: `127.0.0.1`,
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		servers, err := vulinbox.NewVulinServerEx(context.Background(), c.Bool("nohttps"), c.Bool("safe"), c.String("host"), c.Int("port"))
		if err != nil {
			log.Errorf("new vulinbox server failed: %v", err)
			return err
		}
		ifs, _ := net.Interfaces()
		for _, i := range ifs {
			addrs, _ := i.Addrs()
			for _, addr := range addrs {
				ip := addr.String()
				ip, _, _ = strings.Cut(ip, "/")
				if !utils.IsIPv4(ip) {
					continue
				}
				log.Infof("checking on: %v:%v", ip, c.Int("port"))
			}
		}
		log.Infof("VULINBOX RUNNING IN: %s", servers)
		for {
			time.Sleep(time.Second)
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
