package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func utf16LE(s string) []byte {
	out := make([]byte, len(s)*2)
	for i, c := range s {
		out[i*2] = byte(c)
	}
	return out
}

func TestSMB2TreeConnectCreateReadWriteClose(t *testing.T) {
	path := utf16LE(`\\srv\share`)
	tcBody := make([]byte, 8+len(path))
	binary.LittleEndian.PutUint16(tcBody[0:], 9)
	binary.LittleEndian.PutUint16(tcBody[4:], 72)
	binary.LittleEndian.PutUint16(tcBody[6:], uint16(len(path)))
	copy(tcBody[8:], path)
	raw := append(smb2SyncHeader(3, 0, 4), tcBody...)
	smb := parseRule(t, raw, "application-layer.smb2", "SMB2")
	req := mustChild(t, smb, "Tree Connect Request")
	require.Equal(t, uint64(9), uintVal(t, req.Child("StructureSize")))
	require.Equal(t, path, bytesVal(t, req.Child("Path")))

	tcr := make([]byte, 16)
	binary.LittleEndian.PutUint16(tcr[0:], 16)
	tcr[2] = 1 // disk
	raw = append(smb2SyncHeader(3, 1, 4), tcr...)
	resp := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Tree Connect Response")
	require.Equal(t, uint64(1), uintVal(t, resp.Child("ShareType")))

	name := utf16LE("file.txt")
	cr := make([]byte, 56+len(name))
	binary.LittleEndian.PutUint16(cr[0:], 57)
	binary.LittleEndian.PutUint32(cr[4:], 2) // impersonation
	binary.LittleEndian.PutUint32(cr[24:], 0x00120189)
	binary.LittleEndian.PutUint32(cr[36:], 1) // FILE_OPEN
	binary.LittleEndian.PutUint16(cr[44:], 120)
	binary.LittleEndian.PutUint16(cr[46:], uint16(len(name)))
	copy(cr[56:], name)
	raw = append(smb2SyncHeader(5, 0, 5), cr...)
	creq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Create Request")
	require.Equal(t, uint64(57), uintVal(t, creq.Child("StructureSize")))
	require.Equal(t, name, bytesVal(t, creq.Child("Name")))
	require.Equal(t, uint64(1), uintVal(t, creq.Child("CreateDisposition")))

	fid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	cresp := make([]byte, 88)
	binary.LittleEndian.PutUint16(cresp[0:], 89)
	binary.LittleEndian.PutUint32(cresp[4:], 1)
	copy(cresp[64:80], fid)
	raw = append(smb2SyncHeader(5, 1, 5), cresp...)
	crr := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Create Response")
	require.Equal(t, uint64(89), uintVal(t, crr.Child("StructureSize")))
	require.Equal(t, fid, bytesVal(t, crr.Child("FileId")))

	rd := make([]byte, 48)
	binary.LittleEndian.PutUint16(rd[0:], 49)
	binary.LittleEndian.PutUint32(rd[4:], 8)
	copy(rd[16:32], fid)
	raw = append(smb2SyncHeader(8, 0, 6), rd...)
	rreq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Read Request")
	require.Equal(t, uint64(8), uintVal(t, rreq.Child("Length")))

	rdata := []byte("abcdefgh")
	rr := make([]byte, 16+len(rdata))
	binary.LittleEndian.PutUint16(rr[0:], 17)
	rr[2] = 80
	binary.LittleEndian.PutUint32(rr[4:], uint32(len(rdata)))
	copy(rr[16:], rdata)
	raw = append(smb2SyncHeader(8, 1, 6), rr...)
	rresp := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Read Response")
	require.Equal(t, rdata, bytesVal(t, rresp.Child("Data")))

	wr := make([]byte, 48+len(rdata))
	binary.LittleEndian.PutUint16(wr[0:], 49)
	binary.LittleEndian.PutUint16(wr[2:], 112)
	binary.LittleEndian.PutUint32(wr[4:], uint32(len(rdata)))
	copy(wr[16:32], fid)
	copy(wr[48:], rdata)
	raw = append(smb2SyncHeader(9, 0, 7), wr...)
	wreq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Write Request")
	require.Equal(t, rdata, bytesVal(t, wreq.Child("Data")))

	cl := make([]byte, 24)
	binary.LittleEndian.PutUint16(cl[0:], 24)
	copy(cl[8:24], fid)
	raw = append(smb2SyncHeader(6, 0, 8), cl...)
	closeReq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Close")
	require.Equal(t, uint64(24), uintVal(t, closeReq.Child("StructureSize")))

	loff := make([]byte, 4)
	binary.LittleEndian.PutUint16(loff[0:], 4)
	raw = append(smb2SyncHeader(2, 0, 9), loff...)
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Logoff", "StructureSize")))

	ioctl := make([]byte, 56)
	binary.LittleEndian.PutUint16(ioctl[0:], 57)
	binary.LittleEndian.PutUint32(ioctl[4:], 0x0011c017) // FSCTL_PIPE_TRANSCEIVE
	raw = append(smb2SyncHeader(11, 0, 10), ioctl...)
	io := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "IOCTL")
	require.Equal(t, uint64(0x0011c017), uintVal(t, io.Child("CtlCode")))

	echo := make([]byte, 4)
	binary.LittleEndian.PutUint16(echo[0:], 4)
	raw = append(smb2SyncHeader(13, 0, 11), echo...)
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Echo", "StructureSize")))

	td := make([]byte, 4)
	binary.LittleEndian.PutUint16(td[0:], 4)
	raw = append(smb2SyncHeader(4, 0, 12), td...)
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Tree Disconnect", "StructureSize")))

	parseMustFail(t, append(smb2SyncHeader(3, 0, 4), tcBody[:4]...), "application-layer.smb2", "SMB2")
}

