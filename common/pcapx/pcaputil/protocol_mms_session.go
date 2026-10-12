package pcaputil

import (
	"bytes"
	"fmt"
	"time"
)

type mmsPending struct {
	binding       *mmsListBinding
	control       *mmsListControl
	query         map[string]any
	names         []map[string]any
	specification bool
	event         uint64
	service       byte
	items         int
}
type mmsTSDU struct {
	wire   []byte
	events []uint64
	refs   []PacketReference
	first  time.Time
}
type binMMS struct {
	listObserved                                    bool
	listClock                                       uint64
	lists                                           [2]map[mmsListKey]mmsListEntry
	listVersions                                    [2]map[mmsListKey]uint64
	values                                          bool
	validateDescriptions                            bool
	nesting                                         uint32
	hasNesting                                      bool
	proposedNesting                                 uint32
	hasProposedNesting                              bool
	pduLimit                                        [2]uint32
	initAccepted, released                          bool
	concludeDir, releaseDir                         int
	supported                                       [2][]byte
	client                                          int
	crEvent, initEvent, concludeEvent, releaseEvent uint64
	crRef, ccRef                                    uint16
	tpdu                                            int
	transport, active, concluded, poison            bool
	offered, contexts                               []mmsPresentationContext
	accepted                                        []bool
	proposed                                        [2]uint32
	limits                                          [2]uint32
	pending                                         [2]map[uint32]mmsPending
	retired                                         [2]map[uint32]bool
	fragment                                        [2]mmsTSDU
}

