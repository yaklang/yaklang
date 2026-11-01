package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The dependency replaces IPv4's zero Total Length with the captured length
// for TSO compatibility. Preserve its transport view, but never present that
// effective length as the literal field or certify that offload occurred.
func capturedZeroLengthIPv4(wire []byte, ip *layers.IPv4, at int) bool {
	header := ip.LayerContents()
	return len(header) >= 20 && header[0]>>4 == 4 && int(header[0]&15)*4 == len(header) &&
		binary.BigEndian.Uint16(header[2:4]) == 0 && at >= 0 && at <= len(wire)-len(header) &&
		bytes.Equal(header, wire[at:at+len(header)])
}

func hasCapturedZeroLengthIPv4(raw *capturedPacket, view *packetTransportView) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	at := 0
	for _, l := range inspectionPacketLayers(p, view) {
		if ip, ok := l.(*layers.IPv4); ok && capturedZeroLengthIPv4(raw.data, ip, at) {
			return true
		}
		at += len(l.LayerContents())
	}
	return false
}

func appendZeroLengthIPv4Fields(d *packetDetail, wire []byte, ip *layers.IPv4, at int) bool {
	if !capturedZeroLengthIPv4(wire, ip, at) {
		return false
	}
	header := ip.LayerContents()
	d.fields = append(d.fields, field{text: "IPv4 · captured-prefix observation", branch: true, start: at, end: at + len(header), hasRange: true})
	add := func(name string, value any, start, end int) {
		d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("%s: %v", name, value), start: at + start, end: at + end, hasRange: true})
	}
	add("Version", header[0]>>4, 0, 1)
	add("Header Length", fmt.Sprintf("%d bytes (%d words)", len(header), header[0]&15), 0, 1)
	add("Type of Service", fmt.Sprintf("0x%02x", header[1]), 1, 2)
	add("Total Length", 0, 2, 4)
	add("Identification", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(header[4:6])), 4, 6)
	add("Flags and Fragment Offset", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(header[6:8])), 6, 8)
	add("TTL", header[8], 8, 9)
	add("Protocol", header[9], 9, 10)
	add("Header Checksum", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(header[10:12])), 10, 12)
	add("Source", net.IP(header[12:16]).String(), 12, 16)
	add("Destination", net.IP(header[16:20]).String(), 16, 20)
	if len(header) > 20 {
		add("Captured Options", fmt.Sprintf("%x", header[20:]), 20, len(header))
	}
	d.fields = append(d.fields,
		field{depth: 1, text: "Parent scope: captured prefix; IPv4 declared extent unavailable"},
		field{depth: 1, text: "Offload: TSO origin unconfirmed"},
		field{depth: 1, text: fmt.Sprintf("Captured Prefix Bytes: %d (not IPv4 Total Length)", len(header)+len(ip.LayerPayload()))},
	)
	return true
}
