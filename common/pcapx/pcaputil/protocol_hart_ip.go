package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"math"
	"sort"
	"time"
)

// Finite passive HART-IP v1 observer based on fixed FieldCommGroup wire
// implementations. The sealed source manifest distinguishes source review,
// actual dissector execution and unverified device semantics. Advertised
// inactivity and session pairs are observations, never authentication.
const hartIPProfile = "hart-ip-v1-universal-observation"

type hartIPToken struct {
	command byte
	address string
}

func hartIPErr(k ProtocolErrorKind, s string) error { return protocolError(k, "HART-IP: "+s) }
func hartIPNumber(w []byte) map[string]any {
	bits := binary.BigEndian.Uint32(w)
	f := map[string]any{"bits": uint64(bits), "classification": "finite"}
	if bits>>23&255 == 255 {
		kind := "positive-infinity"
		if bits&0x7fffff != 0 {
			kind = "nan"
		} else if bits>>31 != 0 {
			kind = "negative-infinity"
		}
		f["classification"] = kind
	} else {
		f["value"] = float64(math.Float32frombits(bits))
	}
	return f
}
func hartIPCommandData(cmd byte, d []byte) (map[string]any, error) {
	switch cmd {
	case 0:
		if len(d) < 12 {
			return nil, hartIPErr(ErrMalformedMessage, "identity prefix12")
		}
		rev := d[4]
		n := 0
		switch rev {
		case 5:
			n = 12
		case 6:
			n = 17
		case 7:
			n = 22
		default:
			return nil, hartIPErr(ErrUnsupportedVersion, "identity universal revision5/6/7")
		}
		if len(d) != n {
			return nil, hartIPErr(ErrMalformedMessage, "identity exact version layout")
		}
		if d[0] != 254 {
			return nil, hartIPErr(ErrUnsupportedFeature, "selected identity formatFE")
		}
		f := map[string]any{"expansion_code": uint64(d[0]), "legacy_manufacturer_code": uint64(d[1]), "legacy_device_code": uint64(d[2]), "minimum_request_preambles": uint64(d[3]), "universal_revision": uint64(rev), "device_revision": uint64(d[5]), "software_revision": uint64(d[6]), "hardware_signal_raw": uint64(d[7]), "flags_raw": uint64(d[8]), "device_id": uint64(d[9])<<16 | uint64(d[10])<<8 | uint64(d[11])}
		if rev >= 6 {
			f["expanded_device_type"] = uint64(binary.BigEndian.Uint16(d[1:3]))
			f["minimum_response_preambles"], f["device_variable_count"] = uint64(d[12]), uint64(d[13])
			f["configuration_change_counter"], f["extended_device_status"] = uint64(binary.BigEndian.Uint16(d[14:16])), uint64(d[16])
		}
		if rev == 7 {
			f["manufacturer_id"], f["private_label_distributor"], f["device_profile"] = uint64(binary.BigEndian.Uint16(d[17:19])), uint64(binary.BigEndian.Uint16(d[19:21])), uint64(d[21])
		}
		return f, nil
	case 1:
		if len(d) != 5 {
			return nil, hartIPErr(ErrMalformedMessage, "primary units and IEEE32")
		}
		return map[string]any{"primary_variable": map[string]any{"units_code": uint64(d[0]), "number": hartIPNumber(d[1:])}}, nil
	case 2:
		if len(d) != 8 {
			return nil, hartIPErr(ErrMalformedMessage, "loop current and percent range IEEE32")
		}
		return map[string]any{"loop_current": hartIPNumber(d[:4]), "percent_range": hartIPNumber(d[4:])}, nil
	case 3:
		if len(d) != 19 && len(d) != 24 {
			return nil, hartIPErr(ErrUnsupportedFeature, "selected three/four variable layouts")
		}
		vars := make([]any, 0, (len(d)-4)/5)
		for i := 0; i < (len(d)-4)/5; i++ {
			at := 4 + 5*i
			vars = append(vars, map[string]any{"ordinal": uint64(i), "units_code": uint64(d[at]), "number": hartIPNumber(d[at+1 : at+5])})
		}
		return map[string]any{"loop_current": hartIPNumber(d[:4]), "variables": vars}, nil
	}
	return nil, hartIPErr(ErrUnsupportedFeature, "selected universal commands0/1/2/3")
}
func hartIPMessage(w []byte, b ParserBudget) (map[string]any, *hartIPToken, error) {
	if len(w) > min(b.MaxFrameBytes, b.MaxMessageBytes) {
		return nil, nil, hartIPErr(ErrResourceExceeded, "message byte budget")
	}
	if b.MaxCollectionElements < 128 || b.MaxRecursionDepth < 7 {
		return nil, nil, hartIPErr(ErrResourceExceeded, "shared projection budget")
	}
	if len(w) < 8 {
		return nil, nil, hartIPErr(ErrMalformedMessage, "eight-byte header")
	}
	if w[0] != 1 {
		return nil, nil, hartIPErr(ErrUnsupportedVersion, "selected HARTIPv1")
	}
	n := int(binary.BigEndian.Uint16(w[6:8]))
	if n < 8 || n != len(w) {
		return nil, nil, hartIPErr(ErrMalformedMessage, "exact total byte count")
	}
	typ, mid, status := w[1], w[2], w[3]
	if typ != 0 && typ != 1 && typ != 2 && typ != 15 {
		return nil, nil, hartIPErr(ErrUnsupportedFeature, "request/response/publish/NAK")
	}
	if typ == 0 && status != 0 {
		return nil, nil, hartIPErr(ErrMalformedMessage, "request status must zero")
	}
	f := map[string]any{"version": uint64(w[0]), "message_type": uint64(typ), "message_id": uint64(mid), "status": uint64(status), "sequence_number": uint64(binary.BigEndian.Uint16(w[4:6])), "byte_count": uint64(n), "authentication_verified": false, "device_operation_verified": false}
	d := w[8:]
	if typ == 15 || typ == 1 && status != 0 {
		if len(d) != 0 {
			return nil, nil, hartIPErr(ErrUnsupportedFeature, "selected empty peer envelope error")
		}
		f["peer_error_observed"] = true
		return f, nil, nil
	}
	switch mid {
	case 0:
		if (typ != 0 && typ != 1) || len(d) != 5 {
			return nil, nil, hartIPErr(ErrMalformedMessage, "session initiate exact5")
		}
		if d[0] != 1 {
			return nil, nil, hartIPErr(ErrUnsupportedFeature, "selected primary master type1")
		}
		f["initiate"] = map[string]any{"master_type": uint64(d[0]), "inactivity_timeout_ms": uint64(binary.BigEndian.Uint32(d[1:]))}
		return f, nil, nil
	case 1, 2:
		if (typ != 0 && typ != 1) || len(d) != 0 {
			return nil, nil, hartIPErr(ErrMalformedMessage, "empty close/keepalive request/response")
		}
		return f, nil, nil
	case 3:
	default:
		return nil, nil, hartIPErr(ErrUnsupportedFeature, "selected session and token-passing messages")
	}
	if len(d) < 5 {
		return nil, nil, hartIPErr(ErrMalformedMessage, "token-passing prefix")
	}
	delim := d[0]
	long := delim&0x80 != 0
	addrlen := 1
	if long {
		addrlen = 5
	}
	if delim>>5&3 != 0 || delim>>3&3 != 0 {
		return nil, nil, hartIPErr(ErrUnsupportedFeature, "selected no expansion FSK token")
	}
	want := byte(2)
	if typ == 1 {
		want = 6
	} else if typ == 2 {
		want = 1
	}
	if delim&7 != want {
		return nil, nil, hartIPErr(ErrMalformedMessage, "message/delimiter type mismatch")
	}
	if len(d) < 1+addrlen+3 {
		return nil, nil, hartIPErr(ErrMalformedMessage, "token address/command/count/checksum")
	}
	addr := d[1 : 1+addrlen]
	cmd, size := d[1+addrlen], int(d[2+addrlen])
	start := 3 + addrlen
	if len(d) != start+size+1 {
		return nil, nil, hartIPErr(ErrMalformedMessage, "exact token byte count")
	}
	sum := byte(0)
	for _, v := range d {
		sum ^= v
	}
	if sum != 0 {
		return nil, nil, hartIPErr(ErrMalformedMessage, "token XOR checksum")
	}
	logical := bytes.Clone(addr)
	logical[0] &= 0x3f
	token := &hartIPToken{cmd, hex.EncodeToString(logical)}
	tp := map[string]any{"delimiter": uint64(delim), "long_address": long, "address_hex": hex.EncodeToString(addr), "logical_address_hex": token.address, "command": uint64(cmd), "data_count": uint64(size), "checksum": uint64(d[len(d)-1]), "checksum_valid": true}
	payload := d[start : len(d)-1]
	if cmd > 3 {
		return nil, nil, hartIPErr(ErrUnsupportedFeature, "selected universal commands0/1/2/3")
	}
	if typ == 0 {
		if len(payload) != 0 {
			return nil, nil, hartIPErr(ErrMalformedMessage, "selected read commands request no data")
		}
	} else {
		if len(payload) < 2 {
			return nil, nil, hartIPErr(ErrMalformedMessage, "response code and device status")
		}
		tp["response_code"], tp["device_status"], tp["command_success_observed"] = uint64(payload[0]), uint64(payload[1]), payload[0] == 0
		if payload[0] != 0 {
			if len(payload) != 2 {
				return nil, nil, hartIPErr(ErrUnsupportedFeature, "unselected peer error result bytes")
			}
		} else {
			result, err := hartIPCommandData(cmd, payload[2:])
			if err != nil {
				return nil, nil, err
			}
			tp["result"] = result
		}
	}
	f["token_passing"] = tp
	return f, token, nil
}

