package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// SMB2 packet header layout: [MS-SMB2] 2.2.1.2 (SYNC).
func smb2SyncHeader(command uint16, flags uint32, messageID uint64) []byte {
	h := make([]byte, 64)
	binary.LittleEndian.PutUint32(h[0:], 0x424d53fe) // \xfeSMB
	binary.LittleEndian.PutUint16(h[4:], 64)
	binary.LittleEndian.PutUint16(h[12:], command)
	binary.LittleEndian.PutUint16(h[14:], 1)
	binary.LittleEndian.PutUint32(h[16:], flags)
	binary.LittleEndian.PutUint64(h[24:], messageID)
	return h
}

// SMB2 NEGOTIATE request: [MS-SMB2] 2.2.3
func smb2NegotiateRequest(dialects ...uint16) []byte {
	body := make([]byte, 36+2*len(dialects))
	binary.LittleEndian.PutUint16(body[0:], 36)
	binary.LittleEndian.PutUint16(body[2:], uint16(len(dialects)))
	binary.LittleEndian.PutUint16(body[4:], 0x0001) // signing enabled
	copy(body[12:28], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00})
	for i, d := range dialects {
		binary.LittleEndian.PutUint16(body[36+2*i:], d)
	}
	return append(smb2SyncHeader(0, 0, 1), body...)
}

// SMB2 NEGOTIATE response: [MS-SMB2] 2.2.4
func smb2NegotiateResponse(dialect uint16) []byte {
	body := make([]byte, 64)
	binary.LittleEndian.PutUint16(body[0:], 65)
	binary.LittleEndian.PutUint16(body[2:], 0x0001)
	binary.LittleEndian.PutUint16(body[4:], dialect)
	copy(body[8:24], []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
	binary.LittleEndian.PutUint32(body[24:], 0x00000001) // DFS
	binary.LittleEndian.PutUint32(body[28:], 0x00800000)
	binary.LittleEndian.PutUint32(body[32:], 0x00800000)
	binary.LittleEndian.PutUint32(body[36:], 0x00800000)
	binary.LittleEndian.PutUint16(body[56:], 128) // SecurityBufferOffset
	binary.LittleEndian.PutUint16(body[58:], 0)   // empty GSS blob
	return append(smb2SyncHeader(0, 0x00000001, 1), body...)
}

// SMB1 header: [MS-CIFS] 2.2.3.1
func smb1Header(command byte, flags byte, flags2, mid uint16) []byte {
	h := make([]byte, 32)
	binary.LittleEndian.PutUint32(h[0:], 0x424d53ff) // \xffSMB
	h[4] = command
	h[9] = flags
	binary.LittleEndian.PutUint16(h[10:], flags2)
	binary.LittleEndian.PutUint16(h[30:], mid)
	return h
}

// SMB1 NEGOTIATE request: [MS-CIFS] 2.2.4.52
func smb1NegotiateRequest(dialects ...string) []byte {
	var buf []byte
	for _, d := range dialects {
		buf = append(buf, 0x02)
		buf = append(buf, []byte(d)...)
		buf = append(buf, 0x00)
	}
	body := make([]byte, 3+len(buf))
	body[0] = 0 // WordCount
	binary.LittleEndian.PutUint16(body[1:], uint16(len(buf)))
	copy(body[3:], buf)
	return append(smb1Header(0x72, 0x18, 0xc807, 1), body...)
}

func nbssSession(payload []byte) []byte {
	h := []byte{0x00, 0x00, 0x00, 0x00}
	binary.BigEndian.PutUint16(h[2:], uint16(len(payload)))
	return append(h, payload...)
}

func TestSMB2NegotiateRequest(t *testing.T) {
	raw := smb2NegotiateRequest(0x0202, 0x0210, 0x0300, 0x0311)
	smb := parseRule(t, raw, "application-layer.smb2", "SMB2")
	require.Equal(t, uint64(0x424d53fe), uintVal(t, smb.Child("ProtocolId")))
	require.Equal(t, uint64(64), uintVal(t, smb.Child("StructureSize")))
	require.Equal(t, uint64(0), uintVal(t, smb.Child("Command")))
	require.Equal(t, uint64(0), uintVal(t, smb.Child("Flags")))
	req := mustChild(t, smb, "Negotiate Request")
	require.Equal(t, uint64(36), uintVal(t, req.Child("StructureSize")))
	require.Equal(t, uint64(4), uintVal(t, req.Child("DialectCount")))
	dialects := req.Child("Dialects")
	require.True(t, dialects.IsList())
	require.Len(t, dialects.Children(), 4)
	require.Equal(t, uint64(0x0202), uintVal(t, dialects.Children()[0]))
	require.Equal(t, uint64(0x0311), uintVal(t, dialects.Children()[3]))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, raw))
	wired := mustChild(t, eth, "IP", "TCP", "SMB2")
	require.Equal(t, uint64(0), uintVal(t, wired.Child("Command")))
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, wired, "Negotiate Request", "DialectCount")))
}

