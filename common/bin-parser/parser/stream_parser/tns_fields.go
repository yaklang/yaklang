package stream_parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const tnsFieldsMaxBytes = 1 << 20
const tnsFieldsMaxItems = 4096
const tnsFieldsMaxLeaves = 65536

// Layout and direction are supplied by the caller. In particular, a zero high
// length word is not evidence that a conversation negotiated 32-bit lengths.
// Native TTC layouts are separate from the thin driver's universal encoding.
type tnsFieldsReader struct {
	wire           []byte
	at, end, items int
	err            error
	fields         []tlsCertificateField
}

func (r *tnsFieldsReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("tns-fields: %s at byte %d", s, r.at)
	}
}
func (r *tnsFieldsReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.end-r.at {
		r.fail("incomplete " + name)
		return nil
	}
	if len(r.fields) >= tnsFieldsMaxLeaves {
		r.fail("field resource limit")
		return nil
	}
	s := r.at
	r.at += n
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, s, r.at))
	return r.wire[s:r.at]
}
func (r *tnsFieldsReader) uint(name string, n int, little bool) uint64 {
	b := r.take(name, "uint64", n)
	var v uint64
	if little {
		if r.err == nil {
			r.fields[len(r.fields)-1].Endian = "little"
		}
		for i := len(b) - 1; i >= 0; i-- {
			v = v<<8 | uint64(b[i])
		}
	} else {
		for _, x := range b {
			v = v<<8 | uint64(x)
		}
	}
	return v
}
func (r *tnsFieldsReader) expect(name string, n int, value uint64) {
	if r.uint(name, n, false) != value {
		r.fail("unexpected " + name)
	}
}
func (r *tnsFieldsReader) item() bool {
	if r.err != nil {
		return false
	}
	r.items++
	if r.items > tnsFieldsMaxItems {
		r.fail("item resource limit")
		return false
	}
	return true
}
func (r *tnsFieldsReader) octets(name string, n int) map[string]any {
	s := r.at
	b := r.take(name, "raw", n)
	return map[string]any{"Bytes": bytes.Clone(b), "Byte Range": [2]int{s, r.at}}
}
func (r *tnsFieldsReader) counted(name string) map[string]any {
	n := r.uint(name+" Length", 1, false)
	return r.octets(name, int(n))
}
func (r *tnsFieldsReader) terminated(name string) map[string]any {
	if r.err != nil {
		return nil
	}
	n := bytes.IndexByte(r.wire[r.at:r.end], 0)
	if n < 0 {
		r.fail("missing " + name + " terminator")
		return nil
	}
	m := r.octets(name, n)
	r.expect(name+" Terminator", 1, 0)
	return m
}

// Keep descriptor structure and exact bytes; do not resolve addresses or
// interpret a descriptor as an instruction. This profile accepts the sampled
// unquoted syntax; quoted/escaped dialects require another explicit profile.
func (r *tnsFieldsReader) descriptor(depth int) map[string]any {
	if depth > 32 || !r.item() {
		r.fail("descriptor nesting limit")
		return nil
	}
	s := r.at
	r.expect("Descriptor Open", 1, '(')
	start := r.at
	for r.err == nil && r.at < r.end && r.wire[r.at] != '=' {
		c := r.wire[r.at]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == ' ' || c == '\t') {
			r.fail("unsupported descriptor key")
			break
		}
		r.at++
	}
	n := r.at - start
	r.at = start
	key := strings.TrimSpace(string(r.take("Descriptor Key", "string", n)))
	if key == "" {
		r.fail("empty descriptor key")
	}
	r.expect("Descriptor Equals", 1, '=')
	m := map[string]any{"Key": key}
	if r.err == nil && r.at < r.end && r.wire[r.at] == '(' {
		var children []map[string]any
		for r.err == nil && r.at < r.end && r.wire[r.at] == '(' {
			children = append(children, r.descriptor(depth+1))
		}
		m["Children"] = children
	} else {
		start = r.at
		for r.err == nil && r.at < r.end && r.wire[r.at] != ')' {
			c := r.wire[r.at]
			if c == '(' || c == '"' || c == '\'' || c == '\\' || c < 32 || c > 126 {
				r.fail("unsupported descriptor value")
				break
			}
			r.at++
		}
		n = r.at - start
		r.at = start
		m["Value"] = r.octets("Descriptor Value", n)
	}
	r.expect("Descriptor Close", 1, ')')
	m["Byte Range"] = [2]int{s, r.at}
	return m
}

