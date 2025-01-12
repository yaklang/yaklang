package main

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/rwendpoint"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"

	"github.com/davecgh/go-spew/spew"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	tun "github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
)

type PcapEndpoint struct {
	handle       *pcap.Handle
	linkEndpoint *channel.Endpoint
	stopChan     chan struct{}
}

func NewPcapEndpoint(nicName string, mtu uint32) (*PcapEndpoint, error) {
	handle, err := pcap.OpenLive(nicName, int32(mtu), true, pcap.BlockForever)
	if err != nil {
		return nil, err
	}

	ep := channel.New(512, mtu, "")

	p := &PcapEndpoint{
		handle:       handle,
		linkEndpoint: ep,
		stopChan:     make(chan struct{}),
	}

	go p.gvsior2pcap()
	go p.pcap2gvisor()

	return p, nil
}

func (ep *PcapEndpoint) pcap2gvisor() {
	packetSource := gopacket.NewPacketSource(ep.handle, ep.handle.LinkType())
	for {
		select {
		case <-ep.stopChan:
			return
		default:
			packet, err := packetSource.NextPacket()
			if err != nil {
				log.Error("read from pcap error:", err)
				continue
			}

			linkLayer := packet.LinkLayer()
			if linkLayer == nil {
				log.Info("fetch link layer empty")
				continue
			}
			networkLayer := packet.NetworkLayer()
			if networkLayer == nil {
				// log.Info("fetch network layer empty")
				continue
			}

			// log.Infof("start to build packet from pcap, len: %v", len(packet.Data()))
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(packet.Data()),
			})
			switch ret := networkLayer.LayerType(); ret {
			case layers.LayerTypeIPv4:
				ep.linkEndpoint.InjectInbound(header.IPv4ProtocolNumber, pkt)
			case layers.LayerTypeIPv6:
				ep.linkEndpoint.InjectInbound(header.IPv6ProtocolNumber, pkt)
			default:
				log.Infof("fetch network layer type %v not support", ret.String())
			}
		}
	}
}

func (ep *PcapEndpoint) gvsior2pcap() {
	var wq waiter.Queue
	we, ch := waiter.NewChannelEntry(waiter.ReadableEvents)
	wq.EventRegister(&we)
	defer wq.EventUnregister(&we)
	for {
		select {
		case <-ep.stopChan:
			return
		default:
			pkt := ep.linkEndpoint.Read()
			if pkt == nil {
				select {
				case <-ch:
					log.Infof("readable message event from link endpoint")
					continue
				case <-time.After(30 * time.Second):
					log.Debugf("read from link endpoint empty")
					continue
				case <-ep.stopChan:
					return
				}
			}
			bytes := pkt.Data().AsRange().ToSlice()
			log.Infof("read from link endpoint len:%v to pcap", len(bytes))
			if len(bytes) > 0 {
				err := ep.handle.WritePacketData(bytes)
				if err != nil {
					log.Error("write to pcap error:", err)
				}
			}
		}
	}
}

