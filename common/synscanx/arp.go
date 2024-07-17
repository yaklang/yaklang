package synscanx

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

func (s *Scannerx) getGatewayMac() (net.HardwareAddr, error) {
	gateway := s.config.GatewayIP.String()
	if gateway != "" && gateway != "<nil>" {
		var retry int
		for {
			s.arp(gateway)
			if retry > 2 {
				return nil, utils.Errorf("cannot fetch hw addr for %v[%v]", s.sampleIP, s.config.Iface.Name)
			}
			dstHw, ok := s.macTable.Load(gateway)
			if ok {
				s.config.RemoteMac = dstHw.(net.HardwareAddr)
				log.Infof("use arpx proto to fetch gateway's hw address: %s", dstHw)
				return dstHw.(net.HardwareAddr), nil
			}
			retry++
			time.Sleep(time.Millisecond * 50)
		}
	}
	return nil, utils.Errorf("cannot fetch hw addr for %v[%v]", s.sampleIP, s.config.Iface.Name)
	//macCh := make(chan net.HardwareAddr)
	//
	//wg := sync.WaitGroup{}
	//
	//ctx, cancel := context.WithTimeout(context.Background(), s.config.FetchGatewayHardwareAddressTimeout)
	//
	//wg.Add(1)
	//go func() {
	//	defer wg.Done()
	//	err := pcaputil.Start(
	//		pcaputil.WithContext(ctx),
	//		pcaputil.WithDevice(s.config.Iface.Name),
	//		pcaputil.WithDisableAssembly(true),
	//		pcaputil.WithBPFFilter("udp dst port 65321"),
	//		pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
	//			if ethLayer := packet.Layer(layers.LayerTypeEthernet); ethLayer != nil {
	//				if !bytes.Equal(ethLayer.(*layers.Ethernet).DstMAC, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
	//					log.Infof("MAC Address found: %s", ethLayer.(*layers.Ethernet).DstMAC)
	//					macCh <- ethLayer.(*layers.Ethernet).DstMAC
	//					cancel()
	//				}
	//			}
	//		}),
	//	)
	//	if err != nil {
	//		log.Errorf("pcaputil.Start failed: %v", err)
	//		return
	//	}
	//
	//}()
	//
	//connectUdp := func() error {
	//	conn, err := yaklib.ConnectUdp(s.sampleIP, "65321")
	//	if err != nil {
	//		log.Errorf("connect udp failed: %v", err)
	//		return err
	//	}
	//	defer conn.Close()
	//	_, err = conn.Write([]byte("hello"))
	//	if err != nil {
	//		return err
	//	}
	//
	//	err = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	//	if err != nil {
	//		return err
	//	}
	//	buf := make([]byte, 1024)
	//	_, _ = conn.Read(buf)
	//	return nil
	//}
	//
	//wg.Add(1)
	//go func() {
	//	defer wg.Done()
	//	for i := 0; i < 3; i++ {
	//		err := connectUdp()
	//		if err != nil {
	//			return
	//		}
	//	}
	//}()
	//go func() {
	//	wg.Wait()
	//	close(macCh)
	//}()
	//
	//timer := time.NewTimer(time.Second * 3)
	//defer timer.Stop()
	//
	//select {
	//case <-timer.C:
	//	return utils.Errorf("cannot fetch hw addr for %v[%v]", s.sampleIP, s.config.Iface.Name)
	//case hw := <-macCh:
	//	s.config.RemoteMac = hw
	//	log.Infof("use pcap proto to fetch gateway's hw address: %s", hw.String())
	//	return nil
	//}
}

func (s *Scannerx) onArp(ip net.IP, hw net.HardwareAddr) {
	log.Debugf("ARP: %s -> %s", ip.String(), hw.String())
	if s.MacHandlers != nil {
		s.MacHandlers(ip, hw)
	}

	s.macTable.Store(ip.String(), hw)
}

func (s *Scannerx) arpScan() {
	addrs, _ := s.config.Iface.Addrs()

	var ifaceIPNetV4 *net.IPNet
	var ifaceIPNetV6 *net.IPNet

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet == nil {
			continue
		}
		if ipNet.IP.To4() != nil {
			ifaceIPNetV4 = ipNet
		} else if ipNet.IP.To16() != nil {
			ifaceIPNetV6 = ipNet
		}
	}
	for target := range s.hosts.Hosts() {
		s.rateLimit()
		select {
		case <-s.ctx.Done():
			return
		default:
			targetIP := net.ParseIP(target)
			if targetIP == nil {
				continue
			}
			if (targetIP.To4() != nil && ifaceIPNetV4 != nil && ifaceIPNetV4.Contains(targetIP)) ||
				(targetIP.To16() != nil && ifaceIPNetV6 != nil && ifaceIPNetV6.Contains(targetIP)) {
				s.arp(target)
			}
		}
	}
}
func (s *Scannerx) arp(target string) {
	packet, err := s.assemblePacket(target, 0, ARP)
	if err != nil {
		log.Errorf("assemble packet failed: %v", err)
		return
	}
	err = s.Handle.WritePacketData(packet)
	if err != nil {
		log.Errorf("write to device arp failed: %v", s.handleError(err))
		return
	}
}
