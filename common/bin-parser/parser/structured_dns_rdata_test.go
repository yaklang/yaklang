package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// These L4 controls are deliberately not labelled captured traffic. They cover
// complete DNS messages, all RR sections, and exact RDATA windows. The separate
// Ethernet test uses unchanged upstream bytes for the AAAA branch.
func dnsRDataControl(section int, typ uint16, rdata []byte) []byte {
	w := make([]byte, 12)
	binary.BigEndian.PutUint16(w[2:], 0x8180)
	binary.BigEndian.PutUint16(w[4+2*section:], 1)
	w = append(w, 0) // root owner name
	w = binary.BigEndian.AppendUint16(w, typ)
	w = binary.BigEndian.AppendUint16(w, 1)
	w = binary.BigEndian.AppendUint32(w, 60)
	w = binary.BigEndian.AppendUint16(w, uint16(len(rdata)))
	return append(w, rdata...)
}

func TestCaptureDNSRDataParity(t *testing.T) {
	require.True(t, captureStructuredRules()["dns"], "regenerate rules and audit projection before changing the fingerprint")
	address := []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	cases := []struct {
		name  string
		typ   uint16
		data  []byte
		child string
		want  any
	}{
		{"aaaa", 28, address, "DNSAAAA", map[string]any{"Address": bytes.Clone(address)}},
		{"txt-empty-string", 16, []byte{0}, "DNSTXT", map[string]any{"Strings": []any{map[string]any{"Length": uint8(0)}}}},
		{"txt-multiple", 16, []byte{3, 'o', 'n', 'e', 0, 3, 't', 'w', 'o'}, "DNSTXT", map[string]any{"Strings": []any{map[string]any{"Length": uint8(3), "Text": "one"}, map[string]any{"Length": uint8(0)}, map[string]any{"Length": uint8(3), "Text": "two"}}}},
		{"txt-max-string", 16, append([]byte{255}, bytes.Repeat([]byte{'x'}, 255)...), "DNSTXT", map[string]any{"Strings": []any{map[string]any{"Length": uint8(255), "Text": string(bytes.Repeat([]byte{'x'}, 255))}}}},
		{"caa-issue", 257, append([]byte{0, 5}, []byte("issueexample.net")...), "DNSCAA", map[string]any{"Flags": uint8(0), "TagLength": uint8(5), "Tag": "issue", "Value": []byte("example.net")}},
		{"caa-unknown-critical-binary", 257, []byte{0x81, 3, 'X', '9', 'a', 0, 0xff}, "DNSCAA", map[string]any{"Flags": uint8(0x81), "TagLength": uint8(3), "Tag": "X9a", "Value": []byte{0, 0xff}}},
		{"caa-empty-value", 257, []byte{0, 1, 'x'}, "DNSCAA", map[string]any{"Flags": uint8(0), "TagLength": uint8(1), "Tag": "x"}},
		{"unknown-opaque", 65000, []byte{1, 2}, "RData", []byte{1, 2}},
	}
	for section := 1; section <= 3; section++ {
		sectionName := []string{"", "Answers", "Authority", "Additional"}[section]
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", sectionName, tc.name), func(t *testing.T) {
				w := dnsRDataControl(section, tc.typ, tc.data)
				legacy, err := captureLegacy(t, w, "application-layer.dns", nil, "DNS")
				require.NoError(t, err)
				row := legacy["fields"].(map[string]any)[sectionName].([]any)[0].(map[string]any)
				require.Equal(t, tc.want, row[tc.child])
				got, ok := captureDNSStructured(w)
				require.True(t, ok)
				require.Equal(t, legacy, got)
				actual, err := ParseStructured(w, "application-layer.dns", "DNS")
				require.NoError(t, err)
				require.Equal(t, legacy, actual)
				for n := range w {
					_, ok := captureDNSStructured(w[:n])
					require.False(t, ok, "prefix %d", n)
				}
				for i := range w {
					w[i] = 0xff
				}
				require.Equal(t, legacy, got, "adapter borrows input")
				require.Equal(t, legacy, actual, "public output borrows input")
			})
		}
	}
}

