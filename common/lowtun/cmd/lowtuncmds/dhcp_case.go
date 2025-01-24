package lowtuncmds

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	gvisorDHCP "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/arp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/rwendpoint"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"net/netip"
	"time"
)

var DHCPCommand = cli.Command{
	Name: "dhcp",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "iface", Value: ""},
	},
	Action: func(c *cli.Context) error {
		var iname string = c.String("iface")
		if iname == "" {
			ifaceDefault, _, _, err := netutil.GetPublicRoute()
			if err != nil {
				log.Errorf("failed to get public route: %v", err)
				return err
			}
			iname = ifaceDefault.Name
		}
		rwc, ep, err := rwendpoint.NewPcapReadWriteCloserEndpointEx(iname, 1600)
		if err != nil {
			log.Errorf("failed to create pcap read write closer endpoint: %v", err)
			return err
		}
		defer rwc.Close()
		defer ep.Close()

		macAddr := rwc.GetDeviceHardwareAddr()
		ipAddr := rwc.GetIP4Address()
		getawayAddr := rwc.GetGatewayIP4Address()
		stackInstance, err := netstack.NewDefaultStack(
			ipAddr.String(),
			macAddr.String(),
			getawayAddr.String(),
			ep,
		)
		if err != nil {
			log.Errorf("failed to create default stack: %v", err)
			return err
		}
		for k, v := range stackInstance.NICInfo() {
			_ = v
			log.Infof("nic: %v", k)
		}

		// 解析 MAC 地址
		// mac, err := net.ParseMAC("72:72:f1:d2:6f:66")
		// if err != nil {
		// 	log.Fatalf("Failed to parse MAC address: %v", err)
		// }

		ch := make(chan struct{})

		stackInstance.RemoveAddress(1, tcpip.AddrFrom4(rwc.GetIP4Address().As4()))
		stackInstance.SetRouteTable(nil)

		client := gvisorDHCP.NewClient(stackInstance, 1, 5*time.Second, 5*time.Second, 5*time.Second, func(ctx context.Context, lost, acquired tcpip.AddressWithPrefix, cfg gvisorDHCP.Config) {
			preferIp, perferNet, err := net.ParseCIDR(acquired.String())
			if err != nil {
				log.Errorf("failed to parse cidr: %v", err)
				return
			}

			log.Infof("reset nic addr: %v mac: %v", preferIp.String(), macAddr.String())

			err = netstack.WithMainNICIP(1, tcpip.AddrFromSlice(preferIp.To4()), macAddr)(stackInstance)
			if err != nil {
				log.Errorf("set nic ip failed: %v", err)
				return
			}
			stackInstance.AddRoute(tcpip.Route{
				Destination: header.IPv4EmptySubnet,
				Gateway: tcpip.AddrFrom4([4]byte{
					192, 168, 0, 1,
				}),
				NIC: 1,
				MTU: 1420,
			})
			stackInstance.SetForwardingDefaultAndAllNICs(header.IPv4ProtocolNumber, true)
			arpNep, epErr := stackInstance.GetNetworkEndpoint(1, arp.ProtocolNumber)
			if epErr != nil {
				log.Errorf("failed to create arp endpoint: %v", epErr)
				return
			}

			if arpErr := sendARPAnnouncement(stackInstance, arpNep, 1, tcpip.AddrFrom4([4]byte(preferIp.To4()))); arpErr != nil {
				log.Errorf("failed to send arp announcement: %v", arpErr)
			}

			_ = arpNep
			go func() {
				for {
					// arpNep.HandlePacket()
					// spew.Dump(arpNep.Stats())
					time.Sleep(time.Second)
				}
			}()

			ch <- struct{}{}

			getaway := cfg.ServerAddress.String()
			log.Infof("dhcp client fetched preferIp: %v, perferNet: %v, getaway: %v", preferIp, perferNet, getaway)
		})

		log.Info("start to run gvisor dhcp client")
		go func() {
			result := client.Run(context.Background())
			_ = result
		}()

		go func() {
			<-ch
			time.Sleep(4 * time.Second)
			log.Infof("dhcp finished, ip fetched")
			ctx := utils.TimeoutContextSeconds(10)
			target := tcpip.FullAddress{
				NIC:  1,
				Addr: tcpip.AddrFrom4(netip.MustParseAddr("23.192.228.150").As4()),
				Port: 80,
			}
			conn, err := gonet.DialContextTCP(ctx, stackInstance, target, header.IPv4ProtocolNumber)
			if err != nil {
				log.Errorf("failed to dial tcp: %v", err)
				return
			}
			conn.Write([]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"))
			results := utils.StableReaderEx(conn, 5*time.Second, 1024)
			spew.Dump(results)
		}()
		stackInstance.Wait()
		return nil
	},
}

func sendARPAnnouncement(s *stack.Stack, netEp stack.NetworkEndpoint, nicID tcpip.NICID, ipAddr tcpip.Address) error {
	// 获取网卡信息
	nic, ok := s.NICInfo()[nicID]
	if !ok {
		return fmt.Errorf("NIC %d not found", nicID)
	}

	// 创建 ARP 请求包
	buf := make([]byte, header.ARPSize)
	arpHdr := header.ARP(buf)
	arpHdr.SetIPv4OverEthernet()

	// 设置为 ARP 请求
	arpHdr.SetOp(header.ARPRequest)

	// 设置发送方的 MAC 和 IP
	copy(arpHdr.HardwareAddressSender(), nic.LinkAddress)
	copy(arpHdr.ProtocolAddressSender(), ipAddr.AsSlice())

	// 设置目标地址（在 Gratuitous ARP 中，协议地址与源相同）
	copy(arpHdr.HardwareAddressTarget(), header.EthernetBroadcastAddress)
	copy(arpHdr.ProtocolAddressTarget(), ipAddr.AsSlice())

	//// 创建以太网头
	//ethHdr := make([]byte, header.EthernetMinimumSize)
	//eth := header.Ethernet(ethHdr)
	//eth.Encode(&header.EthernetFields{
	//	SrcAddr: nic.LinkAddress,
	//	DstAddr: header.EthernetBroadcastAddress,
	//	Type:    header.ARPProtocolNumber,
	//})
	nicIns, tcpErr := s.GetNICByID(nicID)
	if tcpErr != nil {
		log.Errorf("failed to get nic by id: %v", tcpErr)
		return utils.Error(tcpErr.String())
	}
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(arpHdr),
	})
	nicIns.WritePacketToRemote(tcpip.LinkAddress(header.EthernetBroadcastAddress), pkt)

	// 可选：发送多次以提高可靠性
	for i := 0; i < 2; i++ {
		time.Sleep(100 * time.Millisecond)
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(arpHdr),
		})
		nicIns.WritePacketToRemote(tcpip.LinkAddress(header.EthernetBroadcastAddress), pkt)
	}

	return nil
}
