package pcaputil

import (
	"crypto/sha256"
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
	Ref  PacketReference
	refs []PacketReference
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
		case *layers.Dot1Q:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/vlan:%d", v.VLANIdentifier)
		case *layers.VXLAN:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/vxlan:%s:%d", outer, v.VNI)
		case *layers.GRE:
			e.Ref.Domain.Encapsulation += fmt.Sprintf("/gre:%s:%d", outer, v.Key)
		}
	}
	return e
}
func (e *ProtocolEvent) finalizeEvidence() {
	if e.Profile == "" {
		e.Profile = e.Protocol
	}
	if e.Completeness == "" {
		e.Completeness = "envelope"
		switch e.Status {
		case "unrecognized":
			e.Completeness = "identified"
		case "incomplete", "limited", "context-required", "malformed":
			e.Completeness = e.Status
		}
	}
	if e.Session != nil && e.Session["Content Visibility"] == "encrypted" {
		e.Completeness = "encrypted"
	}
	switch e.Status {
	case "incomplete", "limited", "context-required", "malformed":
		e.Completeness = e.Status
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
	p := gopacket.NewPacket(raw, link, gopacket.Default)
	p.Metadata().CaptureInfo = ci
	return packetEvidence(p).Ref.Domain
}

// CapturePacketReference returns the reader's original record identity. Number
// is zero for a source that has not assigned packet numbers yet.
func CapturePacketReference(ci gopacket.CaptureInfo) PacketReference { return evidenceFrom(ci).Ref }

// PacketEventIDs finds retained PDUs contributed by this packet, including
// PDUs completed by later packets. Evicted history is deliberately unavailable.
func (v *ProtocolInspector) PacketEventIDs(ref PacketReference, limit int) []uint64 {
	if limit <= 0 || limit > 128 {
		limit = 128
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	var ids []uint64
	for i := 0; i < v.count; i++ {
		row := v.rows[(v.head+i)%len(v.rows)]
		if row == nil {
			continue
		}
		for _, r := range row.SourceBytes.PacketRefs {
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
