package netstackvm

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

func (m *NetStackVirtualMachine) persistentARPAnnouncement() error {
	var addrs []string
	m.arpPersistentMap.Range(func(key, value any) bool {
		addrs = append(addrs, key.(string))
		return true
	})

	wg := sync.WaitGroup{}
	for _, addr := range addrs {
		ipAddr := net.ParseIP(addr)
		if ipAddr == nil {
			log.Errorf("failed to parse ip in persistentARPAnnouncement: %v", addr)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.sendARPAnnouncement(m.config.ctx, tcpip.AddrFrom4([4]byte(ipAddr.To4())))
			if err != nil {
				log.Errorf("failed to send arp announcement: %v", err)
			} else {
				log.Infof("send arp announcement success: %v", ipAddr)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (vm *NetStackVirtualMachine) sendARPAnnouncement(ctx context.Context, ipAddr tcpip.Address) error {
	nicID := vm.MainNICID()
	s := vm.stack
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
	log.Infof("send arp announcement: %v", ipAddr)
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData([]byte(string(arpHdr))),
	})
	nicIns.WritePacketToRemote(tcpip.LinkAddress(header.EthernetBroadcastAddress), pkt)
	pkt.DecRef() // 修复第一个包的内存泄漏

	// 可选：发送多次以提高可靠性
	fastInterval := vm.config.ARPAnnouncementFastInterval
	if fastInterval <= 0 {
		fastInterval = time.Second * 1
	}
	timer := time.NewTicker(fastInterval)
	defer timer.Stop()
	for i := 0; i < vm.config.ARPAnnouncementFastTimes; i++ {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData([]byte(string(arpHdr))),
			})
			nicIns.WritePacketToRemote(tcpip.LinkAddress(header.EthernetBroadcastAddress), pkt)
			pkt.DecRef()
		}
	}

	return nil
}

func (m *NetStackVirtualMachine) StartAnnounceARP() error {
	if m.config.ARPDisabled {
		return utils.Error("arp is disabled")
	}
	if m.arpServiceStarted.IsSet() {
		return utils.Error("arp service already started")
	}
	m.arpServiceStarted.Set()

	go func() {
		err := m.persistentARPAnnouncement()
		if err != nil {
			log.Errorf("failed to persistentARPAnnouncement: %v", err)
		}
		for {
			select {
			case <-m.config.ctx.Done():
				return
			case <-time.After(m.config.ARPAnnouncementSlowInterval):
				m.persistentARPAnnouncement()
			case <-time.After(time.Second):
				if m.arpPersistentTrigger.IsSet() {
					m.arpPersistentTrigger.UnSet()
					m.persistentARPAnnouncement()
				}
			}
		}
	}()

	return nil
}
