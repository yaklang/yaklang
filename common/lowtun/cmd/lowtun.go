package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/lowtun/cmd/lowtuncmds"
	"io"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/rwendpoint"
	"github.com/yaklang/yaklang/common/utils/netutil"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/ports"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"

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

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "",
		},
	}

	app.Commands = []cli.Command{
		{
			Name: "nat",
			Action: func(c *cli.Context) error {
				// 理论上需要 nat
				// nat 转换的情况如下：
				//    10.1.1.1 -> 10.2.2.2 -> en0 (192.168.0.134) -> 8.8.8.8
				//
				return nil
			},
		},
		{
			Name: "sdhcp",
			Action: func(c *cli.Context) error {
				return nil
			},
		},
		lowtuncmds.DHCPCommand,
		{
			Name: "synscan",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "host",
					Value: "47.52.100.1/24",
				},
				cli.StringFlag{
					Name:  "port",
					Value: "22,80,443",
				},
				cli.StringFlag{
					Name:  "iface",
					Value: "",
				},
			},
			Action: func(c *cli.Context) error {
				ifaceName := c.String("iface")
				if ifaceName == "" {
					iface, preferred, getaway, err := netutil.GetPublicRoute()
					if err != nil {
						return utils.Errorf("failed to get public route: %v", err)
					}
					_, _ = preferred, getaway
					ifaceName = iface.Name
				}
				log.Infof("using iface: %v", ifaceName)
				handler, err := rwendpoint.NewPcapReadWriteCloser(ifaceName, 1600)
				if err != nil {
					log.Errorf("Failed to create PcapReadWriteCloserEndpoint: %v", err)
					return err
				}
				defer handler.Close()

				localAddressStr := handler.GetIP4Address()
				log.Infof("local address: %v", localAddressStr)
				gatewayAddressStr := handler.GetGatewayIP4Address()
				log.Infof("gateway address: %v", gatewayAddressStr)

				ep, err := rwendpoint.NewReadWriteCloserEndpoint(handler, 1500, 0)
				if err != nil {
					log.Errorf("Failed to create ReadWriteCloserEndpoint: %v", err)
					return err
				}
				defer ep.Close()

				s, err := netstack.NewDefaultStack(
					handler.GetIP4Address().String(), "",
					handler.GetGatewayIP4Address().String(),
					ep,
				)
				if err != nil {
					log.Errorf("Failed to create default network stack: %v", err)
					return err
				}

				finished := new(int64)
				addFinished := func() {
					atomic.AddInt64(finished, 1)
				}

				count := new(int64)
				addTask := func() {
					atomic.AddInt64(count, 1)
				}
				go func() {
					var lastCount int64
					for {
						currentCount := atomic.LoadInt64(count)
						if currentCount != lastCount {
							log.Infof("count: %v", currentCount)
							lastCount = currentCount
						}
						time.Sleep(1 * time.Second)
					}
				}()

				swg := utils.NewSizedWaitGroup(10000)
				for _, host := range utils.ParseStringToHosts(c.String("host")) {
					for _, port := range utils.ParseStringToPorts(c.String("port")) {
						host := host
						port := port
						swg.Add(1)
						addTask()
						go func() {
							time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)
							ctx := utils.TimeoutContextSeconds(5)
							defer swg.Done()

							lport, tcpErr := s.PortManager.PickEphemeralPort(s.SecureRNG(), func(p uint16) (bool, tcpip.Error) {
								return true, nil
							})
							if tcpErr != nil {
								log.Errorf("failed to pick ephemeral port: %v", tcpErr)
							}
							defer func() {
								s.PortManager.ReleasePort(ports.Reservation{
									Networks:  []tcpip.NetworkProtocolNumber{tcpip.NetworkProtocolNumber(ipv4.ProtocolNumber)},
									Transport: header.TCPProtocolNumber,
									Port:      uint16(lport),
								})
							}()

							conn, err := gonet.DialTCPWithBind(ctx, s, tcpip.FullAddress{
								Port: uint16(lport),
								NIC:  1,
								Addr: tcpip.AddrFrom4(netip.MustParseAddr(handler.GetIP4Address().String()).As4()),
							}, tcpip.FullAddress{
								Port: uint16(port),
								NIC:  1,
								Addr: tcpip.AddrFrom4(netip.MustParseAddr(host).As4()),
							}, ipv4.ProtocolNumber)
							if err != nil {
								return
							}
							defer conn.Close()
							log.Infof("%4s: Remote Port %23s OPEN from: localhost:%v", fmt.Sprint(atomic.LoadInt64(finished)), utils.HostPort(host, port), lport)
							addFinished()
						}()
					}
				}
				swg.Wait()
				log.Infof("finished: %v", atomic.LoadInt64(finished))
				return nil
			},
		},
		{
			Name: "a4",
			Action: func(c *cli.Context) error {
				// 这个有比较大的问题，网络栈会冲突，直接导致操作系统会 RST 外挂连接的数据包
				log.Infof("Starting to create PcapReadWriteCloserEndpoint")
				ep, err := rwendpoint.NewPcapReadWriteCloserEndpoint("en0", 65535)
				if err != nil {
					log.Errorf("Failed to create PcapReadWriteCloserEndpoint: %v", err)
					return err
				}
				log.Infof("Starting to create default network stack")
				defaultStack, err := netstack.NewDefaultStack(
					"192.168.0.251",
					"",
					"192.168.0.1",
					ep,
					//netstack.WithTCPHandler(func(conn netstack.TCPConn) {
					//	log.Infof("start to handle tcp connection")
					//	conn.Write([]byte("from hijacked tcp"))
					//	conn.Close()
					//}),
					//netstack.WithUDPHandler(func(conn netstack.UDPConn) {
					//	log.Infof("start to handle udp connection")
					//	conn.Write([]byte("hello"))
					//	conn.Close()
					//}),
				)
				if err != nil {
					log.Errorf("Failed to create default network stack: %v", err)
					return err
				}
				log.Infof("Network stack created successfully, waiting for network stack to work")
				go func() {
					time.Sleep(1 * time.Second)
					// 使用 gvisor 的 gonet 进行网络连接
					tcpConn, err := gonet.DialTCP(defaultStack, tcpip.FullAddress{
						Port: 443,
						NIC:  1,
						Addr: tcpip.AddrFrom4(netip.MustParseAddr("93.184.215.14").As4()),
					}, ipv4.ProtocolNumber)
					if err != nil {
						// Check if routing table is correct
						routes := defaultStack.GetRouteTable()
						if len(routes) == 0 {
							log.Error("Routing table is empty, please check network configuration")
							return
						}
						log.Infof("Current routing table: %v", routes)

						// Check network interface status
						nics := defaultStack.NICInfo()
						if len(nics) == 0 {
							log.Error("No available network interfaces found")
							return
						}
						log.Errorf("Connection failed: %v", err)
						return
					}
					log.Infof("成功建立连接")
					tcpConn.Write([]byte("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n"))
					results := utils.StableReaderEx(tcpConn, 1*time.Second, 1024)
					log.Infof("read %v bytes\n%v", len(results), spew.Sdump(results))
					tcpConn.Close()
				}()
				defaultStack.Wait()
				log.Infof("Network stack has exited")
				return nil
			},
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		log.Infof("Starting to create TUN device: utun113")
		tunDev, err := tun.CreateTUN("utun113", 1420)
		if err != nil {
			log.Errorf("Failed to create TUN device: %v", err)
			return err
		}
		defer tunDev.Close()

		log.Infof("TUN device created successfully, starting to create WireGuard endpoint")
		ep, err := rwendpoint.NewWireGuardDeviceEndpoint(tunDev)
		if err != nil {
			log.Errorf("Failed to create WireGuard endpoint: %v", err)
			return err
		}
		var _ stack.LinkEndpoint = ep
		log.Infof("Starting to create default network stack")
		defaultStack, err := netstack.NewDefaultStack(
			"", "", "",
			ep,
			netstack.WithTCPHandler(func(conn netstack.TCPConn) {
				defer conn.Close()
				log.Infof("start to handle tcp connection from: %v to %v", conn.RemoteAddr().String(), conn.LocalAddr().String())

				targetAddr := conn.LocalAddr().String()
				_ = targetAddr

				nativeDial, err := net.DialTCP("tcp", &net.TCPAddr{
					IP: net.ParseIP("192.168.0.134"),
				}, &net.TCPAddr{
					IP:   net.ParseIP("93.184.215.14"),
					Port: 80,
				})
				if err != nil {
					log.Errorf("failed to dial tcp: %v", err)
					return
				}
				defer nativeDial.Close()

				go io.Copy(nativeDial, conn)
				go io.Copy(conn, nativeDial)

				time.Sleep(10 * time.Second)
			}),
			netstack.WithUDPHandler(func(conn netstack.UDPConn) {
				log.Infof("start to handle udp connection")
				conn.Write([]byte("hello"))
				conn.Close()
			}),
		)
		if err != nil {
			log.Errorf("Failed to create default network stack: %v", err)
			return err
		}
		log.Infof("Network stack created successfully, waiting for network stack to work")
		defaultStack.Wait()
		log.Infof("Network stack has exited")
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		os.Exit(1)
	}
}
