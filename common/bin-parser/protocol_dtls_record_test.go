package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func dtlsRuleRecord(epoch uint16, body []byte) []byte {
	w := []byte{22, 0xfe, 0xfd, byte(epoch >> 8), byte(epoch), 0, 0, 0, 0, 0, 0}
	w = binary.BigEndian.AppendUint16(w, uint16(len(body)))
	return append(w, body...)
}
func TestDTLSRuleKeepsEncryptedAndPartialFragmentsOpaque(t *testing.T) {
	// Epoch-one bytes intentionally look like a plaintext ClientHello. Neither
	// VM nor prepared rule may invent a handshake from ciphertext.
	clear := []byte{1, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 2, 0xfe, 0xfd}
	for _, epoch := range []uint16{0, 1} {
		wire := dtlsRuleRecord(epoch, clear)
		val := parseRule(t, wire, "dtls", "DTLS")
		fragment := val.Child("Fragment")
		require.NotNil(t, fragment)
		if epoch == 0 {
			require.EqualValues(t, 0xfefd, uintVal(t, fragment.Child("Client Version")))
		} else {
			require.Nil(t, fragment.Child("Handshake Type"))
			require.Equal(t, clear, bytesVal(t, fragment.Child("Handshake Body")))
		}
		reference, err := structuredNodeReference(wire, "dtls", "DTLS")
		require.NoError(t, err)
		got, err := ParseStructured(wire, "dtls", "DTLS")
		require.NoError(t, err)
		require.Equal(t, reference, got)
		_, err = parser.ParseBinary(bytes.NewReader(wire[:len(wire)-1]), "dtls", "DTLS")
		require.Error(t, err)
	}
	partial := bytes.Clone(clear)
	partial[3] = 4
	partial[8] = 2
	val := parseRule(t, dtlsRuleRecord(0, partial), "dtls", "DTLS")
	require.Nil(t, val.Child("Fragment").Child("Client Version"))
	invalid := bytes.Clone(clear)
	invalid[8] = 2 // offset 2 + length 2 escapes total 2
	_, err := parser.ParseBinary(bytes.NewReader(dtlsRuleRecord(0, invalid)), "dtls", "DTLS")
	require.Error(t, err)
	invalid = dtlsRuleRecord(0, clear)
	invalid[2] = 0xfc
	_, err = parser.ParseBinary(bytes.NewReader(invalid), "dtls", "DTLS")
	require.Error(t, err)
}
