package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

// The fixture writer models CDR's message-origin alignment independently of
// the decoder. Nonzero gap bytes ensure that gaps are not treated as values or
// incorrectly validated as reserved bytes.
type giopTestCDR struct {
	b     []byte
	order binary.ByteOrder
	marks map[string]int
}

func newGIOPTestCDR(little bool) *giopTestCDR {
	var order binary.ByteOrder = binary.BigEndian
	if little {
		order = binary.LittleEndian
	}
	return &giopTestCDR{order: order, marks: make(map[string]int)}
}

func (w *giopTestCDR) align(n int) {
	for (12+len(w.b))%n != 0 {
		w.b = append(w.b, 0xa5)
	}
}

func (w *giopTestCDR) mark(name string) { w.marks[name] = len(w.b) }
func (w *giopTestCDR) raw(b []byte)     { w.b = append(w.b, b...) }
func (w *giopTestCDR) u8(v byte)        { w.b = append(w.b, v) }

func (w *giopTestCDR) u16(v uint16) {
	w.align(2)
	b := make([]byte, 2)
	w.order.PutUint16(b, v)
	w.raw(b)
}

func (w *giopTestCDR) u32(v uint32) {
	w.align(4)
	b := make([]byte, 4)
	w.order.PutUint32(b, v)
	w.raw(b)
}

func (w *giopTestCDR) seq(b []byte) { w.u32(uint32(len(b))); w.raw(b) }
func (w *giopTestCDR) str(s string) { w.seq(append([]byte(s), 0)) }

func (w *giopTestCDR) contexts(count int) {
	w.align(4)
	w.mark("Context Count")
	w.u32(uint32(count))
	for i := 0; i < count; i++ {
		w.u32(uint32(10 + i))
		w.seq(bytes.Repeat([]byte{byte(0x30 + i)}, i%5+1))
	}
}

func (w *giopTestCDR) profile(id uint32, data []byte) {
	w.u32(id)
	w.seq(data)
}

func (w *giopTestCDR) ior(count int) {
	w.str("IDL:Example:1.0")
	w.align(4)
	w.mark("Profile Count")
	w.u32(uint32(count))
	for i := 0; i < count; i++ {
		w.profile(uint32(i), []byte{byte(i), 0xff, 0x22})
	}
}

func (w *giopTestCDR) target(disc uint16, key []byte) {
	w.align(2)
	w.mark("Disc")
	w.u16(disc)
	switch disc {
	case 0:
		w.seq(key)
	case 1:
		w.profile(3, []byte{0xff, 0, 7, 8, 9})
	case 2:
		w.align(4)
		w.mark("Selected Index")
		w.u32(1)
		w.ior(2)
	}
}

func (w *giopTestCDR) systemException() {
	w.str("IDL:omg.org/CORBA/UNKNOWN:1.0")
	w.u32(0x10203040)
	w.mark("Completion Status")
	w.u32(2)
}

func giopTestRequest(minor byte, little bool, target uint16, contexts int, key, stub []byte) *giopTestCDR {
	w := newGIOPTestCDR(little)
	if minor < 2 {
		w.contexts(contexts)
	}
	w.u32(0x10203040)
	w.mark("Response Flags")
	if minor < 2 {
		w.u8(1)
	} else {
		w.u8(3)
	}
	if minor == 0 {
		w.align(4)
	} else {
		w.mark("Reserved")
		w.raw([]byte{0, 0, 0})
	}
	if minor < 2 {
		w.seq(key)
	} else {
		w.target(target, key)
	}
	w.align(4)
	w.mark("Operation Length")
	w.str("echo")
	w.mark("Operation End")
	if minor < 2 {
		w.seq([]byte{1, 0, 0xff})
	} else {
		w.contexts(contexts)
	}
	w.mark("Header End")
	if len(stub) > 0 && minor >= 2 {
		w.align(8)
	}
	w.mark("Stub")
	w.raw(stub)
	return w
}

func giopTestReply(minor byte, little bool, status uint32) *giopTestCDR {
	w := newGIOPTestCDR(little)
	if minor < 2 {
		w.contexts(1)
	}
	w.u32(0x10203040)
	w.u32(status)
	if minor >= 2 {
		w.contexts(1)
		w.align(8)
	}
	w.mark("Body")
	switch status {
	case 0:
		w.raw([]byte{0xde, 0xad, 0xbe, 0xef})
	case 1:
		w.str("IDL:Example/Failure:1.0")
		w.raw([]byte{0x10, 0x20, 0x30})
	case 2:
		w.systemException()
	case 3, 4:
		w.ior(2)
	case 5:
		w.u16(2)
	}
	return w
}