func (ep *PcapEndpoint) Close() error {
	close(ep.stopChan)
	return nil
}

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
				log.Infof("%v: IPv4 packet, len: %v", prompt, len(packet))
			case 6:
				log.Infof("%v: IPv6 packet", prompt)
				// spew.Dump(packet)
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

	app.Commands = []cli.Command{
		{
			Name: "a4",
			Action: func(c *cli.Context) error {
				log.Infof("Starting to create PcapReadWriteCloserEndpoint")
				ep, err := rwendpoint.NewPcapReadWriteCloserEndpoint("en0", 65535)
				if err != nil {
					log.Errorf("Failed to create PcapReadWriteCloserEndpoint: %v", err)
					return err
				}
				log.Infof("Starting to create default network stack")
				defaultStack, err := netstack.NewDefaultStack(
					"192.168.0.251",
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
		{
			Name: "a3",
			Action: func(c *cli.Context) error {
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
					"", "",
					ep,
					// netstack.WithTCPHandler(func(conn netstack.TCPConn) {
					// 	log.Infof("start to handle tcp connection")
					// 	conn.Write([]byte("hello"))
					// 	conn.Close()
					// }),
					// netstack.WithUDPHandler(func(conn netstack.UDPConn) {
					// 	log.Infof("start to handle udp connection")
					// 	conn.Write([]byte("hello"))
					// 	conn.Close()
					// }),
				)
				if err != nil {
					log.Errorf("Failed to create default network stack: %v", err)
					return err
				}
				log.Infof("Network stack created successfully, waiting for network stack to work")
				defaultStack.Wait()
				log.Infof("Network stack has exited")
				return nil
			},
		},
		{
			Name: "a2",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "iface,i",
				},
			},
			Action: func(c *cli.Context) error {
				name := "en0"
				if c.IsSet("iface") {
					name = c.String("iface")
				}

				name, err := pcaputil.IfaceNameToPcapIfaceName(name)
				if err != nil {
					return err
				}
				iface, err := pcaputil.PcapIfaceNameToNetInterface(name)
				if err != nil {
					return err
				}
				if iface.HardwareAddr == nil || len(iface.HardwareAddr) <= 0 {
					return utils.Errorf("cannot fetch %v 's hardware addr", name)
				}

				mtu := 1420
				if iface.MTU > 0 {
					mtu = iface.MTU
				}
				ep, err := NewPcapEndpoint(name, uint32(mtu))
				if err != nil {
					return err
				}
				_ = ep

				log.Infof("start to create basic network stack")
				s := stack.New(stack.Options{
					NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
					TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol6, icmp.NewProtocol4},
					HandleLocal:        true,
				})

				log.Infof("start to create nic - en0")
				nicErr := s.CreateNICWithOptions(1, ep.linkEndpoint, stack.NICOptions{
					Name:     "en0",
					Disabled: false,
				})
				if nicErr != nil {
					return errors.Errorf("create nic failed: %v", nicErr.String())
				}

				log.Infof("add default route ipv4 for en0")
				// 这里的路由设置可能有问题:
				// 1. 网关地址192.168.0.1是硬编码的,应该根据实际网络环境获取
				// 2. 没有检查网关地址是否可达
				// 3. 只设置了IPv4路由,如果网卡支持IPv6也应该设置IPv6路由
				// 建议:
				// 1. 从系统获取默认网关地址
				// 2. 添加网关可达性检查
				// 3. 根据网卡支持的协议添加对应路由
				routes := []tcpip.Route{
					{
						Destination: header.IPv4EmptySubnet,
						Gateway:     tcpip.AddrFrom4(netip.MustParseAddr("192.168.0.1").As4()),
						NIC:         1,
						MTU:         uint32(mtu),
					},
				}
				s.SetRouteTable(routes)

				addrs, err := iface.Addrs()
				if err != nil {
					return err
				}

				ipv4addrString := ""
				for _, addr := range addrs {
					ipAddr, _, ipNetErr := net.ParseCIDR(addr.String())
					if ipNetErr != nil {
						continue
					}

					if utils.IsIPv4(ipAddr.String()) {
						ipv4addrString = ipAddr.String()
					}
				}

				s.AddProtocolAddress(
					1,
					tcpip.ProtocolAddress{
						Protocol: header.IPv4ProtocolNumber,
						AddressWithPrefix: tcpip.AddressWithPrefix{
							Address:   tcpip.AddrFrom4(netip.MustParseAddr(ipv4addrString).As4()),
							PrefixLen: 24,
						},
					},
					stack.AddressProperties{},
				)
				log.Infof("start nic address to %v", ipv4addrString)
				tcpErr := s.SetNICAddress(1, tcpip.LinkAddress(ipv4addrString))
				if tcpErr != nil {
					log.Errorf("set nic address failed: %v", tcpErr)
					return utils.Errorf("set nic address failed: %v", tcpErr)
				}

				log.Infof("start to create tcp endpoint")
				var wq waiter.Queue
				clientEp, tcpErr := s.NewEndpoint(tcp.ProtocolNumber, header.IPv4ProtocolNumber, &wq)
				if tcpErr != nil {
					log.Errorf("create endpoint failed: %v", tcpErr)
					return utils.Errorf("create endpoint failed: %v", tcpErr)
				}
				if err := clientEp.Bind(tcpip.FullAddress{
					NIC: 1, Port: 0,
				}); err != nil {
					clientEp.Close()
					return utils.Errorf("bind to %v failed: %v", ipv4addrString, err)
				}

				log.Info("start to create remote addr")
				remote := tcpip.FullAddress{
					NIC:  1,
					Addr: tcpip.AddrFrom4(netip.MustParseAddr("123.56.31.221").As4()),
					Port: 443,
				}

				waitEntry, notifyCh := waiter.NewChannelEntry(waiter.WritableEvents)
				wq.EventRegister(&waitEntry)
				defer wq.EventUnregister(&waitEntry)

				clientErr := clientEp.Connect(remote)
				if clientErr != nil {
					switch ret := clientErr.(type) {
					case *tcpip.ErrConnectStarted:
						log.Infof("start to connect to %v", remote)
						_ = ret
					default:
						return utils.Errorf("connect to %v failed: %v", remote, clientErr)
					}

				}
				log.Infof("waiting for connect to %v", remote)
				select {
				case <-notifyCh:
					log.Infof("connect to %v success", remote)
					err := clientEp.LastError()
					if err != nil {
						log.Errorf("connect to %v failed: %v", remote, err)
						return utils.Errorf("connect to %v failed: %v", remote, err)
					}
					//case <-time.After(5 * time.Second):
					//	log.Errorf("connect to %v timeout", remote)
				}

				conn := gonet.NewTCPConn(&wq, clientEp)
				conn.Write([]byte("hello"))
				conn.Close()
				return nil

				//dev, tnet, err := netstack.CreateFromIface(iface, ep.linkEndpoint, []netip.Addr{
				//	netip.MustParseAddr("8.8.8.8"),
				//})
				//if err != nil {
				//	return err
				//}
				//_ = dev
				//_ = tnet
				//
				//// yaklang.com 123.56.31.221
				//log.Info("start to dial www.example.com:443")
				//conn, err := tnet.Dial("tcp", "123.56.31.221:443")
				//if err != nil {
				//	return err
				//}
				//log.Info("start to write hello")
				//conn.Write([]byte("hello"))
				//log.Info("start to read")
				//results := utils.StableReaderEx(conn, 1*time.Second, 1024)
				//log.Infof("read %v bytes\n%v", len(results), spew.Sdump(results))
				//conn.Close()
				//
				//time.Sleep(1 * time.Minute)

				return nil
			},
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		log.Info("start to create net tun in gvisor")
		sdev, sdial, err := netstack.CreateNetTUN([]netip.Addr{
			netip.MustParseAddr("8.8.8.8"),
		}, []netip.Addr{}, 1420)
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

		log.Info("start to sniff in en1")

		go func() {
			sniffErr := pcaputil.Sniff(""+
				"en1",
				pcaputil.WithBPFFilter(`tcp and host 8.8.8.8`),
				pcaputil.WithEnableCache(true),
				pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
					fmt.Println(packet.String())
				}),
			)
			if sniffErr != nil {
				log.Errorf("failed to sniff en0: %v", err)
			}
		}()

		log.Infof("start to fetch stack")
		st := sdial.Stack()

		st.SetTransportProtocolHandler(tcp.ProtocolNumber, func(id stack.TransportEndpointID, buffer *stack.PacketBuffer) bool {
			ipHeader := header.IPv4(buffer.NetworkHeader().View().ToSlice())
			tcpHeader := header.TCP(buffer.TransportHeader().View().ToSlice())
			_ = ipHeader
			_ = tcpHeader

			if tcpHeader.Flags() == header.TCPFlagSyn {
				// first syn
				var wq waiter.Queue
				ep, err := st.NewEndpoint(tcp.ProtocolNumber, header.IPv4ProtocolNumber, &wq)
				if err != nil {
					log.Errorf("create endpoint failed: %v", err)
					return false
				}
				if err := ep.Bind(tcpip.FullAddress{
					NIC:  1,
					Addr: id.LocalAddress,
					Port: id.LocalPort,
				}); err != nil {
					log.Errorf("bind to %v failed: %v", id.LocalAddress, err)
					return false
				}

				sopt := ep.SocketOptions()
				sopt.SetKeepAlive(true)

				if err := ep.Listen(1); err != nil {
					ep.Close()
					return false
				}

				go func() {
					waitEntry, notifyCh := waiter.NewChannelEntry(waiter.ReadableEvents)
					wq.EventRegister(&waitEntry)
					defer wq.EventUnregister(&waitEntry)
					_ = notifyCh

					for {
						newEp, wq, err := ep.Accept(&tcpip.FullAddress{
							NIC:  1,
							Addr: id.LocalAddress,
							Port: id.LocalPort,
						})
						if _, ok := err.(*tcpip.ErrWouldBlock); ok {
							select {
							case <-notifyCh:
								continue
							case <-time.After(30 * time.Second):
							}
						} else if err != nil {
							log.Errorf("accept failed: %v", err)
							spew.Dump(ep.Stats())
							ep.Close()
							return
						}

						conn := gonet.NewTCPConn(wq, newEp)
						go func() {
							for {
								results := utils.StableReaderEx(conn, 1*time.Second, 1024)
								if len(results) > 0 {
									log.Infof("read %v bytes\n%v", len(results), spew.Sdump(results))
								}
								if results == nil || results[0] == 0 {
									continue
								}
								conn.Write([]byte(fmt.Sprintf("Echo %v", spew.Sdump(results))))
							}
						}()
					}
				}()

				return true
			}
			return false
		})

		// tun -> gvisor -> en0
		// en0 -> gvisor -> tun

		go func() {
			defer func() {
				wg.Done()
				if err := recover(); err != nil {
					log.Errorf("panic: %v", err)
				}
			}()
			// tun -> gvsior
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