func TestCaptureDNSRDataRejectsInvalidWindows(t *testing.T) {
	cases := []struct {
		name string
		typ  uint16
		data []byte
	}{
		{"aaaa-empty", 28, nil}, {"aaaa-short", 28, make([]byte, 15)}, {"aaaa-long", 28, make([]byte, 17)},
		{"txt-empty-rdata", 16, nil}, {"txt-short-string", 16, []byte{2, 'a'}}, {"txt-second-string-truncated", 16, []byte{0, 2, 'a'}},
		{"caa-empty", 257, nil}, {"caa-short", 257, []byte{0, 1}}, {"caa-zero-tag", 257, []byte{0, 0, 'x'}},
		{"caa-tag-overrun", 257, []byte{0, 2, 'x'}}, {"caa-tag-punctuation", 257, []byte{0, 1, '-'}}, {"caa-tag-non-ascii", 257, []byte{0, 1, 0xff}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := dnsRDataControl(1, tc.typ, tc.data)
			_, ok := captureDNSStructured(w)
			require.False(t, ok)
			_, err := captureLegacy(t, w, "application-layer.dns", nil, "DNS")
			require.Error(t, err)
			_, err = ParseStructured(w, "application-layer.dns", "DNS")
			require.Error(t, err)
		})
	}
	// A malformed TXT/CAA record may not borrow bytes from a following RR.
	for _, typ := range []uint16{16, 257} {
		data := []byte{2, 'a'}
		if typ == 257 {
			data = []byte{0, 2, 'x'}
		}
		w := dnsRDataControl(1, typ, data)
		binary.BigEndian.PutUint16(w[6:], 2)
		w = append(w, dnsRDataControl(1, 1, []byte{127, 0, 0, 1})[12:]...)
		_, ok := captureDNSStructured(w)
		require.False(t, ok)
		_, err := captureLegacy(t, w, "application-layer.dns", nil, "DNS")
		require.Error(t, err)
	}
}

func TestDNSRDataOriginalNodeCardinality(t *testing.T) {
	w := dnsRDataControl(1, 16, []byte{3, 'o', 'n', 'e', 0, 3, 't', 'w', 'o'})
	node, err := ParseBinary(bytes.NewReader(w), "application-layer.dns", "DNS")
	require.NoError(t, err)
	answers := base.GetNodeByPath(node, "Answers")
	require.NotNil(t, answers)
	require.Len(t, answers.Children, 1)
	strings := base.GetNodeByPath(answers.Children[0], "DNSTXT.Strings")
	require.NotNil(t, strings)
	require.Len(t, strings.Children, 3, "a terminal probe must not append an unparsed fourth child")
	for i, span := range [][2]uint64{{23 * 8, 27 * 8}, {27 * 8, 28 * 8}, {28 * 8, 32 * 8}} {
		length := base.GetNodeByPath(strings.Children[i], "Length")
		require.NotNil(t, length)
		require.Equal(t, [2]uint64{span[0], span[0] + 8}, length.Cfg.GetItem(stream_parser.CfgNodeResult))
		if span[1] > span[0]+8 {
			text := base.GetNodeByPath(strings.Children[i], "Text")
			require.NotNil(t, text)
			require.Equal(t, [2]uint64{span[0] + 8, span[1]}, text.Cfg.GetItem(stream_parser.CfgNodeResult))
		}
	}
}

func TestDNSRDataTXTCollectionBoundary(t *testing.T) {
	for _, count := range []int{4096, 4097} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			w := dnsRDataControl(1, 16, make([]byte, count))
			// A following RR must not be swallowed or change the TXT bound.
			binary.BigEndian.PutUint16(w[6:], 2)
			w = append(w, dnsRDataControl(1, 1, []byte{127, 0, 0, 1})[12:]...)
			got, ok := captureDNSStructured(w)
			legacy, err := captureLegacy(t, w, "application-layer.dns", nil, "DNS")
			if count > 4096 {
				require.False(t, ok)
				require.Error(t, err)
				return
			}
			require.True(t, ok)
			require.NoError(t, err)
			require.Equal(t, legacy, got)
			rows := got["fields"].(map[string]any)["Answers"].([]any)
			require.Len(t, rows, 2)
			require.Len(t, rows[0].(map[string]any)["DNSTXT"].(map[string]any)["Strings"], count)
			require.Equal(t, map[string]any{"Address": []byte{127, 0, 0, 1}}, rows[1].(map[string]any)["DNSA"])
		})
	}
}
