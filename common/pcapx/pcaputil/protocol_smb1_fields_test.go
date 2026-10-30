package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func smb1FieldsTestSource(names ...string) []byte {
	w := make([]byte, 39)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x72})
	w[13] = 0x18
	for _, name := range names {
		w = append(w, 2)
		w = append(w, name...)
		w = append(w, 0)
	}
	binary.LittleEndian.PutUint16(w[37:39], uint16(len(w)-39))
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func TestSMB1NegotiateFieldsMatchCanonicalOwnedProjection(t *testing.T) {
	w := smb1FieldsTestSource("SMB 2.002", "SMB 2.002", "", "OEM-\x80\xfe")
	original := bytes.Clone(w)
	want, err := parser.ParseStructured(w, "application-layer.cifs", "CIFSDirectTCP")
	require.NoError(t, err)
	got, err := decodeSMB1NegotiateFields(w, ParserBudget{})
	require.NoError(t, err)
	require.Equal(t, want["fields"], got)
	require.Equal(t, original, w)
	rows := got["Dialects"].([]map[string]any)
	require.Len(t, rows, 4)
	require.NotContains(t, rows[3]["Wire Field Ranges"], "Name")
	rows[0]["Raw Name Bytes"].([]byte)[0] = '!'
	fresh, err := decodeSMB1NegotiateFields(w, DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, []byte("SMB 2.002"), fresh["Dialects"].([]map[string]any)[0]["Raw Name Bytes"])
	require.Equal(t, true, got["SMB2 Upgrade Offered"])
	require.Equal(t, false, got["Dialect Negotiated"])
	require.NotContains(t, got, "Message ID", "legacy MID is not an SMB2 MessageId")
	for i := range w {
		w[i] = 0
	}
	require.Equal(t, []byte("OEM-\x80\xfe"), rows[3]["Raw Name Bytes"])
}

func TestSMB1NegotiateFieldBudgetsAndRefusals(t *testing.T) {
	w := smb1FieldsTestSource("SMB 2.002", "SMB 2.???")
	for _, budget := range []ParserBudget{
		{MaxCollectionElements: 1}, {MaxRecursionDepth: 3}, {MaxFrameBytes: len(w) - 1},
		{MaxMessageBytes: len(w) - 1}, {MaxBufferedBytes: len(w) - 1}, {MaxBufferedBytes: len(w)},
		{MaxCollectionElements: -1},
	} {
		fields, err := decodeSMB1NegotiateFields(w, budget)
		require.Error(t, err)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrResourceExceeded, typed.Kind)
		require.Nil(t, fields)
	}
	for _, malformed := range [][]byte{
		w[:38], w[:len(w)-1], append(bytes.Clone(w), 0),
		func() []byte { v := bytes.Clone(w); v[13] |= 0x80; return v }(),
		func() []byte { v := bytes.Clone(w); v[36] = 1; return v }(),
		func() []byte { v := bytes.Clone(w); v[39] = 3; return v }(),
		func() []byte { v := bytes.Clone(w); v[len(v)-1] = 1; return v }(),
	} {
		fields, err := decodeSMB1NegotiateFields(malformed, DefaultParserBudget())
		require.Error(t, err)
		require.Nil(t, fields)
	}
	fields, err := decodeSMB1NegotiateFields(smb1FieldsTestSource("SMB 2.002-private"), DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, false, fields["SMB2 Upgrade Offered"], "offer recognition is an exact observed dialect name")
}
