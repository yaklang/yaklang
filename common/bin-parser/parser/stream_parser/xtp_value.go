package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// XTP 4.0's wire VER is 1, not 4. The explicit size cap is an implementation
// profile, not a claim that the 32-bit DLEN cannot describe larger packets.
const xtpMaxBytes = 65535

type xtpField struct {
	Name, Type string
	Start, End int // message-relative bits
	Children   []xtpField
}

func xtpLeaf(name, typ string, start, size int) xtpField {
	return xtpField{Name: name, Type: typ, Start: start * 8, End: (start + size) * 8}
}

// Internet one's-complement checksum, including an odd final octet as the
// high-order octet of a word. There is no IP pseudo-header in the XTP checksum.
func xtpChecksum(wire []byte) uint16 {
	var sum uint32
	for len(wire) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(wire))
		wire = wire[2:]
	}
	if len(wire) != 0 {
		sum += uint32(wire[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

type xtpReader struct {
	wire []byte
	pos  int
}

func (r *xtpReader) fixed(name string, size int, fields ...xtpField) (xtpField, error) {
	if size < 0 || size > len(r.wire)-r.pos {
		return xtpField{}, fmt.Errorf("xtp: truncated %s", name)
	}
	group := xtpField{Name: name, Start: r.pos * 8, End: (r.pos + size) * 8}
	for _, f := range fields {
		f.Start += r.pos * 8
		f.End += r.pos * 8
		group.Children = append(group.Children, f)
	}
	r.pos += size
	return group, nil
}

func (r *xtpReader) control(traffic bool) (xtpField, error) {
	fields := []xtpField{xtpLeaf("Receive Sequence", "uint64", 0, 8), xtpLeaf("Allocation", "uint64", 8, 8), xtpLeaf("Echo", "uint32", 16, 4)}
	if traffic {
		fields = append(fields, xtpLeaf("Control Reserved", "uint32", 20, 4), xtpLeaf("Exchange Key", "uint64", 24, 8))
		return r.fixed("Traffic Control", 32, fields...)
	}
	return r.fixed("Common Control", 20, fields...)
}

func (r *xtpReader) address() (xtpField, error) {
	if len(r.wire)-r.pos < 4 {
		return xtpField{}, fmt.Errorf("xtp: truncated address segment prefix")
	}
	length := int(binary.BigEndian.Uint16(r.wire[r.pos:]))
	format := r.wire[r.pos+3]
	fields := []xtpField{xtpLeaf("Address Length", "uint16", 0, 2), xtpLeaf("Address Domain", "uint8", 2, 1), xtpLeaf("Address Format", "uint8", 3, 1)}
	switch format {
	case 0:
		if length != 8 {
			return xtpField{}, fmt.Errorf("xtp: null address length must be 8")
		}
		fields = append(fields, xtpLeaf("Null Address Data", "raw", 4, 4))
	case 1:
		if length != 16 {
			return xtpField{}, fmt.Errorf("xtp: IPv4 address length must be 16")
		}
		fields = append(fields, xtpLeaf("Destination IPv4", "raw", 4, 4), xtpLeaf("Source IPv4", "raw", 8, 4), xtpLeaf("Destination Port", "uint16", 12, 2), xtpLeaf("Source Port", "uint16", 14, 2))
	default:
		return xtpField{}, fmt.Errorf("xtp: unsupported address format %d", format)
	}
	return r.fixed("Address Segment", length, fields...)
}

func (r *xtpReader) traffic() (xtpField, error) {
	if len(r.wire)-r.pos < 4 {
		return xtpField{}, fmt.Errorf("xtp: truncated traffic specifier prefix")
	}
	length := int(binary.BigEndian.Uint16(r.wire[r.pos:]))
	format := r.wire[r.pos+3]
	fields := []xtpField{xtpLeaf("Traffic Length", "uint16", 0, 2), xtpLeaf("Service", "uint8", 2, 1), xtpLeaf("Traffic Format", "uint8", 3, 1)}
	switch format {
	case 0:
		if length != 8 {
			return xtpField{}, fmt.Errorf("xtp: null traffic length must be 8")
		}
		fields = append(fields, xtpLeaf("Null Traffic Data", "raw", 4, 4))
	case 1:
		if length != 24 {
			return xtpField{}, fmt.Errorf("xtp: explicit traffic length must be 24")
		}
		for i, name := range []string{"Maximum Data", "Incoming Rate", "Incoming Burst", "Outgoing Rate", "Outgoing Burst"} {
			fields = append(fields, xtpLeaf(name, "uint32", 4+i*4, 4))
		}
	default:
		return xtpField{}, fmt.Errorf("xtp: unsupported traffic format %d", format)
	}
	return r.fixed("Traffic Specifier", length, fields...)
}

func (r *xtpReader) data(btag bool) (xtpField, error) {
	start := r.pos
	fields := []xtpField{}
	if btag {
		if len(r.wire)-r.pos < 8 {
			return xtpField{}, fmt.Errorf("xtp: BTAG requires eight data segment bytes")
		}
		fields = append(fields, xtpLeaf("Byte Tag", "uint64", 0, 8))
	}
	// Include an explicit empty leaf when appropriate; empty data is not a
	// fabricated omitted protocol field, and round-trip remains byte-exact.
	offset := 0
	if btag {
		offset = 8
	}
	fields = append(fields, xtpLeaf("Data", "raw", offset, len(r.wire)-start-offset))
	return r.fixed("Data Segment", len(r.wire)-start, fields...)
}

func decodeXTPMessage(wire []byte) ([]xtpField, error) {
	if len(wire) < 32 || len(wire) > xtpMaxBytes {
		return nil, fmt.Errorf("xtp: packet boundary must be 32..65535 bytes")
	}
	version, format := wire[11]>>5, wire[11]&31
	if version != 1 {
		return nil, fmt.Errorf("xtp: unsupported version %d (XTP 4.0 requires wire version 1)", version)
	}
	if uint64(binary.BigEndian.Uint32(wire[12:16])) != uint64(len(wire)-32) {
		return nil, fmt.Errorf("xtp: DLEN does not match exact payload boundary")
	}
	checkLength := len(wire)
	if wire[8]&0x40 != 0 {
		checkLength = 32
	}
	if xtpChecksum(wire[:checkLength]) != 0 {
		return nil, fmt.Errorf("xtp: invalid Internet checksum")
	}
	command := xtpField{Name: "Command", Start: 64, End: 96}
	flags := []string{"Command Reserved Bit", "NOCHECK", "EDGE", "NOERR", "MULTI", "RES", "SORT", "NOFLOW", "FASTNAK", "SREQ", "DREQ", "RCLOSE", "WCLOSE", "EOM", "END", "BTAG"}
	for i, name := range flags {
		command.Children = append(command.Children, xtpField{Name: name, Type: "uint8", Start: 64 + i, End: 65 + i})
	}
	command.Children = append(command.Children, xtpLeaf("Command Reserved Octet", "uint8", 10, 1), xtpField{Name: "Version", Type: "uint8", Start: 88, End: 91}, xtpField{Name: "Packet Format", Type: "uint8", Start: 91, End: 96})
	fields := []xtpField{xtpLeaf("Key", "uint64", 0, 8), command, xtpLeaf("Data Length", "uint32", 12, 4), xtpLeaf("Checksum", "uint16", 16, 2), xtpLeaf("Sort", "uint16", 18, 2), xtpLeaf("Synchronization", "uint32", 20, 4), xtpLeaf("Sequence", "uint64", 24, 8)}
	r := &xtpReader{wire: wire, pos: 32}
	add := func(field xtpField, err error) error {
		if err == nil {
			fields = append(fields, field)
		}
		return err
	}
	var err error
	switch format {
	case 0: // DATA
		err = add(r.data(wire[9]&1 != 0))
	case 1: // CNTL
		err = add(r.control(false))
	case 2: // FIRST: unlike an old dissector, advance by ALEN, not a fixed 16.
		if err = add(r.address()); err == nil {
			err = add(r.traffic())
		}
		if err == nil {
			err = add(r.data(wire[9]&1 != 0))
		}
	case 5, 7: // TCNTL / JCNTL (XTP 4.0 multicast addendum)
		err = add(r.control(true))
		if err == nil && format == 7 {
			err = add(r.address())
		}
		if err == nil {
			err = add(r.traffic())
		}
	case 8: // DIAG: diagnostic message is retained, not interpreted as actions.
		if len(wire)-r.pos < 8 {
			err = fmt.Errorf("xtp: diagnostic segment requires eight header bytes")
		} else {
			err = add(r.fixed("Diagnostic Segment", len(wire)-r.pos, xtpLeaf("Diagnostic Code", "uint32", 0, 4), xtpLeaf("Diagnostic Value", "uint32", 4, 4), xtpLeaf("Diagnostic Message", "raw", 8, len(wire)-r.pos-8)))
		}
	default:
		// ECNTL spans, obsolete JOIN, and unassigned formats are deliberately
		// not claimed by this profile. In particular, do not copy conflicting
		// ECNTL edge byte order from historical display code without evidence.
		err = fmt.Errorf("xtp: unsupported packet format %d", format)
	}
	if err != nil {
		return nil, err
	}
	if r.pos != len(wire) {
		return nil, fmt.Errorf("xtp: unexpected trailing bytes after control segment")
	}
	return fields, nil
}
