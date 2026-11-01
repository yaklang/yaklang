package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
)

// This is a passive observer's connection-level RFC9114 7.1 finding. Keep
// transport/other-stream evidence usable, without claiming the peer closed or
// that subsequent individually decoded messages make the connection conformant.
type h3ConnectionFault struct {
	pdu, stream, offset, end uint64
	direction                int
}

func (f *h3ConnectionFault) snapshot() map[string]any {
	return map[string]any{
		"Error Name": "H3_FRAME_ERROR", "Error Code": uint64(0x106), "Error Scope": "connection",
		"First PDU ID": f.pdu, "Stream ID": f.stream, "Direction": uint64(f.direction),
		"Frame Offset": f.offset, "FIN Stream Offset": f.end,
		"Scope": "clean FIN after complete QUIC stream reassembly; connection violation observed; peer closure and delivery are not inferred",
	}
}

type h3ConnectionObservationError struct{ cause error }

func (e *h3ConnectionObservationError) Error() string { return e.cause.Error() }
func (e *h3ConnectionObservationError) Unwrap() error { return e.cause }
func isH3ConnectionObservationError(err error) bool {
	var e *h3ConnectionObservationError
	return errors.As(err, &e)
}
func isRecoverableH3ObservationError(err error) bool {
	return isH3FieldStreamError(err) || isH3ConnectionObservationError(err)
}

func (q *binQUIC) annotateH3ConnectionFault(info map[string]any) {
	if q.h3Fault != nil {
		info["HTTP3 Connection Error"] = q.h3Fault.snapshot()
		info["HTTP3 Connection Conformance"] = "protocol-violation-observed; later frames are passive observations"
	}
}

func (q *binQUIC) rejectH3FINFrame(st *quicStream, dir int, sid uint64, fr, info map[string]any, frames []map[string]any) error {
	h3 := st.h3
	current := &h3ConnectionFault{q.eventID, sid, uint64(h3.parsed[dir]), uint64(len(h3.buf[dir])), dir}
	if q.h3Fault == nil {
		q.h3Fault = current
	}
	m := current.snapshot()
	partial := h3.buf[dir][h3.parsed[dir]:]
	m["Available Wire Bytes"] = uint64(len(partial))
	m["Partial Frame Bytes"] = bytes.Clone(partial)
	m["Partial Frame SHA256"] = fmt.Sprintf("%x", sha256.Sum256(partial))
	m["Byte Source"] = st.sources[dir].describe(sid, dir, current.offset, current.end)
	m["Incomplete Component"] = "type-varint"
	if typ, n, err := quicVarint(partial); err == nil {
		m["Frame Type Value"] = typ
		m["Incomplete Component"] = "length-varint"
		if length, n2, err := quicVarint(partial[n:]); err == nil {
			m["Incomplete Component"] = "payload"
			m["Declared Payload Length"] = length
			m["Available Payload Bytes"] = uint64(len(partial) - n - n2)
		}
	}
	h3.framingFailure[dir] = true
	fr["HTTP3"], info["HTTP3"] = true, true
	fr["HTTP3 Stream State"] = "frame-layout-violation"
	fr["HTTP3 Frame Error"] = m
	conformance, _ := h3.checkBody(dir, sid, false)
	conformance["Drained FIN"] = true
	conformance["Length Check"] = "unverified-frame-layout"
	conformance["Error"] = "H3_FRAME_ERROR; only complete DATA frames are counted"
	fr["HTTP3 Message Conformance"] = conformance
	if len(frames) > 0 {
		fr["HTTP3 Frames"] = frames
		frames[len(frames)-1]["Message Conformance"] = conformance
	}
	info["Protocol Transition"], info["HTTP3 Stream ID"] = "quic->http3", sid
	q.annotateH3ConnectionFault(info)
	if dir == 0 {
		st.requestPDU = 0
		h3.requestMethod = h3MethodUnknown
		delete(fr, "Request PDU ID")
		delete(fr, "Transaction ID")
	}
	st.sources[dir].prune(^uint64(0))
	return &h3ConnectionObservationError{protocolError(ErrMalformedMessage, "HTTP3 H3_FRAME_ERROR: clean FIN truncates a frame (%s)", m["Incomplete Component"])}
}
