package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket/pcap"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"os"
	"os/signal"
	"yaklang/common/utils"
	"yaklang/common/utils/netutil"
	"runtime"
	"strconv"
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

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "target",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		// not implemented
		iface, gateway, srcIP, err := netutil.Route(10*time.Second, c.String("target"))
		if err != nil {
			return err
		}

		println("..........................................")
		println("..........................................")
		switch runtime.GOOS {
		case "windows":
			devs, err := pcap.FindAllDevs()
			if err != nil {
				return utils.Errorf("pcap find dev failed: %s", err)
			}
			spew.Dump(devs)
		}

		println("..........................................")
		_, _ = gateway, srcIP
		ifaceName, err := utils.IfaceNameToPcapIfaceName(iface.Name)
		if err != nil {
			return err
		}
		handler, err := pcap.OpenLive(ifaceName, 65535, false, pcap.BlockForever)
		if err != nil {
			return errors.Errorf("open device[%v-%v] failed: %s", iface.Name, strconv.QuoteToASCII(iface.Name), err)
		}
		_ = handler
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
