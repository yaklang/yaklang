package synscanx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// windows 的pcap 错误信息是gb18030编码的，需要转换成utf8
func handleError(err error) error {
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
		return handleError(err)
	}
	log.Infof("pcap open live success: %s", s.config.Iface.Name)
	var bpf string
	if s.config.Iface.Flags&net.FlagLoopback == 0 {
		// Interface is not loopback, set the filter.
		if s.config.Iface.HardwareAddr == nil || !strings.Contains(s.config.Iface.HardwareAddr.String(), ":") {
			bpf = "arp || udp || tcp[tcpflags] == tcp-syn|tcp-ack"
		} else {
			bpf = fmt.Sprintf("ether dst %s && (arp || udp || tcp[tcpflags] == tcp-syn|tcp-ack)", s.config.Iface.HardwareAddr.String())
		}
	} else {
		// Interface is loopback, set a different filter.
		// Replace the following line with the appropriate filter for your use case.
		bpf = "udp || tcp[tcpflags] == tcp-syn|tcp-ack"
	}
	err = handle.SetBPFFilter(bpf)
	if err != nil {
		if ferr := handle.SetBPFFilter(`arp || udp || tcp[tcpflags] == tcp-syn|tcp-ack`); ferr != nil {
			return utils.Errorf("SetBPFFilter failed: %v, bpf: %v", err, bpf)
		}
	}

	log.Infof("pcap set filter success: %s", bpf)
	s.Handle = handle

	return nil
}

func (s *Scannerx) initHandlerStart(ctx context.Context) error {
	var err error
	// 通知
	captureReady := make(chan struct{})

	go func() {
		defer func() {
			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		var adapters []*pcaputil.DeviceAdapter

		if s.config.Iface != nil {
			adapters = append(adapters, &pcaputil.DeviceAdapter{
				DeviceName: s.config.Iface.Name,
				BPF:        fmt.Sprintf("ether dst %s && (arp || udp  || tcp[tcpflags] == tcp-syn|tcp-ack)", s.config.Iface.HardwareAddr.String()),
				Snaplen:    128,
				Promisc:    false,
				Timeout:    pcap.BlockForever,
			})
		}
		var loop *net.Interface
		loop, err = pcaputil.GetLoopBackNetInterface()
		if err == nil {
			adapters = append(adapters, &pcaputil.DeviceAdapter{
				DeviceName: loop.Name,
				BPF:        "udp || tcp[tcpflags] == tcp-syn|tcp-ack",
				Snaplen:    128,
				Promisc:    false,
				Timeout:    pcap.BlockForever,
			})
		} else {
			log.Errorf("get loopback net interface failed: %v", err)
		}
		err = pcaputil.Start(
			pcaputil.WithDeviceAdapter(adapters...),
			pcaputil.WithContext(ctx),
			pcaputil.WithEnableCache(true),
			pcaputil.WithDisableAssembly(true),
			pcaputil.WithCaptureStartedCallback(func() {
				// 通知主协程，捕获器已经准备就绪
				close(captureReady)
			}),
			pcaputil.WithNetInterfaceCreated(func(handle *pcaputil.PcapHandleWrapper) {
				go func() {
					defer func() {
						if err := recover(); err != nil {
							utils.PrintCurrentGoroutineRuntimeStack()
						}
					}()
					if handle.IsLoopback() {
						for {
							select {
							case <-s.ctx.Done():
								return
							case lpacket, ok := <-s.LoopPacket:
								if !ok {
									return
								}
								err := handle.WritePacketData(lpacket)
								if err != nil {
									log.Errorf("write packet failed: %v", handleError(err))
									continue
								}
							}
						}
					} else {
						for {
							select {
							case <-s.ctx.Done():
								return
							case packet, ok := <-s.PacketChan:
								if !ok {
									return
								}

								err := handle.WritePacketData(packet)
								if err != nil {
									log.Errorf("write packet failed: %v", handleError(err))
									continue
								}
							}
						}
					}

				}()
			}),

			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
				defer func() {
					if err := recover(); err != nil {
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				if packet == nil {
					return
				}
				s.handlePacket(packet)
			}),
		)
		if err != nil {
			log.Errorf("pcaputil start failed: %v", err)
		}
	}()

	// 等待捕获器准备就绪
	select {
	case <-captureReady:
		log.Infof("pcap handle %s is ready", s.config.Iface.Name)
	case <-ctx.Done():
		return ctx.Err()
	}

	return err
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
		case packet := <-packetSource.PacketsCtx(ctx):
			if packet == nil {
				continue
			}
			s.handlePacket(packet)
		}
	}
}

func (s *Scannerx) HandlerZeroCopyReadPacket(ctx context.Context, resultCh chan *synscan.SynScanResult) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if s.Handle == nil {
				log.Errorf("pcap handle is nil")
				return
			}
			data, _, err := s.Handle.ZeroCopyReadPacketData()
			if errors.Is(err, pcap.NextErrorTimeoutExpired) || errors.Is(err, pcap.NextErrorReadError) || errors.Is(err, io.EOF) {
				continue
			} else if err != nil {
				log.Errorf("error reading packet: %v", err)
				continue
			}

			packet := gopacket.NewPacket(data, s.Handle.LinkType(), gopacket.Default)
			s.handlePacket(packet)
		}
	}
}

func (s *Scannerx) handlePacket(packet gopacket.Packet) {
	if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
		switch arpLayer.LayerType() {
		case layers.LayerTypeARP:
			arp, ok := arpLayer.(*layers.ARP)
			if !ok {
				return
			}
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
