package synscanx

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
)

func (s *Scannerx) initHandle() error {
	if s.config.Iface == nil {
		return utils.Errorf("iface is nil")
	}
	pcapIface, err := pcaputil.IfaceNameToPcapIfaceName(s.config.Iface.Name)
	if err != nil {
		return utils.Errorf("iface name to pcap iface name failed: %v", err)
	}
	handle, err := pcap.OpenLive(pcapIface, 128, false, pcap.BlockForever)

	if err != nil {
		info, err := codec.GB18030ToUtf8([]byte(err.Error()))
		if err != nil {
			return utils.Errorf("cannot find pcap ifaceDevs: %v", err)
		}
		return utils.Errorf("cannot find pcap ifaceDevs: %v", string(info))
	}

	err = handle.SetBPFFilter(fmt.Sprintf("ether dst %s && (arp || tcp[tcpflags] == tcp-syn|tcp-ack)", s.config.Iface.HardwareAddr.String()))
	if err != nil {
		return utils.Errorf("SetBPFFilter failed: %v", err)
	}
	s.Handle = handle
	return nil
}

func (s *Scannerx) HandlerReadPacket(ctx context.Context, resultCh chan *synscan.SynScanResult) {
	packetSource := gopacket.NewPacketSource(s.Handle, s.Handle.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true
	packetSource.DecodeStreamsAsDatagrams = true

	for {
		select {
		case <-ctx.Done():
			return
		case packet := <-packetSource.Packets():
			if packet == nil {
				continue
			}
			s.handlePacket(packet, resultCh)
		}
	}
}

func (s *Scannerx) HandlerZeroCopyReadPacket(ctx context.Context, resultCh chan *synscan.SynScanResult) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			data, _, err := s.Handle.ZeroCopyReadPacketData()
			if errors.Is(err, pcap.NextErrorTimeoutExpired) || errors.Is(err, pcap.NextErrorReadError) || errors.Is(err, io.EOF) {
				continue
			} else if err != nil {
				log.Errorf("Error reading packet: %v", err)
				continue
			}

			packet := gopacket.NewPacket(data, s.Handle.LinkType(), gopacket.Default)
			s.handlePacket(packet, resultCh)
		}
	}
}

func (s *Scannerx) handlePacket(packet gopacket.Packet, resultCh chan *synscan.SynScanResult) {
	if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
		arp := arpLayer.(*layers.ARP)
		if arp.Operation == 2 {
			srcIP := net.IP(arp.SourceProtAddress)
			srcHw := net.HardwareAddr(arp.SourceHwAddress)
			s.onArp(srcIP, srcHw)
		}
	}

	if tcpSynLayer := packet.TransportLayer(); tcpSynLayer != nil {
		l, ok := tcpSynLayer.(*layers.TCP)
		if !ok {
			return
		}

		if l.SYN && l.ACK {
			if nl := packet.NetworkLayer(); nl != nil {
				s.OpenPortHandlers(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
			}
			return
		}
	}

}
