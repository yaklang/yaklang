package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	tun "github.com/yaklang/yaklang/common/lowtun"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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
		// 创建缓冲区用于读取数据
		// WireGuard 默认 MTU 是 1420，我们使用这个大小作为缓冲区
		buf := make([][]byte, 1)
		buf[0] = make([]byte, 1420)
		sizes := make([]int, 1)

		// 持续读取数据
		for {
			// 从 TUN 设备读取数据包
			// offset 通常设置为 0
			n, err := tdev.Read(buf, sizes, 16)
			if err != nil {
				log.Errorf("Error reading from TUN: %v", err)
				continue
			}

			if n > 0 {
				// 获取实际收到的数据
				packet := buf[0][:sizes[0]]
				if len(packet) > 16 {
					packet = packet[16:]
				}
				// 解析 IP 包头
				version := packet[0] >> 4

				// 根据 IP 版本处理数据
				switch version {
				case 4:
					log.Infof("IPv4 packet")
					spew.Dump(packet)
				case 6:
					log.Infof("IPv6 packet")
					spew.Dump(packet)
				default:
					log.Warnf("Unknown IP version: %d", version)
				}
			}
		}

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
