package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"
)

const finsProfile = "fins-selected-wire-observation"

// W342-E1-18 and W421-E1-04: this is a passive, finite wire observer.
// An observed response link does not authenticate a PLC or prove execution.
type finsPending struct {
	wire []byte
	id   uint64
	at   time.Time
}
type binFINS struct {
	clock       time.Time
	clientKnown bool
	clientDir   int
	seen        [2][256]bool
	pending     [2]map[byte]*finsPending
	blocked     bool
	nodeSeen    bool
	nodePending bool
	nodeDir     int
	nodeID      uint64
	requested   uint32
	assigned    [2]uint32
	nodeReady   bool
	closed      bool
}

func (s *binFINS) storage() int64 {
	n := int64(1024)
	for _, m := range s.pending {
		for _, p := range m {
			n += 256 + int64(len(p.wire))
		}
	}
	return n
}
func (s *binFINS) retire() {
	s.pending = [2]map[byte]*finsPending{}
	s.blocked = true
	s.nodePending = false
	s.nodeReady = false
	s.nodeSeen = true
}
func (s *binFINS) expire(now time.Time) {
	if now.After(s.clock) {
		s.clock = now
	} else {
		now = s.clock
	}

	for d, m := range s.pending {
		for sid, p := range m {
			if now.Sub(p.at) >= 30*time.Second {
				delete(s.pending[d], sid)
			}
		}
	}
}
func (s *binFINS) counts() [2]int {
	n := [2]int{len(s.pending[0]), len(s.pending[1])}
	if s.nodePending {
		n[s.nodeDir]++
	}
	return n
}
func finsErr(k ProtocolErrorKind, why string) error { return protocolError(k, "FINS: "+why) }

// Fixed TCP envelope. Length excludes magic and length itself. Reserved error
// words in commands0/2/6 are observations, not receiver rejection criteria.
func finsFrameSize(w []byte, limit int) (int, error) {
	if len(w) < 4 {
		return 0, nil
	}
	if !bytes.Equal(w[:4], []byte("FINS")) {
		return 0, finsErr(ErrDesynchronized, "TCP magic changed")
	}
	if len(w) < 8 {
		return 0, nil
	}
	n := uint64(binary.BigEndian.Uint32(w[4:8]))
	if n+8 > uint64(limit) {
		return 0, finsErr(ErrResourceExceeded, "TCP length exceeds byte budget")
	}
	if n < 8 || n > 2020 {
		return 0, finsErr(ErrMalformedMessage, "TCP envelope length")
	}
	if len(w) < 16 {
		return 0, nil
	}
	cmd := binary.BigEndian.Uint32(w[8:12])
	valid := cmd == 0 && n == 12 || cmd == 1 && n == 16 || cmd == 2 && n >= 20 || (cmd == 3 || cmd == 6) && n == 8
	if !valid {
		if cmd != 0 && cmd != 1 && cmd != 2 && cmd != 3 && cmd != 6 {
			return 0, finsErr(ErrUnsupportedFeature, "TCP command")
		}
		return 0, finsErr(ErrMalformedMessage, "TCP command length")
	}
	if uint64(len(w)) < n+8 {
		return 0, nil
	}
	return int(n + 8), nil
}
func (f *binFlow) frameFINS(w []byte) (int, *binSpec, error) {
	n, err := finsFrameSize(w, min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes))
	if err == nil && n > 0 {
		err = f.reserveSession(f.fins.storage() + 8192 + int64(n)*256)
	}
	if err != nil {
		f.fins.retire()
		var pe *ProtocolError
		if errors.As(err, &pe) {
			err = pe
		}
	}
	return n, &binSpec{}, err
}
func finsRecoverable(e *ProtocolError) bool {
	return e != nil && (e.Kind == ErrMalformedMessage || e.Kind == ErrContextRequired || e.Kind == ErrUnsupportedFeature || e.Kind == ErrUnsupportedVersion)
}
func finsHex(w []byte) string { return hex.EncodeToString(w) }
func finsBE(w []byte) uint64  { return uint64(binary.BigEndian.Uint16(w)) }

