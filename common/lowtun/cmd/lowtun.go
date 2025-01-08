package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kataras/golog"
	"github.com/yaklang/yaklang/common/lowtun/conn"
	"github.com/yaklang/yaklang/common/lowtun/device"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
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

	app.Commands = []cli.Command{
		{
			Name:    "transparent-route",
			Aliases: []string{"tr"},
			Action: func(c *cli.Context) error {
				return nil
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		log.SetLevel(golog.DebugLevel)
		log.Info("lowtun start")
		tun, tnet, err := netstack.CreateNetTUN(
			[]netip.Addr{netip.MustParseAddr("192.168.4.29")},
			[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
			1420)
		if err != nil {
			return err
		}
		dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelVerbose, ""))
		err = dev.IpcSet(`private_key=087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
public_key=c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28
allowed_ip=0.0.0.0/0
endpoint=127.0.0.1:58120
`)

		_ = tun

		socket, err := tnet.Dial("ping4", "zx2c4.com")
		if err != nil {
			return err
		}
		requestPing := icmp.Echo{
			Seq:  rand.Intn(1 << 16),
			Data: []byte("gopher burrow"),
		}
		icmpBytes, _ := (&icmp.Message{Type: ipv4.ICMPTypeEcho, Code: 0, Body: &requestPing}).Marshal(nil)
		socket.SetReadDeadline(time.Now().Add(time.Second * 10))
		start := time.Now()
		_, err = socket.Write(icmpBytes)
		if err != nil {
			return err
		}
		n, err := socket.Read(icmpBytes[:])
		if err != nil {
			return err
		}
		replyPacket, err := icmp.ParseMessage(1, icmpBytes[:n])
		if err != nil {
			return err
		}
		replyPing, ok := replyPacket.Body.(*icmp.Echo)
		if !ok {
			// log.Panicf("invalid reply type: %v", replyPacket)
			return utils.Errorf("invalid reply type: %v", replyPacket)
		}
		if !bytes.Equal(replyPing.Data, requestPing.Data) || replyPing.Seq != requestPing.Seq {
			// log.Panicf("invalid ping reply: %v", replyPing)
			return utils.Errorf("invalid ping reply: %v", replyPing)
		}
		log.Printf("Ping latency: %v", time.Since(start))
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