func giopFindField(fields []giopField, name string) *giopField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
		if f := giopFindField(fields[i].Children, name); f != nil {
			return f
		}
	}
	return nil
}

func giopAssertField(t *testing.T, body []byte, fields []giopField, name string, expected []byte) *giopField {
	t.Helper()
	f := giopFindField(fields, name)
	if f == nil {
		t.Fatalf("missing field %q", name)
	}
	if !bytes.Equal(body[f.Start:f.End], expected) {
		t.Fatalf("%s [%d:%d] = %x, want %x", name, f.Start, f.End, body[f.Start:f.End], expected)
	}
	return f
}

func giopAssertSpans(t *testing.T, fields []giopField, start, end int) {
	t.Helper()
	pos := start
	for _, f := range fields {
		if f.Start != pos || f.End < f.Start || f.End > end {
			t.Fatalf("non-contiguous/out-of-bounds %s [%d:%d], cursor %d, end %d", f.Name, f.Start, f.End, pos, end)
		}
		if f.Type == "" {
			giopAssertSpans(t, f.Children, f.Start, f.End)
		} else {
			size := 0
			switch f.Type {
			case "uint8":
				size = 1
			case "uint16":
				size = 2
			case "uint32":
				size = 4
			}
			if size != 0 && (f.End-f.Start != size || (12+f.Start)%size != 0) {
				t.Fatalf("bad scalar alignment/span: %+v", f)
			}
		}
		pos = f.End
	}
	if pos != end {
		t.Fatalf("unrepresented wire bytes [%d:%d]", pos, end)
	}
}

func giopDecodeForTest(t *testing.T, body []byte, minor byte, little bool, messageType byte) []giopField {
	t.Helper()
	flags := byte(0)
	if little {
		flags = 1
	}
	fields, err := decodeGIOPBody(body, 1, minor, flags, messageType)
	if err != nil {
		t.Fatalf("decode 1.%d type %d: %v; body=%x", minor, messageType, err, body)
	}
	giopAssertSpans(t, fields, 0, len(body))
	return fields
}

func TestGIOPBodyRequestVersionsTargetsAndAlignment(t *testing.T) {
	for minor := byte(0); minor <= 3; minor++ {
		for _, little := range []bool{false, true} {
			for disc := uint16(0); disc <= 2; disc++ {
				if minor < 2 && disc != 0 {
					continue
				}
				for _, contextCount := range []int{0, 2} {
					for _, keyLength := range []int{0, 1, 3, 4, 5} {
						for _, stub := range [][]byte{nil, {0xde, 0xad, 0xbe, 0xef, 0x11}} {
							name := fmt.Sprintf("1.%d/LE=%t/target=%d/contexts=%d/key=%d/stub=%d", minor, little, disc, contextCount, keyLength, len(stub))
							t.Run(name, func(t *testing.T) {
								key := bytes.Repeat([]byte{0xff}, keyLength)
								w := giopTestRequest(minor, little, disc, contextCount, key, stub)
								fields := giopDecodeForTest(t, w.b, minor, little, 0)
								giopAssertField(t, w.b, fields, "Operation", []byte{'e', 'c', 'h', 'o', 0})
								if disc == 0 {
									giopAssertField(t, w.b, fields, "Object Key", key)
								}
								f := giopAssertField(t, w.b, fields, "Stub Data", stub)
								if f.Start != w.marks["Stub"] {
									t.Fatalf("stub start %d, want %d", f.Start, w.marks["Stub"])
								}
								contexts := giopFindField(fields, "Service Contexts")
								if contexts == nil || !contexts.List || len(contexts.Children) != contextCount {
									t.Fatalf("context list = %+v", contexts)
								}
								if contextCount == 2 {
									first := contexts.Children[0].Children
									if first[len(first)-1].Name != "Context Padding" || giopFindField(contexts.Children[1].Children, "Context Padding") != nil {
										t.Fatal("inter-entry context padding must belong to the preceding entry")
									}
								}
								if len(stub) == 0 && giopFindField(fields, "Body Padding") != nil {
									t.Fatal("empty body unexpectedly padded")
								}
							})
						}
					}
				}
			}
		}
	}
}

