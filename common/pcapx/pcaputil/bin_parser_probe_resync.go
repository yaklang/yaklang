package pcaputil

import "time"

// tcpProbeResyncProtocols is the protocol capability boundary for retrying at a
// later TCP delivery. These probes require a strong signature, a validated
// start line, or a complete self-delimited message. Weak length-prefix
// protocols stay excluded even when they can be admitted at a stream's known
// beginning.
func tcpProbeResyncProtocol(protocol string) bool {
	switch protocol {
	case "http", "http2", "bgp", "tls", "ssh", "smb", "smb2", "mqtt", "ajp", "rtmp", "enip", "dnp3", "doip":
		return true
	default:
		return false
	}
}

func (f *binFlow) tcpProbeResyncCandidate(dir int, wire []byte) bool {
	if len(wire) == 0 {
		return false
	}
	limit := min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes)
	if isHTTPStartLineCandidate(wire) && f.probeHTTPStartLine(wire).Verdict != ProbeReject {
		return true
	}
	if probeHTTP2(wire, limit).Verdict != ProbeReject ||
		probeBGP(wire, limit).Verdict != ProbeReject ||
		probeAJP(wire, limit).Verdict != ProbeReject ||
		probeRTMP(wire, limit).Verdict != ProbeReject ||
		probeMQTT(wire, f.a.config.ProbeBytes).Verdict != ProbeReject ||
		probeENIP(wire).Verdict != ProbeReject ||
		probeDNP3(wire, limit).Verdict != ProbeReject ||
		probeDoIP(wire, limit).Verdict != ProbeReject ||
		probeSSH(wire, limit).Verdict != ProbeReject ||
		f.tcpClientKnown && dir != f.tcpClientDir && probeSSHServerPreamble(wire, f.a.budget.MaxFrameBytes).Verdict != ProbeReject {
		return true
	}
	if probeSMB1Negotiate(wire, 65582).Verdict != ProbeReject ||
		probeSMB1FileCommand(wire, 65582).Verdict != ProbeReject ||
		probeSMB1SessionSetupExtended(wire, 65598).Verdict != ProbeReject ||
		probeSMB1NegotiateExtendedResponse(wire, 65608).Verdict != ProbeReject ||
		probeSMB2(wire, limit).Verdict != ProbeReject {
		return true
	}
	if len(wire) == 0 || wire[0] < 20 || wire[0] > 23 {
		return false
	}
	if len(wire) == 1 {
		return true
	}
	if wire[1] != 3 {
		return false
	}
	if len(wire) == 2 {
		return true
	}
	if wire[2] > 4 {
		return false
	}
	if len(wire) < 5 {
		return true
	}
	return int(wire[3])<<8|int(wire[4]) <= 18432
}

// probeArrival identifies a real ordered TCP delivery boundary. Transport
// boundaries are candidate indices for an unclassified direction; they do not
// constrain message framing after a protocol has been accepted.
type probeArrival struct {
	offset    uint64
	timestamp time.Time
}

// noteTCPProbeArrival records the first byte contributed by a newly delivered
// TCP frame. Reassembly invokes this before feeding the frame to the parser.
func (f *binFlow) noteTCPProbeArrival(dir int, ts time.Time) {
	if f.a.config.DisableTCPProbeResync || !f.captureTCP || f.protocol != "" {
		return
	}
	d := &f.directions[dir]
	offset := d.offset + uint64(len(d.buffer))
	if n := len(d.probeArrivals); n > 0 && d.probeArrivals[n-1].offset == offset {
		return
	}
	f.pruneTCPProbeArrivals(d)
	if len(d.probeArrivals) >= f.a.budget.MaxCollectionElements {
		// Keep the bounded candidates already known. Once one is selected it is
		// pruned, making room for a later delivery without unbounded history.
		return
	}
	d.probeArrivals = append(d.probeArrivals, probeArrival{offset: offset, timestamp: ts})
}

func (f *binFlow) pruneTCPProbeArrivals(d *binDirection) {
	n := 0
	for _, arrival := range d.probeArrivals {
		if arrival.offset > d.offset {
			d.probeArrivals[n] = arrival
			n++
		}
	}
	clear(d.probeArrivals[n:])
	d.probeArrivals = d.probeArrivals[:n]
}

func (f *binFlow) clearTCPProbeArrivals() {
	for i := range f.directions {
		f.directions[i].probeArrivals = nil
	}
}

// nextTCPProbeArrival returns the nearest later transport boundary. Only bytes
// already present in the ordered buffer are eligible, so resynchronization can
// never wait for or invent an arrival that has not been captured.
func (f *binFlow) nextTCPProbeArrival(dir int, wire []byte) (int, time.Time, bool) {
	if f.a.config.DisableTCPProbeResync || !f.captureTCP {
		return 0, time.Time{}, false
	}
	d := &f.directions[dir]
	f.pruneTCPProbeArrivals(d)
	limit := d.offset + uint64(len(wire))
	for _, arrival := range d.probeArrivals {
		if arrival.offset > limit {
			break
		}
		if arrival.offset > d.offset {
			n := int(arrival.offset - d.offset)
			suffix := wire[n:]
			candidate := *f
			candidate.detectDirection(dir, suffix)
			if tcpProbeResyncProtocol(candidate.protocol) || f.tcpProbeResyncCandidate(dir, suffix) {
				return n, arrival.timestamp, true
			}
		}
	}
	return 0, time.Time{}, false
}

// probeLatestTCPArrival gives a complete message starting in a new delivery
// the same chance as a longer prefix that began in an earlier delivery. This
// matters for short self-framed messages and avoids requiring the total
// unclassified history to reach ProbeBytes before a later boundary is tried.
func (f *binFlow) probeLatestTCPArrival(dir int, wire []byte) (int, time.Time, bool) {
	if f.a.config.DisableTCPProbeResync || !f.captureTCP || len(wire) == 0 {
		return 0, time.Time{}, false
	}
	d := &f.directions[dir]
	f.pruneTCPProbeArrivals(d)
	for i := len(d.probeArrivals) - 1; i >= 0; i-- {
		arrival := d.probeArrivals[i]
		if arrival.offset <= d.offset {
			continue
		}
		n := int(arrival.offset - d.offset)
		if n >= len(wire) {
			return 0, time.Time{}, false
		}
		f.a.probes.Add(1)
		candidate := *f
		candidate.detectDirection(dir, wire[n:])
		if !tcpProbeResyncProtocol(candidate.protocol) {
			return 0, time.Time{}, false
		}
		f.a.probes.Add(1)
		f.detectDirection(dir, wire[n:])
		if f.protocol != candidate.protocol {
			return 0, time.Time{}, false
		}
		f.clearTCPProbeArrivals()
		return n, arrival.timestamp, true
	}
	return 0, time.Time{}, false
}

// skipRejectedTCPProbePrefix advances over bytes that are real transport data
// but not part of any admitted application message. Their packet ownership is
// deliberately pruned from this direction; the original captured frames remain
// available through the packet store.
func (f *binFlow) skipRejectedTCPProbePrefix(dir, n int, ts time.Time) {
	d := &f.directions[dir]
	d.offset += uint64(n)
	d.ts = ts
	f.a.unclassified.Add(uint64(n))
	d.pruneContributions(f.a)
	f.pruneTCPProbeArrivals(d)
}
