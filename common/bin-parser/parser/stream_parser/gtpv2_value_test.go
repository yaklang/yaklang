package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func gtpv2TestIE(typ byte, value []byte, instance byte) []byte {
	w := []byte{typ, byte(len(value) >> 8), byte(len(value)), instance}
	return append(w, value...)
}

func gtpv2TestMessage(typ, flags byte, teid, seq uint32, body []byte) []byte {
	w := []byte{flags, typ, 0, 0}
	if flags&8 != 0 {
		w = binary.BigEndian.AppendUint32(w, teid)
	}
	w = append(w, byte(seq>>16), byte(seq>>8), byte(seq), 0)
	w = append(w, body...)
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)-4))
	return w
}

func TestGTPv2ConditionalHeadersSpareBitsAndInactivePriority(t *testing.T) {
	echo := gtpv2TestMessage(1, 0x47, 0, 0x010203, gtpv2TestIE(3, []byte{7}, 0))
	echo[7] = 0xf5 // Whole last octet is spare in the T0 path-management header.
	fields, err := DecodeGTPv2Wire(echo, 64)
	require.NoError(t, err)
	h := fields["Messages"].([]map[string]any)[0]["Header"].(map[string]any)
	require.NotContains(t, h, "TEID")
	require.NotContains(t, h, "Message Priority")
	require.Equal(t, uint64(0x010203), h["Sequence Number"])
	require.Equal(t, uint64(0xf5), h["Ignored Last Octet Spare"])
	w := gtpv2TestMessage(32, 0x4b, 0, 1, gtpv2TestIE(73, []byte{0xa5}, 0xf2))
	w[11] = 0xa5
	fields, err = DecodeGTPv2Wire(w, 64)
	require.NoError(t, err)
	m := fields["Messages"].([]map[string]any)[0]
	h = m["Header"].(map[string]any)
	require.Equal(t, uint64(0), h["TEID"], "present TEID zero is a literal observation")
	require.NotContains(t, h, "Message Priority")
	ie := m["IEs"].([]map[string]any)[0]
	require.Equal(t, uint64(2), ie["Instance"])
	require.Equal(t, uint64(15), ie["Ignored IE Spare"])
	require.Equal(t, uint64(5), ie["Body"].(map[string]any)["EPS Bearer ID"])
	w[0] |= 4
	w[11] = 0xf0
	fields, err = DecodeGTPv2Wire(w, 64)
	require.NoError(t, err)
	h = fields["Messages"].([]map[string]any)[0]["Header"].(map[string]any)
	require.Equal(t, uint64(15), h["Message Priority"])
	require.NotContains(t, h["Wire Field Ranges"], "Message Priority", "masked bits have no fabricated integer octets")
	for _, bad := range [][]byte{gtpv2TestMessage(1, 0x48, 1, 1, nil), gtpv2TestMessage(32, 0x40, 0, 1, nil)} {
		_, err := DecodeGTPv2Wire(bad, 64)
		require.ErrorIs(t, err, ErrGTPv2WireFormat)
	}
}

func TestGTPv2IEOnlyZeroBudgetPreservesHeadersAndNoEchoPriority(t *testing.T) {
	headerOnly := gtpv2TestMessage(1, 0x40, 0, 1, nil)
	fields, err := DecodeGTPv2Wire(headerOnly, 0)
	require.NoError(t, err)
	require.Len(t, fields["Messages"], 1)
	withIE := gtpv2TestMessage(1, 0x40, 0, 1, gtpv2TestIE(3, []byte{7}, 0))
	fields, err = DecodeGTPv2Wire(withIE, 0)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.Equal(t, uint64(1), fields["Messages"].([]map[string]any)[0]["Header"].(map[string]any)["Sequence Number"])
	require.Empty(t, fields["Messages"].([]map[string]any)[0]["IEs"])
	wrongEcho := gtpv2TestMessage(1, 0x4c, 1, 1, nil)
	wrongEcho[11] = 0xf0
	fields, err = DecodeGTPv2Wire(wrongEcho, 64)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
	require.NotContains(t, fields["Messages"].([]map[string]any)[0]["Header"], "Message Priority", "path-management spare bits never gain ordinary EPC priority semantics")
}

