package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"math"
	"time"
	"unsafe"
)

const bacnetReadPropertyProfile = "bacnet-ip-read-property"
const bacnetReadPropertyTTL = 30 * time.Second
const bacnetConversationBytes = int64(unsafe.Sizeof(binFlow{})) + 1024

type bacnetProperty struct {
	object, property, index uint32
	hasIndex                bool
}
type bacnetReadMessage struct {
	fields        map[string]any
	property      bacnetProperty
	invoke        byte
	request       bool
	terminal      bool
	routes        *bacnetRoutes
	namespaceOnly bool
}

func bacnetReadError(kind ProtocolErrorKind, why string) error {
	return protocolError(kind, "BACnet ReadProperty: %s", why)
}

// Only primitive application values are decoded here. Context-specific and
// constructed property values need their own schema; retaining their bytes as
// a successfully decoded primitive would invent a property interpretation.
func bacnetApplicationValue(w []byte) (map[string]any, int, error) {
	bad := func(why string) (map[string]any, int, error) {
		return nil, 0, bacnetReadError(ErrMalformedMessage, why)
	}
	if len(w) == 0 {
		return bad("missing application tag")
	}
	tag, lvt := w[0]>>4, w[0]&7
	if w[0]&8 != 0 || tag > 12 {
		return nil, 0, bacnetReadError(ErrUnsupportedFeature, "context or extended application value")
	}
	n, at := int(lvt), 1
	if tag == 1 {
		if lvt > 1 {
			return bad("Boolean LVT must be zero or one")
		}
		return map[string]any{"Type": "Boolean", "Value": lvt == 1, "Encoded": bytes.Clone(w[:1])}, 1, nil
	}
	if lvt >= 6 {
		return bad("opening or closing application tag")
	}
	if lvt == 5 {
		if len(w) < 2 {
			return bad("truncated extended length")
		}
		n, at = int(w[1]), 2
		if n == 254 {
			if len(w) < 4 {
				return bad("truncated two-byte length")
			}
			n, at = int(binary.BigEndian.Uint16(w[2:4])), 4
		} else if n == 255 {
			if len(w) < 6 {
				return bad("truncated four-byte length")
			}
			u := uint64(binary.BigEndian.Uint32(w[2:6]))
			if u > uint64(len(w)-6) {
				return bad("extended length exceeds datagram")
			}
			n, at = int(u), 6
		}
	}
	if n > len(w)-at {
		return bad("application length exceeds datagram")
	}
	p := w[at : at+n]
	names := []string{"Null", "Boolean", "Unsigned", "Signed", "Real", "Double", "OctetString", "CharacterString", "BitString", "Enumerated", "Date", "Time", "ObjectIdentifier"}
	f := map[string]any{"Type": names[tag], "Encoded": bytes.Clone(w[:at+n])}
	unsigned := func() uint64 {
		var x uint64
		for _, b := range p {
			x = x<<8 | uint64(b)
		}
		return x
	}
	switch tag {
	case 0:
		if n != 0 {
			return bad("Null requires zero length")
		}
		f["Value"] = nil
	case 2, 3, 9:
		if n < 1 || n > 8 || (tag == 9 || tag == 3) && n > 4 {
			return bad("integer width exceeds selected unsigned64 or signed/enumerated32")
		}
		u := unsigned()
		if tag == 3 {
			if p[0]&128 != 0 && n < 8 {
				u |= ^uint64(0) << (uint(n) * 8)
			}
			f["Value"] = int64(u)
		} else if tag == 9 {
			f["Value"] = uint32(u)
		} else {
			f["Value"] = u
		}
	case 4, 5:
		if tag == 4 && n != 4 || tag == 5 && n != 8 {
			return bad("IEEE value length mismatch")
		}
		f["Raw Bits"] = hex.EncodeToString(p)
		var v float64
		if tag == 4 {
			v = float64(math.Float32frombits(binary.BigEndian.Uint32(p)))
		} else {
			v = math.Float64frombits(binary.BigEndian.Uint64(p))
		}
		if math.IsNaN(v) {
			f["Value"], f["Nonfinite"] = nil, "NaN"
		} else if math.IsInf(v, 1) {
			f["Value"], f["Nonfinite"] = nil, "+Infinity"
		} else if math.IsInf(v, -1) {
			f["Value"], f["Nonfinite"] = nil, "-Infinity"
		} else {
			f["Value"] = v
		}
	case 6:
		f["Value"] = bytes.Clone(p)
	case 7:
		if n < 1 {
			return bad("character string requires encoding octet")
		}
		f["Encoding"], f["Value"] = p[0], bytes.Clone(p[1:])
	case 8:
		if n < 1 || p[0] > 7 || n == 1 && p[0] != 0 {
			return bad("invalid bit-string unused count")
		}
		if n > 1 && p[len(p)-1]&byte((1<<p[0])-1) != 0 {
			return bad("nonzero unused bit-string bits")
		}
		f["Unused Bits"], f["Bit Count"], f["Value"] = p[0], (n-1)*8-int(p[0]), bytes.Clone(p[1:])
	case 10, 11:
		if n != 4 {
			return bad("date/time requires four octets")
		}
		// Preserve wildcard/special calendar values. No wall clock or timezone
		// is inferred from these wire observations.
		f["Value"] = bytes.Clone(p)
	case 12:
		if n != 4 {
			return bad("object identifier requires four bytes")
		}
		u := binary.BigEndian.Uint32(p)
		f["Object Type"], f["Object Instance"] = u>>22, u&0x3fffff
	}
	return f, at + n, nil
}

