package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"time"
	"unsafe"
)

const droneCANStatsSignature uint64 = 0xbe6f76a7ec312b04
const droneCANTransferTimeout = 2 * time.Second

type droneCANStatsKey struct {
	domain     CaptureDomain
	descriptor uint32 // service, direction and both nodes; priority is checked separately
}
type droneCANStatsReceiver struct {
	cost                  int64
	at, clock             time.Time
	tid, toggle, priority byte
	active                bool
	size, count           int
	body                  []byte // signature-bound CRC plus bounded selected DSDL body
	refs                  []PacketReference
	profile, content      string
}
type droneCANStatsStore struct {
	entries map[droneCANStatsKey]*droneCANStatsReceiver
	cost    int64
}

func (s *droneCANStatsReceiver) retire() {
	clear(s.body[:])
	clear(s.refs[:])
	s.size, s.count = 0, 0
	s.active, s.toggle = false, 0
	s.tid = (s.tid + 1) & 31
}

// This opt-in receiver observes selected service4 response transfers. It does
// not associate requests, authenticate nodes, verify device counters, or infer
// redundant interfaces. Capture section/interface/encapsulation are independent
// receiver domains. The fixed allocation includes bounded duplicate history;
// exhaustion fails closed rather than evicting a marker and accepting late data.
func (a *binParser) decodeDroneCANStats(id uint32, data []byte, ref PacketReference, at time.Time) (map[string]any, []PacketReference, error) {
	return a.decodeDroneCANResponse(id, data, ref, at, droneCANResponseSpec{4, droneCANStatsSignature, 72, "dronecan-v0-transport-stats", "uavcan.protocol.GetTransportStats"})
}

type droneCANResponseSpec struct {
	typeID           uint32
	signature        uint64
	maxBody          int
	profile, content string
}

