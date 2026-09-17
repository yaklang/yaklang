package stream_parser

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func http3ReviewHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func http3ReviewSpan(t *testing.T, f http3Field) {
	t.Helper()
	if f.End < f.Start {
		t.Fatalf("%s: reversed span", f.Name)
	}
	if len(f.Children) == 0 {
		return
	}
	at := f.Start
	for _, child := range f.Children {
		if child.Start != at || child.End > f.End {
			t.Fatalf("%s/%s: child [%d,%d) after %d, parent [%d,%d)", f.Name, child.Name, child.Start, child.End, at, f.Start, f.End)
		}
		http3ReviewSpan(t, child)
		at = child.End
	}
	if at != f.End {
		t.Fatalf("%s: children stop at %d, want %d", f.Name, at, f.End)
	}
}

func http3ReviewSection(t *testing.T, wire []byte, table *http3QPACKTable, required, base uint64, want [][2]string) []map[string]any {
	t.Helper()
	f, headers, err := http3QPACKSection(wire, 0, len(wire), table)
	if err != nil {
		t.Fatal(err)
	}
	http3ReviewSpan(t, f)
	if f.Info["Required Insert Count"] != required || f.Info["Base"] != base {
		t.Fatalf("section prefix: %#v", f.Info)
	}
	got := make([][2]string, len(headers))
	for i, h := range headers {
		got[i] = [2]string{h["Name"].(string), h["Value"].(string)}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
	return headers
}

// Independent fixed wire/expected values from RFC 9204 Appendix B.1-B.5.
// Decoder acknowledgments here are syntax only; the table replay does not
// claim to validate encoder outstanding-reference/acknowledgment lifecycles.
// https://www.rfc-editor.org/rfc/rfc9204.html#appendix-B
func TestHTTP3QPACKReviewRFC9204AppendixB(t *testing.T) {
	http3ReviewSection(t, http3ReviewHex(t, "0000510b2f696e6465782e68746d6c"), &http3QPACKTable{}, 0, 0, [][2]string{{":path", "/index.html"}})
	b2 := "3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468"
	b3 := "4a637573746f6d2d6b65790c637573746f6d2d76616c7565"
	b4 := "02"
	b5 := "810d637573746f6d2d76616c756532"
	var b2Table, b4Table, b5Table *http3QPACKTable
	for _, tc := range []struct {
		name, instructions string
		count, size, first uint64
		out                **http3QPACKTable
	}{
		{"B2", b2, 2, 106, 0, &b2Table},
		{"B3", b2 + b3, 3, 160, 0, nil},
		{"B4", b2 + b3 + b4, 4, 217, 0, &b4Table},
		{"B5", b2 + b3 + b4 + b5, 5, 215, 1, &b5Table},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := http3ReviewHex(t, tc.instructions)
			table, fields, err := http3QPACKEncoder(wire, 0, 220)
			if err != nil {
				t.Fatal(err)
			}
			http3ReviewSpan(t, http3Group("Encoder", 0, len(wire)*8, fields...))
			if table.insertCount != tc.count || table.size != tc.size || table.capacity != 220 || table.entries[0].absolute != tc.first {
				t.Fatalf("table = %#v", table)
			}
			if tc.out != nil {
				*tc.out = table
			}
		})
	}
	http3ReviewSection(t, http3ReviewHex(t, "03811011"), b2Table, 2, 0, [][2]string{{":authority", "www.example.com"}, {":path", "/sample/path"}})
	http3ReviewSection(t, http3ReviewHex(t, "050080c181"), b4Table, 4, 4, [][2]string{{":authority", "www.example.com"}, {":path", "/"}, {"custom-key", "custom-value"}})
	if _, err := b5Table.absolute(0); err == nil {
		t.Fatal("B5 oldest entry was not evicted")
	}
	if e, err := b5Table.absolute(4); err != nil || e.name != "custom-key" || e.value != "custom-value2" {
		t.Fatalf("B5 new entry = %#v, %v", e, err)
	}
	for _, tc := range []struct {
		hex, name string
		value     uint64
	}{{"84", "Section Acknowledgment", 4}, {"01", "Insert Count Increment", 1}, {"48", "Stream Cancellation", 8}} {
		fields, err := http3QPACKDecoder(http3ReviewHex(t, tc.hex), 0)
		if err != nil || len(fields) != 1 || fields[0].Info["Instruction"] != tc.name || fields[0].Info["Value"] != tc.value {
			t.Fatalf("decoder %s = %#v, %v", tc.hex, fields, err)
		}
		http3ReviewSpan(t, fields[0])
	}
}