type hartIPKey struct {
	mid      byte
	sequence uint16
}
type hartIPPending struct {
	id    uint64
	wire  []byte
	token *hartIPToken
	dir   int
	at    time.Time
}
type binHARTIP struct {
	client        int
	phase         string
	pending       map[hartIPKey]hartIPPending
	seen          map[hartIPKey]bool
	peak          int
	clock         time.Time
	blocked       bool
	reserveMemory func(int64) error
}

func (p *binHARTIP) storage() int64 {
	n := int64(1024 + 512*len(p.seen) + 512*p.peak)
	for _, q := range p.pending {
		n += int64((len(q.wire) + 63) &^ 63)
	}
	return n
}

// Keep first-use history after expiry: a passive capture cannot prove that a
// reused sequence belongs to a new generation. Retries do not refresh its owner.
func (p *binHARTIP) expire(now time.Time) {
	if now.After(p.clock) {
		p.clock = now
	}
	for k, q := range p.pending {
		if p.clock.Sub(q.at) >= 30*time.Second {
			delete(p.pending, k)
		}
	}
}
func (p *binHARTIP) retire() { p.pending = nil; p.blocked = true }
func (p *binHARTIP) remember(k hartIPKey, b ParserBudget) error {
	if p.seen[k] {
		return nil
	}
	if len(p.seen) >= min(4096, b.MaxCollectionElements/8) {
		return hartIPErr(ErrResourceExceeded, "context/history count")
	}
	if p.reserveMemory != nil {
		if err := p.reserveMemory(p.storage() + 512); err != nil {
			return err
		}
	}
	if p.seen == nil {
		p.seen = make(map[hartIPKey]bool)
	}
	p.seen[k] = true
	return nil
}
func (p *binHARTIP) reject(w []byte, b ParserBudget) {
	if len(w) < 6 {
		p.retire()
		return
	}
	k := hartIPKey{w[2], binary.BigEndian.Uint16(w[4:6])}
	delete(p.pending, k)
	if err := p.remember(k, b); err != nil {
		p.retire()
	}
}
func (p *binHARTIP) consume(dir int, w []byte, id uint64, ts time.Time, b ParserBudget) (map[string]any, uint64, uint64, error) {
	p.expire(ts)
	if p.reserveMemory != nil {
		if err := p.reserveMemory(p.storage() + 8192 + int64(len(w))*256); err != nil {
			p.reject(w, b)
			return nil, 0, 0, err
		}
	}
	f, token, err := hartIPMessage(w, b)
	if err != nil {
		p.reject(w, b)
		return nil, 0, 0, err
	}
	if p.phase == "" {
		p.phase = "unverified"
	}
	typ, mid := w[1], w[2]
	key := hartIPKey{mid, binary.BigEndian.Uint16(w[4:6])}
	owner, reply := uint64(0), uint64(0)
	association := ""
	refuse := func(s string) (map[string]any, uint64, uint64, error) {
		p.reject(w, b)
		return nil, 0, 0, hartIPErr(ErrContextRequired, s)
	}
	if typ == 0 {
		if p.client < 0 {
			p.client = dir
		}
		if p.client != dir {
			return refuse("request in peer observed direction")
		}
		if old, ok := p.pending[key]; ok {
			if !bytes.Equal(old.wire, w) {
				return refuse("active sequence collision")
			}
			owner = old.id
			association = "active-request-retry"
		} else if p.blocked || p.seen[key] {
			association = "unverified-sequence-reuse"
		} else {
			if err = p.remember(key, b); err != nil {
				p.retire()
				return nil, 0, 0, err
			}
			next := p.storage() + int64(512*(max(p.peak, len(p.pending)+1)-p.peak)+((len(w)+63)&^63))
			if p.reserveMemory != nil {
				if err = p.reserveMemory(next); err != nil {
					p.retire()
					return nil, 0, 0, err
				}
			}
			if p.pending == nil {
				p.pending = make(map[hartIPKey]hartIPPending)
			}
			p.peak = max(p.peak, len(p.pending)+1)
			p.pending[key] = hartIPPending{id, bytes.Clone(w), token, dir, p.clock}
			owner = id
			association = "request-observed"
			if mid == 0 {
				p.phase = "initiate-request-observed"
			}
		}
	} else if typ == 1 || typ == 15 {
		if p.client >= 0 && p.client == dir {
			return refuse("response in originator observed direction")
		}
		q, ok := p.pending[key]
		delete(p.pending, key)
		if err = p.remember(key, b); err != nil {
			p.retire()
			return nil, 0, 0, err
		}
		association = "unverified-response"
		if ok && !p.blocked {
			if mid == 3 && token != nil && (q.token == nil || q.token.command != token.command || q.token.address != token.address) {
				return refuse("token command/address differs from request")
			}
			owner, reply = q.id, q.id
			association = "observed-pair"
			if typ == 1 && w[3] == 0 && (mid == 0 || mid == 1) {
				if mid == 0 {
					p.phase = "initiate-pair-observed"
				} else {
					p.phase = "close-pair-observed"
				}
				ids := make([]uint64, 0, len(p.pending))
				for _, old := range p.pending {
					ids = append(ids, old.id)
				}
				sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
				retired := make([]any, 0, len(ids))
				for _, v := range ids {
					retired = append(retired, v)
				}
				f["retired_request_ids"] = retired
				p.pending = nil
			}
		}
	} else {
		if p.client >= 0 && p.client == dir {
			return refuse("publish in originator observed direction")
		}
		association = "publish-observed"
	}
	f["association_status"], f["session_observation"] = association, p.phase
	if owner != 0 {
		f["observed_request_id"] = owner
	}
	return f, reply, owner, nil
}
func probeHARTIP(w []byte) ProbeResult {
	if len(w) < 8 || w[0] != 1 || int(binary.BigEndian.Uint16(w[6:8])) != len(w) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if _, _, err := hartIPMessage(w, DefaultParserBudget()); err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("hart-ip", "v1/universal-0-3", 98)
}
func (f *binFlow) frameHARTIP(w []byte) (int, *binSpec, error) {
	if len(w) < 8 {
		return 0, nil, nil
	}
	n := int(binary.BigEndian.Uint16(w[6:8]))
	if n < 8 {
		return 0, nil, hartIPErr(ErrMalformedMessage, "total byte count below header")
	}
	if n > f.a.config.MaxMessageBytes {
		return n, nil, nil
	}
	if len(w) < n {
		return 0, nil, nil
	}
	return n, &binSpec{}, nil
}

