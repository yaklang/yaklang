package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
)

const dcpIdentifyProfile = "profinet-dcp-identify-observed-properties"

// The selected Identify layout is checked against fixed p-net public/v1.0.2
// 459e043 and Wireshark4.2.5 4aa814ac. All selector length0 is p-net's explicit
// PNIOtester compatibility. This is not a full IEC/PI conformance assertion.
func dcpError(kind ProtocolErrorKind, why string) error {
	return protocolError(kind, "DCP Identify: %s", why)
}
func dcpText(v []byte) map[string]any {
	r := make([]rune, len(v))
	for i, b := range v {
		r[i] = rune(b)
	}
	return map[string]any{"value_hex": hex.EncodeToString(v), "latin1": string(r), "configuration_validity": "not asserted"}
}
func dcpProperty(opt, sub byte, body []byte, response bool, limit int) (map[string]any, error) {
	out := map[string]any{"option": opt, "suboption": sub, "length": len(body), "body_hex": hex.EncodeToString(body), "block_info": nil}
	known := opt == 1 && (sub == 1 || sub == 2) || opt == 2 && sub >= 1 && sub <= 8
	if !known {
		out["handling"] = "opaque unknown block; no property semantics"
		return out, nil
	}
	v := body
	if response {
		if len(v) < 2 {
			return nil, dcpError(ErrMalformedMessage, "known response property lacks BlockInfo")
		}
		out["block_info"] = binary.BigEndian.Uint16(v)
		v = v[2:]
	}
	fixed := func(n int) error {
		if len(v) < n {
			return dcpError(ErrMalformedMessage, "known property fixed fields incomplete")
		}
		if len(v) > n {
			return dcpError(ErrUnsupportedFeature, "unverified additional bytes in fixed property")
		}
		return nil
	}
	switch {
	case opt == 1 && sub == 1:
		if err := fixed(6); err != nil {
			return nil, err
		}
		out["kind"], out["mac"], out["mac_hex"] = "MACAddress", net.HardwareAddr(v).String(), hex.EncodeToString(v)
	case opt == 1 && sub == 2:
		if err := fixed(12); err != nil {
			return nil, err
		}
		out["kind"] = "IPParameter"
		out["ipv4"], out["subnet_mask"], out["gateway"] = net.IP(v[:4]).String(), net.IP(v[4:8]).String(), net.IP(v[8:]).String()
		out["ip_parameter_raw_hex"] = hex.EncodeToString(v)
		if response {
			out["ip_conflict_indicated"] = binary.BigEndian.Uint16(body)&128 != 0
		}
	case opt == 2 && (sub == 1 || sub == 2 || sub == 6):
		out["kind"] = map[byte]string{1: "DeviceVendor", 2: "NameOfStation", 6: "AliasName"}[sub]
		out["text"] = dcpText(v)
	case opt == 2 && (sub == 3 || sub == 8):
		if err := fixed(4); err != nil {
			return nil, err
		}
		out["kind"] = "DeviceID"
		if sub == 8 {
			out["kind"] = "OEMDeviceID"
		}
		out["vendor_id"], out["device_id"] = binary.BigEndian.Uint16(v), binary.BigEndian.Uint16(v[2:])
	case opt == 2 && sub == 4:
		if !response {
			return nil, dcpError(ErrUnsupportedFeature, "request role width disagreement requires a normative clause")
		}
		if err := fixed(2); err != nil {
			return nil, err
		}
		out["kind"], out["role_raw"], out["role_reserved_raw"] = "DeviceRole", v[0], v[1]
		out["io_device"], out["io_controller"], out["io_multidevice"], out["pn_supervisor"], out["uninterpreted_role_bits_raw"] = v[0]&1 != 0, v[0]&2 != 0, v[0]&4 != 0, v[0]&8 != 0, v[0]&240
	case opt == 2 && sub == 5:
		if len(v)%2 != 0 {
			return nil, dcpError(ErrMalformedMessage, "DeviceOptions pair incomplete")
		}
		if len(v)/2 > limit {
			return nil, dcpError(ErrResourceExceeded, "DeviceOptions collection limit")
		}
		rows := []map[string]any{}
		for at := 0; at < len(v); at += 2 {
			rows = append(rows, map[string]any{"option": v[at], "suboption": v[at+1]})
		}
		out["kind"], out["options"] = "DeviceOptions", rows
	case opt == 2 && sub == 7:
		if err := fixed(2); err != nil {
			return nil, err
		}
		out["kind"], out["instance_high"], out["instance_low"] = "DeviceInstance", v[0], v[1]
	}
	out["handling"] = "decoded selected property"
	return out, nil
}
func decodeDCPIdentify(w []byte, limit int) (map[string]any, error) {
	return decodeDCPIdentifyBounded(w, limit, limit)
}