func (s *binFINS) consumeTCP(w []byte, dir int, id uint64, now time.Time, b ParserBudget) (map[string]any, uint64, error) {
	// Every validated envelope advances capture time, including discarded
	// confirmations. A later timestamp regression cannot revive old ownership.
	s.expire(now)
	now = s.clock

	cmd, code := binary.BigEndian.Uint32(w[8:12]), binary.BigEndian.Uint32(w[12:16])
	out := map[string]any{"tcp_command": uint64(cmd), "tcp_error": uint64(code), "tcp_length": uint64(len(w) - 8), "authenticated": false}
	if b.MaxCollectionElements < 32 || b.MaxRecursionDepth < 4 {
		s.retire()
		return nil, 0, finsErr(ErrResourceExceeded, "field projection budget")
	}
	if s.closed {
		return nil, 0, finsErr(ErrContextRequired, "TCP notification retired this observation")
	}
	switch cmd {
	case 0:
		n := binary.BigEndian.Uint32(w[16:20])
		if n > 254 {
			s.retire()
			return nil, 0, finsErr(ErrMalformedMessage, "client node request")
		}
		out["client_node"] = uint64(n)
		// W421 node0 is the TCP client's address-exchange request. A known
		// SYN initiator contradicting that role cannot establish allocation.
		if s.clientKnown && dir != s.clientDir {
			s.retire()
			out["allocation_status"] = "unverified-tcp-role"
			return out, 0, nil
		}
		if s.nodeSeen {
			s.retire()
			out["allocation_status"] = "unverified-repeated-exchange"
			return out, 0, nil
		}
		s.nodeSeen, s.nodePending, s.nodeDir, s.nodeID, s.requested = true, true, dir, id, n
		out["allocation_status"] = "request-observed"
	case 1:
		c, n := binary.BigEndian.Uint32(w[16:20]), binary.BigEndian.Uint32(w[20:24])
		out["client_node"], out["server_node"] = uint64(c), uint64(n)
		if code == 0 && (c == 0 || c > 254 || n == 0 || n > 254) {
			s.retire()
			return nil, 0, finsErr(ErrMalformedMessage, "normal node response needs nodes1..254")
		}
		eligible := s.nodePending && dir != s.nodeDir && code == 0 && c > 0 && c <= 254 && n > 0 && n <= 254 && (s.requested == 0 || s.requested == c)
		s.nodePending = false
		s.nodeSeen = true
		if eligible {
			s.nodeReady = true
			s.assigned[s.nodeDir] = c
			s.assigned[1-s.nodeDir] = n
			out["allocation_status"] = "observed-pair"
			return out, s.nodeID, nil
		}
		s.retire()
		out["allocation_status"] = "unverified-response"
	case 2:
		fields, reply, err := s.consume(w[16:], dir, id, now, b, true)
		if err != nil {
			return nil, 0, err
		}
		out["fins"] = fields
		return out, reply, nil
	case 3:
		s.retire()
		s.closed = true
		out["notification"] = "connection-retired"
	case 6:
		out["notification"] = "confirmation-discarded"
	}
	return out, 0, nil
}

