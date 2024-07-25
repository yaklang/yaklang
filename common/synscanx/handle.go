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
	"unicode/utf8"
)

// windows 的pcap 错误信息是gb18030编码的，需要转换成utf8
func (s *Scannerx) handleError(err error) error {
	if err == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		errMsg := err.Error()
		if !utf8.ValidString(errMsg) {
			info, convertErr := codec.GB18030ToUtf8([]byte(errMsg))
			if convertErr != nil {
				return utils.Wrapf(convertErr, "pcap ifaceDevs")
			}
			return utils.Wrapf(errors.New(string(info)), "pcap ifaceDevs")
		} else {
			return utils.Wrapf(err, "pcap ifaceDevs")
		}
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
	log.Infof("pcap open live success: %s", s.config.Iface.Name)
	var bpf string
	if s.config.Iface.Flags&net.FlagLoopback == 0 {
		// Interface is not loopback, set the filter.
		bpf = fmt.Sprintf("ether dst %s && (arp || udp  || tcp[tcpflags] == tcp-syn|tcp-ack)", s.config.Iface.HardwareAddr.String())
	} else {
		// Interface is loopback, set a different filter.
		// Replace the following line with the appropriate filter for your use case.
		bpf = "udp || tcp[tcpflags] == tcp-syn|tcp-ack"
	}
	err = handle.SetBPFFilter(bpf)
	if err != nil {
		return utils.Errorf("SetBPFFilter failed: %v", err)
	}

	log.Infof("pcap set filter success: %s", bpf)
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

	//if icmpLayer := packet.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
	//	icmp := icmpLayer.(*layers.ICMPv4)
	//
	//	// Check if the ICMP message is a port unreachable error
	//	if icmp.TypeCode == layers.ICMPv4TypeDestinationUnreachable && icmp.TypeCode.Code() == layers.ICMPv6CodePortUnreachable {
	//		// Handle ICMP port unreachable error here
	//		fmt.Println("ICMP port unreachable error received")
	//
	//		if nl := packet.NetworkLayer(); nl != nil {
	//			s.ClosedPortHandlers(net.ParseIP(nl.NetworkFlow().Src().String()), int(icmp.Seq))
	//		}
	//	}
	//}

	if transportLayer := packet.TransportLayer(); transportLayer != nil {
		switch layer := transportLayer.(type) {
		case *layers.TCP:
			if layer.SYN && layer.ACK {
				if nl := packet.NetworkLayer(); nl != nil {
					s.OpenPortHandlers(net.ParseIP(nl.NetworkFlow().Src().String()), int(layer.SrcPort))
				}
				return
			}
		case *layers.UDP:
			if nl := packet.NetworkLayer(); nl != nil {
				s.OpenPortHandlers(net.ParseIP(nl.NetworkFlow().Src().String()), int(layer.SrcPort))
			}
		}
	}

}