func TestGTPv2UnselectedInterfaceRetainsOnlyCommonHeaderAndOwnedBody(t *testing.T) {
	body := append(gtpv2TestIE(3, []byte{7}, 0), gtpv2TestIE(73, []byte{5}, 0)...)
	for _, typ := range []byte{4, 178, 240, 255} {
		w := gtpv2TestMessage(typ, 0x5c, 1, 2, body)
		w[11] = 0xf0
		tail := gtpv2TestMessage(1, 0x40, 0, 99, nil)
		f, err := DecodeGTPv2Wire(append(bytes.Clone(w), tail...), 0)
		require.ErrorIs(t, err, ErrObservedWireContext)
		require.False(t, errors.Is(err, ErrObservedWireBudget))
		require.False(t, errors.Is(err, ErrGTPv2WireFormat))
		require.False(t, f["Wire Grammar Complete"].(bool))
		rows := f["Messages"].([]map[string]any)
		require.Len(t, rows, 1, "unselected P semantics cannot admit a second message")
		row := rows[0]
		require.NotContains(t, row, "IEs", "unparsed body is not a proven empty IE list")
		require.Equal(t, body, row["Unparsed Body"])
		require.Equal(t, []any{uint64(12), uint64(len(w))}, row["Wire Field Ranges"].(map[string]any)["Unparsed Body"])
		h := row["Header"].(map[string]any)
		require.Equal(t, uint64(1), h["TEID"])
		require.Equal(t, uint64(2), h["Sequence Number"])
		require.Equal(t, uint64(0xf0), h["Priority or Spare Octet"])
		require.NotContains(t, h, "Message Priority")
		require.NotContains(t, h, "Ignored Last Octet Spare")
		w[12] ^= 255
		require.Equal(t, body, row["Unparsed Body"], "unselected bytes are independently owned")
	}
}

func TestGTPv2PiggybackBoundariesAndIndependentBudgetCause(t *testing.T) {
	one := gtpv2TestMessage(33, 0x58, 0x01020304, 5, gtpv2TestIE(2, []byte{16, 0}, 0))
	two := gtpv2TestMessage(95, 0x48, 0x01020304, 6, gtpv2TestIE(3, []byte{9}, 0))
	w := append(bytes.Clone(one), two...)
	f, err := DecodeGTPv2Wire(w, 64)
	require.NoError(t, err)
	rows := f["Messages"].([]map[string]any)
	require.Len(t, rows, 2)
	h := rows[1]["Header"].(map[string]any)
	require.Equal(t, []any{uint64(len(one) + 8), uint64(len(one) + 11)}, h["Wire Field Ranges"].(map[string]any)["Sequence Number"])
	require.Equal(t, "Create Bearer Request", rows[1]["Message Name"])
	third := gtpv2TestMessage(1, 0x40, 0, 99, gtpv2TestIE(3, []byte{77}, 0))
	bad := bytes.Clone(two)
	bad[0] |= 16
	f, err = DecodeGTPv2Wire(append(append(bytes.Clone(one), bad...), third...), 64)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
	require.Len(t, f["Messages"], 2)
	require.Equal(t, uint64(9), f["Messages"].([]map[string]any)[1]["IEs"].([]map[string]any)[0]["Body"].(map[string]any)["Recovery Counter"], "second message fields retain their own bounded extent")
	_, err = DecodeGTPv2Wire(one, 64)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
	firstP0 := bytes.Clone(one)
	firstP0[0] &^= 16
	f, err = DecodeGTPv2Wire(append(firstP0, gtpv2TestIE(250, []byte{1}, 0)...), 64)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
	require.Len(t, f["Messages"].([]map[string]any)[0]["IEs"], 1)
	// Independent P contradiction survives a local IE budget refusal.
	bad = gtpv2TestMessage(95, 0x58, 1, 6, append(gtpv2TestIE(3, []byte{1}, 0), gtpv2TestIE(3, []byte{2}, 0)...))
	_, err = DecodeGTPv2Wire(append(bytes.Clone(one), bad...), 2)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
}

func TestGTPv2CompletePrimitivePrefixesDoNotBorrowFromAnotherExtent(t *testing.T) {
	w := gtpv2TestMessage(32, 0x48, 0x11223344, 0x010203, nil)
	for _, cut := range []int{0, 1, 3, 10} {
		f, err := DecodeGTPv2Wire(w[:cut], 64)
		require.Error(t, err)
		rows := f["Messages"].([]map[string]any)
		if cut == 0 {
			require.Empty(t, rows)
			continue
		}
		h := rows[0]["Header"].(map[string]any)
		if cut < 4 {
			require.NotContains(t, h, "Message Length")
		}
		if cut == 10 {
			require.Equal(t, uint64(0x11223344), h["TEID"])
		}
		require.NotContains(t, h, "Sequence Number")
		require.NotContains(t, rows[0], "IEs")
	}
	bad := bytes.Clone(w)
	binary.BigEndian.PutUint16(bad[2:], 4)
	f, err := DecodeGTPv2Wire(append(bad, bytes.Repeat([]byte{0xff}, 20)...), 64)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
	h := f["Messages"].([]map[string]any)[0]["Header"].(map[string]any)
	require.Equal(t, uint64(0x11223344), h["TEID"], "complete primitive fits inside the declared eight-byte extent")
	require.NotContains(t, h, "Sequence Number")
	f, err = DecodeGTPv2Wire([]byte{0x30, 1, 0, 4, 1, 2, 3, 4}, 64)
	require.Error(t, err)
	h = f["Messages"].([]map[string]any)[0]["Header"].(map[string]any)
	require.Equal(t, uint64(1), h["Version"])
	require.NotContains(t, h, "Piggyback")
	require.NotContains(t, h, "TEID Present")
	require.NotContains(t, h, "Message Type")
}

