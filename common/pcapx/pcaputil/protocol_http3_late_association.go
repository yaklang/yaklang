package pcaputil

// A header observation can be completed by QPACK context in another PDU.
// Its observation ID and original byte contributors are separate namespaces.
// Retain only bounded references, never response payloads or mutable events.
type h3ResponseObservation struct {
	pdu                   uint64
	origins               []uint64
	refs                  []PacketReference
	originsFull, refsFull bool
	linked                bool
	body                  *h3BodyObservation
}

const h3LateAssociationScope = "validated header field sections on the current captured QUIC connection and stream; not whole-message conformance or peer delivery"

func (o *h3ResponseObservation) memory() int64 {
	n := int64(128 + cap(o.origins)*8 + cap(o.refs)*64)
	for _, ref := range o.refs {
		n += int64(len(ref.Domain.Encapsulation))
	}
	if o.body != nil {
		n += o.body.memory()
	}
	return n
}

func h3HeaderObservation(pdu uint64, frame map[string]any) *h3ResponseObservation {
	o := &h3ResponseObservation{pdu: pdu}
	source, _ := frame["Byte Source"].(map[string]any)
	if parents, ok := source["Parent PDUs"].([]uint64); ok {
		o.origins = append([]uint64(nil), parents[:min(len(parents), 64)]...)
		o.originsFull = len(parents) == len(o.origins) && source["Byte Coverage Complete"] == true
	}
	if refs, ok := source["Packet References"].([]map[string]any); ok {
		o.refsFull = len(refs) <= 64 && source["Packet Coverage Complete"] == true
		for _, ref := range refs[:min(len(refs), 64)] {
			number, n := ref["Packet"].(uint64)
			section, s := ref["Section"].(uint32)
			iface, i := ref["Interface"].(int)
			encapsulation, e := ref["Encapsulation"].(string)
			if !n || !s || !i || !e {
				o.refsFull = false
				continue
			}
			o.refs = append(o.refs, PacketReference{Number: number, Domain: CaptureDomain{Section: section, Interface: iface, Encapsulation: encapsulation}})
		}
	}
	return o
}

func (q *binQUIC) observeH3HeaderAssociation(st *quicStream, dir int, sid uint64, frame, info map[string]any) {
	if !q.native || q.alpn != "h3" || q.closed || q.retired || q.eventID == 0 || sid&3 != 0 || st.reset || st.resetDirection[0] || st.resetDirection[1] || frame["Frame Type"] != "HEADERS" {
		return
	}
	h3 := st.h3
	q.recheckH3Tunnel(st, sid, info)
	if dir == 0 && frame["Header Kind"] == "request" && h3.response == nil {
		st.responseFIN = nil // a future response will already know this method
	}
	if dir == 1 && frame["Header Kind"] == "response" && h3.response == nil {
		if h3.reqHeaders > 0 && st.requestPDU != 0 {
			return
		}
		h3.response = h3HeaderObservation(q.eventID, frame)
		return
	}
	if dir != 0 || frame["Header Kind"] != "request" || st.requestPDU == 0 || h3.response == nil || h3.response.linked {
		return
	}
	response := h3.response
	request := h3HeaderObservation(q.eventID, frame)
	link := map[string]any{
		"Stream ID": sid, "Request PDU ID": st.requestPDU, "Request Headers PDU ID": q.eventID,
		"Response PDU ID": response.pdu, "Association Scope": h3LateAssociationScope,
		"Request Header Origin PDUs": request.origins, "Request Header Packet References": quicSourceReferences(request.refs),
		"Request Header References Complete": request.originsFull && request.refsFull,
		"Response Header Origin PDUs":        append([]uint64(nil), response.origins...), "Response Header Packet References": quicSourceReferences(response.refs),
		"Response Header References Complete": response.originsFull && response.refsFull,
		"PDU Namespace":                       "header observation PDUs; byte-origin PDUs and packet references are listed separately",
	}
	link["Response Message Revalidation"] = q.revalidateH3LateResponse(st, sid, response, frame, info)
	links, _ := info["HTTP3 Late Associations"].([]map[string]any)
	info["HTTP3 Late Associations"] = append(links, link)
	// Existing response snapshots and their ResponseTo values are immutable.
	// This request observation supplies the later connection/stream relation.
	frame["Late Associated Response PDU"] = response.pdu
	response.linked = true
	response.origins, response.refs = nil, nil
	response.body, st.responseFIN = nil, nil
}
