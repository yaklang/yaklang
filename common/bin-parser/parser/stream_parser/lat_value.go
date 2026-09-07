package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// LAT message spans are in bits so nibble/flag fields never overlap raw leaves.
// Multi-octet numbers are little endian; service data is never interpreted using
// a service class guessed from earlier slots or packets.
type latField struct {
	Name, Type string
	Start, End int
	List       bool
	Children   []latField
}

type latReader struct {
	wire       []byte
	pos, limit int
	err        error
}

func (r *latReader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("lat: offset %d: %s", r.pos, fmt.Sprintf(format, args...))
	}
}

func (r *latReader) raw(fields *[]latField, name, typ string, length int) []byte {
	if r.err != nil {
		return nil
	}
	if length < 0 || length > r.limit-r.pos {
		r.fail("truncated %s", name)
		return nil
	}
	start := r.pos
	r.pos += length
	*fields = append(*fields, latField{Name: name, Type: typ, Start: start * 8, End: r.pos * 8})
	return r.wire[start:r.pos]
}

func (r *latReader) number(fields *[]latField, name string, size int) int {
	typ := "uint8"
	if size == 2 {
		typ = "uint16"
	}
	b := r.raw(fields, name, typ, size)
	if r.err != nil {
		return 0
	}
	if size == 2 {
		return int(binary.LittleEndian.Uint16(b))
	}
	return int(b[0])
}

func (r *latReader) counted(fields *[]latField, name string, nonempty, identifier bool) {
	n := r.number(fields, name+" Length", 1)
	if nonempty && n == 0 {
		r.fail("%s must be nonempty", name)
	}
	b := r.raw(fields, name, "string", n)
	if identifier {
		// AA-NL26A-TE section 3.3 also permits DEC multinational bytes C0..FF.
		// Preserve original spelling; this parser performs no name comparison.
		for _, c := range b {
			if !(c == '$' || c == '-' || c == '.' || c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0xc0) {
				r.fail("invalid character in %s", name)
				break
			}
		}
	}
}

func (r *latReader) parameters(fields *[]latField, slot bool) {
	list := latField{Name: "Parameters", Start: r.pos * 8, List: true}
	for r.err == nil {
		entry := latField{Name: "Parameter", Start: r.pos * 8}
		code := r.number(&entry.Children, "Parameter Code", 1)
		if code != 0 {
			n := r.number(&entry.Children, "Parameter Length", 1)
			if slot && ((code >= 1 && code <= 3 && n != 2) || (code == 6 && n > 32)) {
				r.fail("invalid class 1 parameter length")
			}
			r.raw(&entry.Children, "Parameter Data", "raw", n)
		}
		entry.End = r.pos * 8
		list.Children = append(list.Children, entry)
		if code == 0 {
			break
		}
	}
	list.End = r.pos * 8
	*fields = append(*fields, list)
}

func decodeLATMessage(wire []byte) ([]latField, error) {
	if len(wire) < 8 || len(wire) > 1500 {
		return nil, fmt.Errorf("lat: message boundary must be 8..1500 bytes")
	}
	r := latReader{wire: wire, limit: len(wire)}
	control, command := wire[0], wire[0]>>2
	if command > 2 {
		return nil, fmt.Errorf("lat: unsupported message type %d outside virtual-circuit profile", command)
	}
	master := control&2 != 0
	if control&1 != 0 && (master || command != 0) {
		return nil, fmt.Errorf("lat: response-required flag only belongs to host Run messages")
	}
	fields := []latField{
		{Name: "Message Type", Type: "uint8", Start: 0, End: 6},
		{Name: "Master", Type: "uint8", Start: 6, End: 7},
		{Name: "Response Required", Type: "uint8", Start: 7, End: 8},
	}
	r.pos = 1
	slots := r.number(&fields, "Slot Count", 1)
	destination := r.number(&fields, "Destination Circuit ID", 2)
	source := r.number(&fields, "Source Circuit ID", 2)
	r.number(&fields, "Message Sequence Number", 1)
	r.number(&fields, "Message Acknowledgment Number", 1)
	switch command {
	case 0:
		if destination == 0 || source == 0 {
			return nil, fmt.Errorf("lat: Run circuit identifiers must be nonzero")
		}
		list := latField{Name: "Slots", Start: r.pos * 8, List: true}
		for index := 0; index < slots && r.err == nil; index++ {
			list.Children = append(list.Children, r.slot(master))
		}
		list.End = r.pos * 8
		fields = append(fields, list)
	case 1:
		if slots != 0 || source == 0 || (master && destination != 0) || (!master && destination == 0) {
			return nil, fmt.Errorf("lat: invalid Start slot count or directional circuit identifiers")
		}
		maximum := r.number(&fields, "Receive Datagram Size", 2)
		if maximum < 576 || maximum > 1518 {
			r.fail("receive datagram size must be 576..1518 including Ethernet overhead")
		}
		version := r.number(&fields, "Protocol Version", 1)
		eco := r.number(&fields, "Protocol ECO", 1)
		if version != 5 || eco != 1 {
			r.fail("unsupported Start version outside 5.1 profile")
		}
		r.number(&fields, "Maximum Simultaneous Slots", 1)
		r.number(&fields, "Extra Data Link Buffers", 1)
		timer := r.number(&fields, "Server Circuit Timer", 1)
		if master && timer == 0 {
			r.fail("terminal-server circuit timer must be nonzero")
		}
		r.number(&fields, "Keep Alive Timer", 1)
		r.number(&fields, "Facility Number", 2)
		r.number(&fields, "Product Type Code", 1)
		r.number(&fields, "Product Version", 1)
		r.counted(&fields, "Slave Node Name", true, true)
		r.counted(&fields, "Master Node Name", true, true)
		r.counted(&fields, "Location Text", false, false)
		r.parameters(&fields, false)
		if r.pos < r.limit {
			r.raw(&fields, "Unpredictable Tail", "raw", r.limit-r.pos)
		}
	case 2:
		if slots != 0 || source != 0 || destination == 0 {
			return nil, fmt.Errorf("lat: invalid Stop slot count or circuit identifiers")
		}
		r.number(&fields, "Circuit Disconnect Reason", 1)
		r.counted(&fields, "Reason Text", false, false)
	}
	if r.err == nil && r.pos != r.limit {
		r.fail("unexpected trailing message bytes")
	}
	if r.err != nil {
		return nil, r.err
	}
	return fields, nil
}

