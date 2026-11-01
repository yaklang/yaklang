package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// CaptureDomain is part of the connection identity, not an endpoint role.
// Encapsulation records observed VLAN/tunnel identifiers in outer-to-inner order.
type CaptureDomain struct {
	Section       uint32
	Interface     int
	Encapsulation string
}
type PacketReference struct {
	Number uint64
	Domain CaptureDomain
}

// ByteSource identifies logical bytes. Packet references identify contributors;
// they deliberately do not invent offsets for decrypted/reassembled fields.
type ByteSource struct {
	Kind       string
	SHA256     string
	ParentPDU  uint64
	ParentPDUs []uint64
	PacketRefs []PacketReference
	// CompletedIn is the latest numbered byte contributor of a complete
	// decoded observation, not necessarily the frame where another dissector
	// chooses to display it or the later time when context became available.
	CompletedIn *PacketReference `json:",omitempty"`
}
type captureEvidence struct {
	byteSourceKind string
	carrierLayers  []string
	parents        []uint64
	carrierOwned   bool // refs retain physical domains after pseudowire normalization
	// Header layers removed by validated tunnel normalization / IP reassembly.
	// Names carry no synthetic byte offsets or application identity.
	networkPrefix        []gopacket.LayerType
	networkPrefixLimited bool
	tcpMissingTail       uint32 // validated captured TCP payload suffix; no invented bytes
	Ref                  PacketReference
	refs                 []PacketReference
	// Applies to the current IP packet, not an outer tunnel carrier.
	ipv6Fragment bool
	// Count normalization across reconstructed outer/inner fragment layers.
	normalizedTunnels int
}

func evidenceFrom(ci gopacket.CaptureInfo) captureEvidence {
	for _, v := range ci.AncillaryData {
		if e, ok := v.(captureEvidence); ok {
			return e
		}
	}
	return captureEvidence{Ref: PacketReference{Domain: CaptureDomain{Interface: ci.InterfaceIndex}}}
}
func withEvidence(ci gopacket.CaptureInfo, e captureEvidence) gopacket.CaptureInfo {
	data := make([]interface{}, 0, len(ci.AncillaryData)+1)
	for _, v := range ci.AncillaryData {
		if _, ok := v.(captureEvidence); !ok {
			data = append(data, v)
		}
	}
	ci.AncillaryData = append(data, e)
	return ci
}
func packetEvidence(p gopacket.Packet) captureEvidence {
	e := evidenceFrom(p.Metadata().CaptureInfo)
	outer, directionalOuter := "", ""
	for _, l := range p.Layers() {
		if n, ok := l.(gopacket.NetworkLayer); ok {
			a, b := n.NetworkFlow().Src().String(), n.NetworkFlow().Dst().String()
			directionalOuter = a + ">" + b
			if a > b {
				a, b = b, a
			}
			outer = a + "~" + b
		}
		switch v := l.(type) {
		case *capturePIM:
			if v.Type == 1 && !v.NullRegister && v.ChecksumVerified && v.MessageExtentComplete {
				e.Ref.Domain.Encapsulation += "/pim-register:" + outer
			}
		case *captureERSPAN3:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/erspan3:%d:%d:%t:%d:%d", v.SessionID, v.HardwareID, v.OptionalPlatform, v.PlatformID, v.SourceIndex)
			if v.PlatformID == 1 && v.OptionalPlatform {
				e.Ref.Domain.Encapsulation += fmt.Sprintf(":vsm%d", binary.BigEndian.Uint32(v.contents[12:])&0xfff)
			}
		case *captureMPacket:
			e.Ref.Domain.Encapsulation += "/mpacket:express"
		case *layers.PPPoE:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/pppoe:%d", v.SessionId)
		case *layers.IPv6:
			if v.NextHeader == layers.IPProtocolIPv4 || v.NextHeader == layers.IPProtocolIPv6 {
				e.Ref.Domain.Encapsulation += fmt.Sprintf("/ip6-tunnel:%s:%d", outer, v.NextHeader)
			}
		case *capturePPILayer:
			e.Ref.Domain.Encapsulation += "/ppi:ethernet"
			if v.aggregation {
				e.Ref.Domain.Encapsulation += fmt.Sprintf("/aggregation:%d", v.interfaceID)
			}
		case *layers.Dot1Q:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/vlan:%d", v.VLANIdentifier)
		case *layers.VXLAN:
			if len(v.LayerContents()) >= 7 {
				e.Ref.Domain.Encapsulation += fmt.Sprintf("/vxlan:%s:%d", outer, v.VNI)
			}
		case *captureVXLANGPE:
			h := v.LayerContents()
			vni := uint32(h[4])<<16 | uint32(h[5])<<8 | uint32(h[6])
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/vxlan-gpe:%s:%d", outer, vni)
		case *layers.GRE:
			if directionalOuter != "" && selectedPPTPGRE(v) {
				// RFC 2637 divides Key into changing Payload Length and the
				// receiver's Call ID. Missing call-control traffic does not
				// authorize pairing the peers' independently assigned IDs.
				e.Ref.Domain.Encapsulation += fmt.Sprintf("/pptp:%s:%d", directionalOuter, uint16(v.Key))
			} else {
				e.Ref.Domain.Encapsulation += fmt.Sprintf("/gre:%s:%d", outer, v.Key)
			}
		case *layers.Geneve:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/geneve:%s:%d", outer, v.VNI)
		case *layers.IPv4:
			// A fragmented outer packet has no decoded inner carrier yet. Defer
			// that domain suffix until reassembly instead of applying it twice.
			if v.FragOffset == 0 && v.Flags&layers.IPv4MoreFragments == 0 {
				e.Ref.Domain.Encapsulation += ipEncapsulation(v.Protocol, outer)
			}
		}
	}
	return e
}