func decodeBACnetReadProperty(w []byte, maxBytes, limit int) (*bacnetReadMessage, error) {
	return decodeBACnetReadPropertyProfile(w, maxBytes, limit, false)
}

func decodeBACnetReadPropertyProfile(w []byte, maxBytes, limit int, routed bool) (*bacnetReadMessage, error) {
	bad := func(why string) (*bacnetReadMessage, error) { return nil, bacnetReadError(ErrMalformedMessage, why) }
	if len(w) > maxBytes {
		return nil, bacnetReadError(ErrResourceExceeded, "message budget")
	}
	if len(w) < 6 || w[0] != 0x81 || int(binary.BigEndian.Uint16(w[2:4])) != len(w) {
		return bad("BVLC header/length")
	}
	if w[1] != 10 {
		return nil, bacnetReadError(ErrUnsupportedFeature, "original unicast required")
	}
	if w[4] != 1 {
		return nil, bacnetReadError(ErrUnsupportedVersion, "NPDU version one required")
	}
	if !routed && w[5]&0xf8 != 0 {
		return nil, bacnetReadError(ErrUnsupportedFeature, "routed or network NPDU")
	}
	offset := 6
	var routes *bacnetRoutes
	if routed {
		var err error
		routes, err = decodeBACnetRoutes(w)
		if err != nil {
			return nil, err
		}
		offset = routes.offset
	}
	a := w[offset:]
	if len(a) >= 2 && a[0] == 0x10 {
		return nil, bacnetReadError(ErrUnsupportedFeature, "selected unsegmented request/ComplexACK required")
	}
	if len(a) < 3 {
		return bad("short APDU")
	}
	m := &bacnetReadMessage{fields: map[string]any{"BVLC Type": w[0], "BVLC Function": w[1], "BVLC Length": uint16(len(w)), "NPDU Version": w[4], "NPDU Control": w[5], "Complete BVLC PDU": bytes.Clone(w), "APDU": bytes.Clone(a), "Service": "ReadProperty", "Service Choice": uint8(12)}}
	m.routes = routes
	if routes != nil {
		routes.fields(m.fields)
	}
	var body []byte
	switch a[0] >> 4 {
	case 0:
		if a[0]&12 != 0 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "segmented confirmed request")
		}
		if a[0]&1 != 0 || len(a) < 4 {
			return bad("confirmed request flags/header")
		}
		if a[1]&0x80 != 0 || a[1]>>4 > 7 || a[1]&15 > 5 {
			return bad("max-segments/max-APDU encoding")
		}
		if a[3] != 12 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "selected service twelve required")
		}
		m.invoke, m.request, body = a[2], true, a[4:]
		m.fields["APDU Type"], m.fields["Max Segments Code"], m.fields["Max APDU Code"], m.fields["Segmented Response Accepted"] = "Confirmed-Request", a[1]>>4, a[1]&15, a[0]&2 != 0
	case 3:
		if a[0]&12 != 0 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "segmented complex acknowledgement")
		}
		if a[0]&3 != 0 {
			return bad("complex acknowledgement flags")
		}
		if a[2] != 12 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "selected service twelve required")
		}
		m.invoke, body = a[1], a[3:]
		m.fields["APDU Type"] = "ComplexACK"
	case 5:
		if a[0]&15 != 0 {
			return bad("Error APDU flags")
		}
		if a[2] != 12 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "selected Error service twelve required")
		}
		m.invoke, m.terminal = a[1], true
		m.fields["Invoke ID"], m.fields["APDU Type"] = m.invoke, "Error"
		body = a[3:]
		for _, key := range []string{"Error Class", "Error Code"} {
			v, n, err := bacnetApplicationValue(body)
			if err != nil {
				return nil, err
			}
			if v["Type"] != "Enumerated" {
				return bad("Error requires enumerated class/code")
			}
			m.fields[key] = v["Value"]
			body = body[n:]
		}
		if len(body) != 0 {
			return bad("trailing Error arguments")
		}
		return m, nil
	case 6, 7:
		if len(a) != 3 || a[0]&14 != 0 || a[0]>>4 == 6 && a[0]&1 != 0 {
			return bad("Reject/Abort flags or length")
		}
		if a[0]>>4 == 7 && a[0]&1 == 0 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "client Abort observation requires requester-side cancellation profile")
		}
		m.invoke, m.terminal = a[1], true
		m.fields["Invoke ID"], m.fields["Reason"] = m.invoke, a[2]
		delete(m.fields, "Service Choice")
		m.fields["Service Choice Present"] = false
		if a[0]>>4 == 6 {
			m.fields["APDU Type"] = "Reject"
		} else {
			m.fields["APDU Type"], m.fields["Server"] = "Abort", true
		}
		return m, nil
	default:
		return nil, bacnetReadError(ErrUnsupportedFeature, "selected unsegmented request/ComplexACK required")
	}
	readContext := func(tag byte) ([]byte, error) {
		if len(body) == 0 || body[0]>>4 != tag || body[0]&8 == 0 || body[0]&7 >= 6 {
			return nil, bacnetReadError(ErrMalformedMessage, "unsigned context tag/length")
		}
		n, at := int(body[0]&7), 1
		if n == 5 {
			if len(body) < 2 {
				return nil, bacnetReadError(ErrMalformedMessage, "short extended context length")
			}
			n, at = int(body[1]), 2
			if n == 254 {
				if len(body) < 4 {
					return nil, bacnetReadError(ErrMalformedMessage, "short extended context length")
				}
				n, at = int(binary.BigEndian.Uint16(body[2:4])), 4
			} else if n == 255 {
				if len(body) < 6 {
					return nil, bacnetReadError(ErrMalformedMessage, "short extended context length")
				}
				u := uint64(binary.BigEndian.Uint32(body[2:6]))
				if u > uint64(len(body)-6) {
					return nil, bacnetReadError(ErrMalformedMessage, "short unsigned context value")
				}
				n, at = int(u), 6
			}
		}
		if n < 1 || n > 4 {
			return nil, bacnetReadError(ErrMalformedMessage, "unsigned context tag/length")
		}
		if n > len(body)-at {
			return nil, bacnetReadError(ErrMalformedMessage, "short unsigned context value")
		}
		p := body[at : at+n]
		body = body[at+n:]
		return p, nil
	}
	obj, objectErr := readContext(0)
	if objectErr != nil || len(obj) != 4 {
		return bad("object identifier context tag zero")
	}
	m.property.object = binary.BigEndian.Uint32(obj)
	readUint := func(tag byte) (uint32, error) {
		p, err := readContext(tag)
		if err != nil {
			return 0, err
		}
		var u uint32
		for _, b := range p {
			u = u<<8 | uint32(b)
		}
		return u, nil
	}
	var err error
	if m.property.property, err = readUint(1); err != nil {
		return nil, err
	}
	if len(body) > 0 && body[0]>>4 == 2 {
		m.property.hasIndex = true
		if m.property.index, err = readUint(2); err != nil {
			return nil, err
		}
	}
	f := m.fields
	f["Invoke ID"], f["Object Type"], f["Object Instance"], f["Property Identifier"], f["Array Index Present"] = m.invoke, m.property.object>>22, m.property.object&0x3fffff, m.property.property, m.property.hasIndex
	if m.property.hasIndex {
		f["Array Index"] = m.property.index
	}
	if m.request {
		if len(body) != 0 {
			return bad("trailing request arguments")
		}
	} else {
		if len(body) < 2 || body[0] != 0x3e {
			return bad("opening value context three")
		}
		body = body[1:]
		values := []any{}
		for len(body) > 0 && body[0] != 0x3f {
			if body[0]&15 == 15 {
				return bad("unexpected closing value context")
			}
			if len(values) >= limit {
				return nil, bacnetReadError(ErrResourceExceeded, "application value count")
			}
			v, n, e := bacnetApplicationValue(body)
			if e != nil {
				return nil, e
			}
			values = append(values, v)
			body = body[n:]
		}
		if len(body) != 1 || body[0] != 0x3f {
			return bad("closing value context three/trailing bytes")
		}
		f["Values"] = values
	}
	if len(f) > limit {
		return nil, bacnetReadError(ErrResourceExceeded, "field collection budget")
	}
	return m, nil
}