func newBinMMS() *binMMS {
	return &binMMS{client: -1, tpdu: 65531, pending: [2]map[uint32]mmsPending{{}, {}}, retired: [2]map[uint32]bool{{}, {}}}
}
func (m *binMMS) storage() int64 {
	n := int64(256 + len(m.supported[0]) + len(m.supported[1]) + (len(m.pending[0])+len(m.pending[1]))*96 + (len(m.retired[0])+len(m.retired[1]))*32)
	for _, pending := range m.pending {
		for _, p := range pending {
			n += mmsQueryStorage(p.query) + mmsNamesStorage(p.names)
		}
	}
	n += m.namedStorage()
	for _, c := range m.offered {
		n += 192 + int64(len(c.syntax))
		for _, t := range c.transfers {
			n += 32 + int64(len(t))
		}
	}
	for _, x := range m.fragment {
		n += int64(len(x.wire) + len(x.events)*8 + len(x.refs)*192)
	}
	return n
}
func (f *binFlow) mmsReserve(target int64) error {
	delta := target - f.sessionBytes
	if delta <= 0 {
		f.a.buffered.Add(delta)
		f.sessionBytes = target
		return nil
	}
	for {
		now := f.a.buffered.Load()
		if target < 0 || now+delta > int64(f.a.config.MaxBufferedBytes) {
			return protocolError(ErrResourceExceeded, "MMS shared context/projection budget")
		}
		if f.a.buffered.CompareAndSwap(now, now+delta) {
			f.sessionBytes = target
			for peak := f.a.peak.Load(); now+delta > peak; peak = f.a.peak.Load() {
				if f.a.peak.CompareAndSwap(peak, now+delta) {
					break
				}
			}
			return nil
		}
	}
}
func (m *binMMS) refuse() {
	m.poison = true
	m.active = false
	m.pending = [2]map[uint32]mmsPending{{}, {}}
	m.fragment = [2]mmsTSDU{}
	m.lists = [2]map[mmsListKey]mmsListEntry{}
	m.listVersions = [2]map[mmsListKey]uint64{}
}
func (m *binMMS) snapshot() map[string]any {
	f := map[string]any{"Observation": "unverified-MMS-association", "Transport Connected": m.transport, "Association Active": m.active, "Association Concluded": m.concluded, "Context Refused": m.poison, "Client Direction": m.client, "Max TPDU Bytes": m.tpdu, "Outstanding Direction0": len(m.pending[0]), "Outstanding Direction1": len(m.pending[1]), "Calling Limit": m.limits[0], "Called Limit": m.limits[1]}
	if m.values {
		f["Data Services Enabled"] = true
		f["TPDU Size Negotiated"] = m.transport
		f["Association Only Start"] = m.crEvent == 0 && m.initEvent != 0
		f["Data Nesting Negotiated"] = m.hasNesting
		f["Data Nesting Limit"] = nil
		if m.hasNesting {
			f["Data Nesting Limit"] = m.nesting
		}
	}
	if m.validateDescriptions {
		f["Description Checks Enabled"] = true
	}
	if m.listObserved {
		f["Named List Context Observed"] = true
		f["Observed Lists Direction0"], f["Observed Lists Direction1"] = len(m.lists[0]), len(m.lists[1])
	}
	return f
}
func (f *binFlow) consumeMMSISO(dir int, e *ProtocolEvent) (out map[string]any, err error) {
	m := f.mms
	e.Profile = m.profile()
	e.nativeMMSObservation = true
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	defer func() {
		if err != nil {
			m.refuse()
			e.Completeness = "refused"
			e.ResponseTo, e.TransactionID = 0, 0
			e.Session = nil
		}
		_ = f.mmsReserve(m.storage())
	}()
	if m.poison || m.released {
		return nil, protocolError(ErrContextRequired, "MMS carrier/context previously refused; new TCP flow required")
	}
	d, err := parseMMSCOTP(e.Raw, f.a.budget)
	if err != nil {
		return nil, err
	}
	out = map[string]any{"Carrier": d.fields, "Observation": "unverified-MMS-wire"}
	switch d.code {
	case 0xe0:
		if m.crEvent != 0 || m.initEvent != 0 || d.dst != 0 || d.src == 0 {
			return nil, protocolError(ErrDesynchronized, "MMS COTP connection request identity")
		}
		m.client = dir
		m.crRef = d.src
		m.crEvent = e.ID
		m.tpdu = d.tpduSize
	case 0xd0:
		if m.crEvent == 0 || dir == m.client || d.dst != m.crRef || d.src == 0 || m.transport || d.tpduSize > m.tpdu {
			return nil, protocolError(ErrContextRequired, "MMS COTP confirmation without matching request")
		}
		m.ccRef = d.src
		m.tpdu = d.tpduSize
		m.transport = true
		e.ResponseTo, e.TransactionID = m.crEvent, m.crEvent
	case 0xf0:
		if !m.transport {
			// A capture may begin at an actual presentation CONNECT. The selected
			// extended profile can observe the complete CP/CPA/MMS exchange, while
			// explicitly retaining that COTP connection/TPDU negotiation was unseen.
			prefix := len(d.data) > 0 && d.data[0] == 13
			if !m.values || m.initEvent == 0 && len(m.fragment[dir].events) == 0 && !prefix {
				return nil, protocolError(ErrContextRequired, "MMS DT requires observed COTP connection or presentation association start")
			}
		}
		if len(e.Raw)-4 > m.tpdu {
			return nil, protocolError(ErrMalformedMessage, "MMS DT exceeds observed TPDU size")
		}
		frag := &m.fragment[dir]
		if !frag.first.IsZero() && e.Timestamp.Sub(frag.first) > 30*time.Second {
			return nil, protocolError(ErrContextRequired, "MMS incomplete TSDU expired; unlabelled late fragment cannot be associated")
		}
		total := len(frag.wire) + len(d.data)
		if total > f.a.budget.MaxMessageBytes {
			return nil, protocolError(ErrResourceExceeded, "MMS TSDU byte budget")
		}
		if len(frag.events) >= f.a.budget.MaxCollectionElements || len(frag.refs)+len(e.SourceBytes.PacketRefs) > f.a.budget.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "MMS fragment contribution budget")
		}
		if !d.end {
			if err = f.mmsReserve(m.storage() + int64(len(d.data)+8+len(e.SourceBytes.PacketRefs)*192)); err != nil {
				return nil, err
			}
			if frag.first.IsZero() {
				frag.first = e.Timestamp
			}
			wire := make([]byte, total)
			copy(wire, frag.wire)
			copy(wire[len(frag.wire):], d.data)
			frag.wire = wire
			events := make([]uint64, len(frag.events)+1)
			copy(events, frag.events)
			events[len(frag.events)] = e.ID
			frag.events = events
			var refs []PacketReference
			if len(frag.refs)+len(e.SourceBytes.PacketRefs) > 0 {
				refs = make([]PacketReference, len(frag.refs)+len(e.SourceBytes.PacketRefs))
			}
			copy(refs, frag.refs)
			copy(refs[len(frag.refs):], e.SourceBytes.PacketRefs)
			frag.refs = refs
			e.Completeness = "fragment"
			out["Pending TSDU Bytes"] = len(frag.wire)
			out["Contributor Event IDs"] = cloneScalarValues(frag.events)
			out["Observed Context"] = m.snapshot()
			return out, nil
		}
		if err = f.mmsReserve(m.storage() + 32768 + m.projectionFactor(f.a.budget)*int64(total)); err != nil {
			return nil, err
		}
		data := d.data
		if len(frag.events) > 0 {
			data = append(bytes.Clone(frag.wire), d.data...)
			out["Reconstructed TSDU"] = bytes.Clone(data)
			out["Contributor Event IDs"] = append(cloneScalarValues(frag.events), e.ID)
			out["Contributor Packet Refs"] = append(cloneScalarValues(frag.refs), e.SourceBytes.PacketRefs...)
		}
		m.fragment[dir] = mmsTSDU{}
		if err = parseMMSSession(data, f.a.budget, &d, m.values); err != nil {
			return nil, err
		}
		out["Carrier"] = d.fields
		cs := m.contexts
		if d.session == 0x0d {
			cs = d.offer
		} else if d.session == 0x0e {
			cs = m.offered
		}
		id := d.fields["Presentation Context ID"].(uint32)
		role, ok := mmsContextRole(cs, id)
		if !ok {
			return nil, protocolError(ErrContextRequired, "MMS presentation context is not offered/accepted BER")
		}
		if d.session == 1 {
			if role != "mms" || !m.active || m.concluded {
				return nil, protocolError(ErrContextRequired, "MMS application data without active accepted context")
			}
		} else {
			if role != "acse" {
				return nil, protocolError(ErrContextRequired, "MMS ACSE carrier context mismatch")
			}
		}
		if d.acse == 0x62 {
			if m.releaseEvent != 0 || len(m.pending[0])+len(m.pending[1]) != 0 {
				return nil, protocolError(ErrDesynchronized, "MMS ACSE release request direction")
			}
			m.releaseEvent = e.ID
			m.releaseDir = dir
			m.active = false
		}
		if d.acse == 0x63 {
			if dir == m.releaseDir || m.releaseEvent == 0 || m.released {
				return nil, protocolError(ErrContextRequired, "MMS ACSE release response without request")
			}
			m.released = true
			e.ResponseTo, e.TransactionID = m.releaseEvent, m.releaseEvent
			m.active = false
		}
		if m.values && !m.transport && m.initEvent == 0 {
			if d.session != 13 || d.acse != 0x60 {
				return nil, protocolError(ErrContextRequired, "MMS unseen COTP requires actual presentation CONNECT/AARQ")
			}
			m.client = dir
		}
		if len(d.pdu) > 0 {
			if d.session != 1 {
				mid := d.fields["MMS Context ID"].(uint32)
				r, ok := mmsContextRole(cs, mid)
				if !ok || r != "mms" {
					return nil, protocolError(ErrContextRequired, "MMS EXTERNAL indirect reference lacks MMS BER context")
				}
			}
			var values *mmsValueContext
			if m.values {
				values = &mmsValueContext{left: d.valueElements, state: m, dir: dir, reserve: func(extra int64) error { return f.mmsReserve(f.sessionBytes + extra) }}
			}
			app, e2 := decodeMMSNode(*d.application, values)
			if e2 != nil {
				return nil, e2
			}
			out["MMS"] = app.fields
			e.Completeness = "message"
			if err = m.applyMMS(app, dir, e, f.a.budget.MaxCollectionElements, d, f); err != nil {
				return nil, err
			}
		} else {
			e.Completeness = "carrier"
		}
	default:
		return nil, protocolError(ErrUnsupportedFeature, "MMS transport code")
	}
	if e.Completeness == "" {
		e.Completeness = "carrier"
	}
	out["Observed Context"] = m.snapshot()
	return out, nil
}
func (m *binMMS) applyMMS(d mmsDecoded, dir int, e *ProtocolEvent, limit int, carrier mmsISOCarrier, flow *binFlow) error {
	if d.tag != 0xa8 && d.tag != 0xa9 && m.pduLimit[1-dir] != 0 && len(d.fields["Raw PDU"].([]byte)) > int(m.pduLimit[1-dir]) {
		return protocolError(ErrMalformedMessage, "MMS PDU exceeds observed peer receive limit")
	}
	if m.values && m.hasNesting && d.valueDepth > m.nesting {
		return protocolError(ErrMalformedMessage, "MMS Data/type nesting exceeds observed negotiated limit")
	}
	switch d.tag {
	case 0xa8:
		if carrier.acse != 0x60 || carrier.session != 0x0d || dir != m.client || m.initEvent != 0 {
			return protocolError(ErrDesynchronized, "MMS Initiate request direction/phase")
		}
		m.initEvent = e.ID
		if n, ok := d.fields["Local Detail"].(uint32); ok {
			m.pduLimit[dir] = n
		}
		m.offered = carrier.offer
		m.proposed = [2]uint32{d.calling, d.called}
		m.supported[dir] = bytes.Clone(d.services)
		if m.values {
			if n, ok := d.fields["Nesting Level"].(uint32); ok {
				m.proposedNesting = n
				m.hasProposedNesting = true
			}
		}
	case 0xa9:
		if carrier.acse != 0x61 || carrier.session != 0x0e || dir == m.client || m.initEvent == 0 || m.initAccepted {
			return protocolError(ErrContextRequired, "MMS Initiate response missing request")
		}
		if !carrier.hasResult || carrier.acseResult != 0 {
			return protocolError(ErrContextRequired, "MMS ACSE association was not accepted")
		}
		if len(carrier.results) != len(m.offered) {
			return protocolError(ErrMalformedMessage, "MMS presentation result cardinality")
		}
		if d.calling > m.proposed[0] || d.called > m.proposed[1] {
			return protocolError(ErrMalformedMessage, "MMS negotiated outstanding limits exceed offer")
		}
		m.contexts = nil
		for i, c := range m.offered {
			if carrier.results[i] {
				m.contexts = append(m.contexts, c)
			}
		}
		m.limits = [2]uint32{d.calling, d.called}
		m.active = true
		m.initAccepted = true
		if m.values {
			if n, ok := d.fields["Nesting Level"].(uint32); ok {
				if m.hasProposedNesting && n > m.proposedNesting {
					return protocolError(ErrMalformedMessage, "MMS nesting negotiation exceeds offer")
				}
				m.nesting = n
				m.hasNesting = true
			}
		}
		if n, ok := d.fields["Local Detail"].(uint32); ok {
			m.pduLimit[dir] = n
		}
		m.supported[dir] = bytes.Clone(d.services)
		acseID := carrier.fields["Presentation Context ID"].(uint32)
		mmsID := carrier.fields["MMS Context ID"].(uint32)
		ar, aok := mmsContextRole(m.contexts, acseID)
		mr, mok := mmsContextRole(m.contexts, mmsID)
		if !aok || !mok || ar != "acse" || mr != "mms" {
			return protocolError(ErrContextRequired, "MMS Initiate response uses rejected presentation context")
		}
		e.ResponseTo, e.TransactionID = m.initEvent, m.initEvent
	case 0xa0:
		if !m.active {
			return protocolError(ErrContextRequired, "MMS confirmed request without observed association")
		}
		if !mmsSupports(m.supported[1-dir], d.service) {
			return protocolError(ErrContextRequired, "MMS requested service not advertised by observed peer")
		}
		if m.concludeEvent != 0 {
			return protocolError(ErrContextRequired, "MMS request after conclude requested")
		}
		if m.retired[dir][d.invoke] {
			return protocolError(ErrContextRequired, "MMS InvokeID was retired; passive generation reuse needs extra evidence")
		}
		if _, ok := m.pending[dir][d.invoke]; ok {
			return protocolError(ErrDesynchronized, "MMS active InvokeID reused")
		}
		max := m.limits[0]
		if dir != m.client {
			max = m.limits[1]
		}
		if len(m.pending[dir]) >= int(max) || len(m.pending[0])+len(m.pending[1])+len(m.retired[0])+len(m.retired[1]) >= limit {
			return protocolError(ErrResourceExceeded, "MMS outstanding/history budget")
		}
		p := mmsPending{event: e.ID, service: d.service, items: d.items}
		if m.values && d.service == 6 {
			if address, ok := d.fields["Address"]; ok {
				p.query = map[string]any{"Address": cloneSessionValue(address)}
			}
		}
		if m.values && d.service == 1 {
			p.query = map[string]any{"Object Class": d.fields["Object Class"], "Object Scope": cloneSessionValue(d.fields["Object Scope"])}
			if after, ok := d.fields["Continue After"]; ok {
				p.query["Continue After"] = after
			}
		}
		if m.values && d.service == 5 && d.binding == nil {
			p.names = cloneSessionValue(d.fields["Variables"]).([]map[string]any)
		}
		if m.values && d.service == 4 {
			p.specification, _ = d.fields["Specification With Result"].(bool)
			if (p.specification || m.validateDescriptions) && d.binding == nil {
				p.names = cloneSessionValue(d.fields["Variables"]).([]map[string]any)
			}
		}
		if m.values {
			if err := flow.namedMMSRequest(d, &p, dir); err != nil {
				return err
			}
		}
		if m.validateDescriptions && d.service == 5 {
			if err := mmsDescriptionOperands(p.names, d.fields["Data"].([]map[string]any), limit); err != nil {
				return err
			}
		}
		m.pending[dir][d.invoke] = p
		e.TransactionID = e.ID
	case 0xa1, 0xa2:
		if !m.active {
			return protocolError(ErrContextRequired, "MMS confirmed reply without active association")
		}
		p, ok := m.pending[1-dir][d.invoke]
		if !ok {
			return protocolError(ErrContextRequired, "MMS reply lacks a unique opposite-direction InvokeID")
		}
		if d.tag == 0xa1 && d.service != p.service {
			return protocolError(ErrMalformedMessage, "MMS response service differs from request")
		}
		if d.tag == 0xa1 && (p.service == 4 || p.service == 5) && p.items >= 0 && p.items != d.items {
			if p.service == 4 {
				return protocolError(ErrMalformedMessage, "MMS Read result cardinality differs from request")
			}
			return protocolError(ErrMalformedMessage, "MMS Write result cardinality differs from request")
		}
		if m.values && d.tag == 0xa1 && p.service == 4 {
			names, has := d.fields["Variables"]
			list, hasList := d.fields["Variable List"]
			matches := has && !hasList && mmsNamesEqual(p.names, names.([]map[string]any))
			if p.binding != nil {
				matches = matches || hasList && !has && p.binding.key == mmsKey(list.(map[string]any))
			}
			if p.specification != (has || hasList) || p.specification && !matches {
				return protocolError(ErrMalformedMessage, "MMS returned variable specification differs from observed request")
			}
		}
		if m.values {
			if err := flow.namedMMSResponse(d, p, dir, e); err != nil {
				return err
			}
		}
		if m.validateDescriptions && d.tag == 0xa1 && p.service == 4 {
			results := d.fields["Access Results"].([]map[string]any)
			budget := limit
			for i, result := range results {
				if v, ok := result["Success"].(map[string]any); ok {
					if typ, ok := p.names[i]["Type Specification"].(map[string]any); ok {
						if err := mmsDescriptionValue(typ, v, &budget); err != nil {
							return err
						}
					}
				}
			}
		}
		if m.values && (p.service == 1 || p.service == 6 && p.query != nil) {
			d.fields["Observed Request Query"] = cloneSession(p.query)
		}
		if m.values && p.service == 5 {
			d.fields["Observed Write Variables"] = cloneSessionValue(p.names)
		}
		delete(m.pending[1-dir], d.invoke)
		m.retired[1-dir][d.invoke] = true
		e.ResponseTo, e.TransactionID = p.event, p.event
	case 0x8b:
		if !m.active || m.concludeEvent != 0 || len(m.pending[0])+len(m.pending[1]) != 0 {
			return protocolError(ErrContextRequired, "MMS conclude while unmatched requests remain")
		}
		m.concludeEvent = e.ID
		m.concludeDir = dir
	case 0x8c:
		if m.concludeEvent == 0 || dir == m.concludeDir || m.concluded {
			return protocolError(ErrContextRequired, "MMS conclude reply missing peer request")
		}
		m.active = false
		m.concluded = true
		e.ResponseTo, e.TransactionID = m.concludeEvent, m.concludeEvent
	default:
		return fmt.Errorf("MMS unsupported selected PDU")
	}
	return nil
}