func TestGTPv2GroupedOpaqueSiblingSelectedValuesAndBounds(t *testing.T) {
	group := gtpv2TestIE(93, append(gtpv2TestIE(250, []byte("ABC"), 2), gtpv2TestIE(73, []byte{5}, 0)...), 0)
	ft := gtpv2TestIE(87, []byte{0x85, 0x12, 0x34, 0x56, 0x78, 192, 0, 2, 9}, 0)
	w := gtpv2TestMessage(32, 0x48, 1, 1, append(group, ft...))
	f, err := DecodeGTPv2Wire(w, 64)
	require.NoError(t, err)
	ies := f["Messages"].([]map[string]any)[0]["IEs"].([]map[string]any)
	children := ies[0]["IEs"].([]map[string]any)
	require.Len(t, children, 2)
	require.Equal(t, []byte("ABC"), children[0]["Body"].(map[string]any)["Opaque Value"])
	require.Equal(t, uint64(5), children[1]["Body"].(map[string]any)["EPS Bearer ID"])
	require.Equal(t, "192.0.2.9", ies[1]["Body"].(map[string]any)["IPv4 Address"])
	require.Equal(t, uint64(5), ies[1]["Body"].(map[string]any)["Interface Type"])
	bad := gtpv2TestMessage(1, 0x40, 0, 1, []byte{93, 0, 9, 0, 'X'})
	f, err = DecodeGTPv2Wire(bad, 64)
	require.ErrorIs(t, err, ErrGTPv2WireFormat)
	ie := f["Messages"].([]map[string]any)[0]["IEs"].([]map[string]any)[0]
	require.Equal(t, []byte{'X'}, ie["Captured Value"])
	require.NotContains(t, ie, "IEs")
	deep := gtpv2TestIE(73, []byte{5}, 0)
	for i := 0; i < 10; i++ {
		deep = gtpv2TestIE(93, deep, 0)
	}
	_, err = DecodeGTPv2Wire(gtpv2TestMessage(32, 0x48, 1, 1, deep), 64)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	require.False(t, errors.Is(err, ErrGTPv2WireFormat))
}

