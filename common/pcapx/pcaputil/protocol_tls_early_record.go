package pcaputil

import (
	"bytes"
	"crypto/cipher"
	"encoding/binary"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// TLS 1.3 early traffic, handshake traffic and application traffic have
// independent sequence spaces. A secret or an early_data offer is not server
// acceptance. Retained records belong to this TCP generation/capture domain.
type tlsEarlyRecordState struct {
	decisionKnown, accepted, ended bool
	record, replaying, gap         bool
	suite                          uint16
	seq                            uint64
	decisionPDU                    uint64
	key                            cipher.AEAD
	iv                             []byte
	pending                        []*tlsEarlyPending
	bytes, charged                 int64
}

type tlsEarlyPending struct {
	meta ProtocolEvent
	wire []byte
	cost int64
}

func (s *tlsEarlyRecordState) storage() int64 {
	n := s.bytes
	if s.key != nil {
		n += 512
	}
	return n
}

func (t *binTLS) earlyEligible(dir int) bool {
	return t.client >= 0 && dir == t.client && t.earlyDataOfferKnown && t.earlyDataOffered &&
		!t.app[dir] && !t.early.ended && (t.version == 0 || t.version == 0x304)
}

func (f *binFlow) usableTLSEarlyKey() bool {
	if f.tls.early.key != nil {
		return true
	}
	if f.a.tlsSecrets == nil {
		return false
	}
	s, ok := f.a.tlsSecrets.LookupTLSSecret("CLIENT_EARLY_TRAFFIC_SECRET", f.tls.random)
	return ok && (len(s) == 32 || len(s) == 48)
}

// Preserve complete framed ciphertext before the observed acceptance decision.
// Do not try it with a handshake key, advance that sequence, or deliver it as
// an application request. The cap bounds ciphertext and owned provenance.
func (f *binFlow) retainTLSEarlyRecord(dir int, e *ProtocolEvent, info map[string]any) (bool, error) {
	t := f.tls
	if f.orderedTLSCarrier || !t.earlyEligible(dir) || t.early.replaying || e.Raw[0] != 23 ||
		t.early.decisionKnown && len(t.early.pending) == 0 {
		return false, nil
	}
	limit := min(64, f.a.budget.MaxCollectionElements)
	cost := int64(2048 + len(e.Raw) + len(e.Source) + len(e.Destination) + len(e.Domain.Encapsulation))
	cost += int64(len(e.SourceBytes.PacketRefs))*64 + int64(len(e.SourceBytes.ParentPDUs))*8
	for _, ref := range e.SourceBytes.PacketRefs {
		cost += int64(len(ref.Domain.Encapsulation))
	}
	for _, layer := range e.CarrierLayers {
		cost += int64(24 + len(layer))
	}
	if len(t.early.pending) >= limit || cost > 65536-t.early.bytes {
		return true, protocolError(ErrResourceExceeded, "TLS pending early record byte/count budget")
	}
	before := f.sessionBytes
	if err := f.reserveSession(t.storage() + cost); err != nil {
		return true, err
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	meta := ProtocolEvent{ID: e.ID, FlowID: f.id, TCPFlowID: e.TCPFlowID, Timestamp: e.Timestamp, Transport: e.Transport,
		Source: e.Source, Destination: e.Destination, Direction: dir, Domain: e.Domain,
		Offset: e.Offset, SourceBytes: e.SourceBytes, CarrierLayers: e.CarrierLayers,
		nativeTLSObservation: true, Profile: e.Profile, Admission: e.Admission, Completeness: "record"}
	cloneEvidence(&meta)
	t.early.pending = append(t.early.pending, &tlsEarlyPending{meta: meta, wire: bytes.Clone(e.Raw), cost: cost})
	t.early.bytes += cost
	t.early.charged += f.sessionBytes - before
	info["Secret Lookup Status"] = "early-context-pending"
	info["Early Data Decision"] = "awaiting observed server acceptance"
	info["Early Data Pending Records"] = len(t.early.pending)
	return true, nil
}

func (f *binFlow) popTLSEarlyRecord() *tlsEarlyPending {
	t := f.tls
	p := t.early.pending[0]
	t.early.pending[0] = nil
	t.early.pending = t.early.pending[1:]
	t.early.bytes -= p.cost
	if len(t.early.pending) == 0 {
		t.early.pending = nil
	}
	minimum := t.storage()
	if f.wcfTLS != nil {
		minimum += 256
	}
	release := min(p.cost, min(t.early.charged, max(int64(0), f.sessionBytes-minimum)))
	t.early.charged -= release
	f.sessionBytes -= release
	f.a.buffered.Add(-release)
	return p
}

// A resumed connection may negotiate a different cipher with the same KDF
// hash when it rejects 0-RTT. Never use that selection to guess the early AEAD.
// At most two supported AEADs match an NSS secret's hash length; only a unique
// successful tag establishes a profile, with an independent early sequence.
func (f *binFlow) openTLSEarlyRecord(dir int, wire []byte, info map[string]any) ([]byte, byte, bool, error) {
	t := f.tls
	if !t.earlyEligible(dir) || !t.early.decisionKnown || f.a.tlsSecrets == nil {
		return nil, 0, false, nil
	}
	if len(wire) < 22 || len(wire) > 16645 || wire[0] != 23 || binary.BigEndian.Uint16(wire[1:3]) != 0x303 || int(binary.BigEndian.Uint16(wire[3:5])) != len(wire)-5 {
		return nil, 0, false, nil // the shared negotiated envelope gate owns this refusal
	}
	if t.early.seq == ^uint64(0) {
		return nil, 0, true, protocolError(ErrResourceExceeded, "TLS early record sequence exhausted")
	}
	type candidate struct {
		suite uint16
		key   cipher.AEAD
		iv    []byte
	}
	var candidates []candidate
	if t.early.key != nil {
		candidates = append(candidates, candidate{t.early.suite, t.early.key, t.early.iv})
	} else {
		secret, ok := f.a.tlsSecrets.LookupTLSSecret("CLIENT_EARLY_TRAFFIC_SECRET", t.random)
		if !ok {
			return nil, 0, false, nil
		}
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			size := 32
			if suite == 0x1302 {
				size = 48
			}
			if len(secret) != size {
				continue
			}
			trial := binTLS{suite: suite}
			if err := trial.install13(dir, secret); err != nil {
				return nil, 0, true, err
			}
			candidates = append(candidates, candidate{suite, trial.keys[dir], trial.iv[dir]})
		}
	}
	var found *candidate
	var plain []byte
	var sequence uint64
	window := uint64(0)
	if t.early.gap {
		window = 16
	}
	for _, c := range candidates {
		for skip := uint64(0); skip <= window && t.early.seq < ^uint64(0)-skip; skip++ {
			seq := t.early.seq + skip
			nonce := bytes.Clone(c.iv)
			for i := 0; i < 8; i++ {
				nonce[11-i] ^= byte(seq >> uint(i*8))
			}
			p, err := c.key.Open(nil, nonce, wire[5:], wire[:5])
			if err != nil {
				continue
			}
			if found != nil {
				return nil, 0, true, protocolError(ErrContextRequired, "TLS early AEAD profile is ambiguous")
			}
			owned := c
			found = &owned
			plain = p
			sequence = seq
		}
	}
	if found == nil {
		return nil, 0, false, nil
	} // a handshake-key record may legitimately follow
	t.early.key, t.early.iv, t.early.suite = found.key, found.iv, found.suite
	t.early.seq = sequence + 1
	t.early.record = true
	info["Authentication Verified"], info["TLS Key Epoch"] = true, "early"
	info["Secret Lookup Status"], info["Content Visibility"] = "found", "authenticated-plaintext"
	t.protectedRecordObserved[dir] = true
	info["Early Cipher Suite"], info["Early Record Sequence"] = found.suite, sequence
	info["Early Data Decision PDU"] = t.early.decisionPDU
	if sequence > 0 && t.early.gap {
		info["Missing Earlier Plaintext"], info["Record Recovery Verified"] = true, true
	}
	t.early.gap = false
	i := len(plain) - 1
	for i >= 0 && plain[i] == 0 {
		i--
	}
	if i < 0 || plain[i] < 21 || plain[i] > 23 {
		return nil, 0, true, protocolError(ErrMalformedMessage, "TLS early inner content type")
	}
	return plain[:i], plain[i], true, nil
}

func (t *binTLS) admitEarlyRecord(kind byte, plain []byte, info map[string]any) error {
	info["Early Data Decision"] = "rejected by observed server"
	if t.early.accepted {
		info["Early Data Decision"] = "accepted by observed server"
		if !t.finalHello || t.retrySeen || !t.earlyDataPSKSelected || t.earlyDataIdentity != 0 || t.early.suite != t.suite {
			return protocolError(ErrMalformedMessage, "TLS accepted early data cipher or PSK disagrees with observed handshake")
		}
	}
	if kind == 22 && !bytes.Equal(plain, []byte{5, 0, 0, 0}) {
		return protocolError(ErrMalformedMessage, "TLS early epoch permits only one complete empty EndOfEarlyData handshake")
	}
	if kind != 22 && kind != 23 {
		return protocolError(ErrUnsupportedFeature, "TLS early alert observation is not admitted as application data")
	}
	return nil
}

func (t *binTLS) admitEndOfEarlyData(dir int, wire []byte, authenticated bool) error {
	if t.version == 0 {
		return protocolError(ErrContextRequired, "TLS EndOfEarlyData requires TLS 1.3 context")
	}
	if t.version != 0x304 || !authenticated || t.client < 0 || dir != t.client || t.app[dir] ||
		!t.early.record || !t.early.decisionKnown || !t.early.accepted || t.early.ended {
		return protocolError(ErrMalformedMessage, "TLS EndOfEarlyData requires one empty client message in an accepted early record epoch")
	}
	if _, _, err := stream_parser.DecodeTLSEndOfEarlyDataLayout(wire); err != nil {
		return protocolError(ErrMalformedMessage, "TLS EndOfEarlyData must be one complete empty handshake")
	}
	return nil
}

// Called after the trigger's public callback. Replayed observations use new
// identities and original contributors. The acceptance trigger is context,
// never a ciphertext/plaintext byte contributor. Published events are untouched.
func (f *binFlow) resumeTLSEarly(trigger *ProtocolEvent) {
	t := f.tls
	if t == nil || f.orderedTLSCarrier || len(t.early.pending) == 0 {
		return
	}
	if t.retrySeen {
		// HRR rejects the first flight. ClientHello2 and its handshake
		// sequence must never inherit the retained first-flight records.
		for len(t.early.pending) > 0 {
			f.popTLSEarlyRecord()
		}
		return
	}
	if !t.early.decisionKnown || !t.finalHello {
		return
	}
	if !f.usableTLSEarlyKey() {
		if !t.early.accepted {
			for len(t.early.pending) > 0 {
				f.popTLSEarlyRecord()
			}
			return
		}
		// Give a late early secret a chance before another client record or
		// server application bytes require progress. Missing early plaintext
		// must not hide an independently authenticated client Finished.
		if trigger.Direction != t.client && t.plaintext == nil {
			return
		}
	}
	currentPlain, currentContent := t.plaintext, t.content
	t.plaintext = nil
	defer func() { t.plaintext, t.content = currentPlain, currentContent; t.early.replaying = false }()
	t.early.replaying = true
	for len(t.early.pending) > 0 {
		p := f.popTLSEarlyRecord()
		e := p.meta
		e.ID, e.Raw, e.Protocol = f.a.ids.Add(1), p.wire, "tls"
		_, spec, err := f.frame(p.wire)
		var result map[string]any
		if err == nil {
			result, err = f.consumeTLS(e.Direction, &e)
		}
		if e.Session == nil {
			e.Session = map[string]any{}
		}
		e.Session["Reprocessed From PDU"], e.Session["Early Data Decision PDU"] = p.meta.ID, t.early.decisionPDU
		e.Session["Reprocessed In PDU"] = trigger.ID
		e.semanticFields = cloneSession(e.Session)
		if result != nil {
			e.Structured = map[string]any{"fields": cloneSession(e.Session)}
		}
		f.a.finishProtocolDatagram(&e, p.wire, spec, err)
		parent := e
		cloneEvidence(&parent)
		isolated := e.Session["Record Failure Isolated"] == true
		f.a.emit(&e)
		f.deliverTLSHTTPGap(parent.Direction, &parent)
		if err == nil {
			f.deliverTLS(parent.Direction, &parent)
		} else if !isolated {
			f.directions[parent.Direction].stopped = true
			f.release(&f.directions[parent.Direction])
			f.closeSession()
			return
		}
	}
}
