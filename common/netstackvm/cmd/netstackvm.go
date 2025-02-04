package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"

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
			Name:  "iface",
			Usage: "指定物理网卡名称",
		},
		cli.StringFlag{
			Name:  "vmac",
			Usage: "指定虚拟机MAC地址",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "synscan",
			Usage: "synscan <ip>",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "iface",
					Usage: "指定物理网卡名称",
				},
				cli.StringFlag{
					Name:  "ports,p",
					Usage: "specify target ports to scan",
					Value: "22,80",
				},
			},
			Action: func(c *cli.Context) error {
				ifaceName := c.String("iface")
				if ifaceName == "" {
					route, gateway, srcIP, err := netutil.GetPublicRoute()
					if err != nil {
						return err
					}
					ifaceName = route.Name
					_ = gateway
					_ = srcIP
				}
				vm, err := netstackvm.NewNetStackVirtualMachine(
					netstackvm.WithPcapDevice(ifaceName),
				)
				if err != nil {
					return err
				}

				err = vm.InheritPcapInterfaceIP()
				if err != nil {
					return err
				}

				swg := utils.NewSizedWaitGroup(2000)

				ports := utils.ParseStringToPorts(c.String("ports"))
				if len(ports) == 0 {
					ports = []int{80, 22, 443, 3306, 3389}
				}
				log.Infof("start to scan ports: %v", ports)
				hostsRaw := strings.Join(c.Args(), ",")
				log.Infof("start to scan hosts: %v", hostsRaw)
				hosts := utils.ParseStringToHosts(hostsRaw)
				for _, host := range hosts {
					for _, port := range ports {
						host := host
						port := port
						swg.Add()
						go func() {
							defer swg.Done()
							addr := utils.HostPort(host, port)
							conn, err := vm.DialTCP(5*time.Second, addr)
							if err != nil {
								// log.Infof("CLOSE: %v, REASON: %v", addr, err)
								return
							}
							log.Infof("OPEN: %v", addr)
							conn.Close()
						}()
					}
				}
				swg.Wait()
				return nil
			},
		},
	}

	app.Action = func(c *cli.Context) error {
		ifaceName := c.String("iface")
		if c.String("iface") == "" {
			route, gateway, srcIP, err := netutil.GetPublicRoute()
			if err != nil {
				return err
			}
			_ = gateway
			_ = srcIP
			ifaceName = route.Name
		}

		if ifaceName == "" {
			return utils.Errorf("no network interface specified")
		}

		vmac := c.String("vmac")
		if vmac == "" {
			vmac = fmt.Sprintf("f0:2f:4b:ff:%02x:%02x", rand.Intn(255), rand.Intn(255))
			log.Info("no vmac specified, use random mac")
		}

		vm, err := netstackvm.NewNetStackVirtualMachine(
			netstackvm.WithPcapDevice(ifaceName),
			netstackvm.WithMainNICLinkAddress(vmac),
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
