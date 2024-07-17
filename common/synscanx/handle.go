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
	"runtime"
)

func (s *Scannerx) handleError(err error) error {
	if err == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		info, convertErr := codec.GB18030ToUtf8([]byte(err.Error()))
		if convertErr != nil {
			return utils.Wrapf(convertErr, "pcap ifaceDevs")
		}
		return utils.Wrapf(errors.New(string(info)), "pcap ifaceDevs")
	} else {
		return utils.Wrapf(err, "handle Error")
	}
}

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
		return s.handleError(err)
	}
	if s.config.Iface.Flags&net.FlagLoopback == 0 {
		// Interface is not loopback, set the filter.
		err = handle.SetBPFFilter(fmt.Sprintf("ether dst %s && (arp || tcp[tcpflags] == tcp-syn|tcp-ack)", s.config.Iface.HardwareAddr.String()))
		if err != nil {
			return utils.Errorf("SetBPFFilter failed: %v", err)
		}
	} else {
		// Interface is loopback, set a different filter.
		// Replace the following line with the appropriate filter for your use case.
		err = handle.SetBPFFilter("tcp[tcpflags] == tcp-syn|tcp-ack")
		if err != nil {
			return utils.Errorf("Loopback SetBPFFilter failed: %v", err)
		}
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
				log.Errorf("error reading packet: %v", err)
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
