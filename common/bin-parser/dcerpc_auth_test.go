package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

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