func decodeDCPIdentifyBounded(w []byte, limit, blockLimit int) (map[string]any, error) {
	if len(w) < 12 {
		return nil, dcpError(ErrMalformedMessage, "FrameID and header incomplete")
	}
	fid := binary.BigEndian.Uint16(w)
	typ := w[3]
	if fid < 0xfefc || fid > 0xfeff || w[2] != 5 || typ > 1 {
		return nil, dcpError(ErrUnsupportedFeature, "selected Identify5 request/successresponse only")
	}
	want := uint16(0xfefe)
	if typ == 1 {
		want = 0xfeff
	}
	if fid != want {
		return nil, dcpError(ErrMalformedMessage, "FrameID/type disagreement")
	}
	end := 12 + int(binary.BigEndian.Uint16(w[10:]))
	if end > len(w) {
		return nil, dcpError(ErrMalformedMessage, "DCPDataLength exceeds frame")
	}
	if end == 12 {
		return nil, dcpError(ErrUnsupportedFeature, "empty Identify data lacks mandatory property evidence")
	}
	// Root/public session snapshots and the largest selected property maps are
	// bounded before allocation, independently of the block collection limit.
	if limit < 18 {
		return nil, dcpError(ErrResourceExceeded, "Identify field map exceeds collection limit")
	}
	out := map[string]any{"frame_id": fid, "service_id": 5, "service_type": typ, "kind": "IdentifyRequest", "transaction_id": binary.BigEndian.Uint32(w[4:]), "response_delay_factor": nil, "reserved_raw": nil, "data_length": end - 12, "declared_message_hex": hex.EncodeToString(w[:end]), "link_trailer_hex": hex.EncodeToString(w[end:]), "response_claim": nil, "filter_semantics": "observed raw filter; matching properties does not authenticate device"}
	if typ == 0 {
		out["response_delay_factor"] = binary.BigEndian.Uint16(w[8:])
	} else {
		out["kind"] = "IdentifyResponse"
		out["reserved_raw"] = binary.BigEndian.Uint16(w[8:])
		out["response_claim"] = "observed successresponse; no deviceoperation or IO capability proof"
	}
	blocks := []map[string]any{}
	for at := 12; at < end; {
		if end-at < 4 {
			return nil, dcpError(ErrMalformedMessage, "block header exceeds DCPDataLength")
		}
		opt, sub, n := w[at], w[at+1], int(binary.BigEndian.Uint16(w[at+2:]))
		start := at
		at += 4
		if n+(n&1) > end-at {
			return nil, dcpError(ErrMalformedMessage, "value or odd padding crosses DCPDataLength")
		}
		if len(blocks) >= blockLimit {
			return nil, dcpError(ErrResourceExceeded, "block collection limit")
		}
		v := w[at : at+n]
		at += n
		var pad any
		if n&1 != 0 {
			pad = w[at]
			at++
		}
		var b map[string]any
		var err error
		if opt == 255 && sub == 255 {
			if typ == 1 || n != 0 {
				return nil, dcpError(ErrUnsupportedFeature, "All selector response or length2 remains unverified")
			}
			b = map[string]any{"option": opt, "suboption": sub, "length": 0, "body_hex": "", "block_info": nil, "kind": "IdentifyAll", "handling": "p-net PNIOtester length0 compatibility, not normative conformance claim"}
		} else {
			b, err = dcpProperty(opt, sub, v, typ == 1, limit)
			if err != nil {
				return nil, err
			}
		}
		b["padding_raw"], b["wire_offset"], b["wire_length"], b["block_wire_hex"] = pad, start, at-start, hex.EncodeToString(w[start:at])
		blocks = append(blocks, b)
	}
	out["blocks"] = blocks
	return out, nil
}
func dcpEthernet(frame []byte) (map[string]any, []byte, string, error) {
	if len(frame) < 14 {
		return nil, nil, "", dcpError(ErrMalformedMessage, "Ethernet header incomplete")
	}
	typ, at := binary.BigEndian.Uint16(frame[12:]), 14
	vlans := []map[string]any{}
	var tags strings.Builder
	for typ == 0x8100 || typ == 0x88a8 {
		if len(vlans) >= 2 {
			return nil, nil, "", dcpError(ErrUnsupportedFeature, "selected carrier supports at most two VLAN tags")
		}
		if len(frame)-at < 4 {
			return nil, nil, "", dcpError(ErrMalformedMessage, "VLAN header incomplete")
		}
		tci := binary.BigEndian.Uint16(frame[at:])
		vlans = append(vlans, map[string]any{"tpid": typ, "tci": tci, "pcp": tci >> 13, "dei": tci&4096 != 0, "vid": tci & 4095})
		fmt.Fprintf(&tags, "/%04x:%d", typ, tci&4095)
		typ = binary.BigEndian.Uint16(frame[at+2:])
		at += 4
	}
	if typ != 0x8892 {
		return nil, nil, "", dcpError(ErrUnsupportedFeature, "EtherType does not carry PROFINET")
	}
	return map[string]any{"source_mac": net.HardwareAddr(frame[6:12]).String(), "destination_mac": net.HardwareAddr(frame[:6]).String(), "ether_type": typ, "vlan": vlans, "header_bytes": at}, frame[at:], tags.String(), nil
}

