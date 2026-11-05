package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzSMB1ExtendedResponseSources(f *testing.F) {
	f.Add(smb1ExtendedResponse(0))
	f.Add(smb1ExtendedResponse(64))
	f.Add(smb1ExtendedResponse(64)[:128])
	f.Add([]byte{0, 0, 0, 85, 0xff, 'S', 'M', 'B'})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 65609 {
			return
		}
		wire := bytes.Clone(input)
		original := bytes.Clone(wire)
		fields, err := decodeSMB1NegotiateResponseFields(wire, DefaultParserBudget())
		require.Equal(t, original, wire)
		if err != nil {
			require.Nil(t, fields)
			return
		}
		require.Equal(t, ProbeAccept, probeSMB1NegotiateExtendedResponse(wire, 65608).Verdict)
		require.Equal(t, len(wire)-4, smb2DirectLength(wire))
		require.GreaterOrEqual(t, len(wire), 89)
		body := fields["Body"].(map[string]any)
		require.Equal(t, binary.LittleEndian.Uint64(wire[60:68]), body["System Time"])
		require.Equal(t, int16(binary.LittleEndian.Uint16(wire[68:70])), body["Server Time Zone"])
		require.Equal(t, binary.LittleEndian.Uint16(wire[71:73]), body["Byte Count"])
		require.Equal(t, wire[73:89], body["Server GUID"])
		require.Equal(t, wire[89:], body["Security Blob"])
		for _, spec := range []struct {
			name   string
			lo, hi int
		}{{"Word Count", 36, 37}, {"Dialect Index", 37, 39}, {"Security Mode", 39, 40}, {"Max Mpx Count", 40, 42}, {"Max Number Vcs", 42, 44}, {"Max Buffer Size", 44, 48}, {"Max Raw Size", 48, 52}, {"Session Key", 52, 56}, {"Capabilities", 56, 60}, {"System Time", 60, 68}, {"Server Time Zone", 68, 70}, {"Challenge Length", 70, 71}, {"Byte Count", 71, 73}, {"Server GUID", 73, 89}, {"Security Blob", 89, len(wire)}} {
			require.Equal(t, []any{uint64(spec.lo), uint64(spec.hi)}, body["Wire Field Ranges"].(map[string]any)[spec.name])
			literal := wire[spec.lo:spec.hi]
			switch value := body[spec.name].(type) {
			case uint8:
				require.Len(t, literal, 1)
				require.Equal(t, literal[0], value)
			case uint16:
				require.Len(t, literal, 2)
				require.Equal(t, binary.LittleEndian.Uint16(literal), value)
			case uint32:
				require.Len(t, literal, 4)
				require.Equal(t, binary.LittleEndian.Uint32(literal), value)
			case uint64:
				require.Len(t, literal, 8)
				require.Equal(t, binary.LittleEndian.Uint64(literal), value)
			case int16:
				require.Len(t, literal, 2)
				require.Equal(t, int16(binary.LittleEndian.Uint16(literal)), value)
			case []byte:
				require.Equal(t, literal, value)
			default:
				t.Fatalf("untyped CIFS response literal %s: %T", spec.name, value)
			}
		}
		require.Equal(t, false, fields["Dialect Negotiated"])
		require.NotContains(t, fields, "Selected Dialect Name")
		clear(wire)
		require.Equal(t, original[73:89], body["Server GUID"])
		require.Equal(t, original[89:], body["Security Blob"])
	})
}
