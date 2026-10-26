package stream_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func ipfixTestSet(id uint16, body ...byte) []byte {
	w := binary.BigEndian.AppendUint16(nil, id)
	w = binary.BigEndian.AppendUint16(w, uint16(len(body)+4))
	return append(w, body...)
}
func ipfixTestMessage(seq uint32, sets ...[]byte) []byte {
	w := []byte{0, 10, 0, 0}
	w = binary.BigEndian.AppendUint32(w, 1700000000)
	w = binary.BigEndian.AppendUint32(w, seq)
	w = binary.BigEndian.AppendUint32(w, 19)
	for _, s := range sets {
		w = append(w, s...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	return w
}
func TestIPFIXSetOrderAndSchemaOwnership(t *testing.T) {
	template := ipfixTestSet(2, 1, 0, 0, 2, 0, 8, 0, 4, 0, 2, 0, 4)
	data := ipfixTestSet(256, 192, 0, 2, 9, 0, 0, 0, 17)
	out, err := DecodeIPFIXWire(ipfixTestMessage(603, template), nil, "udp", 4096)
	require.NoError(t, err)
	previous := out.Templates
	out, err = DecodeIPFIXWire(ipfixTestMessage(603, data), previous, "udp", 4096)
	require.NoError(t, err)
	require.Equal(t, uint64(1), out.Fields["Data Record Count"])
	values := out.Fields["FlowSets"].([]map[string]any)[0]["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)
	require.Equal(t, "192.0.2.9", values[0]["Value"])
	require.Equal(t, uint64(17), values[1]["Value"])
	require.Equal(t, []any{uint64(20), uint64(24)}, values[0]["Wire Field Ranges"].(map[string]any)["Value"])
	cloned := CloneIPFIXTemplates(previous)
	x := cloned[256]
	x.Fields[0].Type = 12
	require.Equal(t, uint16(8), previous[256].Fields[0].Type)
	out, err = DecodeIPFIXWire(ipfixTestMessage(603, data, template), nil, "udp", 4096)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.NotNil(t, out.Templates, "later valid template is useful for future messages, not earlier data")
	require.Equal(t, uint64(0), out.Fields["Data Record Count"])
	require.Equal(t, false, out.Fields["Record Count Verified"])
}
func TestIPFIXOptionsEnterpriseVariableAndPadding(t *testing.T) {
	// One standard scope, enterprise IE8 (including PEN0), and variable unknown.
	template := ipfixTestSet(3, 1, 0, 0, 3, 0, 1, 0, 10, 0, 4, 0x80, 8, 0, 4, 0, 0, 0, 0, 0x03, 0xe8, 0xff, 0xff)
	out, err := DecodeIPFIXWire(ipfixTestMessage(0, template, ipfixTestSet(256, 0, 0, 0, 7, 192, 0, 2, 9, 255, 0, 2, 'h', 'i', 0, 0)), nil, "tcp", 4096)
	require.NoError(t, err)
	require.Equal(t, uint64(1), out.Fields["Data Record Count"])
	s := out.Fields["FlowSets"].([]map[string]any)[1]
	require.Equal(t, []byte{0, 0}, s["Padding"])
	f := s["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)
	require.Equal(t, true, f[0]["Scope"])
	require.Equal(t, true, f[1]["Enterprise"])
	require.Equal(t, "opaque", f[1]["Value Kind"])
	require.Equal(t, []byte{192, 0, 2, 9}, f[1]["Value"])
	require.Equal(t, []byte("hi"), f[2]["Value"])
	// Three-octet length with a value below255 is still a valid explicit length.
	require.Equal(t, uint64(2), f[2]["Variable Extended Length"])
	for _, body := range [][]byte{{0, 0, 0, 7, 192, 0, 2, 9, 255, 0}, {0, 0, 0, 7, 192, 0, 2, 9, 4, 'h', 'i'}, {0, 0, 0, 7, 192, 0, 2, 9, 0, 0, 1}} {
		_, err = DecodeIPFIXWire(ipfixTestMessage(0, template, ipfixTestSet(256, body...)), nil, "tcp", 4096)
		require.Error(t, err)
	}
}
func TestIPFIXWithdrawalAndRedefinition(t *testing.T) {
	template := ipfixTestSet(2, 1, 0, 0, 1, 0, 8, 0, 4)
	options := ipfixTestSet(3, 1, 1, 0, 1, 0, 1, 0, 10, 0, 4)
	for _, transport := range []string{"udp", "tcp"} {
		out, err := DecodeIPFIXWire(ipfixTestMessage(0, template, options, ipfixTestSet(2, 0, 2, 0, 0)), nil, transport, 4096)
		require.NoError(t, err)
		_, exists := out.Templates[256]
		require.Equal(t, transport == "udp", exists)
		require.Contains(t, out.Templates, uint16(257))
		out, err = DecodeIPFIXWire(ipfixTestMessage(0, ipfixTestSet(3, 1, 1, 0, 0)), out.Templates, transport, 4096)
		require.NoError(t, err)
		_, exists = out.Templates[257]
		require.Equal(t, transport == "udp", exists)
	}
	// A new definition takes effect in wire order, not a future/global pass.
	out, err := DecodeIPFIXWire(ipfixTestMessage(0, template, ipfixTestSet(256, 192, 0, 2, 1), ipfixTestSet(2, 1, 0, 0, 1, 0, 12, 0, 4), ipfixTestSet(256, 198, 51, 100, 2)), nil, "udp", 4096)
	require.NoError(t, err)
	sets := out.Fields["FlowSets"].([]map[string]any)
	require.Equal(t, uint64(8), sets[1]["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)[0]["Field Type"])
	require.Equal(t, uint64(12), sets[3]["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)[0]["Field Type"])
}
func TestIPFIXMalformedBoundsAndStandardTypes(t *testing.T) {
	w := ipfixTestMessage(0, ipfixTestSet(2, 1, 0, 0, 1, 0, 8, 0, 4), ipfixTestSet(256, 192, 0, 2, 9))
	for cut := range w {
		_, err := DecodeIPFIXWire(w[:cut], nil, "udp", 4096)
		require.Error(t, err)
	}
	_, err := DecodeIPFIXWire(append(w, 0), nil, "udp", 4096)
	require.Error(t, err)
	for _, set := range [][]byte{ipfixTestSet(2, 1, 0, 0, 1, 0x80, 8, 0, 4), ipfixTestSet(3, 1, 0, 0, 1, 0, 0, 0, 8, 0, 4), ipfixTestSet(2, 0, 1, 0, 0)} {
		out, err := DecodeIPFIXWire(ipfixTestMessage(0, set), nil, "udp", 4096)
		require.Error(t, err)
		require.Nil(t, out.Templates)
	}
	_, err = DecodeIPFIXWire(w, nil, "udp", 1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	for _, tt := range []struct {
		id   uint16
		b    []byte
		kind string
	}{{6, []byte{1, 2}, "unsigned"}, {6, []byte{1, 2, 3}, "opaque"}, {31, []byte{0, 1, 2, 3}, "unsigned"}, {8, []byte{1, 2}, "opaque"}, {152, make([]byte, 8), "dateTimeMilliseconds"}, {152, make([]byte, 4), "opaque"}} {
		_, kind, _ := ipfixValue(IPFIXField{Type: tt.id}, tt.b)
		require.Equal(t, tt.kind, kind)
	}
}
func FuzzIPFIXWire(f *testing.F) {
	f.Add(ipfixTestMessage(603, ipfixTestSet(2, 1, 0, 0, 1, 0, 8, 0, 4), ipfixTestSet(256, 192, 0, 2, 9)))
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65535 {
			return
		}
		out, err := DecodeIPFIXWire(w, nil, "udp", 256)
		if err == nil {
			require.Equal(t, true, out.Fields["Wire Framing Complete"])
			require.NotNil(t, out.Templates)
		}
	})
}