type bacnetReadPending struct {
	id       uint64
	property bacnetProperty
	at       time.Time
	pending  bool
}
type binBACnetRead struct {
	seen    map[uint16]*bacnetReadPending
	blocked bool
	routed  bool
}

type bacnetInvokeOrigin struct {
	domain CaptureDomain
	source string
	invoke byte
}
type bacnetInvokeHistory struct {
	key   binUDPKey
	flow  *binFlow
	token uint16
}

func (a *binParser) correlateBACnetRead(e *ProtocolEvent, w []byte, m *bacnetReadMessage, inputErr error) error {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	// A complete unconfirmed APDU has no invoke ID. Refusing its unsupported
	// service cannot lose a confirmed transaction identity, so it must not
	// manufacture ambiguity or quarantine a later ReadProperty exchange.
	// Invalid/unknown envelopes still follow the conservative refusal path.
	unconfirmed := false
	if inputErr != nil && m == nil {
		if envelope, ok := inspectBACnetIP(w); ok && envelope.apduType == 1 {
			unconfirmed = true
		}
		if e.nativeBACnetRoutedRead && len(w) >= 6 && w[0] == 0x81 && (w[1] == 10 || w[1] == 11) && int(binary.BigEndian.Uint16(w[2:4])) == len(w) {
			if routes, err := decodeBACnetRoutes(w); err == nil && len(w)-routes.offset >= 2 && w[routes.offset] == 0x10 {
				unconfirmed = true
			}
		}
	}
	if inputErr != nil && m == nil {
		if hint := bacnetForeignInvokeHint(w, e.nativeBACnetRoutedRead); hint != nil {
			m = hint
		}
	}
	namespaceOnly := m != nil && m.namespaceOnly
	if !e.nativeBACnetRoutedRead && a.bacnetReadDisabled || e.nativeBACnetRoutedRead && a.bacnetRoutedReadDisabled {
		return bacnetReadError(ErrResourceExceeded, "capture association history exhausted")
	}
	key := binUDPKey{"bacnet-read/" + e.Source, "bacnet-read/" + e.Destination, e.Domain}
	if e.nativeBACnetRoutedRead {
		key = binUDPKey{"bacnet-routed-read/" + e.Source, "bacnet-routed-read/" + e.Destination, e.Domain}
	}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	s := a.bacnetReadSessions
	if unconfirmed {
		if s != nil {
			for _, el := range s.entries {
				v := el.Value.(*binUDPEntry)
				f := v.flow
				if v.key.domain != e.Domain || f.bacnetRead.routed != e.nativeBACnetRoutedRead || !(f.endpoints[0] == e.Source && f.endpoints[1] == e.Destination || f.endpoints[1] == e.Source && f.endpoints[0] == e.Destination) {
					continue
				}
				if e.Timestamp.After(v.touched) {
					v.touched = e.Timestamp
				}
				for _, p := range f.bacnetRead.seen {
					if p.pending && v.touched.Sub(p.at) >= bacnetReadPropertyTTL {
						p.pending = false
					}
				}
			}
		}
		return inputErr
	}
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.bacnetReadSessions = s
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	if e.nativeBACnetRoutedRead {
		physical := key
		// An unreadable/refused route has no trustworthy logical endpoint.
		// Quarantine the bounded physical relay pair rather than guessing which
		// retained logical conversation that packet affected.
		if (inputErr != nil || m == nil) && !namespaceOnly {
			for _, el := range s.entries {
				v := el.Value.(*binUDPEntry)
				f := v.flow
				if f.bacnetRead.routed && v.key.domain == e.Domain && (f.endpoints[0] == e.Source && f.endpoints[1] == e.Destination || f.endpoints[1] == e.Source && f.endpoints[0] == e.Destination) {
					f.bacnetRead.blocked = true
					for _, p := range f.bacnetRead.seen {
						p.pending = false
					}
				}
			}
		} else {
			if marker := s.entries[physical]; marker != nil && marker.Value.(*binUDPEntry).flow.bacnetRead.blocked {
				return bacnetReadError(ErrResourceExceeded, "physical route association quarantined")
			}
			key = binUDPKey{"bacnet-routed-read/" + e.Source + "|" + bacnetLogicalAddress(e.Source, m.routes.source), "bacnet-routed-read/" + e.Destination + "|" + bacnetLogicalAddress(e.Destination, m.routes.destination), e.Domain}
			if key.a > key.b {
				key.a, key.b = key.b, key.a
			}
		}
	}
	el := s.entries[key]
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			if e.nativeBACnetRoutedRead {
				a.bacnetRoutedReadDisabled = true
			} else {
				a.bacnetReadDisabled = true
			}
			return bacnetReadError(ErrResourceExceeded, "UDP conversation budget")
		}
		f := &binFlow{a: a, id: a.flows.Add(1), protocol: "bacnet", bacnetRead: &binBACnetRead{seen: map[uint16]*bacnetReadPending{}, routed: e.nativeBACnetRoutedRead}, endpoints: [2]string{e.Source, e.Destination}}
		if err := f.reserveSession(bacnetConversationBytes); err != nil {
			if e.nativeBACnetRoutedRead {
				a.bacnetRoutedReadDisabled = true
			} else {
				a.bacnetReadDisabled = true
			}
			return err
		}
		el = s.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: e.Timestamp})
		s.entries[key] = el
	}
	v := el.Value.(*binUDPEntry)
	f := v.flow
	state := f.bacnetRead
	e.FlowID = f.id
	dir := 0
	if e.Source != f.endpoints[0] {
		dir = 1
	}
	e.Direction = dir
	clock := v.touched
	if e.Timestamp.After(clock) {
		clock = e.Timestamp
	}
	v.touched = clock
	s.lru.MoveToBack(el)
	for _, p := range state.seen {
		if p.pending && clock.Sub(p.at) >= bacnetReadPropertyTTL {
			p.pending = false
		}
	}
	if state.blocked {
		return bacnetReadError(ErrResourceExceeded, "conversation identity history exhausted")
	}
	// A rejected message may contain an unreliable invoke/direction. Retire
	// all pending identities rather than guessing a narrower safe association.
	if (inputErr != nil || m == nil) && !namespaceOnly {
		for _, p := range state.seen {
			p.pending = false
		}
		state.blocked = true
		return inputErr
	}
	if !namespaceOnly {
		e.TransactionID = uint64(m.invoke)
	}
	requestDir := dir
	if !m.request {
		requestDir = 1 - dir
	}
	token := uint16(requestDir)*256 + uint16(m.invoke)
	p := state.seen[token]
	originEndpoint := e.Source
	var originRoute *bacnetRouteAddress
	if m.routes != nil {
		originRoute = m.routes.source
	}
	if !m.request {
		originEndpoint = e.Destination
		if m.routes != nil {
			originRoute = m.routes.destination
		}
	}
	origin := bacnetInvokeOrigin{e.Domain, bacnetLogicalAddress(originEndpoint, originRoute), m.invoke}
	history, knownOrigin := a.bacnetInvokeOrigins[origin]
	crossPeer := knownOrigin && history.key != key
	if crossPeer {
		if prior := history.flow.bacnetRead.seen[history.token]; prior != nil {
			prior.pending = false
		}
	}
	if e.Timestamp.Before(clock) {
		if p != nil {
			p.pending = false
		}
		return bacnetReadError(ErrContextRequired, "backdated datagram")
	}
	if p == nil {
		if len(state.seen) >= min(512, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
			state.blocked = true
			for _, p := range state.seen {
				p.pending = false
			}
			return bacnetReadError(ErrResourceExceeded, "invoke history budget")
		}
		if !knownOrigin && len(a.bacnetInvokeOrigins) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			a.bacnetReadDisabled, a.bacnetRoutedReadDisabled = true, true
			return bacnetReadError(ErrResourceExceeded, "originating device invoke history budget")
		}
		if err := f.reserveSession(bacnetConversationBytes + int64(len(state.seen)+1)*512); err != nil {
			state.blocked = true
			for _, p := range state.seen {
				p.pending = false
			}
			return err
		}
		p = &bacnetReadPending{id: e.ID, property: m.property, at: clock, pending: m.request && !namespaceOnly && !crossPeer}
		state.seen[token] = p
		if !knownOrigin {
			if a.bacnetInvokeOrigins == nil {
				a.bacnetInvokeOrigins = map[bacnetInvokeOrigin]bacnetInvokeHistory{}
			}
			a.bacnetInvokeOrigins[origin] = bacnetInvokeHistory{key, f, token}
		}
		if namespaceOnly {
			return inputErr
		}
		if crossPeer {
			return bacnetReadError(ErrContextRequired, "source invoke ID reused across peer or service context")
		}
		if !m.request {
			return bacnetReadError(ErrContextRequired, "acknowledgement lacks observed request")
		}
		m.fields["Role"] = "request"
		m.fields["Association"] = "observed-request"
		return nil
	}
	if crossPeer {
		p.pending = false
		if namespaceOnly {
			return inputErr
		}
		return bacnetReadError(ErrContextRequired, "source invoke ID reused across peer or service context")
	}
	if namespaceOnly {
		p.pending = false
		return inputErr
	}
	if m.request {
		// Retries have no independent generation. Even identical bytes cannot
		// prove which attempt a later acknowledgement answered.
		p.pending = false
		return bacnetReadError(ErrContextRequired, "invoke ID reused or retried")
	}
	if !p.pending {
		return bacnetReadError(ErrContextRequired, "acknowledgement uses retired invoke ID")
	}
	p.pending = false
	if !m.terminal && p.property != m.property {
		return bacnetReadError(ErrContextRequired, "acknowledgement descriptor mismatch")
	}
	e.ResponseTo = p.id
	m.fields["Role"], m.fields["Association"], m.fields["Response To"] = "response", "observed-request-match", p.id
	return nil
}

