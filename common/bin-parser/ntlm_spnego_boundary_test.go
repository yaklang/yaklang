package bin_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func spnegoNTLMInit() []byte {
	// RFC 4178 section 4.2.1: NegTokenInit contains a nonempty mechTypes list.
	return []byte{0x60, 0x1c, 0x06, 0x06, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x02, 0xa0, 0x12, 0x30, 0x10, 0xa0, 0x0e, 0x30, 0x0c, 0x06, 0x0a, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}
}

func TestNTLMAndSPNEGOStreamAndMessageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, rule, key string
		input           []byte
	}{
		{"negotiate", "application-layer.ntlm", "NTLMSSP", ntlmsspMessage(1, []byte{0x07, 0x82, 0x08, 0xe2})},
		{"authenticate", "application-layer.ntlm", "NTLMSSP", ntlmsspAuthUser("CORP", "Admin")},
		{"spnego", "application-layer.spnego", "SPNEGO", spnegoNTLMInit()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A plain reader is an open stream, not a bounded zero-length message.
			// Parse one message without allocating an unsigned-underflow remainder
			// or consuming the following message's sentinel bytes.
			suffix := []byte{0xde, 0xad, 0xbe, 0xef}
			stream := bytes.NewReader(append(bytes.Clone(tc.input), suffix...))
			node, err := parser.ParseBinary(stream, tc.rule, tc.key)
			require.NoError(t, err)
			_, err = node.Result()
			require.NoError(t, err)
			require.Equal(t, len(suffix), stream.Len())
			protocolCorpusRequireBoundedRuleParse(t, tc.input, tc.rule, tc.key)
			for cut := 0; cut < len(tc.input); cut++ {
				t.Run(fmt.Sprintf("short-%d", cut), func(t *testing.T) {
					_, err := parser.ParseBinary(bytes.NewReader(tc.input[:cut]), tc.rule, tc.key)
					require.Error(t, err)
					_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(tc.input[:cut]), tc.rule, tc.key)
					require.Error(t, err)
				})
			}
		})
	}
}

func TestSPNEGONestedLengthValidation(t *testing.T) {
	for _, offset := range []int{1, 3, 11, 13, 15, 17, 19} {
		for _, delta := range []byte{1, 255} {
			t.Run(fmt.Sprintf("length-%d-delta-%d", offset, delta), func(t *testing.T) {
				input := spnegoNTLMInit()
				input[offset] += delta
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input), "application-layer.spnego", "SPNEGO")
				require.Error(t, err)
			})
		}
	}
	input := append(spnegoNTLMInit(), 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(input), "application-layer.spnego", "SPNEGO")
	require.ErrorContains(t, err, "declared length differs from token boundary")
	// Empty [0] is not a NegTokenInit SEQUENCE.
	input = spnegoNTLMInit()[:12]
	input[1], input[11] = 10, 0
	parseMustFail(t, input, "application-layer.spnego", "SPNEGO")
}

func TestSPNEGOAdditionalMechanismAndOpaqueOptionalFields(t *testing.T) {
	input := spnegoNTLMInit()
	secondOID := []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 0x01, 0x02, 0x02}
	for _, offset := range []int{1, 11, 13, 15, 17} {
		input[offset] += byte(len(secondOID))
	}
	input = append(input, secondOID...)
	optional := []byte{0xa2, 0x04, 0x04, 0x02, 0x01, 0x02}
	for _, offset := range []int{1, 11, 13} {
		input[offset] += byte(len(optional))
	}
	input = append(input, optional...)
	parsed := parseRule(t, input, "application-layer.spnego", "SPNEGO")
	init := mustChild(t, parsed, "Token", "SPNEGOInit")
	require.Equal(t, secondOID[2:], bytesVal(t, mustChild(t, init, "Additional MechOID", "OID")))
	require.Equal(t, optional, bytesVal(t, init.Child("Optional Fields")))
	protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.spnego", "SPNEGO")
	// An additional OID cannot consume bytes from the optional-field region.
	input[31]++
	parseMustFail(t, input, "application-layer.spnego", "SPNEGO")
}

func TestNBSSStreamAndMessageBoundaries(t *testing.T) {
	input := nbssSession(smb2NegotiateRequest(0x0202))
	stream := bytes.NewReader(append(bytes.Clone(input), 0x85, 0, 0, 0))
	node, err := parser.ParseBinary(stream, "application-layer.nbss", "NBSS")
	require.NoError(t, err)
	parsed, err := node.Result()
	require.NoError(t, err)
	require.NotNil(t, parsed.Child("SMB2"))
	require.Equal(t, 4, stream.Len())
	_, err = parser.ParseBinary(stream, "application-layer.nbss", "NBSS")
	require.NoError(t, err)
	require.Zero(t, stream.Len())
	protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.nbss", "NBSS")
	for cut := 0; cut < len(input); cut++ {
		t.Run(fmt.Sprintf("short-%d", cut), func(t *testing.T) {
			_, err := parser.ParseBinary(bytes.NewReader(input[:cut]), "application-layer.nbss", "NBSS")
			require.Error(t, err)
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(input[:cut]), "application-layer.nbss", "NBSS")
			require.Error(t, err)
		})
	}
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(input), 0)), "application-layer.nbss", "NBSS")
	require.ErrorContains(t, err, "declared length differs from record boundary")
}
