package parser

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Source construction follows MS-CIFS 2.2.4.52.1, independently of the parser.
func cifsProjectionTestSource(direct bool, names ...string) []byte {
	w := make([]byte, 35)
	copy(w, []byte{0xff, 'S', 'M', 'B', 0x72})
	binary.LittleEndian.PutUint32(w[5:9], 0x11223344)
	w[9] = 0x18
	binary.LittleEndian.PutUint16(w[10:12], 0xc853)
	binary.LittleEndian.PutUint16(w[12:14], 0x1122)
	copy(w[14:22], []byte{0, 1, 2, 3, 4, 5, 6, 7})
	for i, value := range []uint16{0x3344, 0x5566, 0x7788, 0x99aa, 0xbbcc} {
		binary.LittleEndian.PutUint16(w[22+i*2:], value)
	}
	for _, name := range names {
		w = append(w, 2)
		w = append(w, name...)
		w = append(w, 0)
	}
	binary.LittleEndian.PutUint16(w[33:35], uint16(len(w)-35))
	if direct {
		prefix := make([]byte, 4)
		binary.BigEndian.PutUint32(prefix, uint32(len(w)))
		w = append(prefix, w...)
	}
	return w
}

func cifsProjectionTestNode(t *testing.T, raw []byte, direct bool) *base.Node {
	t.Helper()
	entry := "CIFSNegotiateRequest"
	if direct {
		entry = "CIFSDirectTCP"
	}
	r := &structuredReader{Reader: bytes.NewReader(raw), bits: uint64(len(raw)) * 8}
	node, err := ParseBinary(r, "application-layer.cifs", entry)
	require.NoError(t, err)
	require.Zero(t, r.Len())
	return node
}

func TestProjectCIFSNegotiateOwnedFieldsAndExactSpans(t *testing.T) {
	names := []string{"SMB 2.002", "SMB 2.002", "", "OEM-\x80\xfe"}
	for _, direct := range []bool{false, true} {
		w := cifsProjectionTestSource(direct, names...)
		original := bytes.Clone(w)
		node := cifsProjectionTestNode(t, w, direct)
		before := stream_parser.NodeToMap(node)
		fields, err := ProjectCIFSNegotiateFields(node, w, 4)
		require.NoError(t, err)
		require.Equal(t, before, stream_parser.NodeToMap(node), "projection must not mutate the parsed Node")
		require.Equal(t, original, w)
		require.Equal(t, "SMB_COM_NEGOTIATE", fields["Packet Name"])
		require.Equal(t, true, fields["SMB2 Upgrade Offered"])
		for _, key := range []string{"OEM Codepage Inferred", "Dialect Negotiated", "Session Established", "Signature Verified"} {
			require.Equal(t, false, fields[key])
			require.NotContains(t, fields["Wire Field Ranges"], key)
		}
		offset := uint64(0)
		if direct {
			offset = 4
			d := fields["Direct TCP"].(map[string]any)
			require.Equal(t, uint8(0), d["Message Type"])
			require.Equal(t, uint32(len(w)-4), d["Payload Length"])
			require.Equal(t, []any{uint64(1), uint64(4)}, d["Wire Field Ranges"].(map[string]any)["Payload Length"])
		} else {
			require.NotContains(t, fields, "Direct TCP")
		}
		header := fields["Header"].(map[string]any)
		require.Equal(t, uint32(0x424d53ff), header["Protocol ID"])
		require.Equal(t, uint8(0x72), header["Command"])
		require.Equal(t, uint32(0x11223344), header["Status"])
		require.Equal(t, uint16(0x1122), header["PID High"])
		require.Equal(t, uint16(0x5566), header["Tree ID"])
		require.Equal(t, uint16(0xbbcc), header["Multiplex ID"])
		require.Equal(t, []any{offset + 14, offset + 22}, header["Wire Field Ranges"].(map[string]any)["Security Features"])
		require.Equal(t, []any{offset + 33, offset + 35}, fields["Wire Field Ranges"].(map[string]any)["Byte Count"])
		rows := fields["Dialects"].([]map[string]any)
		require.Len(t, rows, 4, "same-named dialect nodes must retain order and multiplicity")
		at := offset + 35
		for i, name := range names {
			row := rows[i]
			wr := row["Wire Field Ranges"].(map[string]any)
			require.Equal(t, uint8(2), row["Buffer Format"])
			require.Equal(t, []byte(name), row["Raw Name Bytes"])
			require.Equal(t, []any{at, at + 1}, wr["Buffer Format"])
			require.Equal(t, []any{at + 1, at + 1 + uint64(len(name))}, wr["Raw Name Bytes"])
			require.Equal(t, []any{at + 1 + uint64(len(name)), at + 2 + uint64(len(name))}, wr["NUL Terminator"])
			if i == 3 {
				require.Equal(t, "\\x4f\\x45\\x4d\\x2d\\x80\\xfe", row["Name"])
				require.NotContains(t, wr, "Name", "escaped display is not literal wire text")
			} else {
				require.Equal(t, name, row["Name"])
				require.Equal(t, wr["Raw Name Bytes"], wr["Name"])
			}
			at += uint64(len(name) + 2)
		}
		require.Equal(t, uint64(len(w)), at)
		require.NotNil(t, rows[2]["Raw Name Bytes"], "empty Name is a zero-width observed value")
		encoded, err := json.Marshal(fields)
		require.NoError(t, err)
		var roundtrip map[string]any
		require.NoError(t, json.Unmarshal(encoded, &roundtrip))
		jsonRows := roundtrip["Dialects"].([]any)
		require.Equal(t, base64.StdEncoding.EncodeToString([]byte(names[3])), jsonRows[3].(map[string]any)["Raw Name Bytes"])
		for i := range w {
			w[i] ^= 0xff
		}
		require.Equal(t, []byte(names[3]), rows[3]["Raw Name Bytes"], "result must own OEM bytes")
		header["Security Features"].([]byte)[0] = 0xff
		fresh, err := ProjectCIFSNegotiateFields(node, original, 4)
		require.NoError(t, err)
		require.Equal(t, byte(0), fresh["Header"].(map[string]any)["Security Features"].([]byte)[0])
	}
}

