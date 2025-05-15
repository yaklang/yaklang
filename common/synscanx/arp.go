package synscanx

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

func (s *Scannerx) getGatewayMac() (net.HardwareAddr, error) {
	if s.config.GatewayIP != nil {
		gateway := s.config.GatewayIP.String()
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
				log.Debugf("use arpx proto to fetch gateway's hw address: %s", dstHw)
				return dstHw.(net.HardwareAddr), nil
			}
			retry++
			time.Sleep(time.Millisecond * 50)
		}
	}
	return nil, utils.Errorf("cannot fetch hw addr for %v[%v]", s.sampleIP, s.config.Iface.Name)
}

func (s *Scannerx) onArp(ip net.IP, hw net.HardwareAddr) {
	if s.MacHandlers != nil {
		s.MacHandlers(ip, hw)
	}
	if s.config.SourceIP.Equal(ip) || s.config.GatewayIP.Equal(ip) {
		s.macCacheTable.Store(ip.String(), hw)
		return
	}

	if s.FromPing {
		if !s._hosts.Contains(ip.String()) {
			return
		}
	} else {
		if !s.hosts.Contains(ip.String()) {
			return
		}
	}
	log.Debugf("ARP: %s -> %s", ip.String(), hw.String())

	s.macCacheTable.Store(ip.String(), hw)
}

// getInterfaceNetworks 获取网络接口的 IPv4 和 IPv6 网络范围
func (s *Scannerx) getInterfaceNetworks() (*net.IPNet, *net.IPNet) {
	// 如果接口已经更新过了，直接返回缓存的值
	if s.ifaceUpdated {
		return s.ifaceIPNetV4, s.ifaceIPNetV6
	}

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

	// 更新缓存
	s.ifaceIPNetV4 = ifaceIPNetV4
	s.ifaceIPNetV6 = ifaceIPNetV6
	s.ifaceUpdated = true // 标记接口信息已更新

	return ifaceIPNetV4, ifaceIPNetV6
}

func (s *Scannerx) ResetInterfaceCache() {
	s.ifaceUpdated = false
}

// isInternalAddress 判断目标 IP 是否为内网地址
func (s *Scannerx) isInternalAddress(target string) bool {
	ifaceIPNetV4, ifaceIPNetV6 := s.getInterfaceNetworks()
	targetIP := net.ParseIP(target)
	if targetIP == nil || utils.IsLoopback(target) {
		return false
	}
	return (targetIP.To4() != nil && ifaceIPNetV4 != nil && ifaceIPNetV4.Contains(targetIP.To4())) ||
		(targetIP.To16() != nil && ifaceIPNetV6 != nil && ifaceIPNetV6.Contains(targetIP.To16()))
}

func (s *Scannerx) arpScan() {
	for target := range s.hosts.Hosts() {
		s.rateLimit()
		select {
		case <-s.ctx.Done():
			return
		default:
			if s.isInternalAddress(target) {
				s.arp(target)
			}
		}
	}
}
func (s *Scannerx) arp(target string) {
	packet, err := s.assemblePacket(target, 0, ARP)
	if err != nil {
		log.Errorf("assemble arp packet failed: %v", err)
		return
	}
	select {
	case <-s.ctx.Done():
		return
	case s.PacketChan <- packet:
	}
	//err = s.Handle.WritePacketData(packet)
	//if err != nil {
	//	log.Errorf("write to device arp failed: %v", s.handleError(err))
	//	return
	//}
}
