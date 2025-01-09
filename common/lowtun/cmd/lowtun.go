package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	tun "github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
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

type RWTun interface {
	Read([][]byte, []int, int) (int, error)
	Write([][]byte, int) (int, error)
}

func TUNCopy(prompt string, w, r RWTun, mtu int, offset int) {
	buf := make([][]byte, 1)
	buf[0] = make([]byte, mtu)
	sizes := make([]int, 1)

	for {
		n, err := r.Read(buf, sizes, offset)
		if err != nil {
			log.Errorf("Error reading from TUN: %v", err)
			continue
		}

		if n > 0 {
			packet := buf[0][:sizes[0]]
			if len(packet) > 16 {
				packet = packet[16:]
			}
			version := packet[0] >> 4

			switch version {
			case 4:
				log.Infof("%v: IPv4 packet", prompt)
				spew.Dump(packet)
			case 6:
				log.Infof("%v: IPv6 packet", prompt)
				spew.Dump(packet)
			default:
				log.Warnf("Unknown IP version: %d", version)
			}
			newBuf := make([][]byte, 1)
			newBuf[0] = packet
			w.Write(buf, 16)
		}
	}
}

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		log.Info("start to create net tun in gvisor")
		sdev, sdial, err := netstack.CreateNetTUN([]netip.Addr{
			netip.MustParseAddr("10.252.252.252"),
		}, []netip.Addr{
			netip.MustParseAddr("8.8.8.8"),
		}, 1420)
		if err != nil {
			return err
		}
		_ = sdial

		log.Info("start to create utun113")
		// ifconfig utun113 10.1.1.1 10.2.2.2 up && route add -host 8.8.8.8/32 10.1.1.1 && curl https://8.8.8.8
		tdev, err := tun.CreateTUN("utun113", 1420)
		if err != nil {
			return err
		}
		defer tdev.Close()
		name, err := tdev.Name()
		if err != nil {
			return err
		}
		log.Infof("tun device name: %v", name)

		wg := new(sync.WaitGroup)
		wg.Add(2)

		st := sdial.Stack()
		st.SetTransportProtocolHandler(tcp.ProtocolNumber, func(id stack.TransportEndpointID, buffer *stack.PacketBuffer) bool {
			spew.Dump(buffer, id)
			return false
		})

		go func() {
			defer func() {
				wg.Done()
				if err := recover(); err != nil {
					log.Errorf("panic: %v", err)
				}
			}()
			TUNCopy("tun -> gvisor", sdev, tdev, 1420, 16)
		}()

		go func() {
			defer func() {
				wg.Done()
				if err := recover(); err != nil {
					log.Errorf("panic: %v", err)
				}
			}()
			TUNCopy("gvisor -> tun", tdev, sdev, 1420, 16)
		}()
		wg.Wait()
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