func (a *binParser) decodeBACnetReadDatagram(e *ProtocolEvent, w []byte, selectedRouting ...bool) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "bacnet", bacnetReadPropertyProfile, "explicit-decode-as", "message"
	e.nativeBACnetRead = true
	e.nativeBACnetRoutedRead = len(selectedRouting) > 0 && selectedRouting[0]
	if e.nativeBACnetRoutedRead {
		e.Profile = bacnetRoutedReadProfile
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	temp := &binFlow{a: a}
	defer temp.closeSession()
	var m *bacnetReadMessage
	var err error
	if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		err = bacnetReadError(ErrResourceExceeded, "message budget")
	} else if err = temp.reserveSession(8192 + int64(len(w))*512); err == nil {
		m, err = decodeBACnetReadPropertyProfile(w, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), sessionCollectionLimit(a.budget.MaxCollectionElements), e.nativeBACnetRoutedRead)
		if err == nil {
			m.fields["Observation Scope"], m.fields["Authentication Verified"] = "conversation", false
			m.fields["Role"], m.fields["Association"] = "", ""
			if !m.request {
				m.fields["Response To"] = uint64(0)
			}
			err = bacnetReadProjectionLimit(m.fields, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		}
	}
	err = a.correlateBACnetRead(e, w, m, err)
	if err == nil {
		e.semanticFields = cloneSession(m.fields)
		e.semanticSession = cloneSession(m.fields)
		e.Session = cloneSession(m.fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}

// WithSessionBACnetReadProperty selects the same explicit UDP profile as
// WithProtocolDecodeAs(..., "bacnet-read-property"). Feed boundaries remain
// complete datagrams; it never joins a split APDU across Feed calls.
func WithSessionBACnetReadProperty() ProtocolSessionOption {
	return func(s *captureSession) error {
		s.bacnetRead = true
		return nil
	}
}

// WithSessionBACnetRoutedReadProperty additionally observes selected bounded
// NPDU source/destination addresses. Logical address equality is passive
// correlation evidence, not authenticated routing or proof of device identity.
func WithSessionBACnetRoutedReadProperty() ProtocolSessionOption {
	return func(s *captureSession) error { s.bacnetRead, s.bacnetRoutedRead = true, true; return nil }
}

// Probe checks selected wire structure only. It cannot consume an invoke ID,
// retire pending association state, change clocks or establish peer context.
func (s *captureSession) probeBACnetRead(w []byte) ProbeResult {
	a := s.f.a
	profile := bacnetReadPropertyProfile
	if s.bacnetRoutedRead {
		profile = bacnetRoutedReadProfile
	}
	refuse := func(err error) ProbeResult {
		return ProbeResult{Verdict: ProbeReject, Protocol: "bacnet", Version: profile, Reason: err.Error()}
	}
	if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		return refuse(bacnetReadError(ErrResourceExceeded, "message budget"))
	}
	// Sessions are serialized by contract. Bound this temporary projection
	// before allocating, without mutating the observation/resource ledger.
	if a.buffered.Load()+8192+int64(len(w))*512 > int64(a.config.MaxBufferedBytes) {
		return refuse(bacnetReadError(ErrResourceExceeded, "probe projection budget"))
	}
	m, err := decodeBACnetReadPropertyProfile(w, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), sessionCollectionLimit(a.budget.MaxCollectionElements), s.bacnetRoutedRead)
	if err != nil {
		return refuse(err)
	}
	m.fields["Observation Scope"], m.fields["Authentication Verified"] = "conversation", false
	m.fields["Role"], m.fields["Association"] = "", ""
	if !m.request {
		m.fields["Response To"] = uint64(0)
	}
	if err = bacnetReadProjectionLimit(m.fields, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth); err != nil {
		return refuse(err)
	}
	return probeAccept("bacnet", profile, 98)
}

