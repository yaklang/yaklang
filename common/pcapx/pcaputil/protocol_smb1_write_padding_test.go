package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MS-CIFS 2.2.4.43.1 allows one alignment Pad byte between the fixed words and
// Data, so ByteCount may exceed DataLength by one.
func TestSMB1WriteAndXPaddedDataIsDecoded(t *testing.T) {
	base := smb1LayoutFixture(t, "write-andx-request.bin")
	require.Len(t, base, 74)
	padded := bytes.Clone(base[:67])
	padded = append(padded, 0x00) // alignment pad at the fixed-body boundary
	padded = append(padded, base[67:]...)
	binary.LittleEndian.PutUint16(padded[59:61], 64) // DataOffset is header-relative; 4+64=68
	binary.LittleEndian.PutUint16(padded[65:67], 8)  // ByteCount includes the pad byte
	payload := len(padded) - 4
	padded[1], padded[2], padded[3] = byte(payload>>16), byte(payload>>8), byte(payload)

	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	result := session.Feed(0, time.Unix(1, 0), padded)
	require.Nil(t, result.Err)
	body := result.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, []byte{0x00}, body["Alignment Pad"])
	require.Equal(t, []byte("abc\x00\x00\x00\x00"), body["Data"])
	require.Equal(t, []any{uint64(68), uint64(75)}, body["Wire Field Ranges"].(map[string]any)["Data"])
}

// A zero-length write has no Data and no pad; DataOffset is zero.
func TestSMB1WriteAndXZeroLengthDataIsDecoded(t *testing.T) {
	base := smb1LayoutFixture(t, "write-andx-request.bin")
	zero := bytes.Clone(base[:67])
	binary.LittleEndian.PutUint16(zero[57:59], 0) // Data Length Low
	binary.LittleEndian.PutUint16(zero[59:61], 0) // Data Offset
	binary.LittleEndian.PutUint16(zero[65:67], 0) // ByteCount
	payload := len(zero) - 4
	zero[1], zero[2], zero[3] = byte(payload>>16), byte(payload>>8), byte(payload)

	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	result := session.Feed(0, time.Unix(1, 0), zero)
	require.Nil(t, result.Err)
	body := result.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, uint16(0), body["Data Length Low"])
	require.NotContains(t, body, "Data")
	require.NotContains(t, body, "Alignment Pad")
}

// A WC42 extended NT_CREATE_ANDX response carries 32 extra bytes after the
// standard words, before ByteCount (MS-SMB 2.2.4.9.2).
func TestSMB1NTCreateAndXExtendedResponseIsDecoded(t *testing.T) {
	request := smb1LayoutFixture(t, "nt-create-request.bin")
	standard := smb1LayoutFixture(t, "nt-create-response.bin")
	extended := bytes.Clone(standard[:105]) // through the Directory byte, before Byte Count
	guid := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xf0, 0x01}
	extra := make([]byte, 32)
	copy(extra[0:16], guid)
	binary.LittleEndian.PutUint64(extra[16:24], 0x1122334455667788)
	binary.LittleEndian.PutUint32(extra[24:28], 0x001f01ff)
	binary.LittleEndian.PutUint32(extra[28:32], 0x00120089)
	extended = append(extended, extra...)
	extended = append(extended, 0x00, 0x00) // Byte Count
	extended[36] = 42                       // declared WordCount for the extended form
	payload := len(extended) - 4
	extended[1], extended[2], extended[3] = byte(payload>>16), byte(payload>>8), byte(payload)
	require.Len(t, extended, 139)

	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	opened := session.Feed(0, time.Unix(1, 0), request)
	require.Nil(t, opened.Err)
	result := session.Feed(1, time.Unix(2, 0), extended)
	require.Nil(t, result.Err)
	body := result.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, guid, body["Volume GUID"])
	require.Equal(t, uint64(0x1122334455667788), body["File ID"])
	require.Equal(t, uint32(0x001f01ff), body["Maximal Access"])
	require.Equal(t, uint32(0x00120089), body["Guest Maximal Access"])
	require.Equal(t, uint16(0), body["Byte Count"])
	require.Equal(t, uint16(0x4000), body["FID"])
	require.Equal(t, byte(0), body["Directory"])
	require.Equal(t, "matched observed request; authentication unverified", result.Events[0].Session["Association Status"])
}

func TestSMB1FileCommandProbeAcceptsEmptyErrorReply(t *testing.T) {
	base := smb1LayoutFixture(t, "nt-create-response.bin")
	errorReply := bytes.Clone(base[:39])
	binary.LittleEndian.PutUint32(errorReply[9:13], 0xc0000022) // STATUS_ACCESS_DENIED
	errorReply[13] |= 0x80                                      // response flag
	errorReply[36] = 0
	binary.LittleEndian.PutUint16(errorReply[37:39], 0)
	payload := len(errorReply) - 4
	errorReply[1], errorReply[2], errorReply[3] = byte(payload>>16), byte(payload>>8), byte(payload)

	probe := probeSMB1FileCommand(errorReply, 65582)
	require.Equal(t, ProbeAccept, probe.Verdict, "%s", probe.Reason)
}
