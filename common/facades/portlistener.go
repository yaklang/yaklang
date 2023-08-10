package facades

import (
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

type PortListener struct {
	AvailablePorts string
}

func (p *PortListener) handle(port int) error {
	return nil
}

func (p *PortListener) handleFromPcap(ctx context.Context, ip string, port int) error {
	handler, err := pcaputil.GetPublicInternetPcapHandler()
	if err != nil {
		return err
	}
	defer func() {
		handler.Close()
	}()

	ip = utils.FixForParseIP(ip)

	//if port > 0 {
	//	err = handler.SetBPFFilter(fmt.Sprintf(`tcp port %d`, port))
	//	if err != nil {
	//		log.Error(err)
	//	}
	//}

	log.Infof("start to listen on: %v", handler.LinkType().String())
	packetChannel := gopacket.NewPacketSource(handler, handler.LinkType())
	packets := packetChannel.Packets()
	for {
		select {
		case packet, ok := <-packets:
			if !ok {
				return nil
			}

			// 限制 IP
			if ip != "" {
				nLayer := packet.NetworkLayer()
				if nLayer.LayerType() == layers.LayerTypeIPv4 {
					if ipv4Layer, ok := nLayer.(*layers.IPv4); ok && ipv4Layer != nil {
						if ipv4Layer.DstIP.String() != ip {
							continue
						}
					}
					continue
				} else if nLayer.LayerType() == layers.LayerTypeIPv6 {
					if ipv6Layer, ok := nLayer.(*layers.IPv6); ok && ipv6Layer != nil {
						if ipv6Layer.DstIP.String() != ip {
							continue
						}
					}
					continue
				}
			}

			// 限制 TCP
			tLayer := packet.TransportLayer()
			if tLayer != nil && tLayer.LayerType() == layers.LayerTypeTCP {
				if tcpLayer, ok := tLayer.(*layers.TCP); ok && tcpLayer != nil {
					if payloads := tcpLayer.LayerPayload(); len(payloads) > 0 {
						firstByte := payloads[0]
						if firstByte == 0x16 {
							// tls
							helloInfo, err := tlsutils.ParseClientHello(payloads)
							if err != nil {
								continue
							}
							log.Infof("TLS SNI Found: %v", helloInfo.SNI())
						} else if (firstByte > 'a' && firstByte < 'z') || (firstByte > 'A' && firstByte < 'Z') {
							// http guess
							if lowhttp.IsResp(payloads) {
								rsp, err := lowhttp.ParseStringToHTTPResponse(string(payloads))
								if err != nil {
									continue
								}
								log.Infof("found plain response: %v", rsp)
							} else {
								req, err := lowhttp.ParseBytesToHttpRequest(payloads)
								if err != nil {
									continue
								}
								_ = req
								u, err := lowhttp.ExtractURLFromHTTPRequestRaw(payloads, false)
								if err != nil {
									continue
								}
								log.Infof("found plain request: %v", u.String())
							}
						} else {

						}
					}
					//if tcpLayer.DstPort != layers.TCPPort(port) {
					//	continue
					//}
					//applicationLayer := packet.ApplicationLayer()
					//if applicationLayer != nil {
					//	spew.Dump(applicationLayer.LayerContents())
					//}
				}
			}
		case <-ctx.Done():
			return utils.Error("context canceled or ddl done")
		}
	}
}
