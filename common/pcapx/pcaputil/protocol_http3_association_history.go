package pcaputil

// Associations are later context, not new wire PDUs. Keep them outside the
// original retained event and materialize owned copies only for current views.
// The existing ring, byte budget and eviction policy own their lifetime.
func (v *ProtocolInspector) observeHTTP3Associations(e *ProtocolEvent) {
	if !e.nativeQUICObservation || e.semanticSession["Authentication Verified"] != true {
		return
	}
	links, _ := e.semanticSession["HTTP3 Late Associations"].([]map[string]any)
	// Native frame budgets already bound this observation. Do not silently
	// discard the 65th target: each retained response has its own edge budget.
	for _, link := range links {
		response, r := link["Response PDU ID"].(uint64)
		request, q := link["Request PDU ID"].(uint64)
		stream, s := link["Stream ID"].(uint64)
		flow, f := link["Flow ID"].(uint64)
		observed, o := link["Observed In PDU ID"].(uint64)
		domain, d := link["Capture Domain"].(map[string]any)
		if !r || !q || !s || !f || !o || !d || response == 0 || request == 0 || stream&3 != 0 || flow == 0 || flow != e.FlowID || observed != e.ID ||
			domain["Section"] != e.Domain.Section || domain["Interface"] != e.Domain.Interface || domain["Encapsulation"] != e.Domain.Encapsulation {
			continue
		}
		rows := v.byID[response]
		if len(rows) != 1 {
			continue // absent/evicted/ambiguous IDs cannot be revived by context
		}
		row := rows[0]
		if !row.nativeQUICObservation || row.semanticSession["Authentication Verified"] != true || row.FlowID != flow || row.Domain != e.Domain {
			continue
		}
		duplicate := false
		for _, old := range row.http3Associations {
			if old["Stream ID"] == stream {
				duplicate = true
				if old["Request PDU ID"] != request {
					old["Ambiguous"] = true
					row.http3AssociationLimited = true
					row.http3ContextSequence = v.received
				}
				break
			}
		}
		if duplicate {
			continue
		}
		cost := sessionSnapshotSize(link, false) + 64
		row.http3ContextSequence = v.received
		if len(row.http3Associations) == 64 || cost > v.maxBytes-v.bytes {
			row.http3AssociationLimited = true
			continue
		}
		row.http3Associations = append(row.http3Associations, cloneSession(link))
		row.retainedCost += cost
		v.bytes += cost
	}
}

func (entry *protocolHistoryEntry) materializeHTTP3Associations(e *ProtocolEvent) {
	if len(entry.http3Associations) == 0 && !entry.http3AssociationLimited {
		return
	}
	if e.Session == nil {
		e.Session = make(map[string]any)
	}
	if e.semanticSession == nil {
		e.semanticSession = make(map[string]any)
	}
	for _, session := range []map[string]any{e.Session, e.semanticSession} {
		// Ambiguous edges remain visible but do not yield resolved_response_to.
		// A budget limit still exposes the already retained validated relations.
		session["HTTP3 Resolved Associations"] = cloneSessionValue(entry.http3Associations)
		session["HTTP3 Association View"] = map[string]any{"Scope": "later retained header associations; original wire observation and byte contributors are unchanged", "Complete": !entry.http3AssociationLimited}
	}
	e.nativeFieldsIncomplete = e.nativeFieldsIncomplete || entry.http3AssociationLimited
}
