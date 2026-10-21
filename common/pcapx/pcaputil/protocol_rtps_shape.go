package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"unicode/utf8"
)

// RTPSShapeWriter supplies a type association from outside the untrusted wire.
// It selects the final ShapeType in DDSI-RTPS2.5 section10.7, using the
// PLAIN_CDR1 representation and padding rules of DDS-XTypes1.3. GUID/entity
// matching selects a schema, but does not authenticate that writer.
type RTPSShapeWriter struct {
	SourceGUIDPrefix [12]byte
	WriterID         [4]byte
}

func validateRTPSShapeWriter(binding RTPSShapeWriter) error {
	if binding.SourceGUIDPrefix == [12]byte{} || binding.WriterID == [4]byte{} {
		return fmt.Errorf("RTPS ShapeType needs a nonzero exact source prefix and writer ID")
	}
	return nil
}

// WithProtocolRTPSShapeWriter selects rtps-wire with an exact, caller-supplied
// ShapeType writer association. Unknown writers stay opaque. No schema is
// learned from traffic, and older wire versions require additional context.
func WithProtocolRTPSShapeWriter(port uint16, binding RTPSShapeWriter) CaptureOption {
	return func(c *CaptureConfig) error {
		if err := validateRTPSShapeWriter(binding); err != nil {
			return err
		}
		if err := WithProtocolDecodeAs("udp", port, "rtps-wire")(c); err != nil {
			return err
		}
		if c.rtpsShapeWriters == nil {
			c.rtpsShapeWriters = map[uint16]RTPSShapeWriter{}
		}
		if old, ok := c.rtpsShapeWriters[port]; ok && old != binding {
			return fmt.Errorf("conflicting RTPS ShapeType writer context")
		}
		c.rtpsShapeWriters[port] = binding
		return nil
	}
}

// WithSessionRTPSShapeWriter is the native UDP session counterpart. Transport
// and endpoint ports are supplied separately. The binding is copied by value.
func WithSessionRTPSShapeWriter(port uint16, binding RTPSShapeWriter) ProtocolSessionOption {
	return func(s *captureSession) error {
		if err := validateRTPSShapeWriter(binding); err != nil {
			return err
		}
		if err := WithSessionRTPSWire(port)(s); err != nil {
			return err
		}
		if s.f.a.rtpsShapeWriters == nil {
			s.f.a.rtpsShapeWriters = map[uint16]RTPSShapeWriter{}
		}
		if old, ok := s.f.a.rtpsShapeWriters[port]; ok && old != binding {
			return fmt.Errorf("conflicting RTPS ShapeType writer context")
		}
		s.f.a.rtpsShapeWriters[port] = binding
		return nil
	}
}

func decodeRTPSShapePayload(payload []byte, key bool, remaining *int) (map[string]any, error) {
	if len(payload) < 8 {
		return nil, rtpsMalformed("ShapeType encapsulation/string length is truncated")
	}
	var order binary.ByteOrder
	switch binary.BigEndian.Uint16(payload) {
	case 0:
		order = binary.BigEndian
	case 1:
		order = binary.LittleEndian
	default:
		return nil, rtpsUnsupported("selected ShapeType requires PLAIN_CDR version1")
	}
	if payload[2] != 0 || payload[3]&^byte(3) != 0 {
		return nil, rtpsUnsupported("ShapeType representation options are not selected")
	}
	padding := int(payload[3] & 3)
	// The CDR alignment origin is immediately after encapsulation, not at
	// the DATA body/message beginning (DDSI-RTPS2.5 section10.2).
	w := payload[4:]
	n := uint64(order.Uint32(w))
	if n == 0 || n > 65 || n > uint64(len(w)-4) {
		return nil, rtpsMalformed("ShapeType string length/terminator is invalid")
	}
	color := w[4 : 4+int(n)-1]
	// IDL4.2 section7.4.1.4.4.3.2 bounds Char8 elements, not Unicode
	// codepoints, and excludes embedded NUL. XTypes1.3 encodes them as UTF8.
	if w[4+int(n)-1] != 0 || !utf8.Valid(color) || bytes.IndexByte(color, 0) >= 0 {
		return nil, rtpsMalformed("ShapeType color is not a bounded UTF8 terminated string")
	}
	count := 4
	if key {
		count = 1
	}
	if count > *remaining {
		return nil, protocolError(ErrResourceExceeded, "ShapeType field projection exceeds collection budget")
	}
	at := 4 + int(n)
	if !key {
		at = (at + 3) &^ 3
		if at > len(w) || len(w)-at < 12 {
			return nil, rtpsMalformed("ShapeType aligned coordinate fields are truncated")
		}
	}
	end := at
	if !key {
		end += 12
	}
	// XTypes1.3 section7.6.3.2 declares the exact trailing pad count in the
	// low two option bits; section7.4.1.1 requires zero-valued trailing pads.
	if padding != (-end)&3 || len(w)-end != padding || !bytes.Equal(w[end:], make([]byte, padding)) {
		return nil, rtpsMalformed("ShapeType trailing serialized padding is invalid")
	}
	*remaining -= count
	f := map[string]any{"Color": string(color)}
	if !key {
		f["X"], f["Y"], f["Size"] = int32(order.Uint32(w[at:])), int32(order.Uint32(w[at+4:])), int32(order.Uint32(w[at+8:]))
	}
	return f, nil
}

func (a *binParser) decodeRTPSWireSelected(w []byte, src, dst uint16) (map[string]any, error) {
	remaining := a.budget.MaxCollectionElements
	f, err := decodeRTPSMessageBudgetRemaining(w, remaining, a.budget.MaxRecursionDepth, true, &remaining)
	if err != nil {
		return nil, err
	}
	// Use the same destination-first explicit profile selection as admission.
	// A destination wire-only binding must not inherit a source type selection.
	port := dst
	if a.datagramDecodeAs[port] == "" {
		port = src
	}
	binding, ok := a.rtpsShapeWriters[port]
	if !ok {
		return f, nil
	}
	// Preserve the shared submessage/bitmap/QoS budget for type projection.
	for _, d := range f["Messages"].([]map[string]any) {
		if d["Submessage"] != "DATA" && d["Submessage"] != "NOKEY_DATA" {
			continue
		}
		ctx := d["Receiver Context"].(map[string]any)
		if ctx["Source GUID Prefix"] != hex.EncodeToString(binding.SourceGUIDPrefix[:]) || d["Writer ID"] != hex.EncodeToString(binding.WriterID[:]) {
			continue
		}
		payload, present := d["Serialized Payload"].([]byte)
		if !present {
			continue
		}
		if ctx["Protocol Version"] != "2.4" && ctx["Protocol Version"] != "2.5" {
			return nil, protocolError(ErrContextRequired, "ShapeType older-version CDR alignment needs explicit context")
		}
		app, err := decodeRTPSShapePayload(payload, d["Key Present"] == true, &remaining)
		if err != nil {
			return nil, err
		}
		d["Application Type"], d["Application Context"], d["Application Fields"] = "ShapeType", "caller-selected-unverified-writer", app
	}
	return f, nil
}
