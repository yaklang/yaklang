package pcaputil

import "strings"

// Only evidence needed by a later request method is retained. Byte arrays and
// mutable events are never retained; counter witnesses have explicit limits.
type h3BodyObservation struct {
	bytes    uint64
	origins  []uint64
	refs     []PacketReference
	complete bool
}

type h3FINObservation struct {
	pdu, final uint64
	refs       []PacketReference
	complete   bool
}

func (o *h3BodyObservation) memory() int64 {
	n := int64(96 + cap(o.origins)*8 + cap(o.refs)*64)
	for _, ref := range o.refs {
		n += int64(len(ref.Domain.Encapsulation))
	}
	return n
}

func (o *h3FINObservation) memory() int64 {
	n := int64(64 + cap(o.refs)*64)
	for _, ref := range o.refs {
		n += int64(len(ref.Domain.Encapsulation))
	}
	return n
}

func h3OwnedReference(ref PacketReference) PacketReference {
	ref.Domain.Encapsulation = strings.Clone(ref.Domain.Encapsulation)
	return ref
}

func (q *binQUIC) rememberH3LateFIN(st *quicStream, dir int, sid uint64, fin bool) {
	if !fin || !q.native || q.alpn != "h3" || dir != 1 || sid&3 != 0 || q.closed || q.retired || q.eventID == 0 || st.reset || st.resetDirection[0] || st.resetDirection[1] || st.responseFIN != nil || st.h3 != nil && st.h3.reqHeaders > 0 {
		return
	}
	o := &h3FINObservation{pdu: q.eventID, final: st.reassembly[1].final, complete: len(q.eventRefs) > 0 && len(q.eventRefs) <= 64}
	for _, ref := range q.eventRefs[:min(len(q.eventRefs), 64)] {
		o.refs = append(o.refs, h3OwnedReference(ref))
		o.complete = o.complete && ref.Number != 0
	}
	st.responseFIN = o
}

func (q *binQUIC) observeH3LateBody(st *quicStream, dir int, sid uint64, frame map[string]any) {
	if !q.native || q.alpn != "h3" || dir != 1 || sid&3 != 0 || q.closed || q.retired || st.reset || st.resetDirection[0] || st.resetDirection[1] || st.h3 == nil || st.h3.response == nil || st.h3.response.linked || frame["Frame Type"] != "DATA" {
		return
	}
	n, _ := frame["DATA Length"].(uint64)
	if n == 0 {
		return
	}
	response := st.h3.response
	if response.body == nil {
		response.body = &h3BodyObservation{complete: true}
	}
	o := response.body
	o.bytes += n
	source, _ := frame["Payload Byte Source"].(map[string]any)
	parents, parentsOK := source["Parent PDUs"].([]uint64)
	refs, refsOK := source["Packet References"].([]map[string]any)
	o.complete = o.complete && parentsOK && refsOK && source["Byte Coverage Complete"] == true && source["Packet Coverage Complete"] == true && len(parents) <= 64 && len(refs) <= 64
	for _, pdu := range parents[:min(len(parents), 64)] {
		found := false
		for _, old := range o.origins {
			found = found || old == pdu
		}
		if found {
			continue
		}
		if len(o.origins) == 64 {
			o.complete = false
			continue
		}
		o.origins = append(o.origins, pdu)
	}
	for _, value := range refs[:min(len(refs), 64)] {
		number, n := value["Packet"].(uint64)
		section, s := value["Section"].(uint32)
		iface, i := value["Interface"].(int)
		encapsulation, e := value["Encapsulation"].(string)
		if !n || !s || !i || !e || number == 0 {
			o.complete = false
			continue
		}
		ref := PacketReference{Number: number, Domain: CaptureDomain{Section: section, Interface: iface, Encapsulation: encapsulation}}
		found := false
		for _, old := range o.refs {
			found = found || old == ref
		}
		if found {
			continue
		}
		if len(o.refs) == 64 {
			o.complete = false
			continue
		}
		o.refs = append(o.refs, h3OwnedReference(ref))
	}
}

