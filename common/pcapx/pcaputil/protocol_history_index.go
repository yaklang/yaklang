package pcaputil

// History lookup must not scan all retained PDUs for every packet on every UI
// repaint. Index links are bounded independently; oversized reference graphs
// use the original exact scan until their unindexed entries are evicted.
func uniquePacketRefs(refs []PacketReference, visit func(PacketReference)) {
	if len(refs) == 1 {
		visit(refs[0])
		return
	}
	seen := make(map[PacketReference]struct{}, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref]; !ok {
			seen[ref] = struct{}{}
			visit(ref)
		}
	}
}
func (v *ProtocolInspector) index(row *protocolHistoryEntry) {
	v.byID[row.ID] = append(v.byID[row.ID], row)
	refs := row.SourceBytes.PacketRefs
	if len(refs) > (v.packetIndexLimit-v.packetIndexLinks)/2 {
		v.unindexed++
		return
	}
	row.packetIndexed = true
	uniquePacketRefs(refs, func(ref PacketReference) { v.byPacket[ref] = append(v.byPacket[ref], row); v.packetIndexLinks++ })
	uniquePhysicalRefs(refs, func(ref PacketReference) { v.byPhysical[ref] = append(v.byPhysical[ref], row); v.packetIndexLinks++ })
}
func (v *ProtocolInspector) unindex(row *protocolHistoryEntry) {
	entries := v.byID[row.ID]
	entries[0] = nil
	if len(entries) == 1 {
		delete(v.byID, row.ID)
	} else {
		v.byID[row.ID] = entries[1:]
	}
	if !row.packetIndexed {
		v.unindexed--
		return
	}
	uniquePacketRefs(row.SourceBytes.PacketRefs, func(ref PacketReference) {
		entries := v.byPacket[ref]
		entries[0] = nil
		if len(entries) == 1 {
			delete(v.byPacket, ref)
		} else {
			v.byPacket[ref] = entries[1:]
		}
		v.packetIndexLinks--
	})
	uniquePhysicalRefs(row.SourceBytes.PacketRefs, func(ref PacketReference) {
		entries := v.byPhysical[ref]
		entries[0] = nil
		if len(entries) == 1 {
			delete(v.byPhysical, ref)
		} else {
			v.byPhysical[ref] = entries[1:]
		}
		v.packetIndexLinks--
	})
}

func physicalReference(ref PacketReference) PacketReference {
	ref.Domain.Encapsulation = ""
	return ref
}
func uniquePhysicalRefs(refs []PacketReference, visit func(PacketReference)) {
	if len(refs) == 1 {
		visit(physicalReference(refs[0]))
		return
	}
	seen := make(map[PacketReference]struct{}, len(refs))
	for _, ref := range refs {
		ref = physicalReference(ref)
		if _, ok := seen[ref]; !ok {
			seen[ref] = struct{}{}
			visit(ref)
		}
	}
}