func TestHTTP2SettingsPingGoaway(t *testing.T) {
	// SETTINGS with HEADER_TABLE_SIZE=4096 and ENABLE_PUSH=0
	st := make([]byte, 9+12)
	st[2] = 12
	st[3] = 4
	binary.BigEndian.PutUint16(st[9:], 1)
	binary.BigEndian.PutUint32(st[11:], 4096)
	binary.BigEndian.PutUint16(st[15:], 2)
	binary.BigEndian.PutUint32(st[17:], 0)
	n := parseRule(t, st, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(4), uintVal(t, n.Child("Type")))
	require.Equal(t, st[9:], bytesVal(t, n.Child("Settings")))

	ping := make([]byte, 9+8)
	ping[2] = 8
	ping[3] = 6
	copy(ping[9:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	p := parseRule(t, ping, "application-layer.http2", "HTTP2")
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, p.Child("Ping")))

	ga := make([]byte, 9+8)
	ga[2] = 8
	ga[3] = 7
	binary.BigEndian.PutUint32(ga[13:], 1) // NO_ERROR is 0; PROTOCOL_ERROR=1 at offset 9+4
	g := parseRule(t, ga, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(1), uintVal(t, g.Child("Error Code")))

	wu := make([]byte, 9+4)
	wu[2] = 4
	wu[3] = 8
	binary.BigEndian.PutUint32(wu[9:], 65535)
	w := parseRule(t, wu, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(65535), uintVal(t, w.Child("Window Size Increment")))

	parseMustFail(t, ping[:10], "application-layer.http2", "HTTP2")
	badSet := make([]byte, 9+5)
	badSet[2] = 5
	badSet[3] = 4
	parseMustFail(t, badSet, "application-layer.http2", "HTTP2")
}

func TestMQTTPublishSubscribePing(t *testing.T) {
	// PUBLISH qos0 topic "a" payload skipped
	pub := []byte{0x30, 0x03, 0x00, 0x01, 'a'}
	p := parseRule(t, pub, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(3), uintVal(t, p.Child("Packet Type")))
	require.Equal(t, "a", strVal(t, mustChild(t, p, "Payload", "Publish", "Topic")))

	sub := []byte{0x82, 0x02, 0x00, 0x07}
	s := parseRule(t, sub, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(8), uintVal(t, s.Child("Packet Type")))
	require.Equal(t, uint64(7), uintVal(t, mustChild(t, s, "Payload", "Subscribe", "Packet ID")))

	ping := []byte{0xc0, 0x00}
	pg := parseRule(t, ping, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(12), uintVal(t, pg.Child("Packet Type")))

	pong := []byte{0xd0, 0x00}
	require.Equal(t, uint64(13), uintVal(t, parseRule(t, pong, "application-layer.mqtt", "MQTT").Child("Packet Type")))

	puback := []byte{0x40, 0x02, 0x00, 0x01}
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, puback, "application-layer.mqtt", "MQTT"), "Payload", "Packet ID")))

	parseMustFail(t, []byte{0x30, 0x05, 0x00, 0x10}, "application-layer.mqtt", "MQTT")
}

func TestDCERPCBindAckFaultAndSMB1AndX(t *testing.T) {
	ackBody := make([]byte, 10)
	binary.LittleEndian.PutUint16(ackBody[0:], 5840)
	binary.LittleEndian.PutUint16(ackBody[2:], 5840)
	binary.LittleEndian.PutUint16(ackBody[8:], 0)
	raw := append(dcerpcHeader(12, uint16(16+len(ackBody)), 1), ackBody...)
	n := parseRule(t, raw, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(12), uintVal(t, n.Child("PType")))
	require.Equal(t, uint64(5840), uintVal(t, mustChild(t, n, "PDU", "BindAck", "Max Xmit Frag")))

	nak := append(dcerpcHeader(13, 18, 1), 0, 0)
	require.Equal(t, uint64(13), uintVal(t, parseRule(t, nak, "application-layer.dcerpc", "DCERPC").Child("PType")))

	faultBody := make([]byte, 12)
	binary.LittleEndian.PutUint32(faultBody[8:], 0x00000005)
	fr := append(dcerpcHeader(3, 28, 2), faultBody...)
	f := parseRule(t, fr, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(5), uintVal(t, mustChild(t, f, "PDU", "Fault", "Status")))

	respBody := make([]byte, 8)
	rr := append(dcerpcHeader(2, 24, 2), respBody...)
	require.Equal(t, uint64(2), uintVal(t, parseRule(t, rr, "application-layer.dcerpc", "DCERPC").Child("PType")))

	// SMB1 Tree Connect AndX (0x75)
	body := []byte{4, 0xff, 0, 0, 0, 0, 0}
	raw = append(smb1Header(0x75, 0x18, 0xc807, 2), body...)
	smb := parseRule(t, raw, "application-layer.smb", "SMB")
	require.Equal(t, uint64(0x75), uintVal(t, smb.Child("Command")))
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, smb, "AndX", "WordCount")))
}