func h3LateBodySource(o *h3BodyObservation, observed uint64) map[string]any {
	if o == nil {
		return map[string]any{"Observed Payload Bytes": uint64(0), "Parent PDUs": []uint64(nil), "Packet References": []map[string]any(nil), "References Complete": observed == 0}
	}
	return map[string]any{"Observed Payload Bytes": o.bytes, "Parent PDUs": append([]uint64(nil), o.origins...), "Packet References": quicSourceReferences(o.refs), "References Complete": o.complete && o.bytes == observed}
}

func (q *binQUIC) revalidateH3LateResponse(st *quicStream, sid uint64, response *h3ResponseObservation, request, info map[string]any) map[string]any {
	h3 := st.h3
	fin := st.reassembly[1].deliveredFIN && !h3.blocked[1] && !h3.framingFailure[1] && h3.parsed[1] == len(h3.buf[1])
	m, err := h3.checkBody(1, sid, fin)
	method := "other"
	if h3.requestMethod == h3MethodUnknown || h3.fieldFailure[0] != "" || h3.framingFailure[0] {
		method = "unknown"
	}
	if h3.requestMethod == h3MethodHEAD && method != "unknown" {
		method = "HEAD"
	}
	if h3.requestMethod == h3MethodCONNECT && method != "unknown" {
		method = "CONNECT"
		b := &h3.body[1]
		if b.header && b.hasLength && b.status >= 200 && b.status < 300 {
			err = protocolError(ErrMalformedMessage, "HTTP3 Content-Length forbidden for this response")
			m["Length Check"], m["Error"] = "malformed", "Content-Length forbidden for this response"
		}
	}
	m["Request Method Class"], m["Observed In PDU ID"] = method, q.eventID
	if headers, ok := request["Headers"].([]map[string]any); ok {
		for _, header := range headers {
			if header["Name"] == ":method" {
				m["Request Method"] = header["Value"]
			}
		}
	}
	m["Scope"] = "later request method checked against observed response DATA and FIN; peer delivery and representation size remain unverified"
	m["Observation Verdict"] = "no-violation-observed"
	prior := h3.fieldFailure[1]
	if prior != "" {
		m["Prior Stream Failure"] = string(prior)
		m["Observation Verdict"] = "unverified-prior-failure"
	}
	if h3.framingFailure[1] {
		m["Prior Frame Layout Failure"] = true
		m["Observation Verdict"] = "unverified-prior-failure"
	}
	if err != nil {
		m["Observation Verdict"] = "violation-observed"
		if prior == "" {
			h3.fieldFailure[1] = ErrMalformedMessage
		}
	}
	body := h3LateBodySource(response.body, h3.body[1].observed)
	m["Observed DATA Byte Source"] = body
	finSource := map[string]any{"Available": false, "References Complete": false}
	if o := st.responseFIN; o != nil {
		finSource = map[string]any{"Available": true, "PDU ID": o.pdu, "Final Stream Offset": o.final, "Packet References": quicSourceReferences(o.refs), "References Complete": o.complete}
	}
	m["Observed FIN Source"] = finSource
	m["Body Coverage Complete"] = fin && response.originsFull && response.refsFull && body["References Complete"] == true && finSource["References Complete"] == true
	if rows, ok := info["HTTP3 Tunnel Observations"].([]map[string]any); ok {
		for _, fault := range rows {
			if fault["Stream ID"] == sid {
				m["Tunnel Frame Check"] = cloneSession(fault)
				if fault["Observation Verdict"] == "violation-observed" {
					m["Observation Verdict"] = "violation-observed"
					m["Error"] = "H3_FRAME_UNEXPECTED: known non-DATA frame after successful CONNECT"
				}
				break
			}
		}
	}
	return m
}