func (r *tnsFieldsReader) connect(info map[string]any, accept bool) {
	r.expect("Version", 2, 315)
	if !accept {
		info["Compatible Version"] = r.uint("Compatible Version", 2, false)
	}
	info["Service Options"] = r.uint("Service Options", 2, false)
	info["SDU16"] = r.uint("SDU16", 2, false)
	info["TDU16"] = r.uint("TDU16", 2, false)
	if !accept {
		info["Protocol Characteristics"] = r.uint("Protocol Characteristics", 2, false)
		r.uint("Line Turnaround", 2, false)
	}
	info["Value Of One Bytes"] = r.octets("Value Of One Bytes", 2)
	length := r.uint("Connect Data Length", 2, false)
	offset := r.uint("Connect Data Offset", 2, false)
	if !accept {
		info["Maximum Receive Data"] = r.uint("Maximum Receive Data", 4, false)
	}
	info["Connect Flags 0"] = r.uint("Connect Flags 0", 1, false)
	info["Connect Flags 1"] = r.uint("Connect Flags 1", 1, false)
	if accept {
		r.take("Obsolete Accept Fields", "raw", 8)
	} else {
		r.take("Obsolete Connect Fields", "raw", 24)
	}
	info["SDU32"] = r.uint("SDU32", 4, false)
	info["TDU32"] = r.uint("TDU32", 4, false)
	if accept {
		r.take("Accept Extension Byte", "raw", 1)
	} else {
		info["Connect Options"] = r.uint("Connect Options", 4, false)
	}
	if offset != uint64(r.at) || length != uint64(r.end-r.at) {
		r.fail("connect data boundary differs from explicit v315 layout")
		return
	}
	if length > 0 {
		info["Connect Descriptor"] = r.descriptor(0)
	}
}

func (r *tnsFieldsReader) services(info map[string]any) {
	s := r.at
	r.expect("Services Magic", 4, 0xdeadbeef)
	length := r.uint("Services Length", 2, false)
	if length != uint64(r.end-s) {
		r.fail("services length boundary")
	}
	info["Services Version"] = r.uint("Services Version", 4, false)
	n := r.uint("Service Count", 2, false)
	info["Services Flags"] = r.uint("Services Flags", 1, false)
	var services []map[string]any
	for i := uint64(0); i < n && r.item(); i++ {
		start := r.at
		kind := r.uint("Service Type", 2, false)
		count := r.uint("Subpacket Count", 2, false)
		m := map[string]any{"Type": kind, "Error": r.uint("Service Error", 4, false)}
		var parts []map[string]any
		for j := uint64(0); j < count && r.item(); j++ {
			ps := r.at
			length := r.uint("Subpacket Length", 2, false)
			typ := r.uint("Subpacket Type", 2, false)
			p := map[string]any{"Type": typ}
			if length > uint64(r.end-r.at) {
				r.fail("subpacket boundary")
				break
			}
			end := r.at + int(length)
			switch typ {
			case 2, 3, 5, 6:
				width := map[uint64]uint64{2: 1, 3: 2, 5: 4, 6: 2}[typ]
				if length != width {
					r.fail("numeric subpacket width")
					break
				}
				p["Value"] = r.uint("Subpacket Value", int(width), false)
			case 1:
				if kind == 4 && j == 2 {
					outer := r.end
					r.end = end
					r.expect("Services Array Magic", 4, 0xdeadbeef)
					r.expect("Services Array Element Type", 2, 3)
					entries := r.uint("Services Array Count", 4, false)
					if entries > uint64((r.end-r.at)/2) {
						r.fail("services array boundary")
					}
					var values []uint64
					for k := uint64(0); k < entries && r.item(); k++ {
						values = append(values, r.uint("Services Array Value", 2, false))
					}
					p["Values"] = values
					r.end = outer
				} else {
					p["Data"] = r.octets("Subpacket Data", int(length))
				}
			default:
				p["Data"] = r.octets("Subpacket Data", int(length))
			}
			if r.at != end {
				r.fail("unconsumed subpacket")
			}
			p["Byte Range"] = [2]int{ps, r.at}
			parts = append(parts, p)
		}
		m["Subpackets"], m["Byte Range"] = parts, [2]int{start, r.at}
		services = append(services, m)
	}
	info["Services"] = services
}