type dcpKey struct {
	domain          CaptureDomain
	tags, requester string
}
type dcpRequest struct {
	id                   uint64
	wire                 []byte
	destination          string
	multicast, ambiguous bool
	retries              int
}
type dcpObservation struct {
	owner    *binFlow
	requests map[uint32]*dcpRequest
	blocked  bool
	touched  time.Time
}

// This registry stores passive discovery candidates, never unique peer or
// operational transaction success. Its retained bytes share the capture ledger.
func (a *binParser) dcpCandidate(e *ProtocolEvent, w []byte, tags string, fields map[string]any, decodeErr error) error {
	if len(w) < 12 || w[2] != 5 || w[3] > 1 {
		return decodeErr
	}
	requester := e.Source
	if w[3] == 1 {
		requester = e.Destination
	}
	key := dcpKey{e.Domain, tags, requester}
	xid := binary.BigEndian.Uint32(w[4:])
	a.dcpMu.Lock()
	defer a.dcpMu.Unlock()
	if e.Timestamp.After(a.dcpClock) {
		a.dcpClock = e.Timestamp
	}
	for k, s := range a.dcpObservations {
		if a.dcpClock.Sub(s.touched) >= 30*time.Second {
			s.owner.closeSession()
			delete(a.dcpObservations, k)
		}
	}
	s := a.dcpObservations[key]
	var typed *ProtocolError
	if errors.As(decodeErr, &typed) && typed.Kind == ErrResourceExceeded {
		if s != nil {
			s.owner.closeSession()
			s.requests = nil
			s.blocked = true
			_ = s.owner.reserveSession(512)
		}
		return typed
	}
	if decodeErr != nil {
		if s != nil && w[3] == 0 {
			if r := s.requests[xid]; r != nil {
				r.ambiguous = true
			}
		}
		return decodeErr
	}
	e.Session["Association"] = "unmatched-discovery"
	if s == nil && w[3] == 0 {
		if len(a.dcpObservations) >= a.budget.MaxCollectionElements {
			return dcpError(ErrResourceExceeded, "requester registry collection limit")
		}
		owner := &binFlow{a: a}
		if err := owner.reserveSession(4096); err != nil {
			return err
		}
		s = &dcpObservation{owner: owner, requests: map[uint32]*dcpRequest{}, touched: a.dcpClock}
		if a.dcpObservations == nil {
			a.dcpObservations = map[dcpKey]*dcpObservation{}
		}
		a.dcpObservations[key] = s
	}
	if s == nil {
		return nil
	}
	s.touched = a.dcpClock
	if s.blocked {
		e.Session["Association"] = "ambiguous-discovery"
		return nil
	}
	if w[3] == 0 {
		end := 12 + int(binary.BigEndian.Uint16(w[10:]))
		wire := w[:end]
		if r := s.requests[xid]; r != nil {
			if !bytes.Equal(r.wire, wire) || r.destination != e.Destination {
				r.ambiguous = true
				e.Session["Association"] = "ambiguous-discovery"
			} else if !r.ambiguous {
				if r.retries >= a.budget.MaxCollectionElements {
					return a.dcpBlock(s, "request repetition limit")
				}
				r.retries++
				e.Session["Association"] = "repeated-discovery"
				e.Session["Repeated Request ID"] = r.id
			}
			return nil
		}
		if len(s.requests) >= a.budget.MaxCollectionElements {
			return a.dcpBlock(s, "request XID collection limit")
		}
		target := s.owner.sessionBytes + 512 + 512*int64(len(wire))
		if err := s.owner.reserveSession(target); err != nil {
			a.dcpBlock(s, "request byte ledger limit")
			return err
		}
		s.requests[xid] = &dcpRequest{id: e.ID, wire: bytes.Clone(wire), destination: e.Destination, multicast: len(e.Destination) >= 2 && (frameMACFirst(e.Destination)&1) != 0}
		e.Session["Association"] = "observed-discovery-request"
		return nil
	}
	if r := s.requests[xid]; r != nil && !r.ambiguous && (r.multicast || r.destination == e.Source) {
		if dcpFiltersMatch(r.wire, fields) {
			e.Session["Association"] = "observed-candidate"
			e.Session["Candidate Request ID"] = r.id
		}
	}
	return nil
}
func frameMACFirst(text string) byte {
	m, err := net.ParseMAC(text)
	if err != nil || len(m) == 0 {
		return 0
	}
	return m[0]
}
func (a *binParser) dcpBlock(s *dcpObservation, why string) error {
	s.owner.closeSession()
	s.requests = nil
	s.blocked = true
	_ = s.owner.reserveSession(512)
	return dcpError(ErrResourceExceeded, why)
}
func dcpFiltersMatch(request []byte, response map[string]any) bool {
	end := 12 + int(binary.BigEndian.Uint16(request[10:]))
	properties := map[uint16]string{}
	duplicate := map[uint16]bool{}
	for _, b := range response["blocks"].([]map[string]any) {
		opt, sub := b["option"].(byte), b["suboption"].(byte)
		k := uint16(opt)<<8 | uint16(sub)
		if _, ok := properties[k]; ok {
			duplicate[k] = true
		}
		v, err := hex.DecodeString(b["body_hex"].(string))
		if err != nil || len(v) < 2 {
			continue
		}
		properties[k] = hex.EncodeToString(v[2:])
	}
	seen := map[uint16]bool{}
	for at := 12; at < end; {
		opt, sub, n := request[at], request[at+1], int(binary.BigEndian.Uint16(request[at+2:]))
		v := request[at+4 : at+4+n]
		at += 4 + n + (n & 1)
		if opt == 255 && sub == 255 && n == 0 {
			continue
		}
		k := uint16(opt)<<8 | uint16(sub)
		if seen[k] || duplicate[k] {
			return false
		}
		seen[k] = true
		if !((opt == 1 && (sub == 1 || sub == 2)) || (opt == 2 && (sub == 1 || sub == 2 || sub == 3 || sub == 6 || sub == 7 || sub == 8))) {
			return false
		}
		p, ok := properties[k]
		if !ok || p != hex.EncodeToString(v) {
			return false
		}
	}
	return true
}
func (a *binParser) closeDCPObservations() {
	a.dcpMu.Lock()
	defer a.dcpMu.Unlock()
	for _, s := range a.dcpObservations {
		s.owner.closeSession()
	}
	a.dcpObservations = nil
}

