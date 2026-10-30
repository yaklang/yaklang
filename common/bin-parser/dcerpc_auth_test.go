package bin_parser

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func ntlmType3Token(domain, user, workstation string) []byte {
	encode := func(s string) []byte {
		units := utf16.Encode([]rune(s))
		out := make([]byte, len(units)*2)
		for i, unit := range units {
			binary.LittleEndian.PutUint16(out[i*2:], unit)
		}
		return out
	}
	domainBytes, userBytes, workstationBytes := encode(domain), encode(user), encode(workstation)
	token := make([]byte, 64, 64+len(domainBytes)+len(userBytes)+len(workstationBytes))
	copy(token[:8], "NTLMSSP\x00")
	binary.LittleEndian.PutUint32(token[8:12], 3)
	binary.LittleEndian.PutUint32(token[60:64], 1) // NTLMSSP_NEGOTIATE_UNICODE
	add := func(descriptor int, value []byte) {
		offset := len(token)
		binary.LittleEndian.PutUint16(token[descriptor:descriptor+2], uint16(len(value)))
		binary.LittleEndian.PutUint16(token[descriptor+2:descriptor+4], uint16(len(value)))
		binary.LittleEndian.PutUint32(token[descriptor+4:descriptor+8], uint32(offset))
		token = append(token, value...)
	}
	add(28, domainBytes)
	add(36, userBytes)
	add(44, workstationBytes)
	return token
}

func dcerpcAuthTrailer(authType, authLevel, pad byte, value []byte) []byte {
	trailer := make([]byte, 8+len(value))
	trailer[0] = authType
	trailer[1] = authLevel
	trailer[2] = pad
	copy(trailer[8:], value)
	return trailer
}

func dcerpcAttachAuth(pdu []byte, authType, authLevel, pad byte, value []byte) []byte {
	pdu = append([]byte(nil), pdu...)
	body := append([]byte(nil), pdu[16:]...)
	body = append(body, make([]byte, pad)...)
	body = append(body, dcerpcAuthTrailer(authType, authLevel, pad, value)...)
	binary.LittleEndian.PutUint16(pdu[8:10], uint16(16+len(body)))
	binary.LittleEndian.PutUint16(pdu[10:12], uint16(len(value)))
	return append(pdu[:16], body...)
}

func TestDCERPCStructuredAuthTrailerAndAuth3(t *testing.T) {
	bind := dcerpcBindUUID([]byte{
		0x81, 0xbb, 0x7a, 0x36, 0x44, 0x98, 0xf1, 0x35,
		0xad, 0x32, 0x98, 0xf0, 0x38, 0x00, 0x10, 0x03,
	})
	authValue := make([]byte, 40)
	for i := range authValue {
		authValue[i] = byte(i + 1)
	}
	bind = dcerpcAttachAuth(bind, 10, 6, 0, authValue)

	result, err := parser.ParseStructured(bind, "application-layer.dcerpc", "DCERPC")
	require.NoError(t, err)
	fields, ok := result["fields"].(map[string]any)
	require.True(t, ok)
	pdu, ok := fields["PDU"].(map[string]any)
	require.True(t, ok)
	bindFields, ok := pdu["Bind"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, uint64(1), bindFields["Num Ctx Items"])
	auth, ok := pdu["Auth Verifier"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, uint64(10), auth["Auth Type"])
	require.Equal(t, uint64(6), auth["Auth Level"])
	require.Equal(t, uint64(0), auth["Auth Pad Length"])
	require.Equal(t, authValue, auth["Auth Value"])

	request := dcerpcRequestOp(15, []byte{0x11, 0x22, 0x33, 0x44})
	request = dcerpcAttachAuth(request, 10, 6, 8, authValue)
	result, err = parser.ParseStructured(request, "application-layer.dcerpc", "DCERPC")
	require.NoError(t, err)
	fields = result["fields"].(map[string]any)
	pdu = fields["PDU"].(map[string]any)
	requestFields := pdu["Request"].(map[string]any)
	require.Equal(t, uint64(15), requestFields["OpNum"])
	auth = pdu["Auth Verifier"].(map[string]any)
	require.Equal(t, uint64(8), auth["Auth Pad Length"])

	auth3 := append(dcerpcHeader(16, 20, 2), make([]byte, 4)...)
	binary.LittleEndian.PutUint16(auth3[16:18], 5840)
	binary.LittleEndian.PutUint16(auth3[18:20], 5840)
	auth3 = dcerpcAttachAuth(auth3, 10, 6, 0, authValue)
	result, err = parser.ParseStructured(auth3, "application-layer.dcerpc", "DCERPC")
	require.NoError(t, err)
	fields = result["fields"].(map[string]any)
	pdu = fields["PDU"].(map[string]any)
	auth3Fields := pdu["Auth3"].(map[string]any)
	require.Equal(t, uint64(5840), auth3Fields["Max Xmit Frag"])
	require.Equal(t, uint64(5840), auth3Fields["Max Recv Frag"])
}

func TestDCERPCStructuredNTLMType3Fields(t *testing.T) {
	token := ntlmType3Token("THREEBEESCO", "alice", "WORKSTATION")
	bind := dcerpcBindUUID([]byte{
		0x81, 0xbb, 0x7a, 0x36, 0x44, 0x98, 0xf1, 0x35,
		0xad, 0x32, 0x98, 0xf0, 0x38, 0x00, 0x10, 0x03,
	})
	bind = dcerpcAttachAuth(bind, 10, 6, 0, token)

	result, err := parser.ParseStructured(bind, "application-layer.dcerpc", "DCERPC")
	require.NoError(t, err)
	auth := result["fields"].(map[string]any)["PDU"].(map[string]any)["Auth Verifier"].(map[string]any)
	ntlm, ok := auth["NTLMSSP"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, uint32(3), ntlm["Message Type"])
	require.Equal(t, "THREEBEESCO", ntlm["Domain"])
	require.Equal(t, "alice", ntlm["User"])
	require.Equal(t, "WORKSTATION", ntlm["Workstation"])
}