func (r *tnsFieldsReader) protocol(info map[string]any, response bool) {
	r.expect("Message Code", 1, 1)
	var versions []uint64
	for r.err == nil {
		if len(versions) >= 255 {
			r.fail("version list limit")
			break
		}
		v := r.uint("Protocol Version", 1, false)
		if v == 0 {
			break
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 || response && len(versions) != 1 {
		r.fail("version list shape")
	}
	info["Protocol Versions"] = versions
	info["Platform"] = r.terminated("Platform")
	if !response {
		return
	}
	info["Character Set"] = r.uint("Character Set", 2, true)
	info["Server Flags"] = r.uint("Server Flags", 1, false)
	count := r.uint("Character Conversion Count", 2, true)
	var conversions []map[string]any
	for i := uint64(0); i < count && r.item(); i++ {
		conversions = append(conversions, map[string]any{"First Character Set": r.uint("First Conversion Character Set", 2, true), "Second Character Set": r.uint("Second Conversion Character Set", 2, true), "Flags": r.uint("Conversion Flags", 1, false)})
	}
	info["Character Conversions"] = conversions
	n := r.uint("Representation Descriptor Length", 2, false)
	start := r.at
	fdo := r.take("Representation Descriptor", "raw", int(n))
	info["Representation Descriptor"] = bytes.Clone(fdo)
	info["Representation Descriptor Fully Decoded"] = false
	// The descriptor is a vendor representation table, not a universal type
	// list. Only the national-character-set slot has a documented reader here.
	if len(fdo) < 7 {
		r.fail("short representation descriptor")
		return
	}
	ix := 6 + int(fdo[5]) + int(fdo[6]) + 3
	if ix+2 > len(fdo) {
		r.fail("national character set outside descriptor")
		return
	}
	info["National Character Set"] = uint64(fdo[ix])<<8 | uint64(fdo[ix+1])
	info["National Character Set Byte Range"] = [2]int{start + ix, start + ix + 2}
	info["Compile Capabilities"] = r.counted("Compile Capabilities")
	info["Runtime Capabilities"] = r.counted("Runtime Capabilities")
}

func (r *tnsFieldsReader) timezone(info map[string]any) {
	r.expect("Time Zone Prefix", 4, 0x80000000)
	h, m, s := r.uint("Time Zone Hour Bias", 1, false), r.uint("Time Zone Minute Bias", 1, false), r.uint("Time Zone Second Bias", 1, false)
	if h > 84 || h < 36 || m > 119 || m < 1 || s > 119 || s < 1 {
		r.fail("time zone component range")
	}
	r.expect("Time Zone Suffix", 4, 0x80000000)
	info["Time Zone Offset Seconds"] = (int64(h)-60)*3600 + (int64(m)-60)*60 + int64(s) - 60
	info["Time Zone Data Version"] = r.uint("Time Zone Data Version", 4, false)
}

func (r *tnsFieldsReader) types(info map[string]any, response bool) {
	r.expect("Message Code", 1, 2)
	if !response {
		info["Input Character Set"] = r.uint("Input Character Set", 2, true)
		info["Output Character Set"] = r.uint("Output Character Set", 2, true)
		info["Encoding Flags"] = r.uint("Encoding Flags", 1, false)
		cc := r.counted("Compile Capabilities")
		rc := r.counted("Runtime Capabilities")
		info["Compile Capabilities"], info["Runtime Capabilities"] = cc, rc
		c, runtime := cc["Bytes"].([]byte), rc["Bytes"].([]byte)
		if len(c) <= 37 || len(runtime) < 2 || c[37]&2 == 0 || runtime[1]&1 == 0 {
			r.fail("profile requires time zone and version capabilities")
		}
	}
	r.timezone(info)
	if !response {
		info["National Character Set"] = r.uint("National Character Set", 2, true)
	}
	info["Type Representation Context"] = "caller-selected native layout without conversion table"
}

// Short CLR values use an independent on-wire length. The outer length is a
// conversion capacity, and need not equal the CLR length (the sample uses 3x).
// Long/chunked CLR and other native ABIs are intentionally separate profiles.
func (r *tnsFieldsReader) clr(name string, capacity uint64) map[string]any {
	n := r.uint(name+" CLR Length", 1, false)
	if n >= 254 || n > capacity {
		r.fail("unsupported CLR length or conversion capacity")
	}
	return r.octets(name, int(n))
}
func (r *tnsFieldsReader) parameters(info map[string]any) {
	r.expect("Message Code", 1, 3)
	r.expect("Function Code", 1, 118)
	info["Sequence"] = r.uint("Sequence", 1, false)
	user := r.uint("User Pointer", 8, true)
	capacity := r.uint("User Conversion Capacity", 4, true)
	info["Mode"] = r.uint("Mode", 4, true)
	input := r.uint("Input Parameters Pointer", 8, true)
	count := r.uint("Parameter Count", 4, true)
	r.take("Native Alignment", "raw", 4)
	r.uint("Output Parameters Pointer", 8, true)
	r.uint("Output Count Pointer", 8, true)
	if user == 0 && capacity != 0 || input == 0 && count != 0 {
		r.fail("absent pointer with nonzero count")
	}
	if user != 0 {
		info["User"] = r.clr("User", capacity)
	}
	var pairs []map[string]any
	for i := uint64(0); i < count && r.item(); i++ {
		start := r.at
		keyCapacity := r.uint("Key Conversion Capacity", 4, true)
		key := r.clr("Parameter Key", keyCapacity)
		valueCapacity := r.uint("Value Conversion Capacity", 4, true)
		value := r.clr("Parameter Value", valueCapacity)
		flags := r.uint("Parameter Flags", 4, true)
		pairs = append(pairs, map[string]any{"Key": key, "Value": value, "Flags": flags, "Key Conversion Capacity": keyCapacity, "Value Conversion Capacity": valueCapacity, "Byte Range": [2]int{start, r.at}})
	}
	info["Parameters"] = pairs
}

func decodeTNSFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	r := &tnsFieldsReader{wire: wire, end: len(wire)}
	if len(wire) < 8 || len(wire) > tnsFieldsMaxBytes {
		return nil, nil, fmt.Errorf("tns-fields: packet boundary or resource limit")
	}
	kind, width := uint64(6), 4
	switch profile {
	case "connect315":
		kind, width = 1, 2
	case "accept315":
		kind, width = 2, 2
	case "resend16":
		kind, width = 11, 2
	case "services32", "protocol-request32", "protocol-response32", "types-request-native32", "types-response-native32", "parameters-native-le64-32":
	default:
		return nil, nil, fmt.Errorf("tns-fields: unsupported explicit profile %q", profile)
	}
	length := r.uint("Packet Length", width, false)
	if width == 2 {
		r.uint("Packet Checksum", 2, false)
	}
	r.expect("Packet Type", 1, kind)
	flags := r.uint("Packet Flags", 1, false)
	r.uint("Header Checksum", 2, false)
	if length != uint64(len(wire)) {
		r.fail("packet length differs from supplied boundary")
	}
	info := map[string]any{"Layout Context": profile, "Context Is Caller Supplied": true, "Session State Validated": false, "TCP Reassembly Performed": false, "Connection Opened": false, "Checksums Verified": false, "Packet Flags": flags, "Byte Ranges Are Packet Relative": true}
	if kind == 6 {
		r.expect("Data Flags", 2, 0)
	}
	if r.err == nil {
		switch profile {
		case "connect315", "accept315":
			r.connect(info, kind == 2)
		case "services32":
			r.services(info)
		case "protocol-request32", "protocol-response32":
			r.protocol(info, profile == "protocol-response32")
		case "types-request-native32", "types-response-native32":
			r.types(info, profile == "types-response-native32")
		case "parameters-native-le64-32":
			r.parameters(info)
		}
	}
	if r.at != r.end {
		r.fail("unconsumed packet bytes")
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	return r.fields, info, nil
}

func parseTNSFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	return parseCertificateFieldTree(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeTNSFields(wire, profile)
	}, "tns-fields")
}
