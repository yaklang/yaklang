package parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func cifsEmptyResponseSource() []byte {
	w := make([]byte, 89)
	binary.BigEndian.PutUint32(w[:4], 85)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x72})
	w[13] = 0x80
	binary.LittleEndian.PutUint16(w[14:16], 0x800)
	w[36] = 17
	binary.LittleEndian.PutUint32(w[56:60], 0x80000000)
	binary.LittleEndian.PutUint64(w[60:68], ^uint64(0))
	binary.LittleEndian.PutUint16(w[68:70], 0x8000)
	binary.LittleEndian.PutUint16(w[71:73], 16)
	return w
}

func TestProjectCIFSResponseEmptyBlobDeclarationAndOwnership(t *testing.T) {
	w := cifsEmptyResponseSource()
	r := &structuredReader{Reader: bytes.NewReader(w), bits: uint64(len(w)) * 8}
	node, err := ParseBinary(r, "application-layer.cifs", "CIFSDirectTCPNegotiateExtendedResponse")
	require.NoError(t, err)
	require.Zero(t, r.Len())
	unused := node.Children[2].Children[12].Children[14]
	require.Equal(t, "SecurityBlob", unused.Name)
	require.False(t, unused.Cfg.ResultSettings().Present)
	require.False(t, unused.Cfg.ResultSettings().RawPresent)
	before := stream_parser.NodeToMap(node)
	got, err := ProjectCIFSNegotiateResponseFields(node, w)
	require.NoError(t, err)
	require.Equal(t, before, stream_parser.NodeToMap(node))
	body := got["Body"].(map[string]any)
	require.Equal(t, []byte{}, body["Security Blob"])
	require.Equal(t, []any{uint64(89), uint64(89)}, body["Wire Field Ranges"].(map[string]any)["Security Blob"])
	require.Equal(t, ^uint64(0), body["System Time"])
	require.Equal(t, int16(-32768), body["Server Time Zone"])
	unused.Cfg.SetItem(base.CfgRawResult, []byte{1})
	refused, err := ProjectCIFSNegotiateResponseFields(node, w)
	require.Error(t, err)
	require.Nil(t, refused)
}

func TestCIFSResponseStructuredConfigAndCustomParserContract(t *testing.T) {
	w := cifsEmptyResponseSource()
	plain, err := ParseStructured(w, "application-layer.cifs", "CIFSDirectTCPNegotiateExtendedResponse")
	require.NoError(t, err)
	configured, err := ParseStructuredWithConfig(w, "application-layer.cifs", map[string]any{"sourceLabel": "metadata only"}, "CIFSDirectTCPNegotiateExtendedResponse")
	require.NoError(t, err)
	require.Equal(t, plain["fields"], configured["fields"])
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	legacy, err := ParseStructured(w, "application-layer.cifs", "CIFSDirectTCPNegotiateExtendedResponse")
	require.NoError(t, err)
	require.Positive(t, custom.calls)
	require.NotContains(t, legacy["fields"], "Packet Name")
	require.Contains(t, legacy["fields"], "Negotiate Response")
}
