package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/net/http2/hpack"
)

const (
	http2FieldsMaxBytes    = 1 << 20
	http2FieldsMaxFrames   = 1024
	http2FieldsMaxSettings = 1024
	http2FieldsMaxHeaders  = 4096
	http2FieldsMaxString   = 65536
	http2FieldsPreface     = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
)

// Bit positions always refer to the supplied wire, not decompressed text.
type http2WireField struct {
	Name, Type string
	Start, End int
	Children   []http2WireField
	Info       map[string]any
}

func http2WireLeaf(name, typ string, start, end int) http2WireField {
	return http2WireField{Name: name, Type: typ, Start: start * 8, End: end * 8}
}

// Check representation boundaries and table-update placement independently of
// hpack's dictionary occupancy. The existing decoder accepts late updates with
// an empty table, and can reject a second leading update with a nonempty one.
// This scanner does not decode strings or implement a second HPACK dictionary.
func http2HPACKLeadingUpdates(block []byte) ([]int, error) {
	at := 0
	integer := func(prefix byte) (uint64, error) {
		if at >= len(block) {
			return 0, fmt.Errorf("http2-fields: truncated HPACK integer")
		}
		mask := uint64(1<<prefix) - 1
		value := uint64(block[at]) & mask
		at++
		if value < mask {
			return value, nil
		}
		for shift := uint(0); shift < 64 && at < len(block); shift += 7 {
			b := block[at]
			at++
			part := uint64(b & 127)
			if part > (^uint64(0)-value)>>shift {
				return 0, fmt.Errorf("http2-fields: HPACK integer overflow")
			}
			value += part << shift
			if b < 128 {
				return value, nil
			}
		}
		return 0, fmt.Errorf("http2-fields: incomplete or excessive HPACK integer")
	}
	str := func() error {
		n, err := integer(7)
		if err != nil {
			return err
		}
		if n > uint64(len(block)-at) {
			return fmt.Errorf("http2-fields: truncated HPACK string")
		}
		at += int(n)
		return nil
	}
	var updates []int
	seenField := false
	for at < len(block) {
		b := block[at]
		if b&0xe0 == 0x20 {
			if seenField {
				return nil, fmt.Errorf("http2-fields: HPACK table update after a field")
			}
			if _, err := integer(5); err != nil {
				return nil, err
			}
			updates = append(updates, at)
			continue
		}
		seenField = true
		if b&0x80 != 0 {
			if _, err := integer(7); err != nil {
				return nil, err
			}
			continue
		}
		prefix := byte(4)
		if b&0x40 != 0 {
			prefix = 6
		}
		index, err := integer(prefix)
		if err != nil {
			return nil, err
		}
		if index == 0 {
			if err := str(); err != nil {
				return nil, err
			}
		}
		if err := str(); err != nil {
			return nil, err
		}
	}
	return updates, nil
}

func decodeHTTP2HPACKBlock(decoder *hpack.Decoder, block []byte) error {
	updates, err := http2HPACKLeadingUpdates(block)
	if err != nil {
		return err
	}
	at := 0
	for _, end := range updates {
		if _, err := decoder.Write(block[at:end]); err != nil {
			return err
		}
		// Reset only the decoder's representation-position state, not its table.
		if err := decoder.Close(); err != nil {
			return err
		}
		at = end
	}
	if _, err := decoder.Write(block[at:]); err != nil {
		return err
	}
	return decoder.Close()
}

type http2ObservedFrame struct {
	field                           http2WireField
	end, fragmentStart, fragmentEnd int
	typ, flags                      byte
	stream, promised                uint32
}