func (a *binParser) bacnetReadSelection(src, dst uint16) (bool, bool) {
	profile := a.datagramDecodeAs[dst]
	if profile == "" {
		profile = a.datagramDecodeAs[src]
	}
	return profile == "bacnet-read-property" || profile == "bacnet-routed-read-property", profile == "bacnet-routed-read-property"
}

func (a *binParser) refuseBACnetReadCarrier(e *ProtocolEvent, w []byte, src, dst uint16, err *ProtocolError) bool {
	selected, routed := a.bacnetReadSelection(src, dst)
	if !selected {
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "bacnet", bacnetReadPropertyProfile, "explicit-decode-as", "incomplete"
	e.nativeBACnetRead = true
	e.nativeBACnetRoutedRead = routed
	if routed {
		e.Profile = bacnetRoutedReadProfile
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	_ = a.correlateBACnetRead(e, nil, nil, err)
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}

func bacnetReadProjectionLimit(v any, elements, depth int) error {
	if depth < 1 {
		return bacnetReadError(ErrResourceExceeded, "projected value depth")
	}
	switch x := v.(type) {
	case map[string]any:
		if len(x) > elements {
			return bacnetReadError(ErrResourceExceeded, "projected field collection")
		}
		for _, q := range x {
			if err := bacnetReadProjectionLimit(q, elements, depth-1); err != nil {
				return err
			}
		}
	case []any:
		if len(x) > elements {
			return bacnetReadError(ErrResourceExceeded, "projected value collection")
		}
		for _, q := range x {
			if err := bacnetReadProjectionLimit(q, elements, depth-1); err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *binParser) refuseBACnetReadOversize(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	selected, routed := a.bacnetReadSelection(src, dst)
	if !selected {
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "bacnet", bacnetReadPropertyProfile, "explicit-decode-as", "limited"
	e.nativeBACnetRead = true
	e.nativeBACnetRoutedRead = routed
	if routed {
		e.Profile = bacnetRoutedReadProfile
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	err := bacnetReadError(ErrResourceExceeded, "message budget")
	_ = a.correlateBACnetRead(e, nil, nil, err)
	a.finishProtocolDatagram(e, nil, nil, err)
	a.limited.Add(uint64(len(w)))
	return true
}
