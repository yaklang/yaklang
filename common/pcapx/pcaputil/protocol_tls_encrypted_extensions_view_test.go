package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSEncryptedExtensionsOwnedViewAndLiteralRanges(t *testing.T) {
	wire := reliabilityHS(8, eeScopeBody(tlsResumptionExt{16, eeScopeALPN("http/1.1")}, tlsResumptionExt{0xbeef, []byte("http/1.1")}))
	view, err := DecodeTLSEncryptedExtensions(wire, 0x304, 2)
	require.NoError(t, err)
	extensions := view.Fields["Extensions"].([]map[string]any)
	require.Len(t, extensions, 2)
	want := map[string][2]int{"Extensions Length": {4, 6}, "Extensions/0/Type": {6, 8}, "Extensions/0/Length": {8, 10}, "Extensions/0/Data": {10, 21}, "Extensions/0/ALPN Length": {10, 12}, "Extensions/0/Protocol Length": {12, 13}, "Extensions/0/Protocol": {13, 21}, "Extensions/1/Type": {21, 23}, "Extensions/1/Length": {23, 25}, "Extensions/1/Data": {25, 33}}
	require.Len(t, view.Ranges, len(want))
	for _, r := range view.Ranges {
		span, ok := want[strings.Join(r.Path, "/")]
		require.True(t, ok)
		require.Equal(t, span, [2]int{r.Offset, r.Offset + r.Length})
	}
	require.Equal(t, []byte("http/1.1"), extensions[0]["Protocol"])
	require.Equal(t, []byte("http/1.1"), extensions[1]["Data"])
	before, err := json.Marshal(view)
	require.NoError(t, err)
	for i := range wire {
		wire[i] ^= 0xff
	}
	after, err := json.Marshal(view)
	require.NoError(t, err)
	require.Equal(t, before, after, "returned bytes must not alias the caller")
	extensions[0]["Data"].([]byte)[3] ^= 0xff
	require.Equal(t, []byte("http/1.1"), extensions[0]["Protocol"], "literal projections own their values independently")
}

func TestTLSEncryptedExtensionsViewBoundariesAndBudget(t *testing.T) {
	valid := reliabilityHS(8, eeScopeBody(tlsResumptionExt{16, eeScopeALPN("h2")}))
	for n := 0; n < len(valid); n++ {
		t.Run(fmt.Sprintf("crop%d", n), func(t *testing.T) {
			_, err := DecodeTLSEncryptedExtensions(valid[:n], 0x304, 4096)
			require.Error(t, err)
		})
	}
	for _, version := range []uint16{0, 0x303, 0x305} {
		_, err := DecodeTLSEncryptedExtensions(valid, version, 4096)
		require.ErrorContains(t, err, "UnsupportedVersion")
	}
	for _, body := range [][]byte{eeScopeBody(tlsResumptionExt{16, eeScopeALPN("h2", "h3")}), eeScopeBody(tlsResumptionExt{16, []byte{0, 1, 0}}), eeScopeBody(tlsResumptionExt{0xffff, nil}, tlsResumptionExt{0xffff, nil}), {0, 1, 0}, {0, 4, 0, 16, 0, 5}} {
		_, err := DecodeTLSEncryptedExtensions(reliabilityHS(8, body), 0x304, 0)
		require.ErrorContains(t, err, "MalformedMessage", "framing must precede the collection budget")
	}
	_, err := DecodeTLSEncryptedExtensions(valid, 0x304, 0)
	require.ErrorContains(t, err, "ResourceExceeded")
	_, err = DecodeTLSEncryptedExtensions(reliabilityHS(8, eeScopeBody()), 0x304, 0)
	require.NoError(t, err)
	for _, kind := range []uint16{5, 13, 18, 21, 41, 43, 44, 45, 47, 48, 49, 50, 51} {
		t.Run(fmt.Sprintf("forbidden%d", kind), func(t *testing.T) {
			_, err := DecodeTLSEncryptedExtensions(reliabilityHS(8, eeScopeBody(tlsResumptionExt{kind, nil})), 0x304, 4096)
			require.ErrorContains(t, err, "MalformedMessage")
			state := &binTLS{client: 0, version: 0x304, alpn: "prior"}
			before := *state
			_, err = state.handshakeMessage(1, reliabilityHS(8, eeScopeBody(tlsResumptionExt{kind, nil})), true)
			require.Error(t, err)
			require.Equal(t, before, *state)
		})
	}
}

func FuzzTLSEncryptedExtensionsView(f *testing.F) {
	for _, b := range [][]byte{eeScopeBody(), eeScopeBody(tlsResumptionExt{16, eeScopeALPN("h2")}), eeScopeBody(tlsResumptionExt{0xffff, []byte{0, 1, 0xff}}), {0, 1, 0}} {
		f.Add(reliabilityHS(8, b))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		wire := bytes.Clone(input[:min(len(input), 4096)])
		v, err := DecodeTLSEncryptedExtensions(wire, 0x304, 32)
		if err != nil {
			return
		}
		rebuilt := binary.BigEndian.AppendUint16(nil, v.Fields["Extensions Length"].(uint16))
		for _, e := range v.Fields["Extensions"].([]map[string]any) {
			raw := e["Data"].([]byte)
			require.Equal(t, int(e["Length"].(uint16)), len(raw))
			rebuilt = binary.BigEndian.AppendUint16(rebuilt, e["Type"].(uint16))
			rebuilt = binary.BigEndian.AppendUint16(rebuilt, e["Length"].(uint16))
			rebuilt = append(rebuilt, raw...)
		}
		require.Equal(t, wire, reliabilityHS(8, rebuilt))
		for _, r := range v.Ranges {
			require.Greater(t, r.Length, 0)
			require.GreaterOrEqual(t, r.Offset, 4)
			require.LessOrEqual(t, r.Offset+r.Length, len(wire))
		}
	})
}