func ipEncapsulation(protocol layers.IPProtocol, outer string) string {
	switch protocol {
	case layers.IPProtocolIPv4:
		return "/ipip:" + outer
	case layers.IPProtocolIPv6:
		return "/6in4:" + outer
	default:
		return ""
	}
}
func (e *ProtocolEvent) finalizeEvidence() {
	e.SourceBytes.CompletedIn = nil
	if (e.Status == "decoded" || e.Status == "deferred") && len(e.Raw) > 0 {
		for _, ref := range e.SourceBytes.PacketRefs {
			if ref.Number != 0 && (e.SourceBytes.CompletedIn == nil || ref.Number > e.SourceBytes.CompletedIn.Number) {
				owned := ref
				e.SourceBytes.CompletedIn = &owned
			}
		}
	}
	if e.Profile == "" {
		e.Profile = e.Protocol
	}
	if e.Completeness == "" {
		e.Completeness = "envelope"
		switch e.Status {
		case "unrecognized":
			e.Completeness = "unrecognized"
		case "incomplete", "limited", "context-required", "malformed":
			e.Completeness = e.Status
		}
	}
	if e.Session != nil && e.Session["Content Visibility"] == "encrypted" {
		e.Completeness = "encrypted"
	}
	switch e.Status {
	case "incomplete", "limited", "context-required", "malformed":
		if e.Completeness != "headers" {
			e.Completeness = e.Status
		}
	}
	if e.SourceBytes.Kind == "" {
		e.SourceBytes.Kind = "captured"
		if e.Transport == "tcp" {
			e.SourceBytes.Kind = "reassembled"
		}
	}
	if len(e.Raw) > 0 {
		sum := sha256.Sum256(e.Raw)
		var encoded [64]byte
		hex.Encode(encoded[:], sum[:])
		e.SourceBytes.SHA256 = string(encoded[:])
	}
	if e.sessionError != nil {
		e.ExpertCode = string(e.sessionError.Kind)
	}
}
func cloneEvidence(e *ProtocolEvent) {
	if e.SourceBytes.CompletedIn != nil {
		owned := *e.SourceBytes.CompletedIn
		e.SourceBytes.CompletedIn = &owned
	}
	e.CarrierLayers = append([]string(nil), e.CarrierLayers...)
	e.SourceBytes.ParentPDUs = append([]uint64(nil), e.SourceBytes.ParentPDUs...)
	e.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
}

type contribution struct {
	start, end uint64
	ref        PacketReference
	parent     uint64
}