func (a *binParser) decodeDCPEthernet(frame []byte, evidence captureEvidence, ci gopacket.CaptureInfo) {
	link, w, tags, err := dcpEthernet(frame)
	e := &ProtocolEvent{ID: a.ids.Add(1), Timestamp: ci.Timestamp, Transport: "ethernet", Protocol: "profinet-dcp", Profile: dcpIdentifyProfile, Admission: "ether-type-and-identify-profile", Domain: evidence.Ref.Domain, Length: len(w), SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{evidence.Ref}}, Completeness: "message"}
	if link != nil {
		e.Source = link["source_mac"].(string)
		e.Destination = link["destination_mac"].(string)
	}
	a.input.Add(uint64(len(w)))
	temp := &binFlow{a: a}
	defer temp.closeSession()
	if err == nil && ci.CaptureLength < ci.Length {
		err = dcpError(ErrNeedMore, "capture truncates Ethernet frame")
	}
	if err == nil && (len(frame) > a.budget.MaxFrameBytes || len(w) > a.config.MaxMessageBytes) {
		err = dcpError(ErrResourceExceeded, "frame/message byte limit")
	}
	if err == nil && a.budget.MaxRecursionDepth < 5 {
		err = dcpError(ErrResourceExceeded, "field recursion limit")
	}
	var fields map[string]any
	if err == nil {
		err = temp.reserveSession(32768 + 512*int64(len(frame)))
		if err == nil {
			fields, err = decodeDCPIdentify(w, a.budget.MaxCollectionElements)
			if err == nil {
				fields["ethernet"] = link
				e.Session = cloneSession(fields)
				e.semanticFields = cloneSession(fields)
			}
		}
	}
	err = a.dcpCandidate(e, w, tags, fields, err)
	if err != nil {
		e.Session, e.semanticFields = nil, nil
	}
	raw := w
	var typed *ProtocolError
	if errors.As(err, &typed) && (typed.Kind == ErrResourceExceeded || typed.Kind == ErrNeedMore) {
		raw = nil
		err = typed
	}
	if len(w) >= 2 && (binary.BigEndian.Uint16(w) < 0xfefc || binary.BigEndian.Uint16(w) > 0xfeff) {
		e.Protocol, e.Profile, e.Admission = "profinet", "profinet-carrier-diagnostic", "ether-type"
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if err != nil {
		e.Completeness = e.Status
	}
	a.emit(e)
}
