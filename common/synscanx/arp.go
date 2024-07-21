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
			// 通过 ARP 协议获取网关的 MAC 地址
			s.arp(gateway)
			if retry > 2 {
				return nil, utils.Errorf("cannot fetch hw addr for %v[%v]", s.sampleIP, s.config.Iface.Name)
			}
			dstHw, ok := s.macCacheTable.Load(gateway)
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
}

func (s *Scannerx) onArp(ip net.IP, hw net.HardwareAddr) {
	log.Debugf("ARP: %s -> %s", ip.String(), hw.String())
	if s.MacHandlers != nil {
		s.MacHandlers(ip, hw)
	}

	s.macCacheTable.Store(ip.String(), hw)
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
			if targetIP == nil || targetIP.IsLoopback() || targetIP.IsLinkLocalUnicast() || targetIP.IsLinkLocalMulticast() {
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