func (f *binFlow) contribute(dir int, n int, ref PacketReference, parent ...uint64) {
	if n == 0 {
		return
	}
	d := &f.directions[dir]
	if ref.Number == 0 && (len(parent) == 0 || parent[0] == 0) {
		d.evidenceLimited = true
		return
	}
	if int64(f.a.config.MaxBufferedBytes)-f.a.buffered.Load() < int64(f.a.config.MaxMessageBytes)+80 || !f.a.reserveEvidence(80) {
		d.evidenceLimited = true
		return
	}
	start := d.offset + uint64(len(d.buffer))
	end := start + uint64(n)
	if len(d.contributions) >= f.a.budget.MaxCollectionElements {
		d.evidenceLimited = true
		f.a.buffered.Add(-80)
		return
	}
	item := contribution{start: start, end: end, ref: ref}
	if len(parent) > 0 {
		item.parent = parent[0]
	}
	d.contributions = append(d.contributions, item)
}
func (f *binFlow) eventEvidence(dir int, e *ProtocolEvent) {
	if len(f.captureCarriers) > 0 {
		e.CarrierLayers = append(append([]string(nil), f.captureCarriers...), e.CarrierLayers...)
	}
	e.Domain = f.domain
	e.SourceBytes.Kind = f.byteSource
	e.SourceBytes.ParentPDU = f.parentID
	d := &f.directions[dir]
	end := e.Offset + uint64(e.Length)
	for _, s := range d.contributions {
		if s.start < end && s.end > e.Offset {
			if s.parent != 0 {
				if len(e.SourceBytes.ParentPDUs) == 0 || e.SourceBytes.ParentPDUs[len(e.SourceBytes.ParentPDUs)-1] != s.parent {
					e.SourceBytes.ParentPDUs = append(e.SourceBytes.ParentPDUs, s.parent)
				}
			}
			if s.ref.Number != 0 && (len(e.SourceBytes.PacketRefs) == 0 || e.SourceBytes.PacketRefs[len(e.SourceBytes.PacketRefs)-1] != s.ref) {
				e.SourceBytes.PacketRefs = append(e.SourceBytes.PacketRefs, s.ref)
			}
		}
	}
	// Context discovery can release bytes queued in the opposite direction.
	// The latest connection-wide parent is not necessarily this message's
	// contributor. Keep the scalar link consistent with its byte provenance.
	if len(e.SourceBytes.ParentPDUs) != 0 {
		e.SourceBytes.ParentPDU = e.SourceBytes.ParentPDUs[len(e.SourceBytes.ParentPDUs)-1]
	}
	if d.evidenceLimited {
		e.ExpertCode = "PacketReferencesLimited"
	}
}
func (d *binDirection) pruneContributions(a *binParser) {
	n := 0
	for _, v := range d.contributions {
		if v.end > d.offset {
			d.contributions[n] = v
			n++
		}
	}
	a.buffered.Add(-int64(len(d.contributions)-n) * 80)
	clear(d.contributions[n:])
	d.contributions = d.contributions[:n]
	if n == 0 {
		d.contributions = nil
		d.evidenceLimited = false
	}
}

func (a *binParser) reserveEvidence(n int64) bool {
	for {
		current := a.buffered.Load()
		if n > int64(a.config.MaxBufferedBytes)-current {
			return false
		}
		if a.buffered.CompareAndSwap(current, current+n) {
			for peak := a.peak.Load(); current+n > peak && !a.peak.CompareAndSwap(peak, current+n); peak = a.peak.Load() {
			}
			return true
		}
	}
}

// PacketDomain returns the observed capture section/interface and tunnel domain.
func PacketDomain(raw []byte, ci gopacket.CaptureInfo, link layers.LinkType) CaptureDomain {
	p := gopacket.NewPacket(raw, captureLinkDecoder(link), gopacket.Default)
	p.Metadata().CaptureInfo = ci
	return packetEvidence(p).Ref.Domain
}

// CapturePacketReference returns the reader's original record identity. Number
// is zero for a source that has not assigned packet numbers yet.
func CapturePacketReference(ci gopacket.CaptureInfo) PacketReference { return evidenceFrom(ci).Ref }

// PacketEventIDs finds retained PDUs contributed by this packet, including
// PDUs completed by later packets. Evicted history is deliberately unavailable.
func (v *ProtocolInspector) PacketEventIDs(ref PacketReference, limit int) []uint64 {
	return v.packetEventIDs(ref, limit, false)
}

// PhysicalPacketEventIDs finds all retained PDUs contributing to a capture
// record, including an encapsulation discovered after outer reassembly. It
// ignores Encapsulation but preserves section/interface. Use only when record
// numbers are globally unique in that capture, as assigned by CaptureReader.
// PacketEventIDs retains exact domain matching for caller-supplied references.
func (v *ProtocolInspector) PhysicalPacketEventIDs(ref PacketReference, limit int) []uint64 {
	return v.packetEventIDs(ref, limit, true)
}
func (v *ProtocolInspector) packetEventIDs(ref PacketReference, limit int, physical bool) []uint64 {
	ids, _ := v.packetEventWindow(ref, limit, physical)
	return ids
}