func (r *latReader) slot(master bool) latField {
	field := latField{Name: "Slot", Start: r.pos * 8}
	destination := r.number(&field.Children, "Destination Slot ID", 1)
	source := r.number(&field.Children, "Source Slot ID", 1)
	length := r.number(&field.Children, "Slot Byte Count", 1)
	if r.err != nil || r.pos >= r.limit {
		r.fail("truncated Slot Type")
		return field
	}
	control := r.wire[r.pos]
	typ, low := control>>4, control&15
	lowName := "Credits"
	if typ == 11 {
		lowName = "Must Be Zero"
	} else if typ == 12 || typ == 13 {
		lowName = "Slot Reason"
	}
	field.Children = append(field.Children,
		latField{Name: "Slot Type", Type: "uint8", Start: r.pos * 8, End: r.pos*8 + 4},
		latField{Name: lowName, Type: "uint8", Start: r.pos*8 + 4, End: (r.pos + 1) * 8})
	r.pos++
	if typ != 0 && (typ < 9 || typ > 13) {
		r.fail("unsupported slot type %d", typ)
	}
	if typ == 9 && (source == 0 || (master && destination != 0)) || typ == 13 && source != 0 || master && typ != 9 && destination == 0 || typ == 11 && low != 0 || master && typ == 12 {
		r.fail("invalid directional slot identifiers or slot flags")
	}
	// Section 4.1.3.6 item 11 disallows a zero source in a Run slot;
	// section 4.1.4.5 defines Run_rcv as Data_a, Data_b or Attention.
	if (typ == 0 || typ == 10 || typ == 11) && source == 0 {
		r.fail("Run slot source identifier must be nonzero")
	}
	if length > r.limit-r.pos {
		r.fail("slot length exceeds message boundary")
	}
	if r.err != nil {
		return field
	}
	outerLimit := r.limit
	r.limit = r.pos + length
	if typ == 9 {
		serviceClass := r.number(&field.Children, "Service Class", 1)
		if serviceClass != 1 {
			r.fail("unsupported Start slot service class")
		}
		r.number(&field.Children, "Minimum Attention Slot Size", 1)
		minimum := r.number(&field.Children, "Minimum Data Slot Size", 1)
		if minimum == 0 {
			r.fail("minimum data slot size must be nonzero")
		}
		r.counted(&field.Children, "Object Service Name", false, true)
		r.counted(&field.Children, "Subject Description", false, false)
		r.parameters(&field.Children, true)
		if r.pos < r.limit {
			// Figure A-6 terminates the class 1 status at parameter code 0.
			// Unlike a Start message, this layout declares no internal tail.
			r.fail("unexpected trailing class 1 Start slot status")
		}
	} else {
		name := "Slot Data"
		if typ == 12 || typ == 13 {
			name = "Slot Status"
		}
		r.raw(&field.Children, name, "raw", length)
	}
	r.limit = outerLimit
	if length%2 != 0 {
		r.raw(&field.Children, "Slot Padding", "raw", 1)
	}
	field.End = r.pos * 8
	return field
}