func TestGIOPBodyReplyAndLocateStatusLayouts(t *testing.T) {
	for minor := byte(0); minor <= 3; minor++ {
		for _, little := range []bool{false, true} {
			for status := uint32(0); status <= 5; status++ {
				if minor < 2 && status > 3 {
					continue
				}
				t.Run(fmt.Sprintf("reply/1.%d/LE=%t/status=%d", minor, little, status), func(t *testing.T) {
					w := giopTestReply(minor, little, status)
					fields := giopDecodeForTest(t, w.b, minor, little, 1)
					if status == 2 {
						giopAssertField(t, w.b, fields, "Exception ID", []byte("IDL:omg.org/CORBA/UNKNOWN:1.0\x00"))
					}
					if status == 3 || status == 4 {
						if f := giopFindField(fields, "Profiles"); f == nil || !f.List || len(f.Children) != 2 {
							t.Fatalf("bad IOR profiles: %+v", f)
						}
					}
				})
			}
			for status := uint32(0); status <= 5; status++ {
				if minor < 2 && status > 2 {
					continue
				}
				t.Run(fmt.Sprintf("locate-reply/1.%d/LE=%t/status=%d", minor, little, status), func(t *testing.T) {
					w := newGIOPTestCDR(little)
					w.u32(7)
					w.u32(status)
					switch status {
					case 2, 3:
						w.ior(2)
					case 4:
						w.systemException()
					case 5:
						w.u16(1)
					}
					fields := giopDecodeForTest(t, w.b, minor, little, 4)
					if status == 2 || status == 3 {
						if f := giopFindField(fields, "Type ID Length"); f.Start != 8 {
							t.Fatalf("LocateReply acquired an incorrect eight-octet body gap: %+v", f)
						}
					}
					if giopFindField(fields, "Body Padding") != nil {
						t.Fatal("LocateReply must not apply Request/Reply body alignment")
					}
				})
			}
		}
	}
}

func TestGIOPBodyLocateIndependentWireSpans(t *testing.T) {
	// Request ID 0x01020304, KeyAddr (two-octet discriminant), arbitrary
	// alignment octets, then a three-octet opaque key without a NUL.
	body := []byte{1, 2, 3, 4, 0, 0, 0xde, 0xad, 0, 0, 0, 3, 0xff, 0x80, 1}
	fields := giopDecodeForTest(t, body, 2, false, 3)
	for name, span := range map[string][2]int{"Request ID": {0, 4}, "Addr Disc": {4, 6}, "Addr Pad": {6, 8}, "Key Len": {8, 12}, "Object Key": {12, 15}} {
		f := giopFindField(fields, name)
		if f == nil || f.Start != span[0] || f.End != span[1] {
			t.Fatalf("%s span = %+v, want %v", name, f, span)
		}
	}
	for _, minor := range []byte{0, 1} {
		old := []byte{4, 3, 2, 1, 3, 0, 0, 0, 0xff, 0x80, 1}
		fields := giopDecodeForTest(t, old, minor, true, 3)
		if giopFindField(fields, "Addr Disc") != nil {
			t.Fatal("old LocateRequest has no address discriminant")
		}
		giopAssertField(t, old, fields, "Object Key", old[8:])
	}
}

func TestGIOPBodyControlAndFragmentBoundaries(t *testing.T) {
	for minor := byte(0); minor <= 3; minor++ {
		for _, little := range []bool{false, true} {
			w := newGIOPTestCDR(little)
			w.u32(0x12345678)
			giopDecodeForTest(t, w.b, minor, little, 2)
			for _, typ := range []byte{5, 6} {
				fields := giopDecodeForTest(t, nil, minor, little, typ)
				if len(fields) != 0 {
					t.Fatal("empty control message produced fields")
				}
			}
		}
	}
	for _, tc := range []struct {
		name    string
		minor   byte
		flags   byte
		typ     byte
		body    []byte
		wantID  bool
		wantRaw []byte
	}{
		{"old-initial-partial-header", 1, 2, 0, []byte{0xff}, false, []byte{0xff}},
		{"old-continuation", 1, 0, 7, []byte{7, 8, 9}, false, []byte{7, 8, 9}},
		{"new-initial-only-id", 2, 2, 0, []byte{0, 0, 0, 7}, true, nil},
		{"new-initial-locate", 2, 3, 3, []byte{7, 0, 0, 0}, true, nil},
		{"new-initial-locate-reply", 3, 2, 4, []byte{0, 0, 0, 7}, true, nil},
		{"new-middle", 2, 2, 7, []byte{0, 0, 0, 7, 1, 2, 3, 4, 5, 6, 7, 8}, true, []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{"new-final", 3, 1, 7, []byte{7, 0, 0, 0, 0xff}, true, []byte{0xff}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := decodeGIOPBody(tc.body, 1, tc.minor, tc.flags, tc.typ)
			if err != nil {
				t.Fatal(err)
			}
			giopAssertSpans(t, fields, 0, len(tc.body))
			giopAssertField(t, tc.body, fields, "Fragment Data", tc.wantRaw)
			if (giopFindField(fields, "Request ID") != nil) != tc.wantID {
				t.Fatalf("unexpected fragment request ID: %+v", fields)
			}
			if tc.typ != 7 && fields[0].Name != "Fragmented Message" {
				t.Fatal("partial message presented as a complete envelope")
			}
		})
	}
}

func TestGIOPBodyRejectsInvalidEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name               string
		major, minor, flag byte
		typ                byte
		body               []byte
	}{
		{"major", 2, 2, 0, 2, make([]byte, 4)},
		{"minor", 1, 4, 0, 2, make([]byte, 4)},
		{"old-byte-order-not-bool", 1, 0, 2, 0, make([]byte, 4)},
		{"reserved-flag", 1, 2, 4, 2, make([]byte, 4)},
		{"message-type", 1, 2, 0, 8, make([]byte, 4)},
		{"old-fragment", 1, 0, 0, 7, make([]byte, 4)},
		{"cancel-fragment", 1, 2, 2, 2, make([]byte, 4)},
		{"old-locate-fragment", 1, 1, 2, 3, make([]byte, 4)},
		{"misaligned-fragment", 1, 2, 2, 0, make([]byte, 5)},
		{"short-initial-id", 1, 2, 2, 0, make([]byte, 3)},
		{"short-continuation-id", 1, 2, 0, 7, make([]byte, 3)},
		{"close-tail", 1, 2, 0, 5, []byte{1}},
		{"error-tail", 1, 2, 0, 6, []byte{1}},
		{"cancel-tail", 1, 2, 0, 2, make([]byte, 5)},
		{"max-body", 1, 1, 0, 7, make([]byte, giopMaxBodySize+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := decodeGIOPBody(tc.body, tc.major, tc.minor, tc.flag, tc.typ)
			if err == nil || fields != nil {
				t.Fatalf("invalid message returned fields=%+v, error=%v", fields, err)
			}
		})
	}
	for _, typ := range []byte{0, 1, 3, 4} {
		if _, err := decodeGIOPBody(nil, 1, 2, 0, typ); err == nil {
			t.Fatalf("accepted reserved empty message type %d", typ)
		}
	}
	giopDecodeForTest(t, make([]byte, giopMaxBodySize), 1, false, 7)
}

func TestGIOPBodyRejectsInvalidCDRValues(t *testing.T) {
	for _, little := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			minor  byte
			target uint16
			mutate func(*giopTestCDR)
		}{
			{"response-bool", 0, 0, func(w *giopTestCDR) { w.b[w.marks["Response Flags"]] = 2 }},
			{"response-flags", 2, 0, func(w *giopTestCDR) { w.b[w.marks["Response Flags"]] = 2 }},
			{"old-reserved", 1, 0, func(w *giopTestCDR) { w.b[w.marks["Reserved"]] = 1 }},
			{"target-discriminator", 2, 0, func(w *giopTestCDR) { w.order.PutUint16(w.b[w.marks["Disc"]:], 3) }},
			{"target-index", 2, 2, func(w *giopTestCDR) { w.order.PutUint32(w.b[w.marks["Selected Index"]:], 2) }},
			{"profile-count", 2, 2, func(w *giopTestCDR) { w.order.PutUint32(w.b[w.marks["Profile Count"]:], giopMaxItems+1) }},
			{"context-count", 2, 0, func(w *giopTestCDR) { w.order.PutUint32(w.b[w.marks["Context Count"]:], giopMaxItems+1) }},
			{"context-count-overflow", 2, 0, func(w *giopTestCDR) { w.order.PutUint32(w.b[w.marks["Context Count"]:], ^uint32(0)) }},
			{"string-zero-length", 2, 0, func(w *giopTestCDR) { w.order.PutUint32(w.b[w.marks["Operation Length"]:], 0) }},
			{"string-length-overflow", 2, 0, func(w *giopTestCDR) { w.order.PutUint32(w.b[w.marks["Operation Length"]:], ^uint32(0)) }},
			{"string-not-terminated", 2, 0, func(w *giopTestCDR) { w.b[w.marks["Operation End"]-1] = 'x' }},
		} {
			t.Run(fmt.Sprintf("%s/LE=%t", tc.name, little), func(t *testing.T) {
				w := giopTestRequest(tc.minor, little, tc.target, 1, []byte{0x80}, nil)
				tc.mutate(w)
				flags := byte(0)
				if little {
					flags = 1
				}
				if fields, err := decodeGIOPBody(w.b, 1, tc.minor, flags, 0); err == nil || fields != nil {
					t.Fatalf("malformed CDR returned fields=%+v err=%v", fields, err)
				}
			})
		}
		w := giopTestReply(2, little, 2)
		w.order.PutUint32(w.b[w.marks["Completion Status"]:], 3)
		flags := byte(0)
		if little {
			flags = 1
		}
		if _, err := decodeGIOPBody(w.b, 1, 2, flags, 1); err == nil {
			t.Fatal("accepted invalid completion status")
		}
		for _, mt := range []byte{1, 4} {
			for _, minor := range []byte{0, 1, 2, 3} {
				w := newGIOPTestCDR(little)
				if mt == 1 && minor < 2 {
					w.contexts(0)
				}
				w.u32(7)
				w.u32(6)
				if mt == 1 && minor >= 2 {
					w.contexts(0)
				}
				if _, err := decodeGIOPBody(w.b, 1, minor, flags, mt); err == nil {
					t.Fatalf("accepted unknown status for type %d version 1.%d", mt, minor)
				}
			}
		}
	}
}