// Frame syntax only. The settings advertised here apply in the reverse
// direction and must not alter this sender's HPACK decoder or flow window.
func decodeHTTP2WireFrame(wire []byte, start int) (http2ObservedFrame, error) {
	var out http2ObservedFrame
	fail := func(why string) (http2ObservedFrame, error) { return out, fmt.Errorf("http2-fields: %s", why) }
	if start < 0 || start > len(wire) || len(wire)-start < 9 {
		return fail("truncated frame header")
	}
	size := int(wire[start])<<16 | int(wire[start+1])<<8 | int(wire[start+2])
	if size > len(wire)-start-9 {
		return fail("frame exceeds supplied boundary")
	}
	out.end, out.typ, out.flags = start+9+size, wire[start+3], wire[start+4]
	out.stream = binary.BigEndian.Uint32(wire[start+5:]) & 0x7fffffff
	out.fragmentStart, out.fragmentEnd = -1, -1
	info := map[string]any{"Type": uint64(out.typ), "Stream Identifier": uint64(out.stream), "Payload Layout Decoded": true}
	f := http2WireField{Name: "Frame", Start: start * 8, End: out.end * 8, Info: info}
	f.Children = []http2WireField{
		http2WireLeaf("Length", "uint32", start, start+3), http2WireLeaf("Type", "uint8", start+3, start+4), http2WireLeaf("Flags", "uint8", start+4, start+5),
		{Name: "Reserved", Type: "uint8", Start: (start + 5) * 8, End: (start+5)*8 + 1},
		{Name: "Stream Identifier", Type: "uint32", Start: (start+5)*8 + 1, End: (start + 9) * 8},
	}
	at, end := start+9, out.end
	add := func(name, typ string, n int) {
		f.Children = append(f.Children, http2WireLeaf(name, typ, at, at+n))
		at += n
	}
	word31 := func(reserved, name string) {
		f.Children = append(f.Children, http2WireField{Name: reserved, Type: "uint8", Start: at * 8, End: at*8 + 1}, http2WireField{Name: name, Type: "uint32", Start: at*8 + 1, End: (at + 4) * 8})
		at += 4
	}
	if out.typ == 4 || out.typ == 6 || out.typ == 7 {
		if out.stream != 0 {
			return fail("connection frame has nonzero stream identifier")
		}
	} else if out.typ <= 9 && out.typ != 8 && out.stream == 0 {
		return fail("stream frame has zero stream identifier")
	}
	padded := (out.typ == 0 || out.typ == 1 || out.typ == 5) && out.flags&8 != 0
	if padded {
		if at == end {
			return fail("missing pad length")
		}
		pad := int(wire[at])
		add("Pad Length", "uint8", 1)
		if pad > end-at {
			return fail("padding exceeds payload")
		}
		end -= pad
	}
	priority := func() error {
		if end-at < 5 {
			return fmt.Errorf("http2-fields: truncated priority")
		}
		dependency := binary.BigEndian.Uint32(wire[at:]) & 0x7fffffff
		if dependency == out.stream {
			return fmt.Errorf("http2-fields: self-dependent priority")
		}
		word31("Exclusive", "Stream Dependency")
		info["Effective Weight"] = uint64(wire[at]) + 1
		add("Weight", "uint8", 1)
		return nil
	}
	switch out.typ {
	case 0:
		info["End Stream"] = out.flags&1 != 0
		add("Data", "raw", end-at)
	case 1:
		info["End Stream"] = out.flags&1 != 0
		info["End Headers"] = out.flags&4 != 0
		if out.flags&32 != 0 {
			if err := priority(); err != nil {
				return out, err
			}
		}
		out.fragmentStart, out.fragmentEnd = at, end
		add("Field Block Fragment", "raw", end-at)
	case 2:
		if size != 5 {
			return fail("PRIORITY length must be five")
		}
		if err := priority(); err != nil {
			return out, err
		}
	case 3:
		if size != 4 {
			return fail("RST_STREAM length must be four")
		}
		add("Error Code", "uint32", 4)
	case 4:
		if size%6 != 0 || size/6 > http2FieldsMaxSettings {
			return fail("invalid or excessive SETTINGS list")
		}
		if out.flags&1 != 0 && size != 0 {
			return fail("SETTINGS ACK has a payload")
		}
		info["ACK"] = out.flags&1 != 0
		if size == 0 {
			add("Settings", "raw", 0)
		}
		for i := 0; at < end; i++ {
			id, value := binary.BigEndian.Uint16(wire[at:]), binary.BigEndian.Uint32(wire[at+2:])
			if id == 2 && value > 1 || id == 4 && value > 0x7fffffff || id == 5 && (value < 16384 || value > 0xffffff) {
				return fail("invalid defined SETTINGS value")
			}
			setting := http2WireField{Name: fmt.Sprintf("Setting %d", i), Start: at * 8, End: (at + 6) * 8, Children: []http2WireField{http2WireLeaf("Identifier", "uint16", at, at+2), http2WireLeaf("Value", "uint32", at+2, at+6)}}
			f.Children = append(f.Children, setting)
			at += 6
		}
	case 5:
		if end-at < 4 {
			return fail("truncated PUSH_PROMISE")
		}
		out.promised = binary.BigEndian.Uint32(wire[at:]) & 0x7fffffff
		if out.promised == 0 {
			return fail("zero promised stream identifier")
		}
		word31("Promised Reserved", "Promised Stream Identifier")
		info["End Headers"] = out.flags&4 != 0
		out.fragmentStart, out.fragmentEnd = at, end
		add("Field Block Fragment", "raw", end-at)
	case 6:
		if size != 8 {
			return fail("PING length must be eight")
		}
		info["ACK"] = out.flags&1 != 0
		add("Ping Data", "raw", 8)
	case 7:
		if size < 8 {
			return fail("GOAWAY length below eight")
		}
		word31("Last Stream Reserved", "Last Stream Identifier")
		add("Error Code", "uint32", 4)
		add("Debug Data", "raw", end-at)
	case 8:
		if size != 4 {
			return fail("WINDOW_UPDATE length must be four")
		}
		if binary.BigEndian.Uint32(wire[at:])&0x7fffffff == 0 {
			return fail("zero window increment")
		}
		word31("Window Reserved", "Window Size Increment")
	case 9:
		info["End Headers"] = out.flags&4 != 0
		out.fragmentStart, out.fragmentEnd = at, end
		add("Field Block Fragment", "raw", end-at)
	default:
		info["Payload Layout Decoded"] = false
		add("Unknown Frame Data", "raw", end-at)
	}
	if padded {
		add("Padding", "raw", out.end-at)
	}
	out.field = f
	return out, nil
}