func TestProjectCIFSNegotiateRefusesWrongSourceOrMutableShape(t *testing.T) {
	w := cifsProjectionTestSource(true, "SMB 2.002", "SMB 2.002")
	node := cifsProjectionTestNode(t, w, true)
	for _, budget := range []int{-1, 0, 1} {
		fields, err := ProjectCIFSNegotiateFields(node, w, budget)
		require.ErrorIs(t, err, ErrCIFSProjectionBudget)
		require.Nil(t, fields)
	}
	for _, mutate := range []func(*base.Node, []byte){
		func(_ *base.Node, raw []byte) { raw[9] ^= 1 },
		func(n *base.Node, _ []byte) { n.Children[0].Cfg.SetItem(base.CfgNodeResult, [2]uint64{1, 9}) },
		func(n *base.Node, _ []byte) { n.Children[0].Cfg.SetItem(base.CfgNodeResult, []int{0, 8}) },
		func(n *base.Node, _ []byte) {
			n.Children[2].Children[14].Children = n.Children[2].Children[14].Children[:1]
		},
		func(n *base.Node, _ []byte) {
			n.Children[2].Children[14].Children[0].Children[1].Cfg.SetItem(base.CfgRawResult, "invented")
		},
		func(n *base.Node, _ []byte) {
			n.Children[2].Children[14].Children[0].Children[1].Cfg.SetItem(base.CfgNodeResult, [2]uint64{320, 400})
		},
	} {
		copyNode := node.Copy()
		copyWire := bytes.Clone(w)
		mutate(copyNode, copyWire)
		fields, err := ProjectCIFSNegotiateFields(copyNode, copyWire, 1024)
		require.Error(t, err)
		require.Nil(t, fields)
	}
	fields, err := ProjectCIFSNegotiateFields(nil, w, 1024)
	require.Error(t, err)
	require.Nil(t, fields)
	fields, err = ProjectCIFSNegotiateFields(node, append(bytes.Clone(w), 0), 1024)
	require.Error(t, err)
	require.Nil(t, fields)
}

func TestCIFSStructuredProjectionIsProfileSpecific(t *testing.T) {
	for _, direct := range []bool{false, true} {
		w := cifsProjectionTestSource(direct, "NT LM 0.12", "SMB 2.002", "SMB 2.???")
		entry := "CIFSNegotiateRequest"
		if direct {
			entry = "CIFSDirectTCP"
		}
		node := cifsProjectionTestNode(t, w, direct)
		want, err := ProjectCIFSNegotiateFields(node, w, 1024)
		require.NoError(t, err)
		for _, withConfig := range []bool{false, true} {
			var got map[string]any
			if withConfig {
				got, err = ParseStructuredWithConfig(w, "application-layer.cifs", map[string]any{"sourceLabel": "not-a-field"}, entry)
			} else {
				got, err = ParseStructured(w, "application-layer.cifs", entry)
			}
			require.NoError(t, err)
			require.Equal(t, want, got["fields"])
			require.Equal(t, node.Cfg.GetItem("additionInfo"), got["metadata"], "canonical metadata must not be relocated or fabricated")
		}
	}
	w := []byte{0, 0, 0, 1, 0xff}
	got, err := ParseStructured(w, "application-layer.cifs", "CIFSDirectTCPCarrier")
	require.NoError(t, err)
	require.NotContains(t, got["fields"], "Packet Name")
	require.NotContains(t, got["fields"], "Header")
	for i := 0; i < len(cifsProjectionTestSource(true, "x")); i++ {
		partial, err := ParseStructured(cifsProjectionTestSource(true, "x")[:i], "application-layer.cifs", "CIFSDirectTCP")
		require.Error(t, err)
		require.Nil(t, partial)
	}
}

func TestCIFSStructuredCustomParserKeepsLegacyProjection(t *testing.T) {
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	got, err := ParseStructured(cifsProjectionTestSource(true, "one", "two"), "application-layer.cifs", "CIFSDirectTCP")
	require.NoError(t, err)
	require.Positive(t, custom.calls)
	require.NotContains(t, got["fields"], "Packet Name")
	require.Contains(t, got["fields"], "Negotiate Request")
}