func TestGIOPBodyTruncationAndExactTypedConsumption(t *testing.T) {
	for minor := byte(0); minor <= 3; minor++ {
		for _, little := range []bool{false, true} {
			flags := byte(0)
			if little {
				flags = 1
			}
			for disc := uint16(0); disc <= 2; disc++ {
				if minor < 2 && disc != 0 {
					continue
				}
				w := newGIOPTestCDR(little)
				w.u32(42)
				if minor < 2 {
					w.seq([]byte{1, 2, 3})
				} else {
					w.target(disc, []byte{1, 2, 3})
				}
				giopDecodeForTest(t, w.b, minor, little, 3)
				for n := 0; n < len(w.b); n++ {
					if _, err := decodeGIOPBody(w.b[:n], 1, minor, flags, 3); err == nil {
						t.Fatalf("accepted LocateRequest 1.%d target %d prefix %d/%d", minor, disc, n, len(w.b))
					}
				}
				if _, err := decodeGIOPBody(append(w.b, 0), 1, minor, flags, 3); err == nil {
					t.Fatal("accepted LocateRequest trailing byte")
				}
				req := giopTestRequest(minor, little, disc, 2, []byte{1, 2, 3}, nil)
				for n := 0; n < req.marks["Header End"]; n++ {
					if _, err := decodeGIOPBody(req.b[:n], 1, minor, flags, 0); err == nil {
						t.Fatalf("accepted Request 1.%d target %d header prefix %d/%d", minor, disc, n, len(req.b))
					}
				}
			}
			for _, status := range []uint32{2, 3} {
				w := giopTestReply(minor, little, status)
				for n := 0; n < len(w.b); n++ {
					if _, err := decodeGIOPBody(w.b[:n], 1, minor, flags, 1); err == nil {
						t.Fatalf("accepted typed Reply 1.%d status %d prefix %d/%d", minor, status, n, len(w.b))
					}
				}
				if _, err := decodeGIOPBody(append(w.b, 0), 1, minor, flags, 1); err == nil {
					t.Fatal("accepted typed Reply trailing byte")
				}
			}
		}
	}
}

func TestGIOPBodyBoundedListsAndEmptyReply(t *testing.T) {
	for _, little := range []bool{false, true} {
		w := giopTestRequest(2, little, 0, giopMaxItems, nil, nil)
		fields := giopDecodeForTest(t, w.b, 2, little, 0)
		if len(giopFindField(fields, "Service Contexts").Children) != giopMaxItems {
			t.Fatal("maximum valid context count was not preserved")
		}
		w = newGIOPTestCDR(little)
		w.u32(1)
		w.u32(2)
		w.ior(giopMaxItems)
		fields = giopDecodeForTest(t, w.b, 2, little, 4)
		if len(giopFindField(fields, "Profiles").Children) != giopMaxItems {
			t.Fatal("maximum valid profile count was not preserved")
		}
		for _, minor := range []byte{0, 1, 2, 3} {
			w = newGIOPTestCDR(little)
			if minor < 2 {
				w.contexts(0)
			}
			w.u32(1)
			w.u32(0)
			if minor >= 2 {
				w.contexts(1) // Deliberately leaves the header unaligned to 8.
			}
			fields = giopDecodeForTest(t, w.b, minor, little, 1)
			giopAssertField(t, w.b, fields, "Stub Data", nil)
			if giopFindField(fields, "Body Padding") != nil {
				t.Fatal("empty reply must not require trailing body alignment")
			}
		}
	}
}
