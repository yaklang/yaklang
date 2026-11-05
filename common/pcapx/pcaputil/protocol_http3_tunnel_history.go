package pcaputil

// Reuse the existing per-entry 64-edge and byte budget. These observations can
// target either direction, unlike a later request/response header association.
func (v *ProtocolInspector) observeHTTP3Tunnel(e *ProtocolEvent) {
	rows, _ := e.semanticSession["HTTP3 Tunnel Observations"].([]map[string]any)
	for _, fact := range rows {
		target, t := fact["Frame PDU ID"].(uint64)
		stream, s := fact["Stream ID"].(uint64)
		direction, d := fact["Direction"].(uint64)
		flow, f := fact["Flow ID"].(uint64)
		observed, o := fact["Observed In PDU ID"].(uint64)
		domain, c := fact["Capture Domain"].(map[string]any)
		if !t || !s || !d || !f || !o || !c || target == 0 || target == e.ID || stream&3 != 0 || direction > 1 || flow == 0 || flow != e.FlowID || observed != e.ID || domain["Section"] != e.Domain.Section || domain["Interface"] != e.Domain.Interface || domain["Encapsulation"] != e.Domain.Encapsulation {
			continue
		}
		hits := v.byID[target]
		if len(hits) != 1 {
			continue
		}
		entry := hits[0]
		if !entry.nativeQUICObservation || entry.semanticSession["Authentication Verified"] != true || entry.FlowID != flow || entry.Domain != e.Domain {
			continue
		}
		entry.http3TunnelContext = true
		duplicate := false
		for _, old := range entry.http3Associations {
			if old["Context Kind"] != "tunnel-frame-type" || old["Stream ID"] != stream || old["Direction"] != direction || old["Frame Offset"] != fact["Frame Offset"] {
				continue
			}
			duplicate = true
			if old["Frame Type Value"] != fact["Frame Type Value"] || old["Error Code"] != fact["Error Code"] {
				old["Ambiguous"], entry.http3AssociationLimited = true, true
				entry.http3ContextSequence = v.received
			}
			break
		}
		if duplicate {
			continue
		}
		cost := sessionSnapshotSize(fact, false) + 128
		entry.http3ContextSequence = v.received
		if len(entry.http3Associations) >= 64 || cost > v.maxBytes-v.bytes {
			entry.http3AssociationLimited = true
			continue
		}
		owned := cloneSession(fact)
		owned["Context Kind"] = "tunnel-frame-type"
		entry.http3Associations = append(entry.http3Associations, owned)
		entry.retainedCost += cost
		v.bytes += cost
	}
}