// WithSessionHARTIP selects the finite UDP observer on a nonstandard port.
func WithSessionHARTIP(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		c := &CaptureConfig{datagramDecodeAs: s.f.a.datagramDecodeAs}
		if err := WithProtocolDecodeAs("udp", port, "hart-ip")(c); err != nil {
			return err
		}
		s.f.a.datagramDecodeAs = c.datagramDecodeAs
		return nil
	}
}
func (a *binParser) hartSelected(src, dst uint16) bool {
	v := a.datagramDecodeAs[dst]
	if v == "" {
		v = a.datagramDecodeAs[src]
	}
	return v == "hart-ip" || v == "" && (src == 5094 || dst == 5094)
}

// A datagram never borrows bytes or time from another datagram/conversation.
// Failed unretained admission quarantines future ownership rather than inventing
// a new generation for a key that may already have been observed.
func (a *binParser) decodeHARTIPDatagram(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.nativeHARTIPObservation = true
	e.Protocol, e.Profile, e.Completeness = "hart-ip", hartIPProfile, "message"
	e.Admission = "port-and-wire"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	key := binUDPKey{"hart-ip/" + e.Source, "hart-ip/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	if a.udpSessions == nil {
		a.udpSessions = &binUDPStore{entries: make(map[binUDPKey]*list.Element), clock: e.Timestamp}
	}
	store := a.udpSessions
	el := store.entries[key]
	err := inputErr
	if el == nil && err == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = hartIPErr(ErrResourceExceeded, "UDP conversation count")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "hart-ip", endpoints: [2]string{e.Source, e.Destination}}
			f.hart = &binHARTIP{client: -1, blocked: a.hartUntrackedRefusal, reserveMemory: f.reserveSession}
			if err = f.reserveSession(1024 + 8192 + int64(len(w))*256); err == nil {
				el = store.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
				store.entries[key] = el
			}
		}
	}
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		f := entry.flow
		if e.Timestamp.After(entry.touched) {
			entry.touched = e.Timestamp
		}
		store.lru.MoveToBack(el)
		e.FlowID, e.Direction = f.id, 0
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		f.hart.expire(e.Timestamp)
		if err == nil {
			err = f.reserveSession(f.hart.storage() + 8192 + int64(len(w))*256)
		}
		if err == nil {
			e.Session, e.ResponseTo, e.TransactionID, err = f.hart.consume(e.Direction, w, e.ID, e.Timestamp, a.budget)
		} else {
			f.hart.reject(w, a.budget)
		}
		retained := f.hart.storage()
		if retained < f.sessionBytes {
			a.buffered.Add(retained - f.sessionBytes)
			f.sessionBytes = retained
		}
	} else {
		a.hartUntrackedRefusal = true
	}
	a.udpMu.Unlock()
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
		e.semanticSession = cloneSession(e.Session)
	}
	if inputErr != nil && len(w) > a.config.MaxMessageBytes {
		a.limited.Add(uint64(len(w)))
		w = nil
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}

// A typed refusal of a fully delimited v1 message poisons only its observed
// sequence. Its exact bounded outer length still supplies the next boundary.
// Unknown versions and framing failures do not receive this recovery scope.
func hartIPRecoverable(w []byte, e *ProtocolError) bool {
	if e == nil || len(w) < 8 || w[0] != 1 || int(binary.BigEndian.Uint16(w[6:8])) != len(w) {
		return false
	}
	switch e.Kind {
	case ErrMalformedMessage, ErrUnsupportedFeature, ErrContextRequired, ErrResourceExceeded:
		return true
	}
	return false
}

func (f *binFlow) newHARTIP() *binHARTIP {
	p := &binHARTIP{client: -1, reserveMemory: f.reserveSession}
	if f.tcpClientKnown {
		p.client = f.tcpClientDir
	}
	return p
}
