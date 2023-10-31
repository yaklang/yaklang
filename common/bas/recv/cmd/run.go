// Package cmd
// @Author bcy2007  2023/9/18 11:00
package main

import (
	"github.com/yaklang/yaklang/common/bas/core"
	"github.com/yaklang/yaklang/common/bas/recv"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "Packet Receiver"
	app.Version = "v0.2"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "ip,i",
			Usage: "message send target",
		},
	}
	app.Action = func(c *cli.Context) error {
		ipaddress := c.String("ip")
		return receiving(ipaddress)
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("packet receiver running error: %s", err)
		return
	}
}

func receiving(ipaddress string) error {
	var iface string
	var err error
	var localAddress string
	system := runtime.GOOS
	if system == "darwin" {
		iface, err = core.GetInterfaceInDarwin()
	} else if system == "linux" {
		iface, err = core.GetInterfaceInLinux()
	} else if system == "windows" {
		iface, localAddress, err = core.GetInterfaceInWindows()
	} else {
		return utils.Errorf("system %v not supported", runtime.GOOS)
	}
	if err != nil {
		return utils.Errorf("get interface info error: %v", err)
	}
	if iface == "" {
		return utils.Error("no interface info get")
	}
	receiver := recv.CreateReceiver(
		iface,
		ipaddress,
		localAddress,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh)
	go func() {
		for {
			s := <-sigCh
			if s == syscall.SIGTERM || s == syscall.SIGINT {
				receiver.Cancel()
				time.Sleep(time.Second)
				os.Exit(0)
			}
		}
	}()

	go func() {
		err := receiver.ReceivePacket()
		if err != nil {
			log.Errorf("receive packet error: %v", err)
			time.Sleep(time.Second)
			os.Exit(0)
		}
	}()
	receiver.SendMessage()

	return nil
}
