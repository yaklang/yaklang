package pcaputil

// ProtocolPacketRevision is an opaque cache token for immutable retained
// observations. Compare it only within the same inspector and query identity.
// Evictions invalidate availability (including duplicate-ID replacement).
// Reference-index overflow deliberately falls back to the global revision.
type ProtocolPacketRevision struct {
	Contributions  uint64
	InitialIDs     uint64
	Evictions      uint64
	Fallback       uint64
	Identities     uint64
	IdentityLosses uint64
}

// PacketQueryRevision avoids decoding/copying event trees on UI redraws. The
// initial IDs are the packet's bounded capture-time associations; later PDUs
// are discovered by reference. physical must follow PhysicalPacketEventIDs'
// globally unique record-number contract; false keeps exact domain matching.
// It does not promise an atomic event snapshot: cache results under the token
// read BEFORE evaluating, so a concurrent arrival invalidates the next lookup.
func (v *ProtocolInspector) PacketQueryRevision(ref PacketReference, initialIDs []uint64, physical bool) ProtocolPacketRevision {
	v.mu.Lock()
	defer v.mu.Unlock()
	r := ProtocolPacketRevision{Evictions: v.evicted}
	r.Identities, r.IdentityLosses = v.identities.revision(ref, physical)
	if physical {
		ref = physicalReference(ref)
	}
	entries := v.byPacket[ref]
	if physical {
		entries = v.byPhysical[ref]
	}
	if len(entries) > 0 {
		r.Contributions = entries[len(entries)-1].historySequence
	}
	if v.unindexed > 0 || len(initialIDs) > 128 {
		r.Fallback = v.received
	}
	for _, id := range initialIDs[:min(len(initialIDs), 128)] {
		if rows := v.byID[id]; len(rows) > 0 && rows[0].historySequence > r.InitialIDs {
			r.InitialIDs = rows[0].historySequence
		}
	}
	return r
}
