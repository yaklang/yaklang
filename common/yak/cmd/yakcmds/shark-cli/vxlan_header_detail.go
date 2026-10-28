package sharkcli

import (
	"bytes"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Put captured tunnel facts before the logical application PDU. This is a
// bounded byte-backed observation, not another application dissector. Callers
// first apply the native parent-admission barrier to both inspector paths.
func appendCapturedVXLANHeaders(d *packetDetail, raw *capturedPacket) {
	for _, event := range raw.protocolEvents() {
		if event != nil && event.Transport == "udp" && (event.Protocol == "" || event.Protocol == "udp") && event.Error != "" {
			// A legacy observer can walk a port-selected payload after native
			// UDP carrier admission failed. Do not promote that speculative
			// view into this captured-header observation.
			return
		}
	}
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	offset, count := 0, 0
	for _, layer := range p.Layers() {
		if offset > len(raw.data) || len(layer.LayerContents()) > len(raw.data)-offset {
			return
		}
		start := offset
		offset += len(layer.LayerContents())
		if layer.LayerType() == layers.LayerTypeICMPv4 || layer.LayerType() == layers.LayerTypeICMPv6 {
			return // An ICMP quotation is not a second captured tunnel.
		}
		v, ok := layer.(*layers.VXLAN)
		if !ok {
			continue
		}
		if len(v.LayerContents()) != 8 || !v.ValidIDFlag || !bytes.Equal(raw.data[start:offset], v.LayerContents()) {
			return
		}
		count++
		if count > 4 {
			return
		}
		d.fields = append(d.fields, field{text: fmt.Sprintf("Captured VXLAN header %d", count), branch: true, hasRange: true, start: start, end: offset})
		add := func(text string, lo, hi int) {
			d.fields = append(d.fields, field{depth: 1, text: text, hasRange: true, start: start + lo, end: start + hi})
		}
		add(fmt.Sprintf("Flags: 0x%02x", v.LayerContents()[0]), 0, 1)
		add("I: true", 0, 1)
		add(fmt.Sprintf("VNI: %d", v.VNI), 4, 7)
		add(fmt.Sprintf("G: %t", v.GBPExtension), 0, 1)
		if v.GBPExtension {
			add(fmt.Sprintf("GBP Don't Learn: %t", v.GBPDontLearn), 1, 2)
			add(fmt.Sprintf("GBP Applied: %t", v.GBPApplied), 1, 2)
			add(fmt.Sprintf("GBP Policy ID: %d", v.GBPGroupPolicyID), 2, 4)
		}
	}
}