// Return literal header fields first; response reserved bits and GCT are not
// equality constraints (W342 section2-2). Never infer source node aliases.
func finsHeader(w []byte, b ParserBudget) (map[string]any, error) {
	if len(w) < 12 {
		return nil, finsErr(ErrMalformedMessage, "short inner header/command")
	}
	if len(w) > min(2012, min(b.MaxFrameBytes, b.MaxMessageBytes)) {
		return nil, finsErr(ErrResourceExceeded, "inner frame byte budget")
	}
	if b.MaxCollectionElements < 32 || b.MaxRecursionDepth < 4 {
		return nil, finsErr(ErrResourceExceeded, "field projection budget")
	}
	if w[0]&0x80 == 0 {
		return nil, finsErr(ErrUnsupportedVersion, "selected Ethernet FINS header")
	}
	response := w[0]&0x40 != 0
	if !response && (w[0]&0x3e != 0 || w[1] != 0) {
		return nil, finsErr(ErrMalformedMessage, "command reserved header bits")
	}
	f := map[string]any{"icf": uint64(w[0]), "reserved": uint64(w[1]), "gateway_count": uint64(w[2]), "destination_network": uint64(w[3]), "destination_node": uint64(w[4]), "destination_unit": uint64(w[5]), "source_network": uint64(w[6]), "source_node": uint64(w[7]), "source_unit": uint64(w[8]), "sid": uint64(w[9]), "command": finsBE(w[10:12]), "response": response, "response_required": w[0]&1 == 0, "authenticated": false, "device_operation_verified": false}
	return f, nil
}
func (s *binFINS) consume(w []byte, dir int, id uint64, now time.Time, b ParserBudget, tcp bool) (map[string]any, uint64, error) {
	s.expire(now)
	now = s.clock
	f, err := finsHeader(w, b)
	// Any rejected frame with an observable SID retires that direction's request
	// or the opposite response slot. A refusal cannot leave an old owner alive.
	invalidate := func() {
		if len(w) >= 10 {
			d := dir
			if w[0]&0x40 != 0 {
				d = 1 - dir
			}
			delete(s.pending[d], w[9])
			s.seen[d][w[9]] = true
		} else {
			s.retire()
		}
	}
	if err != nil {
		invalidate()
		return nil, 0, err
	}
	sid, cmd, response := w[9], binary.BigEndian.Uint16(w[10:12]), w[0]&0x40 != 0
	if cmd != 0x0101 && cmd != 0x0501 {
		invalidate()
		return nil, 0, finsErr(ErrUnsupportedFeature, "selected Memory Area Read/CPU Unit Data Read only")
	}
	var p *finsPending
	if response {
		p = s.pending[1-dir][sid]
	}
	if !response {
		if cmd == 0x0101 {
			if len(w) != 18 {
				invalidate()
				return nil, 0, finsErr(ErrMalformedMessage, "DM read request exact length")
			}
			if w[15] > 15 {
				invalidate()
				return nil, 0, finsErr(ErrMalformedMessage, "address bit exceeds four-bit range")
			}
			if w[12] != 0x82 {
				invalidate()
				return nil, 0, finsErr(ErrUnsupportedFeature, "selected DM word area82 only")
			}
			if w[15] != 0 {
				invalidate()
				return nil, 0, finsErr(ErrMalformedMessage, "word address bit must be zero")
			}
			f["memory_area"], f["word_address"], f["bit_address"], f["word_count"] = uint64(w[12]), finsBE(w[13:15]), uint64(w[15]), finsBE(w[16:18])
		} else {
			if len(w) < 12 || len(w) > 13 {
				invalidate()
				return nil, 0, finsErr(ErrMalformedMessage, "CPU request option length")
			}
			option := uint64(255)
			if len(w) == 13 {
				option = uint64(w[12])
				if option > 1 {
					invalidate()
					return nil, 0, finsErr(ErrUnsupportedFeature, "CPU option00/01/absent only")
				}
			}
			f["data_option"] = option
		}
		f["association_status"] = "unverified-request"
		if s.blocked || s.closed || w[0]&1 != 0 || w[3] > 127 || w[6] > 127 || w[4] == 0 || w[4] == 255 || w[7] == 0 || w[7] == 255 {
			invalidate()
			return f, 0, nil
		}
		if tcp && s.nodeReady && (uint32(w[7]) != s.assigned[dir] || uint32(w[4]) != s.assigned[1-dir]) {
			invalidate()
			f["association_status"] = "unverified-node-allocation-mismatch"
			return f, 0, nil
		}
		if old := s.pending[dir][sid]; old != nil && bytes.Equal(old.wire, w) {
			f["association_status"] = "active-request-retry"
			f["observed_request_id"] = old.id
			return f, 0, nil
		}
		if s.seen[dir][sid] {
			invalidate()
			f["association_status"] = "unverified-sid-reuse"
			return f, 0, nil
		}
		s.seen[dir][sid] = true
		if len(s.pending[0])+len(s.pending[1]) >= min(512, b.MaxCollectionElements) {
			return nil, 0, finsErr(ErrResourceExceeded, "pending request count")
		}
		if s.pending[dir] == nil {
			s.pending[dir] = make(map[byte]*finsPending)
		}
		s.pending[dir][sid] = &finsPending{append([]byte(nil), w...), id, now}
		f["association_status"] = "request-observed"
		f["observed_request_id"] = id
		return f, 0, nil
	}
	delete(s.pending[1-dir], sid)
	s.seen[1-dir][sid] = true
	if len(w) < 14 {
		return nil, 0, finsErr(ErrMalformedMessage, "short response end code")
	}
	code := binary.BigEndian.Uint16(w[12:14])
	f["end_code"], f["end_code_base"], f["end_code_flags"] = uint64(code), uint64(code&0x7f3f), uint64(code&0x80c0)
	linked := p != nil && !s.blocked && binary.BigEndian.Uint16(p.wire[10:12]) == cmd && bytes.Equal(p.wire[3:6], w[6:9]) && bytes.Equal(p.wire[6:9], w[3:6])
	f["association_status"] = "unverified-response"
	reply := uint64(0)
	if linked {
		reply = p.id
		f["association_status"] = "observed-pair"
		f["observed_request_id"] = p.id
	}
	if code&0x8000 != 0 {
		return nil, 0, finsErr(ErrUnsupportedFeature, "relay-error envelope not selected")
	}
	data := w[14:]
	f["data_hex"] = finsHex(data)
	if code&0x7f3f != 0 {
		if len(data) != 0 {
			return nil, 0, finsErr(ErrUnsupportedFeature, "unselected error response data")
		}
		return f, reply, nil
	}
	if cmd == 0x0101 {
		if !linked {
			return nil, 0, finsErr(ErrContextRequired, "DM response lacks exact reversed request/address/count context")
		}
		count := int(binary.BigEndian.Uint16(p.wire[16:18]))
		if count > 999 {
			return nil, 0, finsErr(ErrUnsupportedFeature, "successful Ethernet DM read exceeds999 words")
		}
		if len(data) != 2*count {
			return nil, 0, finsErr(ErrMalformedMessage, "DM response word count")
		}
		if count > b.MaxCollectionElements-32 {
			return nil, 0, finsErr(ErrResourceExceeded, "DM word projection count")
		}
		words := make([]any, count)
		for i := range words {
			words[i] = finsBE(data[2*i:])
		}
		f["memory_area"], f["word_address"], f["bit_address"], f["word_count"], f["words"] = uint64(0x82), finsBE(p.wire[13:15]), uint64(0), uint64(count), words
	} else {
		option := uint64(255)
		switch len(data) {
		case 92:
			option = 0
		case 67:
			option = 1
		case 159:
		default:
			return nil, 0, finsErr(ErrMalformedMessage, "CPU response92/67/159 byte layout")
		}
		if linked {
			expected := uint64(255)
			if len(p.wire) == 13 {
				expected = uint64(p.wire[12])
			}
			if expected != option {
				return nil, 0, finsErr(ErrMalformedMessage, "CPU response differs from observed option")
			}
		}
		f["data_option"], f["option_context_observed"] = option, linked
		if b.MaxCollectionElements < 64 || len(data) == 159 && b.MaxCollectionElements < 96 {
			return nil, 0, finsErr(ErrResourceExceeded, "CPU shared projection count")
		}
		if len(data) == 92 || len(data) == 159 {
			area := data[80:92]
			f["cpu_data"] = map[string]any{"model_hex": finsHex(data[:20]), "version_hex": finsHex(data[20:40]), "system_use_hex": finsHex(data[40:80]), "program_kwords": finsBE(area[:2]), "io_kbytes": uint64(area[2]), "dm_words": finsBE(area[3:5]), "timer_counter_kwords": uint64(area[5]), "em_banks": uint64(area[6]), "area_reserved_hex": finsHex(area[7:9]), "memory_card_type": uint64(area[9]), "memory_card_kbytes": finsBE(area[10:12])}
			data = data[92:]
		}
		if len(data) == 67 {
			if b.MaxCollectionElements < 64 {
				return nil, 0, finsErr(ErrResourceExceeded, "CPU bus projection count")
			}
			ids := make([]any, 16)
			for i := range ids {
				ids[i] = finsBE(data[i*2:])
			}
			f["cpu_bus"] = map[string]any{"unit_words": ids, "reserved_hex": finsHex(data[32:64]), "remote_io_reserved": uint64(data[64]), "remote_io_masters": uint64(data[65]), "cpu_unit_info": uint64(data[66])}
		}
	}
	return f, reply, nil
}

