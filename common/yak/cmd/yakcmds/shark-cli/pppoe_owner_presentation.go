package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Only the private native snapshot qualifies a refusal; Session/Profile/Raw and
// JSON DTOs are display projections. Physical ranges need a second byte proof.
func pppoeHeaderRefusalPresentation(e *pcaputil.ProtocolEvent) ([]string, pcaputil.PPPoEHeaderObservation, bool) {
	v, ok := e.NativePPPoEHeaderObservation()
	if !ok || len(v.Raw) != 6 || v.Status != "context-required" && v.Status != "malformed" && v.Status != "limited" || v.ExpertCode != "PPPoEOwnerUnconfirmed" && v.ExpertCode != "PPPoEHeaderAdmission" && v.ExpertCode != "PPPoEHeaderProfile" {
		return nil, v, false
	}
	names := headerLayerNames(v.Fields["Observed Header Layers"])
	return names, v, len(names) > 0 && len(names) <= 128 && names[len(names)-1] == "PPPoE"
}

func pppoeHeaderDisplayEvent(e *pcaputil.ProtocolEvent) *pcaputil.ProtocolEvent {
	v, ok := e.NativePPPoEHeaderObservation()
	if !ok {
		return e
	}
	p := *e
	p.Protocol, p.Transport, p.Profile = "pppoe", "network", "pppoe-header-observation"
	p.Status, p.Completeness, p.ExpertCode, p.Summary = v.Status, "headers", v.ExpertCode, v.Summary
	p.Domain, p.SourceBytes, p.Session = v.Domain, v.SourceBytes, v.Fields
	p.FlowID, p.TCPFlowID, p.ResponseTo, p.TransactionID = 0, 0, 0, 0
	p.Source, p.Destination, p.Admission, p.Rule, p.Entry = "", "", "", "", ""
	p.Length, p.Direction, p.Offset = 0, 0, 0
	p.CarrierLayers = nil
	p.Raw, p.Fields, p.Structured, p.Metadata = nil, nil, nil, nil
	p.Error = ""
	return &p
}

func appendCapturedPPPoEOwnerHeaderFields(d *packetDetail, raw *capturedPacket, p gopacket.Packet, events []*pcaputil.ProtocolEvent) {
	for _, e := range events {
		names, view, ok := pppoeHeaderRefusalPresentation(e)
		if !ok || view.SourceBytes.Kind != "captured" || len(view.SourceBytes.PacketRefs) != 1 {
			continue
		}
		ref := view.SourceBytes.PacketRefs[0]
		if ref.Number != raw.number || ref.Domain != raw.reference.Domain {
			continue
		}
		offset := 0
		for i, layer := range p.Layers() {
			if i >= len(names) || layer.LayerType().String() != names[i] {
				break
			}
			h := layer.LayerContents()
			end := offset + len(h)
			if end > len(raw.data) || !bytes.Equal(h, raw.data[offset:end]) {
				break
			}
			if i != len(names)-1 {
				offset = end
				continue
			}
			_, ok := layer.(*layers.PPPoE)
			if !ok || !e.NativePPPoEHeaderWireMatches(h) || !bytes.Equal(view.Raw, h) {
				break
			}
			d.fields = append(d.fields,
				field{text: "Observed PPPoE header · child identity not admitted", branch: true, start: offset, end: end, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed PPPoE Version: %d · Type: %d", h[0]>>4, h[0]&15), start: offset, end: offset + 1, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed PPPoE Code: %d", h[1]), start: offset + 1, end: offset + 2, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed PPPoE Session ID: %d", binary.BigEndian.Uint16(h[2:4])), start: offset + 2, end: offset + 4, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed PPPoE Payload Length: %d", binary.BigEndian.Uint16(h[4:6])), start: offset + 4, end: offset + 6, hasRange: true},
				field{depth: 1, text: view.Summary},
			)
			return
		}
	}
}

type pppoeHeaderWorkerProof struct {
	Header    []byte
	Reference pcaputil.PacketReference
}

func pppoeWorkerProofFor(request decodeRequest, e *pcaputil.ProtocolEvent) *pppoeHeaderWorkerProof {
	v, ok := e.NativePPPoEHeaderObservation()
	if !ok || len(v.Raw) != 6 || v.SourceBytes.Kind != "captured" || len(v.SourceBytes.PacketRefs) != 1 || v.SourceBytes.PacketRefs[0] != request.Reference || request.Reference.Number != request.Number {
		return nil
	}
	ci := gopacket.CaptureInfo{Length: request.Length, CaptureLength: len(request.Data)}
	fresh, ok := pcaputil.ObservePPPoEHeaderAtCapture(request.Data, ci, request.Link, request.Reference)
	if !ok || !fresh.NativePPPoEHeaderWireMatches(v.Raw) {
		return nil
	}
	actual, _ := fresh.NativePPPoEHeaderObservation()
	if actual.Status != v.Status || actual.ExpertCode != v.ExpertCode {
		return nil
	}
	return &pppoeHeaderWorkerProof{Header: bytes.Clone(v.Raw), Reference: request.Reference}
}

// A metadata-only display copy can retain a negative admission explanation;
// this predicate cannot mint a native witness or any source/range authority.
func projectedPPPoEHeaderMetadata(events []*pcaputil.ProtocolEvent) bool {
	return len(events) == 1 && events[0] != nil && events[0].Protocol == "pppoe" && events[0].Profile == "pppoe-header-observation" && len(events[0].Raw) == 0
}