// PacketEventWindow returns up to 128 retained arrival IDs and whether the
// window covers the known contributor arrivals and their Details binding.
// Detail eviction, identity-index loss and duplicate-ID ambiguity are partial,
// even on a packet's first query. Consumers must not infer absent fields.
// physical has the same globally unique record contract as PhysicalPacketEventIDs.
func (v *ProtocolInspector) PacketEventWindow(ref PacketReference, physical bool) (ids []uint64, complete bool) {
	return v.packetEventWindow(ref, 128, physical)
}

func (v *ProtocolInspector) packetEventWindow(ref PacketReference, limit int, physical bool) ([]uint64, bool) {
	if limit <= 0 || limit > 128 {
		limit = 128
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	var ids []uint64
	if physical {
		ref = physicalReference(ref)
	}
	if v.unindexed == 0 {
		entries := v.byPacket[ref]
		if physical {
			entries = v.byPhysical[ref]
		}
		for _, row := range entries {
			ids = append(ids, row.ID)
			if len(ids) >= limit {
				break
			}
		}
		return ids, len(entries) <= limit && v.packetDetailCoverageLocked(ref, physical)
	}
	for i := 0; i < v.count; i++ {
		row := v.rows[(v.head+i)%len(v.rows)]
		if row == nil {
			continue
		}
		for _, r := range row.SourceBytes.PacketRefs {
			if physical {
				r = physicalReference(r)
			}
			if r == ref {
				if len(ids) == limit {
					return ids, false
				}
				ids = append(ids, row.ID)
				break
			}
		}
	}
	return ids, v.packetDetailCoverageLocked(ref, physical)
}

// CapturePacketEvidence includes the packet's observed encapsulation domain.
// It preserves the original capture-record number through decoding.
func CapturePacketEvidence(packet gopacket.Packet) PacketReference { return packetEvidence(packet).Ref }

// CapturePacketReferences also includes contributors to network reassembly.
func CapturePacketReferences(packet gopacket.Packet) []PacketReference {
	e := packetEvidence(packet)
	if len(e.refs) > 0 {
		return append([]PacketReference(nil), e.refs...)
	}
	if e.Ref.Number == 0 {
		return nil
	}
	return []PacketReference{e.Ref}
}

const maxCaptureTransportLayers = 128

func (e *captureEvidence) preserveNetworkPrefix(packet gopacket.Packet, until gopacket.Layer) {
	if e.networkPrefixLimited {
		return
	}
	prefix := append([]gopacket.LayerType(nil), e.networkPrefix...)
	for _, layer := range packet.Layers() {
		if layer == until {
			break
		}
		if layer.LayerType() == gopacket.LayerTypePayload || layer.LayerType() == gopacket.LayerTypeDecodeFailure {
			continue
		}
		if len(prefix) >= maxCaptureTransportLayers {
			e.networkPrefix = nil
			e.networkPrefixLimited = true
			return
		}
		prefix = append(prefix, layer.LayerType())
	}
	e.networkPrefix = prefix
}

// CaptureTransportLayers returns owned names of actual header layers through
// the selected transport, including envelopes removed by network reassembly.
// False completeness exposes the bounded name projection's limit; it neither
// prevents native processing nor implies transport payload acceptance.
func CaptureTransportLayers(packet gopacket.Packet) ([]string, bool) {
	if packet == nil {
		return nil, false
	}
	e := packetEvidence(packet)
	_, transport := CaptureTransport(packet)
	if transport == nil {
		return nil, false
	}
	out := make([]string, 0, len(e.networkPrefix)+8)
	for _, typ := range e.networkPrefix {
		out = append(out, typ.String())
	}
	for _, layer := range packet.Layers() {
		if layer.LayerType() != gopacket.LayerTypePayload && layer.LayerType() != gopacket.LayerTypeDecodeFailure {
			if len(out) >= maxCaptureTransportLayers {
				return out, false
			}
			out = append(out, layer.LayerType().String())
		}
		if layer == transport {
			break
		}
	}
	return out, !e.networkPrefixLimited
}