func TestGTPv2SelectedIEReceiveLengthRules(t *testing.T) {
	// TS29.274 7.7.7 permits unknown extensions to known extendable prefixes.
	// The complete TLIV envelope still cannot excuse a missing mandatory octet.
	for _, tc := range []struct {
		name            string
		typ             byte
		value           []byte
		format, context bool
		key             string
		want            any
	}{
		{"ebi-extension", 73, []byte{0xa5, 0xee}, false, false, "EPS Bearer ID", uint64(5)},
		{"ebi-empty", 73, nil, true, false, "", nil},
		{"recovery-empty", 3, nil, true, false, "", nil},
		{"recovery-variable-tail", 3, []byte{7, 0xee}, false, true, "Recovery Counter", uint64(7)},
		{"cause-empty", 2, nil, true, false, "", nil},
		{"cause-one-byte", 2, []byte{16}, true, false, "Cause", uint64(16)},
		{"cause-three-bytes", 2, []byte{16, 0, 0xee}, true, false, "Cause", uint64(16)},
		{"cause-seven-bytes", 2, []byte{16, 0, 73, 0, 0, 0, 0xee}, true, false, "Cause Flags", uint64(0)},
		{"fteid-no-address", 87, []byte{0, 1, 2, 3, 4}, true, false, "TEID", uint64(0x01020304)},
		{"fteid-short-prefix", 87, []byte{0x80, 1, 2, 3}, true, false, "F-TEID Flags", uint64(128)},
		{"fteid-v4-short", 87, []byte{0x80, 1, 2, 3, 4, 192, 0, 2}, true, false, "TEID", uint64(0x01020304)},
		{"fteid-v4-extension", 87, []byte{0x80, 1, 2, 3, 4, 192, 0, 2, 9, 0xee}, false, false, "IPv4 Address", "192.0.2.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := gtpv2TestMessage(32, 0x48, 1, 1, gtpv2TestIE(tc.typ, tc.value, 0))
			fields, err := DecodeGTPv2Wire(w, 64)
			require.Equal(t, tc.format, errors.Is(err, ErrGTPv2WireFormat), err)
			require.Equal(t, tc.context, errors.Is(err, ErrObservedWireContext), err)
			if !tc.format && !tc.context {
				require.NoError(t, err)
			}
			require.Equal(t, !tc.format && !tc.context, fields["Wire Grammar Complete"])
			ie := fields["Messages"].([]map[string]any)[0]["IEs"].([]map[string]any)[0]
			body := ie["Body"].(map[string]any)
			if tc.key != "" {
				require.Equal(t, tc.want, body[tc.key])
			}
			if tc.typ == 2 && len(tc.value) != 6 {
				require.NotContains(t, body, "Offending IE Type")
			}
			if tc.name == "ebi-extension" || tc.name == "fteid-v4-extension" {
				require.Equal(t, []byte{0xee}, body["Ignored Extension Bytes"])
				span := body["Wire Field Ranges"].(map[string]any)["Ignored Extension Bytes"].([]any)
				require.Equal(t, []any{uint64(len(w) - 1), uint64(len(w))}, span)
				w[len(w)-1] = 0
				require.Equal(t, []byte{0xee}, body["Ignored Extension Bytes"])
			}
		})
	}
}

func TestGTPv2FTEIDIPv6AndDualAddressExtensionsKeepExactRanges(t *testing.T) {
	for _, both := range []bool{false, true} {
		value := []byte{0x4d, 1, 2, 3, 4}
		if both {
			value[0] |= 0x80
			value = append(value, 192, 0, 2, 9)
		}
		value = append(value, []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}...)
		value = append(value, 0xee, 0xff)
		w := gtpv2TestMessage(32, 0x48, 1, 1, gtpv2TestIE(87, value, 0))
		f, err := DecodeGTPv2Wire(w, 64)
		require.NoError(t, err)
		body := f["Messages"].([]map[string]any)[0]["IEs"].([]map[string]any)[0]["Body"].(map[string]any)
		require.Equal(t, "2001:db8::1", body["IPv6 Address"])
		require.Equal(t, uint64(13), body["Interface Type"])
		ranges := body["Wire Field Ranges"].(map[string]any)
		start := uint64(21)
		if both {
			require.Equal(t, "192.0.2.9", body["IPv4 Address"])
			require.Equal(t, []any{uint64(21), uint64(25)}, ranges["IPv4 Address"])
			start += 4
		} else {
			require.NotContains(t, body, "IPv4 Address")
		}
		require.Equal(t, []any{start, start + 16}, ranges["IPv6 Address"])
		require.Equal(t, []any{start + 16, start + 18}, ranges["Ignored Extension Bytes"])
		require.Equal(t, []byte{0xee, 0xff}, body["Ignored Extension Bytes"])
	}
}

func FuzzGTPv2BoundedOccurrences(f *testing.F) {
	f.Add(gtpv2TestMessage(1, 0x40, 0, 1, gtpv2TestIE(3, []byte{7}, 0)), uint8(1))
	f.Add(gtpv2TestMessage(32, 0x48, 0, 1, gtpv2TestIE(93, gtpv2TestIE(73, []byte{5}, 0), 0)), uint8(32))
	f.Fuzz(func(t *testing.T, w []byte, budget uint8) {
		if len(w) > 65535 {
			return
		}
		before := bytes.Clone(w)
		fields, _ := DecodeGTPv2Wire(w, int(budget)+1)
		require.Equal(t, before, w)
		var walk func(any)
		walk = func(v any) {
			switch v := v.(type) {
			case map[string]any:
				if ranges, ok := v["Wire Field Ranges"].(map[string]any); ok {
					for key, s := range ranges {
						span := s.([]any)
						require.Contains(t, v, key)
						require.LessOrEqual(t, span[0].(uint64), span[1].(uint64))
						require.LessOrEqual(t, span[1].(uint64), uint64(len(w)))
					}
				}
				for key, c := range v {
					if key != "Wire Field Ranges" {
						walk(c)
					}
				}
			case []map[string]any:
				for _, c := range v {
					require.NotNil(t, c)
					walk(c)
				}
			}
		}
		walk(fields)
	})
}
