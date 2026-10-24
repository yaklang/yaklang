package pcaputil

import (
	"bytes"
	"fmt"
)

// carrierTLS consumes only bytes already ordered by a selected carrier's
// receiver. TLS framing and handshake/record semantics remain the same engine
// used for TCP. Packet contributors/parent PDUs survive both carrier and TLS
// fragmentation. No application protocol is guessed from tunnel plaintext.
type carrierTLS struct {
	f              *binFlow
	layers         []string
	broken, closed bool
}

func newCarrierTLS(a *binParser, id uint64, endpoints [2]string, domain CaptureDomain, layers ...string) *carrierTLS {
	return &carrierTLS{f: &binFlow{a: a, id: id, protocol: "tls", endpoints: endpoints, domain: domain, byteSource: "reassembled", orderedTLSCarrier: true}, layers: append([]string(nil), layers...)}
}

func (c *carrierTLS) addEvidence(dir, n int, parent *ProtocolEvent) bool {
	if n == 0 {
		return true
	}
	d := &c.f.directions[dir]
	refs := parent.SourceBytes.PacketRefs
	if len(refs) == 0 {
		d.evidenceLimited = true
		return true
	}
	if len(refs) > c.f.a.budget.MaxCollectionElements-len(d.contributions) || !c.f.a.reserveEvidence(int64(len(refs))*80) {
		d.evidenceLimited = true
		return false
	}
	start := d.offset + uint64(len(d.buffer))
	for _, ref := range refs {
		d.contributions = append(d.contributions, contribution{start: start, end: start + uint64(n), ref: ref, parent: parent.ID})
	}
	return true
}

func (c *carrierTLS) event(dir int, raw []byte, status, detail string) *ProtocolEvent {
	e := c.f.event(dir, raw, status, detail)
	e.Transport = "udp"
	e.CarrierLayers = append([]string(nil), c.layers...)
	e.Profile = "tls-observed-carrier"
	e.Admission = "validated-carrier-order-and-TLS-record"
	e.Completeness = "record"
	return e
}

func (c *carrierTLS) fail(dir int, raw []byte, err error) *ProtocolEvent {
	status, typed := classifySessionError(err)
	e := c.event(dir, raw, status, err.Error())
	e.Error, e.sessionError = err.Error(), typed
	e.Completeness = "incomplete"
	if status == "limited" {
		c.f.a.limited.Add(uint64(len(raw)))
	} else if status == "incomplete" {
		c.f.a.incomplete.Add(1)
	} else if status == "context-required" {
		c.f.a.contextRequired.Add(1)
	} else {
		c.f.a.malformed.Add(1)
	}
	c.broken = true
	for i := range c.f.directions {
		c.f.release(&c.f.directions[i])
	}
	c.f.closeSession()
	return e
}

func (c *carrierTLS) record(dir int, raw []byte) *ProtocolEvent {
	e := c.event(dir, raw, "decoded", "TLS carrier record")
	_, spec, _ := c.f.frame(raw)
	if spec != nil {
		e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
	}
	c.f.a.messages.Add(1)
	c.f.a.messageBytes.Add(uint64(len(raw)))
	result, err := c.f.consumeTLS(dir, e)
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
		if e.Status == "limited" {
			c.f.a.limited.Add(uint64(len(raw)))
		} else if e.Status == "context-required" {
			c.f.a.contextRequired.Add(1)
		} else {
			c.f.a.malformed.Add(1)
		}
		// The TLS engine can isolate a failed authenticated record while keeping
		// its independently framed boundary and reverse-direction state intact.
		c.broken = e.Session["Record Failure Isolated"] != true
	} else {
		// RDP tunnel data, PEAP's inner EAP, and TTLS AVPs are different
		// application grammars. Do not feed any of them to TCP/HTTP sniffing.
		if c.f.tls.plaintext != nil {
			e.Session["Authenticated Plaintext"] = bytes.Clone(c.f.tls.plaintext)
			e.Session["Application Decoding"] = "carrier-specific inner protocol not selected"
			c.f.tls.plaintext = nil
		}
		e.Structured = result
		e.semanticFields = cloneSession(e.Session)
		c.f.a.decoded.Add(1)
	}
	return e
}

