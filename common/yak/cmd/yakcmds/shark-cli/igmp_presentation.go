package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

var igmpDetailPriority = map[string]int{
	"Version": 0, "Message": 1, "Status": 2, "Error": 3,
	"Max Response Code (raw)": 4, "Maximum Response Time (ms)": 5,
	"Report Flags (raw)": 6,
	"Group Address":      6, "Query Flags": 7, "Suppress Router-Side Processing": 8,
	"Querier Robustness Variable": 9, "Extension Flags (raw)": 10,
	"Query Interval Code (raw)": 11, "Query Interval (seconds)": 12,
	"Group Record Count": 13, "Group Records": 14,
	"Index": 0, "Record Type": 1, "Record Type Name": 2,
	"Multicast Address": 15, "Source Count": 16, "Sources": 17,
	"Auxiliary Data Words": 18, "Auxiliary Data (uninterpreted)": 19,
	"Declared Record Bytes": 20, "Additional Data (uninterpreted)": 80,
	"Observation Scope": 90,
}

// Replace legacy parsing only when the entire native projection is reproduced
// from this exact captured source, including its extent, status and completion.
// A matching label or two leading fields cannot suppress a real decoder error.
func appendIGMPCapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	r, _, selected, decodeErr := pcaputil.DecodeIGMPPacketFields(p, len(raw.data), 4096)
	if !selected || r.Fields == nil {
		return false
	}
	status, code, completeness := "decoded", "", "message"
	if decodeErr != nil {
		var failure *stream_parser.IGMPWireError
		if !errors.As(decodeErr, &failure) {
			return false
		}
		status, code, completeness = failure.Kind, failure.Code, failure.Kind
	}
	want, err := json.Marshal(r.Fields)
	if err != nil {
		return false
	}
	for _, e := range events {
		if e == nil || e.Protocol != "igmp" || e.Transport != "network" || e.Profile != "igmp-membership-wire" || e.Status != status || e.Completeness != completeness || e.ExpertCode != code || e.Error != code || e.Summary != r.Summary || e.FlowID != 0 || e.TCPFlowID != 0 || !bytes.Equal(e.Raw, raw.data) {
			continue
		}
		s := e.SourceBytes
		if s.Kind != "captured" || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256(raw.data)) || s.ParentPDU != 0 || len(s.ParentPDUs) != 0 || len(s.PacketRefs) != 1 || s.PacketRefs[0].Number != raw.number || s.PacketRefs[0].Domain != raw.reference.Domain {
			continue
		}
		if status == "decoded" {
			if s.CompletedIn == nil || *s.CompletedIn != s.PacketRefs[0] {
				continue
			}
		} else if s.CompletedIn != nil {
			continue
		}
		actual, marshalErr := json.Marshal(e.Session)
		if marshalErr != nil || !bytes.Equal(actual, want) {
			continue
		}
		var names []string
		for _, layer := range p.Layers() {
			if layer.LayerType() == layers.LayerTypeIPv4 {
				names = append(names, layer.LayerType().String())
				break
			}
			if _, network := layer.(gopacket.NetworkLayer); network || layer.LayerType() == layers.LayerTypeTCP || layer.LayerType() == layers.LayerTypeUDP {
				return false
			}
			names = append(names, layer.LayerType().String())
		}
		headers := capturedNetworkPrefix(p, raw.data, names)
		if len(headers) == 0 {
			return false
		}
		d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
		for _, h := range headers {
			d.fields = append(d.fields, field{text: h.label + " header", branch: true, start: h.start, end: h.end, hasRange: true}, field{depth: 1, text: "Source: " + h.source, start: h.sourceStart, end: h.sourceEnd, hasRange: true}, field{depth: 1, text: "Destination: " + h.destination, start: h.destStart, end: h.destEnd, hasRange: true})
		}
		return true
	}
	return false
}
