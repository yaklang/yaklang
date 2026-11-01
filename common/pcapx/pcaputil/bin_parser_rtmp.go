package pcaputil

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type rtmpAssembly struct {
	header stream_parser.RTMPChunkContext
	data   []byte
	ids    []uint64
	refs   []PacketReference
}
type rtmpTransaction struct {
	dir    int
	stream uint32
	number float64
}
type binRTMP struct {
	phase         [2]int
	initial       [2][]byte
	initialFields [2]map[string]any
	chunkSize     [2]uint32
	messageOffset [2]uint64 // delivered logical payload bytes, independently of wire chunks
	streams       [2]map[uint32]*rtmpAssembly
	pending       map[rtmpTransaction]uint64 // zero quarantines duplicate transaction numbers
}

func newBinRTMP() *binRTMP {
	return &binRTMP{chunkSize: [2]uint32{128, 128}, streams: [2]map[uint32]*rtmpAssembly{{}, {}}, pending: map[rtmpTransaction]uint64{}}
}
func probeRTMP(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 3 || limit < 1537 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 1537 {
		return probeNeed("rtmp", "3", len(w), 1537)
	}
	if _, e := stream_parser.RTMPInitialHandshake(w[:1537]); e != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("rtmp", "3", 99)
}
func (s *binRTMP) storage() int64 {
	n := int64(2048 + len(s.initial[0]) + len(s.initial[1]) + len(s.pending)*64)
	for _, d := range s.streams {
		for _, m := range d {
			n += 256 + int64(len(m.data)+len(m.ids)*8+len(m.refs)*80)
		}
	}
	return n
}
func (f *binFlow) rtmpHeader(dir int, w []byte) (stream_parser.RTMPChunkHeader, error) {
	var prev stream_parser.RTMPChunkContext
	if len(w) > 0 {
		cs := uint32(w[0] & 63)
		if cs == 0 && len(w) >= 2 {
			cs = 64 + uint32(w[1])
		} else if cs == 1 && len(w) >= 3 {
			cs = 64 + uint32(w[1]) + uint32(w[2])*256
		}
		if m := f.rtmp.streams[dir][cs]; m != nil {
			prev = m.header
		}
	}
	return stream_parser.DecodeRTMPChunkHeader(w, prev, f.rtmp.chunkSize[dir], min(f.a.config.MaxMessageBytes, f.a.budget.MaxFrameBytes))
}
func (f *binFlow) frameRTMP(dir int, w []byte) (int, *binSpec, error) {
	s := f.rtmp
	if e := f.reserveSession(s.storage()); e != nil {
		return 0, nil, e
	}
	if s.phase[dir] == 0 {
		return 1537, f.a.specs["rtmp/RTMPInitialHandshakeFields"], nil
	}
	if s.phase[dir] == 1 {
		return 1536, f.a.specs["rtmp/RTMPInitialHandshakeFields"], nil
	}
	h, e := f.rtmpHeader(dir, w)
	return h.WireLength, f.a.specs["rtmp/RTMPInitialChunkFields"], observedWireError(e)
}
func (f *binFlow) consumeRTMP(dir int, e *ProtocolEvent) error {
	s := f.rtmp
	e.nativeSessionMessage = true
	e.Profile = "rtmp-v3-observed"
	e.Admission = "complete-version3-initial-handshake"
	e.Completeness = "message"
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	var fields map[string]any
	var err error
	switch s.phase[dir] {
	case 0:
		fields, err = stream_parser.RTMPInitialHandshake(e.Raw)
		if err == nil {
			if err = f.reserveSession(s.storage() + 1537); err == nil {
				s.initial[dir] = append([]byte(nil), e.Raw...)
				s.initialFields[dir] = cloneSession(fields)
				s.phase[dir]++
			}
		}
	case 1:
		fields, err = stream_parser.RTMPEchoHandshake(e.Raw, s.initial[1-dir], s.initialFields[1-dir])
		// The echo's boundary is fixed even if verification fails. Keep chunks
		// observable without promoting the echo to a verified handshake.
		s.phase[dir]++
		if fields != nil {
			fields["Error Scope"] = "message"
		}
	default:
		h, he := f.rtmpHeader(dir, e.Raw)
		if he != nil {
			return observedWireError(he)
		}
		if h.WireLength != len(e.Raw) {
			return protocolError(ErrMalformedMessage, "RTMP exact chunk boundary changed")
		}
		fields = map[string]any{"Packet Name": "Chunk", "Format": h.Format, "Chunk Stream ID": h.CSID, "Timestamp": h.Context.Timestamp, "Message Length": h.Context.Length, "Message Type": h.Context.Type, "Message Name": stream_parser.RTMPMessageName(h.Context.Type), "Message Stream ID": h.Context.StreamID, "Chunk Size": s.chunkSize[dir], "Payload Length": h.PayloadLength, "Continuation": h.Continuation, "Header Length": h.HeaderLength}
		m := s.streams[dir][h.CSID]
		extra := int64(h.PayloadLength + 8 + len(e.SourceBytes.PacketRefs)*80)
		if m == nil {
			if len(s.streams[dir]) >= f.a.budget.MaxCollectionElements {
				return protocolError(ErrResourceExceeded, "RTMP chunk stream count budget")
			}
			extra += 256
		}
		if err = f.reserveSession(s.storage() + extra); err != nil {
			return err
		}
		if m == nil {
			m = &rtmpAssembly{}
			s.streams[dir][h.CSID] = m
		}
		if !h.Continuation {
			m.data = nil
			m.ids = nil
			m.refs = nil
		}
		m.header = h.Context
		m.data = append(m.data, e.Raw[h.HeaderLength:]...)
		m.header.Received = len(m.data)
		m.ids = append(m.ids, e.ID)
		for _, r := range e.SourceBytes.PacketRefs {
			if len(m.refs) == 0 || m.refs[len(m.refs)-1] != r {
				m.refs = append(m.refs, r)
			}
		}
		fields["Observed Message Bytes"] = len(m.data)
		if len(m.data) == int(m.header.Length) {
			child := f.rtmpMessageEvent(dir, m, "decoded", "")
			childFields, de := stream_parser.DecodeRTMPMessage(child.Raw, m.header.Type, f.a.budget.MaxCollectionElements)
			childFields["Timestamp"], childFields["Chunk Stream ID"], childFields["Message Stream ID"] = m.header.Timestamp, h.CSID, m.header.StreamID
			if m.header.Type >= 1 && m.header.Type <= 6 && (h.CSID != 2 || m.header.StreamID != 0) {
				de = protocolError(ErrMalformedMessage, "RTMP control message requires CSID 2 and message stream 0")
			}
			if de == nil {
				if _, ok := childFields["AMF0 Values"]; ok {
					child.Protocol = "amf0"
					child.Entry = "AMF0MessageFields"
				}
				switch m.header.Type {
				case 1:
					s.chunkSize[dir] = childFields["Chunk Size"].(uint32)
				case 2:
					target := childFields["Abort CSID"].(uint32)
					if a := s.streams[dir][target]; a != nil && len(a.data) > 0 && len(a.data) < int(a.header.Length) {
						ab := f.rtmpMessageEvent(dir, a, "incomplete", "RTMP Abort discarded the observed partial message")
						e.rtmpChildren = append(e.rtmpChildren, ab)
						a.data = nil
						a.ids = nil
						a.refs = nil
						a.header.Received = int(a.header.Length)
					}
				}
				if number, ok := childFields["Transaction Number"].(float64); ok && number != 0 {
					command, _ := childFields["Command"].(string)
					key := rtmpTransaction{dir, m.header.StreamID, number}
					if command == "_result" || command == "_error" {
						key.dir = 1 - dir
						if id, ok := s.pending[key]; ok {
							child.ResponseTo = id
							child.TransactionID = id
							delete(s.pending, key)
						}
					} else {
						if _, exists := s.pending[key]; exists {
							s.pending[key] = 0
							childFields["Transaction Context"] = "duplicate outstanding number; response association quarantined"
						} else if len(s.pending) < f.a.budget.MaxCollectionElements {
							if reserveErr := f.reserveSession(s.storage() + 64); reserveErr != nil {
								de = reserveErr
							} else {
								s.pending[key] = child.ID
								child.TransactionID = child.ID
							}
						} else {
							de = protocolError(ErrResourceExceeded, "RTMP outstanding command budget")
						}
					}
				}
			}
			child.Session = childFields
			child.semanticFields = cloneSession(childFields)
			child.semanticSession = cloneSession(childFields)
			if de != nil {
				child.sessionError = observedWireError(de).(*ProtocolError)
				child.Status = statusForProtocolError(child.sessionError)
				child.Error = child.sessionError.Error()
				child.Completeness = "message"
			}
			child.Summary = fmt.Sprintf("RTMP %v", childFields["Packet Name"])
			if cmd, ok := childFields["Command"].(string); ok {
				child.Summary += " · " + cmd
			}
			e.rtmpChildren = append(e.rtmpChildren, child)
			fields["Completed Message PDU"] = child.ID
			m.data = nil
			m.ids = nil
			m.refs = nil
		}
	}
	e.Session = fields
	e.semanticFields = cloneSession(fields)
	e.semanticSession = cloneSession(fields)
	e.Summary = fmt.Sprintf("RTMP %v", fields["Packet Name"])
	if err == nil {
		err = f.reserveSession(s.storage())
	}
	return observedWireError(err)
}
func statusForProtocolError(err *ProtocolError) string {
	switch err.Kind {
	case ErrContextRequired, ErrUnsupportedFeature:
		return "context-required"
	case ErrResourceExceeded:
		return "limited"
	case ErrNeedMore:
		return "incomplete"
	}
	return "malformed"
}
func (f *binFlow) rtmpMessageEvent(dir int, m *rtmpAssembly, status, why string) *ProtocolEvent {
	e := f.event(dir, m.data, status, why)
	e.ID = f.a.ids.Add(1)
	e.Offset = f.rtmp.messageOffset[dir]
	f.rtmp.messageOffset[dir] += uint64(len(m.data))
	e.Profile = "rtmp-message"
	e.Admission = "observed-chunk-stream-reassembly"
	e.Completeness = "message"
	e.nativeSessionMessage = true
	e.CarrierLayers = append(append([]string(nil), e.CarrierLayers...), "RTMP")
	e.SourceBytes = ByteSource{Kind: "reassembled", ParentPDUs: append([]uint64(nil), m.ids...), PacketRefs: append([]PacketReference(nil), m.refs...)}
	if len(m.ids) > 0 {
		e.SourceBytes.ParentPDU = m.ids[len(m.ids)-1]
	}
	if status == "incomplete" {
		e.Completeness = "incomplete"
		e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: why}
		e.Error = e.sessionError.Error()
		e.Session = map[string]any{"Expected Length": m.header.Length, "Observed Length": len(m.data), "Message Type": m.header.Type}
		e.semanticFields = cloneSession(e.Session)
		e.semanticSession = cloneSession(e.Session)
	}
	return e
}
func (f *binFlow) finishRTMP(reason TrafficFlowCloseReason) {
	if s := f.rtmp; s != nil {
		if len(s.pending) > 0 {
			e := f.event(0, nil, "incomplete", string(reason)+": RTMP outstanding observed command responses")
			e.Completeness = "incomplete"
			e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: e.Summary}
			e.Error = e.sessionError.Error()
			e.Session = map[string]any{"Outstanding Commands": len(s.pending)}
			e.semanticSession = cloneSession(e.Session)
			f.a.incomplete.Add(1)
			f.a.emit(e)
		}
		for dir, d := range s.streams {
			ids := make([]int, 0, len(d))
			for id := range d {
				ids = append(ids, int(id))
			}
			sort.Ints(ids)
			for _, id := range ids {
				m := d[uint32(id)]
				if len(m.data) > 0 {
					e := f.rtmpMessageEvent(dir, m, "incomplete", string(reason)+": RTMP unfinished chunk stream")
					f.a.incomplete.Add(1)
					f.a.emit(e)
				}
			}
		}
	}
}
