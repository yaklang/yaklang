package netstackvm

import (
	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
)

// Only a capture backend may attach this evidence; it is not inferred from
// packet bytes, device names or user-configured checksum exceptions.
type captureChecksumEvidence struct {
	interfaceIndex int
	partial        bool
	validated      bool
}

// The probe receiver has already checked IPv4/TCP lengths and rejected TCP
// payloads. The zero payload checksum below applies only to these control frames.
func capturedTCPChecksumValid(packet gopacket.Packet, ip header.IPv4) bool {
	tcp := header.TCP(ip.Payload())
	if tcp.IsChecksumValid(ip.SourceAddress(), ip.DestinationAddress(), 0, 0) {
		return true
	}
	for _, value := range packet.Metadata().AncillaryData {
		evidence, ok := value.(captureChecksumEvidence)
		if !ok || evidence.interfaceIndex <= 0 || evidence.interfaceIndex != packet.Metadata().InterfaceIndex || evidence.partial == evidence.validated {
			continue
		}
		if evidence.validated {
			return true
		}
		// CHECKSUM_PARTIAL carries the uncomplemented TCP pseudo-header sum.
		// Require both kernel evidence and the expected seed, never the seed alone.
		return tcp.Checksum() == header.PseudoHeaderChecksum(header.TCPProtocolNumber, ip.SourceAddress(), ip.DestinationAddress(), uint16(len(ip.Payload())))
	}
	return false
}