func (a *binParser) decodeDroneCANResponse(id uint32, data []byte, ref PacketReference, at time.Time, spec droneCANResponseSpec) (map[string]any, []PacketReference, error) {
	a.droneCANMu.Lock()
	defer a.droneCANMu.Unlock()
	bad := func(kind ProtocolErrorKind, why string) (map[string]any, []PacketReference, error) {
		return nil, nil, protocolError(kind, "DroneCAN "+strings.TrimPrefix(spec.content, "uavcan.protocol.")+" "+why)
	}
	if a.droneCANStatsDisabled {
		return bad(ErrResourceExceeded, "receiver history exhausted")
	}
	key := droneCANStatsKey{ref.Domain, id & 0xffffff}
	var s *droneCANStatsReceiver
	if a.droneCANStats != nil {
		s = a.droneCANStats.entries[key]
	}
	invalidate := func(kind ProtocolErrorKind, why string) (map[string]any, []PacketReference, error) {
		if s != nil && s.active {
			s.retire()
		}
		return bad(kind, why)
	}
	if id&127 == 0 || id>>8&127 == 0 {
		return invalidate(ErrMalformedMessage, "service node ID is zero")
	}
	if id&0x8000 != 0 {
		return bad(ErrUnsupportedFeature, "request observations are outside selected response profile")
	}
	if len(data) == 0 {
		return invalidate(ErrMalformedMessage, "transfer tail is absent")
	}
	tail := data[len(data)-1]
	start, end, toggle := tail&128 != 0, tail&64 != 0, tail&32 != 0
	if start && toggle {
		return invalidate(ErrMalformedMessage, "first frame toggle must be zero")
	}
	if !end && len(data) != 8 {
		return invalidate(ErrMalformedMessage, "non-final classical frame must fill data field")
	}
	if start && end {
		if spec.typeID == 4 {
			return invalidate(ErrMalformedMessage, "selected response needs at least eighteen body octets")
		}
		return invalidate(ErrMalformedMessage, "selected response needs at least forty-two body octets")
	}
	if s == nil {
		if a.droneCANStats == nil {
			cost := int64(unsafe.Sizeof(droneCANStatsStore{})) + 256
			if !a.reserveEvidence(cost) {
				a.droneCANStatsDisabled = true
				return bad(ErrResourceExceeded, "receiver store byte budget")
			}
			a.droneCANStats = &droneCANStatsStore{entries: make(map[droneCANStatsKey]*droneCANStatsReceiver), cost: cost}
		}
		bodyCapacity := spec.maxBody + 2
		// An aligned body may end with an empty payload plus the tail byte.
		// All preceding frames still carry seven payload octets.
		refCapacity := bodyCapacity/7 + 1
		cost := int64(unsafe.Sizeof(droneCANStatsReceiver{})+unsafe.Sizeof(key)) + 128 + int64(bodyCapacity) + int64(refCapacity)*int64(unsafe.Sizeof(PacketReference{})) + int64(len(ref.Domain.Encapsulation))*int64(refCapacity+1)
		if len(a.droneCANStats.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(cost) {
			a.disableDroneCANStatsLocked()
			return bad(ErrResourceExceeded, "receiver history byte/element budget")
		}
		s = &droneCANStatsReceiver{cost: cost, tid: tail & 31, clock: at, at: at, body: make([]byte, bodyCapacity), refs: make([]PacketReference, refCapacity), profile: spec.profile, content: spec.content}
		a.droneCANStats.entries[key] = s
		if !start {
			s.tid = (s.tid + 1) & 31
			return bad(ErrContextRequired, "start of transfer was not captured")
		}
	}
	if !at.IsZero() && !s.clock.IsZero() && at.Before(s.clock) {
		if s.active {
			s.retire()
		}
		return bad(ErrContextRequired, "backdated contributor cannot alter newer receiver state")
	}
	if at.After(s.clock) {
		s.clock = at
	}
	timedOut := !at.IsZero() && !s.at.IsZero() && at.Sub(s.at) > droneCANTransferTimeout
	tid := tail & 31
	// Non-redundant transport specification: forward distance is (a-b) mod32.
	// An immediate previous completed ID is ignored; gaps may restart on SOT.
	restart := timedOut || start && ((s.tid-tid)&31) > 1
	if restart {
		clear(s.body[:])
		clear(s.refs[:])
		s.size, s.count = 0, 0
		s.active = false
		s.toggle = 0
		s.tid = tid
		s.at = at
		if !start {
			s.tid = (tid + 1) & 31
			return bad(ErrContextRequired, "transfer expired without a new start")
		}
	}
	// A second SOT cannot append a second CRC/prefix to the active body,
	// even when two intervening frames restored the expected toggle to zero.
	if start && s.active {
		return bad(ErrContextRequired, "duplicate start of active transfer")
	}
	if tail>>5&1 != s.toggle || tid != s.tid {
		return bad(ErrContextRequired, "unexpected toggle or transfer ID")
	}
	if !start && !s.active {
		return bad(ErrContextRequired, "start of transfer was not captured")
	}
	priority := byte(id >> 24 & 31)
	if s.active && s.priority != priority {
		return invalidate(ErrMalformedMessage, "priority changed within transfer")
	}
	if start {
		s.at = at
		s.priority = priority
		s.active = true
	}
	n := len(data) - 1
	if s.size+n > len(s.body) || s.size+n-2 > a.budget.MaxMessageBytes || s.count >= min(len(s.refs), sessionCollectionLimit(a.budget.MaxCollectionElements)) {
		return invalidate(ErrResourceExceeded, "transfer payload/contributor budget")
	}
	copy(s.body[s.size:], data[:n])
	s.size += n
	s.refs[s.count] = ref
	s.count++
	s.toggle ^= 1
	if !end {
		return bad(ErrNeedMore, "transfer awaits additional frames")
	}
	refs := append([]PacketReference(nil), s.refs[:s.count]...)
	raw := bytes.Clone(s.body[:s.size])
	count := s.count
	s.retire() // every completed or refused transfer advances bounded history
	if len(raw) < 2 {
		return bad(ErrMalformedMessage, "transfer CRC is absent")
	}
	expected := binary.LittleEndian.Uint16(raw[:2])
	body := raw[2:]
	if spec.typeID == 4 {
		if len(body) < 18 || (len(body)-18)%18 != 0 || len(body) > 72 {
			return bad(ErrMalformedMessage, "selected DSDL response length/TAO interface count")
		}
		if (len(body)-18)/18 > sessionCollectionLimit(a.budget.MaxCollectionElements) {
			return bad(ErrResourceExceeded, "interface array element budget")
		}
	}
	if droneCANTransferCRC(spec.signature, body) != expected {
		return bad(ErrMalformedMessage, "transfer CRC mismatch")
	}
	header := map[string]any{"Protocol Version": "v0", "Transfer Type": "response", "Priority": uint32(priority), "Data Type ID": spec.typeID, "Source Node ID": id & 127, "Destination Node ID": id >> 8 & 127, "Transfer ID": tid, "Frame Start of Transfer": start, "Frame End of Transfer": end, "Frame Toggle": toggle, "Content": spec.content, "Transfer Complete": true, "Frame Count": count, "Transfer Payload": body, "Data Type Signature": fmt.Sprintf("%016x", spec.signature), "Transfer CRC": fmt.Sprintf("%04x", expected)}
	if spec.typeID == 1 {
		decoded, err := decodeDroneCANNodeInfoBody(body, a.budget)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range decoded {
			header[k] = v
		}
		return header, refs, nil
	}
	scalar := func(i int) uint64 {
		p := body[i : i+6]
		return uint64(p[0]) | uint64(p[1])<<8 | uint64(p[2])<<16 | uint64(p[3])<<24 | uint64(p[4])<<32 | uint64(p[5])<<40
	}
	interfaces := make([]any, 0, (len(body)-18)/18)
	for i := 18; i < len(body); i += 18 {
		interfaces = append(interfaces, map[string]any{"Frames TX": scalar(i), "Frames RX": scalar(i + 6), "Errors": scalar(i + 12)})
	}
	header["Observation"] = "unverified-transport-statistics"
	header["Transfers TX"], header["Transfers RX"], header["Transfer Errors"] = scalar(0), scalar(6), scalar(12)
	header["CAN Interface Stats"] = interfaces
	return header, refs, nil
}

func droneCANStatsCRC(body []byte) uint16 {
	return droneCANTransferCRC(droneCANStatsSignature, body)
}

func droneCANTransferCRC(typeSignature uint64, body []byte) uint16 {
	crc := uint16(0xffff)
	var signature [8]byte
	binary.LittleEndian.PutUint64(signature[:], typeSignature)
	for _, p := range [][]byte{signature[:], body} {
		for _, v := range p {
			crc ^= uint16(v) << 8
			for b := 0; b < 8; b++ {
				if crc&0x8000 != 0 {
					crc = (crc << 1) ^ 0x1021
				} else {
					crc <<= 1
				}
			}
		}
	}
	return crc
}

func (a *binParser) disableDroneCANStatsLocked() {
	if a.droneCANStats != nil {
		for _, s := range a.droneCANStats.entries {
			a.buffered.Add(-s.cost)
		}
		a.buffered.Add(-a.droneCANStats.cost)
		a.droneCANStats = nil
	}
	a.droneCANStatsDisabled = true
}

// An outer record refusal cannot leave a partial transfer available for later
// completion. Unknown/truncated headers cannot identify the refused descriptor;
// fail closed for this explicitly selected receiver instead of guessing its ID.
func (a *binParser) refuseDroneCANStatsRecord() {
	a.droneCANMu.Lock()
	defer a.droneCANMu.Unlock()
	a.disableDroneCANStatsLocked()
}
func (a *binParser) closeDroneCANStats() {
	a.droneCANMu.Lock()
	var events []*ProtocolEvent
	if a.droneCANStats != nil {
		for key, s := range a.droneCANStats.entries {
			if s.active {
				err := protocolError(ErrNeedMore, "DroneCAN "+strings.TrimPrefix(s.content, "uavcan.protocol.")+" ended before complete transfer")
				status, typed := classifySessionError(err)
				events = append(events, &ProtocolEvent{nativeCANRecord: true, Timestamp: s.at, Transport: "can", Protocol: "dronecan", Profile: s.profile, Admission: "explicit-interface-decode-as", Domain: key.domain, Status: status, Completeness: status, Error: err.Error(), sessionError: typed, SourceBytes: ByteSource{Kind: "reassembled", PacketRefs: append([]PacketReference(nil), s.refs[:s.count]...)}})
			}
			a.buffered.Add(-s.cost)
		}
		a.buffered.Add(-a.droneCANStats.cost)
		a.droneCANStats = nil
	}
	a.droneCANStatsDisabled = true
	a.droneCANMu.Unlock()
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.SourceBytes.PacketRefs[0].Number != b.SourceBytes.PacketRefs[0].Number {
			return a.SourceBytes.PacketRefs[0].Number < b.SourceBytes.PacketRefs[0].Number
		}
		if a.Domain.Section != b.Domain.Section {
			return a.Domain.Section < b.Domain.Section
		}
		if a.Domain.Interface != b.Domain.Interface {
			return a.Domain.Interface < b.Domain.Interface
		}
		return a.Domain.Encapsulation < b.Domain.Encapsulation
	})
	for _, e := range events {
		a.incomplete.Add(1)
		a.emit(e)
	}
}