// An explicit initial, ordered plaintext direction: HPACK starts empty with
// its RFC default 4096-byte limit. No midstream snapshot, reverse settings,
// TCP/TLS reconstruction, HTTP semantics, or connection lifecycle is inferred.
// Every preceding block in this direction is kept in the local decoder; a
// dynamic reference without its source fails instead of becoming a raw success.
func decodeHTTP2Fields(wire []byte, mode string) ([]http2WireField, map[string]any, error) {
	fail := func(s string) ([]http2WireField, map[string]any, error) {
		return nil, nil, fmt.Errorf("http2-fields: %s", s)
	}
	if len(wire) < 9 || len(wire) > http2FieldsMaxBytes {
		return fail("input outside 9..1048576 bytes")
	}
	if mode != "frame" && mode != "client" && mode != "server" {
		return fail("unknown profile")
	}
	info := map[string]any{"Profile": "HTTP/2 " + mode, "Initial Direction Supplied By Caller": mode != "frame", "HPACK Decoded": mode != "frame", "HPACK Table Limit": uint64(4096), "Peer Settings Applied": false, "Peer Frame Limit Validated": false, "HTTP Semantics Validated": false, "Flow Control Validated": false, "Connection Lifecycle Validated": false, "TCP Reassembly Performed": false, "Structured Generation Supported": false}
	var fields []http2WireField
	at := 0
	if mode == "client" {
		if !bytes.HasPrefix(wire, []byte(http2FieldsPreface)) {
			return fail("exact client preface required")
		}
		fields = append(fields, http2WireLeaf("Client Preface", "raw", 0, 24))
		at = 24
	}
	var decoder *hpack.Decoder
	var headers []map[string]any
	headerCount, headerBytes := 0, 0
	var headerErr error
	if mode != "frame" {
		decoder = hpack.NewDecoder(4096, func(h hpack.HeaderField) {
			if headerErr != nil {
				return
			}
			headerCount++
			headerBytes += len(h.Name) + len(h.Value) + 32
			if headerCount > http2FieldsMaxHeaders || headerBytes > http2FieldsMaxBytes {
				headerErr = fmt.Errorf("http2-fields: decoded header resource limit")
				decoder.SetEmitEnabled(false)
				return
			}
			headers = append(headers, map[string]any{"Name": h.Name, "Value": h.Value, "Sensitive": h.Sensitive})
		})
		decoder.SetMaxStringLength(http2FieldsMaxString)
	}
	frameCount, settingCount := 0, 0
	var pending *http2ObservedFrame
	var compressed []byte
	var ranges [][2]int
	var blocks []map[string]any
	for at < len(wire) {
		if frameCount >= http2FieldsMaxFrames {
			return fail("frame count exceeds local limit")
		}
		frame, err := decodeHTTP2WireFrame(wire, at)
		if err != nil {
			return nil, nil, err
		}
		if frameCount == 0 && mode != "frame" && (frame.typ != 4 || frame.flags&1 != 0) {
			return fail("initial direction must start with non-ACK SETTINGS")
		}
		if mode == "frame" && frame.end != len(wire) {
			return fail("single frame boundary contains trailing bytes")
		}
		if mode == "client" && frame.typ == 5 {
			return fail("client direction cannot send PUSH_PROMISE")
		}
		if frame.typ == 4 {
			if mode == "server" {
				for setting := at + 9; setting < frame.end; setting += 6 {
					if binary.BigEndian.Uint16(wire[setting:]) == 2 && binary.BigEndian.Uint32(wire[setting+2:]) == 1 {
						return fail("server cannot enable push")
					}
				}
			}
			settingCount += (frame.end - at - 9) / 6
			if settingCount > http2FieldsMaxSettings {
				return fail("cumulative SETTINGS resource limit")
			}
		}
		if decoder != nil {
			if pending != nil && (frame.typ != 9 || frame.stream != pending.stream) {
				return fail("field block requires same-stream CONTINUATION")
			}
			if frame.typ == 9 && pending == nil {
				return fail("CONTINUATION without field block")
			}
			if frame.fragmentStart >= 0 {
				if frame.typ != 9 {
					pending = &frame
					headers = nil
					ranges = nil
					compressed = nil
				}
				ranges = append(ranges, [2]int{frame.fragmentStart, frame.fragmentEnd})
				compressed = append(compressed, wire[frame.fragmentStart:frame.fragmentEnd]...)
				frame.field.Info["Header Block Index"] = len(blocks)
				if frame.flags&4 != 0 {
					if err := decodeHTTP2HPACKBlock(decoder, compressed); err != nil {
						return nil, nil, fmt.Errorf("http2-fields: HPACK block: %w", err)
					}
					if headerErr != nil {
						return nil, nil, headerErr
					}
					blocks = append(blocks, map[string]any{"Stream Identifier": uint64(pending.stream), "Promised Stream Identifier": uint64(pending.promised), "Frame Type": uint64(pending.typ), "Headers": headers, "Relative Byte Ranges": ranges, "Prior Header Block Count": len(blocks), "Decoded Strings Have Direct Wire Spans": false})
					pending = nil
				}
			}
		}
		fields = append(fields, frame.field)
		frameCount++
		at = frame.end
	}
	if frameCount == 0 {
		return fail("missing initial SETTINGS")
	}
	if pending != nil {
		return fail("incomplete field block at supplied boundary")
	}
	info["Frame Count"], info["Header Blocks"], info["Header Count"] = frameCount, blocks, headerCount
	return fields, info, nil
}
