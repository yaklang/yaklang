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
}
type captureEvidence struct {
	tcpMissingTail uint32 // validated captured TCP payload suffix; no invented bytes
	Ref            PacketReference
	refs           []PacketReference
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
	outer := ""
	for _, l := range p.Layers() {
		if n, ok := l.(gopacket.NetworkLayer); ok {
			a, b := n.NetworkFlow().Src().String(), n.NetworkFlow().Dst().String()
			if a > b {
				a, b = b, a
			}
			outer = a + "~" + b
		}
		switch v := l.(type) {
		case *capturePIM:
			if v.Type == 1 && !v.NullRegister && v.ChecksumVerified {
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
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/vxlan:%s:%d", outer, v.VNI)
		case *layers.GRE:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/gre:%s:%d", outer, v.Key)
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
	e.SourceBytes.ParentPDUs = append([]uint64(nil), e.SourceBytes.ParentPDUs...)
	e.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
}

type contribution struct {
	start, end uint64
	ref        PacketReference
	parent     uint64
}

func (f *binFlow) contribute(dir int, n int, ref PacketReference) {
	if n == 0 {
		return
	}
	d := &f.directions[dir]
	if ref.Number == 0 {
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
	d.contributions = append(d.contributions, contribution{start: start, end: end, ref: ref})
}
func (f *binFlow) eventEvidence(dir int, e *ProtocolEvent) {
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
		return ids
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
				ids = append(ids, row.ID)
				break
			}
		}
		if len(ids) >= limit {
			break
		}
	}
	return ids
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
