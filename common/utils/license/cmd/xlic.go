package main

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/license"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
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
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{
		{
			Name: "rsa",
			Action: func(c *cli.Context) error {
				pri, pub, err := tlsutils.GeneratePrivateAndPublicKeyPEM()
				if err != nil {
					return err
				}

				println(string(pri))
				println()
				println(string(pub))
				return nil
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "dec",
			Value: "dec.pri.pem",
		},
		cli.StringFlag{
			Name:  "enc",
			Value: "dec.pub.pem",
		},
		cli.StringFlag{
			Name:  "req",
			Value: "license.request.txt",
		},
		cli.StringFlag{
			Name: "org",
		},
		cli.IntFlag{
			Name: "duration-days,d",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		org := c.String("org")
		if org == "" {
			return utils.Errorf("需要签发给的组织不能为空，请设置 --org [org-name]")
		}

		m, err := license.NewMachineFromFile(c.String("enc"), c.String("dec"))
		if err != nil {
			return err
		}

		raw, err := ioutil.ReadFile(c.String("req"))
		if err != nil {
			return err
		}

		resp, err := m.SignLicense(
			strings.TrimSpace(string(raw)), org,
			time.Duration(c.Int64("duration-days"))*(time.Hour*24),
			nil)
		if err != nil {
			return err
		}

		println("--------------------------------------------")
		println(resp)
		println("--------------------------------------------")

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