func TestHTTP3QPACKReviewRepresentationsWrapAndEviction(t *testing.T) {
	// Capacity=100; a:x at absolute 0 and b:y at absolute 1, each 34 bytes.
	table, _, err := http3QPACKEncoder(http3ReviewHex(t, "3f454161017841620179"), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Required=2, Base=1: indexed relative, literal relative name, literal
	// literal name, indexed post-base, literal post-base name, in wire order.
	wire := http3ReviewHex(t, "0380806001753163017610080177")
	headers := http3ReviewSection(t, wire, table, 2, 1, [][2]string{{"a", "x"}, {"a", "u"}, {"c", "v"}, {"b", "y"}, {"b", "w"}})
	for i, want := range []bool{false, true, true, false, true} {
		if headers[i]["Never Index"] != want {
			t.Fatalf("field %d Never Index = %v", i, headers[i]["Never Index"])
		}
	}
	for _, tc := range []struct {
		wire string
		r, b uint64
		want [][2]string
	}{
		{"038010", 2, 1, [][2]string{{"b", "y"}}},
		{"030080", 2, 2, [][2]string{{"b", "y"}}},
		{"030181", 2, 3, [][2]string{{"b", "y"}}},
		{"0005c1", 0, 5, [][2]string{{":path", "/"}}},
	} {
		http3ReviewSection(t, http3ReviewHex(t, tc.wire), table, tc.r, tc.b, tc.want)
	}
	for _, malformed := range []string{"", "03", "0300", "030180", "0382", "038180", "000080", "040080", "0700", "0100", "0000ff24", "00005102ff"} {
		b := http3ReviewHex(t, malformed)
		if _, _, err := http3QPACKSection(b, 0, len(b), table); err == nil {
			t.Fatalf("accepted malformed field section %q", malformed)
		}
	}
	if _, _, err := http3QPACKSection(wire, 0, len(wire), &http3QPACKTable{max: 100}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("missing snapshot should be blocked, got %v", err)
	}
	// RFC 9204 4.5.1.1's example: maximum=100, insert count=10,
	// EncodedInsertCount=4 decodes RequiredInsertCount=9, not 3 or 15.
	encoder := http3ReviewHex(t, "3f45")
	for i := byte(0); i < 10; i++ {
		encoder = append(encoder, 0x41, 'a', 1, '0'+i)
	}
	wrapped, _, err := http3QPACKEncoder(encoder, 0, 100)
	if err != nil || wrapped.insertCount != 10 || len(wrapped.entries) != 2 || wrapped.entries[0].absolute != 8 {
		t.Fatalf("wrapped table = %#v, %v", wrapped, err)
	}
	for _, tc := range []struct {
		wire string
		r, b uint64
		v    string
	}{{"040080", 9, 9, "8"}, {"040181", 9, 10, "8"}, {"048818", 9, 0, "8"}, {"050080", 10, 10, "9"}} {
		http3ReviewSection(t, http3ReviewHex(t, tc.wire), wrapped, tc.r, tc.b, [][2]string{{"a", tc.v}})
	}
	for _, malformed := range []string{"030080", "060080", "010080"} {
		b := http3ReviewHex(t, malformed)
		if _, _, err := http3QPACKSection(b, 0, len(b), wrapped); err == nil {
			t.Fatalf("accepted evicted/blocked reference %s", malformed)
		}
	}
	cleared, _, err := http3QPACKEncoder(append(append([]byte(nil), encoder...), 0x20), 0, 100)
	if err != nil || cleared.size != 0 || len(cleared.entries) != 0 || cleared.insertCount != 10 {
		t.Fatalf("capacity zero cleared insert count incorrectly: %#v, %v", cleared, err)
	}
	for _, suffix := range [][]byte{{0}, {0x80, 0}} {
		wire := append(append([]byte(nil), encoder...), 0x20)
		if _, _, err := http3QPACKEncoder(append(wire, suffix...), 0, 100); err == nil {
			t.Fatal("accepted reference after complete eviction")
		}
	}
	// RFC9204 3.2.3 prohibits even a capacity-zero instruction at max=0;
	// this is different from a zero current capacity with a positive maximum.
	if _, _, err := http3QPACKEncoder([]byte{0x20}, 0, 0); err == nil {
		t.Fatal("accepted encoder instruction at zero maximum")
	}
	if _, _, err := http3QPACKEncoder([]byte{0x20}, 0, 100); err != nil {
		t.Fatal(err)
	}
}

// A small independent serializer for mathematical integer boundary tests;
// fixed RFC hex vectors above are not produced by this helper.
func http3ReviewInteger(v uint64, prefix uint) []byte {
	mask := uint64(1<<prefix - 1)
	if v < mask {
		return []byte{byte(v)}
	}
	b := []byte{byte(mask)}
	for v -= mask; v >= 128; v >>= 7 {
		b = append(b, byte(v&127)|128)
	}
	return append(b, byte(v))
}

func TestHTTP3QPACKReviewIntegersHuffmanAndHTTPValues(t *testing.T) {
	for prefix := uint(1); prefix <= 8; prefix++ {
		mask := uint64(1<<prefix - 1)
		for _, want := range []uint64{0, mask - 1, mask, mask + 1, 127, 128, 16383, 1<<32 - 1, 1<<62 - 1} {
			wire := http3ReviewInteger(want, prefix)
			c := http3Cursor{wire: wire, end: len(wire)}
			got, f, err := c.prefixed("Integer", prefix)
			if err != nil || got != want || c.at != len(wire) {
				t.Fatalf("prefix=%d value=%d got=%d consumed=%d: %v", prefix, want, got, c.at, err)
			}
			http3ReviewSpan(t, f)
			for n := 0; n < len(wire); n++ {
				c := http3Cursor{wire: wire[:n], end: n}
				if _, _, err := c.prefixed("Integer", prefix); err == nil {
					t.Fatalf("accepted integer prefix=%d short=%d/%d", prefix, n, len(wire))
				}
			}
		}
		tooBig := http3ReviewInteger(1<<62, prefix)
		c := http3Cursor{wire: tooBig, end: len(tooBig)}
		if _, _, err := c.prefixed("Integer", prefix); err == nil {
			t.Fatalf("prefix %d accepted 63-bit integer", prefix)
		}
	}
	// RFC7541 C.4.1's Huffman value, shared unmodified by RFC9204 4.1.2.
	const encoded = "f1e3c2e5f23a6ba0ab90f4ff"
	http3ReviewSection(t, http3ReviewHex(t, "0000508c"+encoded), &http3QPACKTable{}, 0, 0, [][2]string{{":authority", "www.example.com"}})
	// Uncompressed entry size is 57, not 54 encoded bytes. Duplicate at
	// capacity 57 evicts its own source but must preserve the copied value.
	table, _, err := http3QPACKEncoder(http3ReviewHex(t, "3f1ac08c"+encoded+"00"), 0, 57)
	if err != nil || table.size != 57 || len(table.entries) != 1 || table.entries[0].absolute != 1 || table.entries[0].value != "www.example.com" {
		t.Fatalf("Huffman duplicate/self eviction: %#v, %v", table, err)
	}
	if _, _, err := http3QPACKEncoder(http3ReviewHex(t, "3f19c08c"+encoded), 0, 56); err == nil {
		t.Fatal("Huffman size used encoded rather than decoded bytes")
	}
	for _, malformed := range []string{"00005084ffffffff", "00005081ff", "0000508100", "0000508c" + encoded[:22]} {
		b := http3ReviewHex(t, malformed)
		if _, _, err := http3QPACKSection(b, 0, len(b), &http3QPACKTable{}); err == nil {
			t.Fatalf("accepted invalid Huffman/padding/truncated string %s", malformed)
		}
	}
	for b := 0; b < 256; b++ {
		// Inside a generic field, HTAB, SP, visible ASCII and obs-text are
		// permitted; control octets and DEL are not (RFC9110 5.5).
		h := []map[string]any{{"Name": "x-review", "Value": "a" + string([]byte{byte(b)}) + "z"}}
		_, err := http3RequestFields(h, true)
		valid := b == 9 || b >= 32 && b != 127
		if (err == nil) != valid {
			t.Fatalf("field-value octet 0x%02x: valid=%v err=%v", b, valid, err)
		}
	}
	request := func(authority string) []map[string]any {
		return []map[string]any{{"Name": ":method", "Value": "GET"}, {"Name": ":scheme", "Value": "https"}, {"Name": ":authority", "Value": authority}, {"Name": ":path", "Value": "/"}}
	}
	for _, authority := range []string{"example.com/x", "example.com?x", "example.com#x", "user@example.com", "example.com\\x"} {
		if _, err := http3RequestFields(request(authority), false); err == nil {
			t.Fatalf("authority component accepted %q", authority)
		}
	}
	for _, te := range []string{"trailers", "Trailers", "TRAILERS"} {
		h := append(request("example.com"), map[string]any{"Name": "te", "Value": te})
		if _, err := http3RequestFields(h, false); err != nil {
			t.Fatal(fmt.Errorf("case-insensitive TE %s: %w", te, err))
		}
	}
}