// feed is called under the owning carrier lock and returns events for delivery
// after unlocking. It never invokes user callbacks while carrier state is held.
func (c *carrierTLS) feed(dir int, data []byte, parent *ProtocolEvent) (events []*ProtocolEvent) {
	if c == nil || c.broken || c.closed || len(data) == 0 {
		return nil
	}
	d := &c.f.directions[dir]
	for len(data) > 0 && !c.broken {
		if len(d.buffer) == 0 {
			d.ts = parent.Timestamp
		}
		if len(d.buffer) == 0 && len(data) >= 5 {
			n, _, err := c.f.frame(data)
			if err != nil || n > c.f.a.budget.MaxMessageBytes {
				if err == nil {
					err = protocolError(ErrResourceExceeded, "carried TLS record exceeds message budget")
				}
				events = append(events, c.fail(dir, data[:min(len(data), 5)], err))
				break
			}
			if n <= len(data) {
				if !c.addEvidence(dir, n, parent) {
					events = append(events, c.fail(dir, nil, protocolError(ErrResourceExceeded, "carried TLS contributor budget")))
					break
				}
				events = append(events, c.record(dir, data[:n]))
				d.offset += uint64(n)
				d.pruneContributions(c.f.a)
				data = data[n:]
				continue
			}
		}
		need := 5
		if len(d.buffer) >= 5 {
			n, _, err := c.f.frame(d.buffer)
			if err != nil || n > c.f.a.budget.MaxMessageBytes {
				if err == nil {
					err = protocolError(ErrResourceExceeded, "carried TLS record exceeds message budget")
				}
				events = append(events, c.fail(dir, d.buffer, err))
				break
			}
			need = n
		}
		n := min(len(data), need-len(d.buffer))
		if !c.addEvidence(dir, n, parent) {
			events = append(events, c.fail(dir, d.buffer, protocolError(ErrResourceExceeded, "carried TLS contributor budget")))
			break
		}
		old := len(d.buffer)
		if cap(d.buffer) < old+n {
			capacity := max(old+n, min(2*cap(d.buffer), min(18437, c.f.a.budget.MaxMessageBytes)))
			if !c.f.a.reserveEvidence(int64(capacity - cap(d.buffer))) {
				events = append(events, c.fail(dir, d.buffer, protocolError(ErrResourceExceeded, "carried TLS buffer budget")))
				break
			}
			grown := make([]byte, old+n, capacity)
			copy(grown, d.buffer)
			d.buffer = grown
		} else {
			d.buffer = d.buffer[:old+n]
		}
		copy(d.buffer[old:], data[:n])
		data = data[n:]
		if len(d.buffer) >= 5 {
			total, _, err := c.f.frame(d.buffer)
			if err != nil {
				events = append(events, c.fail(dir, d.buffer, err))
				break
			}
			if total == len(d.buffer) {
				events = append(events, c.record(dir, d.buffer))
				d.offset += uint64(total)
				d.pruneContributions(c.f.a)
				c.f.release(d)
			}
		}
	}
	if c.broken {
		for i := range c.f.directions {
			c.f.release(&c.f.directions[i])
		}
		c.f.closeSession()
	}
	return events
}

func (c *carrierTLS) close(reason string) (events []*ProtocolEvent) {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	for dir := range c.f.directions {
		d := &c.f.directions[dir]
		if len(d.buffer) > 0 {
			events = append(events, c.fail(dir, d.buffer, protocolError(ErrNeedMore, "%s: incomplete carried TLS record", reason)))
			break
		}
	}
	if !c.broken && c.f.tls != nil {
		for dir, b := range c.f.tls.handshake {
			if len(b) > 0 {
				e := c.event(dir, nil, "incomplete", fmt.Sprintf("%s: incomplete carried TLS handshake", reason))
				e.Error = e.Summary
				e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: e.Summary}
				events = append(events, e)
				c.f.a.incomplete.Add(1)
			}
		}
	}
	for i := range c.f.directions {
		c.f.release(&c.f.directions[i])
	}
	c.f.closeSession()
	return events
}
