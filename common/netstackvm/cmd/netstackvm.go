package main

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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
			netstackvm.WithMainNICLinkAddress(`f0:2f:4b:11:11:11`),
			netstackvm.WithPcapPromisc(true),
			//netstackvm.WithOverrideLinkLayerSrcHardware(`f0:2f:4b:09:df:59`),
		)
		if err != nil {
			return err
		}
		if err := vm.StartDHCP(); err != nil {
			log.Warnf("Start DHCP failed: %v", err)
		}
		log.Infof("start to wait dhcp finished")
		if err := vm.WaitDHCPFinished(context.Background()); err != nil {
			log.Errorf("Wait DHCP finished failed: %v", err)
			return utils.Errorf("Wait DHCP finished failed: %v", err)
		}
		ipAddr := "23.192.228.150"
		log.Info("开始循环连接测试:" + ipAddr)
		log.Infof("bpf: %v", `(eth.addr != cc:e0:da:26:66:f2 && arp) || dhcp || ip.addr == 23.192.228.150`)
		var totalTime time.Duration
		count := 0
		for {
			now := time.Now()
			conn, err := vm.DialTCP(10*time.Second, ipAddr+":80")
			if err != nil {
				log.Errorf("连接 %v 失败: %v", ipAddr, err)
				continue
			}
			_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n"))
			if err != nil {
				log.Errorf("请求发送失败: %v", err)
				conn.Close()
				continue
			}
			results := utils.StableReaderEx(conn, 3*time.Second, 1024)
			elapsed := time.Since(now)
			totalTime += elapsed
			count++

			log.Infof("本次请求耗时: %v, 响应长度: %v", elapsed, len(results))
			log.Infof("平均耗时: %v", totalTime/time.Duration(count))
			conn.Close()
			time.Sleep(500 * time.Millisecond)
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