// WithSessionFINS selects the finite UDP wire observer on a caller-specified
// port. It does not infer node-zero aliases or authenticate PLC/session identity.
func WithSessionFINS(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		c := &CaptureConfig{datagramDecodeAs: s.f.a.datagramDecodeAs}
		if err := WithProtocolDecodeAs("udp", port, "fins")(c); err != nil {
			return err
		}
		s.f.a.datagramDecodeAs = c.datagramDecodeAs
		return nil
	}
}
func (a *binParser) finsSelected(src, dst uint16) bool {
	selected := a.datagramDecodeAs[dst]
	if selected == "" {
		selected = a.datagramDecodeAs[src]
	}
	return selected == "fins"
}
func (a *binParser) decodeFINSDatagram(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.nativeFINSObservation = true
	e.Protocol, e.Profile, e.Admission, e.Completeness = "fins", finsProfile, "explicit-decode-as", "message"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	key := binUDPKey{"fins/" + e.Source, "fins/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	if a.udpSessions == nil {
		a.udpSessions = &binUDPStore{entries: make(map[binUDPKey]*list.Element), clock: e.Timestamp}
	}
	store := a.udpSessions
	if e.Timestamp.After(store.clock) {
		store.clock = e.Timestamp
	}
	for _, el := range store.entries {
		f := el.Value.(*binUDPEntry).flow
		if f.fins != nil {
			f.fins.expire(store.clock)
		}
	}
	el := store.entries[key]
	err := inputErr
	if el == nil && err == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = finsErr(ErrResourceExceeded, "UDP conversation count")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "fins", fins: &binFINS{blocked: a.finsUntrackedRefusal}, endpoints: [2]string{e.Source, e.Destination}}
			if err = f.reserveSession(1024 + 8192 + int64(len(w))*256); err == nil {
				el = store.lru.PushBack(&binUDPEntry{key, f, store.clock})
				store.entries[key] = el
			}
		}
	}
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		f := entry.flow
		entry.touched = store.clock
		store.lru.MoveToBack(el)
		e.FlowID, e.Direction = f.id, 0
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		if err == nil {
			err = f.reserveSession(f.fins.storage() + 8192 + int64(len(w))*256)
		}
		if err == nil {
			e.Session, e.ResponseTo, err = f.fins.consume(w, e.Direction, e.ID, store.clock, a.budget, false)
		} else {
			f.fins.retire()
		}
		// Shrink transient projections and discarded request bytes; retain fixed SID
		// history even after expiry/refusal so old replies cannot claim a new owner.
		retained := f.fins.storage()
		if retained < f.sessionBytes {
			a.buffered.Add(retained - f.sessionBytes)
			f.sessionBytes = retained
		}
	} else {
		a.finsUntrackedRefusal = true
	}
	a.udpMu.Unlock()
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
		e.semanticSession = cloneSession(e.Session)
		if e.ResponseTo != 0 {
			e.TransactionID = e.ResponseTo
		} else if id, ok := e.Session["observed_request_id"].(uint64); ok {
			e.TransactionID = id
		}
	}
	if inputErr != nil && len(w) > a.config.MaxMessageBytes {
		a.limited.Add(uint64(len(w)))
		w = nil
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}
