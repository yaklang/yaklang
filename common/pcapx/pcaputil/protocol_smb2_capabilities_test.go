package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func smb2EchoRequest(mid, sid uint64, tid uint32) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint16(body[0:], 4)
	return smb2TCP(append(smb2Hdr(13, 0, mid, sid, tid), body...))
}

func smb2FlushRequest(mid, sid uint64, tid uint32) []byte {
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 24)
	binary.LittleEndian.PutUint32(body[4:], 0)
	return smb2TCP(append(smb2Hdr(7, 0, mid, sid, tid), body...))
}

func smb2FlushResponse(mid, sid uint64, tid uint32) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint16(body[0:], 4)
	return smb2TCP(append(smb2Hdr(7, 1, mid, sid, tid), body...))
}

func smb2LockRequest(mid, sid uint64, tid uint32, fid []byte) []byte {
	body := make([]byte, 48)
	binary.LittleEndian.PutUint16(body[0:], 48)
	binary.LittleEndian.PutUint16(body[2:], 1)
	binary.LittleEndian.PutUint32(body[4:], 1)
	copy(body[8:24], fid)
	binary.LittleEndian.PutUint64(body[24:], 0)
	binary.LittleEndian.PutUint64(body[32:], 16)
	binary.LittleEndian.PutUint32(body[40:], 1)
	return smb2TCP(append(smb2Hdr(10, 0, mid, sid, tid), body...))
}

func smb2SetInfoRequest(mid, sid uint64, tid uint32, fid, buffer []byte) []byte {
	body := make([]byte, 32+len(buffer))
	binary.LittleEndian.PutUint16(body[0:], 33)
	body[2] = 1
	body[3] = 0x14
	binary.LittleEndian.PutUint32(body[4:], uint32(len(buffer)))
	binary.LittleEndian.PutUint16(body[8:], 96)
	copy(body[16:32], fid)
	copy(body[32:], buffer)
	return smb2TCP(append(smb2Hdr(17, 0, mid, sid, tid), body...))
}

func smb2OplockBreakRequest(mid, sid uint64, tid uint32, fid []byte) []byte {
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 24)
	body[2] = 1
	copy(body[8:24], fid)
	return smb2TCP(append(smb2Hdr(18, 0, mid, sid, tid), body...))
}

func smb2ChangeNotifyRequest(mid, sid uint64, tid uint32, fid []byte) []byte {
	body := make([]byte, 32)
	binary.LittleEndian.PutUint16(body[0:], 32)
	copy(body[8:24], fid)
	binary.LittleEndian.PutUint32(body[24:], 0x17)
	return smb2TCP(append(smb2Hdr(15, 0, mid, sid, tid), body...))
}

// A conformant interim async response carries the ERROR body. The association
// must still learn the AsyncId and match the final command-shaped response.
func TestSMB2StatusPendingErrorBodyLearnsAsyncID(t *testing.T) {
	fid := bytes.Repeat([]byte{0x21}, 16)
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	require.Nil(t, session.Feed(0, time.Unix(1, 0), smb2Read(7, 0x11, 1, fid, 4)).Err)

	interim := smb2AsyncWire(smb2ErrorResponse(8, 7, 0x11, 1, 0x00000103, nil), 0xfeed0001)
	info := smb2ChainFeed(t, session, 1, interim)
	require.Equal(t, true, info["Operation Pending"])
	require.Equal(t, "error", info["Pending Body Layout"])
	require.Equal(t, uint64(0xfeed0001), info["Async ID"])
	state := session.(*captureSession).f.smb2
	require.Len(t, state.pending, 1)
	for _, pending := range state.pending {
		require.True(t, pending.async)
		require.Equal(t, uint64(0xfeed0001), pending.asyncID)
	}

	final := smb2AsyncWire(smb2ReadResp(7, 0x11, 0, []byte("abcd")), 0xfeed0001)
	info = smb2ChainFeed(t, session, 1, final)
	require.Equal(t, "matched", info["Association Status"])
	require.Empty(t, state.pending)
	require.Empty(t, state.async)
}

// A non-final compound CANCEL is padded to the 8-byte NextCommand boundary.
func TestSMB2CancelCompoundPaddingIsDecoded(t *testing.T) {
	pdu := smb2Cancel(12, 0x11, 1)[4:]
	require.Len(t, pdu, 68)
	padded := append(bytes.Clone(pdu), 0, 0, 0, 0)
	info, err := smb2ParseCommand(padded, 64, true)
	require.NoError(t, err)
	body := info["Body"].(map[string]any)
	require.Equal(t, []byte{0, 0, 0, 0}, body["Padding"])
	require.Equal(t, uint16(4), body["Structure Size"])

	_, err = smb2ParseCommand(padded, 64, false)
	require.Error(t, err, "a standalone CANCEL must still be exactly 4 bytes")
}

func TestSMB2RemainingCommandBodiesAreBounded(t *testing.T) {
	fid := bytes.Repeat([]byte{0x31}, 16)
	for _, tc := range []struct {
		name string
		wire []byte
		want string
	}{
		{"flush-request", smb2FlushRequest(1, 0x11, 1)[4:], "FLUSH"},
		{"flush-response", smb2FlushResponse(1, 0x11, 1)[4:], "FLUSH"},
		{"echo", smb2EchoRequest(1, 0x11, 1)[4:], "ECHO"},
		{"lock", smb2LockRequest(1, 0x11, 1, fid)[4:], "LOCK"},
		{"set-info", smb2SetInfoRequest(1, 0x11, 1, fid, []byte{1, 2, 3, 4})[4:], "SET_INFO"},
		{"oplock-break", smb2OplockBreakRequest(1, 0x11, 1, fid)[4:], "OPLOCK_BREAK"},
		{"change-notify", smb2ChangeNotifyRequest(1, 0x11, 1, fid)[4:], "CHANGE_NOTIFY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commands, err := smb2Walk(tc.wire, 64)
			require.NoError(t, err)
			require.Len(t, commands, 1)
			require.Equal(t, tc.want, commands[0]["Packet Name"])
			require.Contains(t, commands[0], "Body")
		})
	}
}

// A body-level collection overflow is a record diagnostic, not a session
// stop: the next complete Direct TCP record must still decode.
func TestSMB2CollectionOverflowKeepsFlowAlive(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	count := DefaultParserBudget().MaxCollectionElements + 1
	body := make([]byte, 36+2*count)
	binary.LittleEndian.PutUint16(body[0:], 36)
	binary.LittleEndian.PutUint16(body[2:], uint16(count))
	binary.LittleEndian.PutUint16(body[4:], 1)
	wire := smb2TCP(append(smb2Hdr(0, 0, 1, 0, 0), body...))
	r := session.Feed(0, time.Unix(1, 0), wire)
	require.Len(t, r.Events, 1)
	require.Equal(t, "smb2-record-body-diagnostic", r.Events[0].Profile)
	require.Equal(t, "headers", r.Events[0].Completeness)

	next := session.Feed(0, time.Unix(2, 0), smb2EchoRequest(2, 0, 0))
	require.Nil(t, next.Err)
	require.Len(t, next.Events, 1)
	require.Equal(t, "decoded", next.Events[0].Status)
	require.Equal(t, "ECHO", next.Events[0].Session["Packet Name"])
}
