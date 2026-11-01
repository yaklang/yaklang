package pcaputil

import "strings"

const maxPacketIdentityWindow = 4096

// ProtocolContributorIdentity proves only a callback's PDU ID and arrival at
// this inspector. It does not recover fields, protocol identity or payload.
type ProtocolContributorIdentity struct {
	ID, Arrival uint64
}

// Complete describes contributor identity coverage, independently of Details.
// A false value must never be interpreted as proof of absent protocol fields.
type ProtocolContributorWindow struct {
	Identities     []ProtocolContributorIdentity
	Complete       bool
	LostReferences uint64
}

type protocolIdentityLink struct {
	ref         PacketReference
	id, arrival uint64
	cost        int
}

// This ledger outlives payload eviction but has its own bounded storage. Once
// a reference is lost, one conservative numeric watermark replaces unbounded
// per-domain tombstones. Older unrelated domains may consequently be partial.
type protocolIdentityIndex struct {
	rows                         []*protocolIdentityLink
	head, count, bytes, maxBytes int
	byExact, byPhysical          map[PacketReference][]*protocolIdentityLink
	lostMaxNumber, losses        uint64
	provenanceUnknown            bool
}

func newProtocolIdentityIndex(budget int) *protocolIdentityIndex {
	const base, perLink = 256, 320
	if budget < base+perLink+8 {
		return nil
	}
	links := min(32768, (budget-base)/(perLink+8))
	return &protocolIdentityIndex{
		rows: make([]*protocolIdentityLink, links), bytes: base + links*8, maxBytes: budget,
		byExact:    make(map[PacketReference][]*protocolIdentityLink),
		byPhysical: make(map[PacketReference][]*protocolIdentityLink),
	}
}

func (x *protocolIdentityIndex) lost(ref PacketReference) {
	x.lostMaxNumber = max(x.lostMaxNumber, ref.Number)
	x.losses++
}

func identityUnlink(index map[PacketReference][]*protocolIdentityLink, ref PacketReference) {
	rows := index[ref]
	rows[0] = nil // FIFO ingestion and eviction have the same order for each key.
	if len(rows) == 1 {
		delete(index, ref)
	} else {
		index[ref] = rows[1:]
	}
}

func (x *protocolIdentityIndex) evict() {
	row := x.rows[x.head]
	identityUnlink(x.byExact, row.ref)
	identityUnlink(x.byPhysical, physicalReference(row.ref))
	x.rows[x.head] = nil
	x.head = (x.head + 1) % len(x.rows)
	x.count--
	x.bytes -= row.cost
	x.lost(row.ref)
}

func (x *protocolIdentityIndex) observe(e *ProtocolEvent, arrival uint64) {
	if x == nil || e == nil {
		return
	}
	if e.ExpertCode == "PacketReferencesLimited" {
		// The omitted numbers are unavailable, so a numeric watermark alone
		// cannot bound their scope. Explicit identities remain usable witnesses.
		x.provenanceUnknown = true
		x.losses++
	}
	seen := make(map[PacketReference]struct{}, min(len(e.SourceBytes.PacketRefs), maxPacketIdentityWindow))
	for i, ref := range e.SourceBytes.PacketRefs {
		if ref.Number == 0 {
			continue
		}
		// Bound map/cloning work; omitted numbers still record loss without
		// allocating or hashing arbitrarily large encapsulation strings.
		if i >= maxPacketIdentityWindow || e.ID == 0 || e.Completeness == "evicted" || len(ref.Domain.Encapsulation) > x.maxBytes/2 {
			x.lost(ref)
			continue
		}
		cost := 320 + 2*len(ref.Domain.Encapsulation)
		if cost > x.maxBytes-(256+8*len(x.rows)) {
			x.lost(ref)
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		for x.count > 0 && (x.count == len(x.rows) || cost > x.maxBytes-x.bytes) {
			x.evict()
		}
		ref.Domain.Encapsulation = strings.Clone(ref.Domain.Encapsulation)
		row := &protocolIdentityLink{ref: ref, id: e.ID, arrival: arrival, cost: cost}
		x.rows[(x.head+x.count)%len(x.rows)] = row
		x.count++
		x.bytes += cost
		x.byExact[ref] = append(x.byExact[ref], row)
		physical := physicalReference(ref)
		x.byPhysical[physical] = append(x.byPhysical[physical], row)
	}
}

func (x *protocolIdentityIndex) window(ref PacketReference, physical bool, limit int) ProtocolContributorWindow {
	out := ProtocolContributorWindow{}
	if x == nil || ref.Number == 0 {
		return out
	}
	if limit <= 0 || limit > maxPacketIdentityWindow {
		limit = maxPacketIdentityWindow
	}
	rows := x.byExact[ref]
	if physical {
		rows = x.byPhysical[physicalReference(ref)]
	}
	out.Complete = !x.provenanceUnknown && ref.Number > x.lostMaxNumber
	out.LostReferences = x.losses
	seen := make(map[uint64]struct{}, min(len(rows), limit))
	for _, row := range rows {
		// Multiple tunnel references can describe one arrival's physical frame.
		// Distinct arrivals sharing a public ID must remain distinct proofs.
		if _, ok := seen[row.arrival]; ok {
			continue
		}
		if len(out.Identities) == limit {
			out.Complete = false
			break
		}
		seen[row.arrival] = struct{}{}
		out.Identities = append(out.Identities, ProtocolContributorIdentity{ID: row.id, Arrival: row.arrival})
	}
	return out
}

// PacketContributorWindow returns owned ID/arrival witnesses from actual
// OnEvent references, including callbacks whose details exceeded retention.
// physical has the globally unique record contract of PhysicalPacketEventIDs.
func (v *ProtocolInspector) PacketContributorWindow(ref PacketReference, physical bool, limit int) ProtocolContributorWindow {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.identities.window(ref, physical, limit)
}

// HistoryArrival is assigned privately by this inspector, independent of IDs.
// Compare it only with witnesses returned by the same inspector.
func (e *ProtocolEvent) HistoryArrival() uint64 {
	if e == nil {
		return 0
	}
	return e.historySequence
}

func (v *ProtocolInspector) packetDetailCoverageLocked(ref PacketReference, physical bool) bool {
	if ref.Number == 0 {
		return true // Legacy unnumbered retained-only associations have no identity proof.
	}
	x := v.identities
	if x == nil || x.provenanceUnknown || ref.Number <= x.lostMaxNumber {
		return false
	}
	rows := x.byExact[ref]
	if physical {
		rows = x.byPhysical[physicalReference(ref)]
	}
	for _, proof := range rows {
		retained := v.byID[proof.id]
		// Details selects the oldest duplicate ID; another arrival's fields
		// cannot satisfy this contributor, even when its public ID is equal.
		if len(retained) == 0 || retained[0].historySequence != proof.arrival {
			return false
		}
	}
	return true
}

func (x *protocolIdentityIndex) revision(ref PacketReference, physical bool) (arrival, losses uint64) {
	if x == nil {
		return
	}
	rows := x.byExact[ref]
	if physical {
		rows = x.byPhysical[physicalReference(ref)]
	}
	if len(rows) > 0 {
		arrival = rows[len(rows)-1].arrival
	}
	return arrival, x.losses
}