func TestSMB2NegotiateResponse(t *testing.T) {
	raw := smb2NegotiateResponse(0x0311)
	smb := parseRule(t, raw, "application-layer.smb2", "SMB2")
	require.Equal(t, uint64(1), uintVal(t, smb.Child("Flags"))&1)
	resp := mustChild(t, smb, "Negotiate Response")
	require.Equal(t, uint64(65), uintVal(t, resp.Child("StructureSize")))
	require.Equal(t, uint64(0x0311), uintVal(t, resp.Child("DialectRevision")))
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}, bytesVal(t, resp.Child("ServerGuid")))
}

func TestSMB2SessionSetupRequest(t *testing.T) {
	// [MS-SMB2] 2.2.5 SESSION_SETUP Request, empty security buffer.
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 25)
	body[2] = 0
	body[3] = 1 // signing enabled
	binary.LittleEndian.PutUint16(body[12:], 88)
	binary.LittleEndian.PutUint16(body[14:], 0)
	raw := append(smb2SyncHeader(1, 0, 2), body...)
	smb := parseRule(t, raw, "application-layer.smb2", "SMB2")
	require.Equal(t, uint64(1), uintVal(t, smb.Child("Command")))
	req := mustChild(t, smb, "Session Setup Request")
	require.Equal(t, uint64(25), uintVal(t, req.Child("StructureSize")))
	require.Equal(t, uint64(1), uintVal(t, req.Child("SecurityMode")))
}

func TestSMB1NegotiateRequest(t *testing.T) {
	raw := smb1NegotiateRequest("PC NETWORK PROGRAM 1.0", "LANMAN1.0", "NT LM 0.12")
	smb := parseRule(t, raw, "application-layer.smb", "SMB")
	require.Equal(t, uint64(0x424d53ff), uintVal(t, smb.Child("ProtocolId")))
	require.Equal(t, uint64(0x72), uintVal(t, smb.Child("Command")))
	neg := mustChild(t, smb, "Negotiate")
	require.Equal(t, uint64(0), uintVal(t, neg.Child("WordCount")))
	dialects := neg.Child("Dialects")
	require.True(t, dialects.IsList())
	require.Len(t, dialects.Children(), 3)
	require.Equal(t, "NT LM 0.12", strVal(t, dialects.Children()[2].Child("Name")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, raw))
	wired := mustChild(t, eth, "IP", "TCP", "SMB")
	require.Equal(t, uint64(0x72), uintVal(t, wired.Child("Command")))
}

func TestNBSSWrapsSMB2(t *testing.T) {
	inner := smb2NegotiateRequest(0x0202)
	raw := nbssSession(inner)
	nbss := parseRule(t, raw, "application-layer.nbss", "NBSS")
	require.Equal(t, uint64(0), uintVal(t, nbss.Child("Type")))
	require.Equal(t, uint64(len(inner)), uintVal(t, nbss.Child("Length")))
	smb := mustChild(t, nbss, "SMB2")
	require.Equal(t, uint64(0), uintVal(t, smb.Child("Command")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 139, raw))
	wired := mustChild(t, eth, "IP", "TCP", "NBSS", "SMB2")
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, wired, "Negotiate Request", "DialectCount")))
}

func TestSMB2RejectsSMB1Magic(t *testing.T) {
	raw := smb1NegotiateRequest("NT LM 0.12")
	_, err := parser.ParseBinary(bytes.NewReader(raw), "application-layer.smb2", "SMB2")
	require.Error(t, err)
}
