package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func smb1Words(values ...any) []byte {
	var out []byte
	for _, value := range values {
		switch n := value.(type) {
		case byte:
			out = append(out, n)
		case uint16:
			out = append(out, byte(n), byte(n>>8))
		case uint32:
			out = append(out, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
		default:
			panic(n)
		}
	}
	return out
}

func smb1Command(command, flags byte, flags2, tid, pid, uid, mid uint16, words, data []byte) []byte {
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header[0:4], 0x424d53ff)
	header[4] = command
	header[9] = flags
	binary.LittleEndian.PutUint16(header[10:12], flags2)
	binary.LittleEndian.PutUint16(header[24:26], tid)
	binary.LittleEndian.PutUint16(header[26:28], pid)
	binary.LittleEndian.PutUint16(header[28:30], uid)
	binary.LittleEndian.PutUint16(header[30:32], mid)
	body := append([]byte{byte(len(words) / 2)}, words...)
	body = append(body, byte(len(data)), byte(len(data)>>8))
	smb := append(append(header, body...), data...)
	return append([]byte{0, byte(len(smb) >> 16), byte(len(smb) >> 8), byte(len(smb))}, smb...)
}

func smb1Session(t *testing.T) ProtocolSession {
	t.Helper()
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { session.Close("test") })
	return session
}

func smb1FeedOK(t *testing.T, session ProtocolSession, dir int, pdu []byte) *ProtocolEvent {
	t.Helper()
	result := session.Feed(dir, time.Unix(int64(dir+1), 0), pdu)
	require.Nil(t, result.Err, "%v", result.Err)
	require.Len(t, result.Events, 1)
	return result.Events[0]
}

func TestSMB1Transaction2SecondaryReassembly(t *testing.T) {
	// Constructed from MS-CIFS 2.2.4.46.1 and 2.2.4.47.1. Not a capture frame.
	primaryWords := smb1Words(uint16(12), uint16(0), uint16(16), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	primaryData := append([]byte{0, 0, 0, 0x07, 0x01, 0, 0, 0, 0})
	primary := smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 7, primaryWords, primaryData)
	secondaryData := append([]byte{0, 0, 0, 0x61, 0x00, 0x62, 0x00, 0x00, 0x00})
	secondaryWords := smb1Words(uint16(12), uint16(0), uint16(6), uint16(56), uint16(6), uint16(0), uint16(0), uint16(0), uint16(0xffff))
	secondary := smb1Command(0x33, 0x08, 0x8000, 2048, 1, 2048, 7, secondaryWords, secondaryData)
	require.Equal(t, 78, len(primary))
	require.Equal(t, 66, len(secondary))

	session := smb1Session(t)
	first := smb1FeedOK(t, session, 0, primary)
	require.Contains(t, first.Session["Body"].(map[string]any)["Transaction Status"], "incomplete")
	require.NotContains(t, first.Session["Body"], "File Name")
	require.Equal(t, []any{uint64(72), uint64(78)}, first.Session["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Parameters"])

	done := smb1FeedOK(t, session, 0, secondary)
	require.Equal(t, "complete", done.Session["Reassembly Status"])
	require.Equal(t, "first complete observation", done.Session["Reassembly Publication"])
	require.Equal(t, "ab", done.Session["Reassembled File Name"])
	require.Len(t, done.Session["Reassembled Parameters"].([]byte), 12)
	require.Equal(t, uint16(6), done.Session["Body"].(map[string]any)["Parameter Displacement"])
	require.Equal(t, []any{uint64(60), uint64(66)}, done.Session["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Parameters"])
	require.NotContains(t, done.Session["Body"].(map[string]any)["Wire Field Ranges"], "Reassembled Parameters")

	again := smb1FeedOK(t, session, 0, secondary)
	require.Equal(t, "retransmit", again.Session["Reassembly Status"])
	require.Equal(t, "retransmit; logical buffer unchanged", again.Session["Reassembly Publication"])
	require.Equal(t, done.Session["Reassembled Parameters"], again.Session["Reassembled Parameters"])

	holeWords := smb1Words(uint16(12), uint16(0), uint16(4), uint16(56), uint16(8), uint16(0), uint16(0), uint16(0), uint16(0xffff))
	hole := smb1Command(0x33, 0x08, 0x8000, 2048, 1, 2048, 8, holeWords, []byte{0, 0, 0, 1, 2, 3, 4})
	holeSession := smb1Session(t)
	require.Nil(t, holeSession.Feed(0, time.Unix(1, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 8, primaryWords, primaryData)).Err)
	opened := smb1FeedOK(t, holeSession, 0, hole)
	require.Equal(t, "hole", opened.Session["Reassembly Status"])
	require.NotContains(t, opened.Session, "Reassembled File Name")

	conflictWords := smb1Words(uint16(12), uint16(0), uint16(4), uint16(56), uint16(4), uint16(0), uint16(0), uint16(0), uint16(0xffff))
	conflict := smb1Command(0x33, 0x08, 0x8000, 2048, 1, 2048, 9, conflictWords, []byte{0, 0, 0, 0xff, 0xff, 0xff, 0xff})
	conflictSession := smb1Session(t)
	require.Nil(t, conflictSession.Feed(0, time.Unix(1, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 9, primaryWords, primaryData)).Err)
	bad := smb1FeedOK(t, conflictSession, 0, conflict)
	require.Equal(t, "overlap conflict", bad.Session["Reassembly Status"])
	require.NotContains(t, bad.Session, "Reassembled File Name")

	short := secondary[:len(secondary)-1]
	short[3] = byte(len(short) - 4)
	truncated := smb1Session(t).Feed(0, time.Unix(1, 0), short)
	require.NotNil(t, truncated.Err)
}

func TestSMB1OEMUnicodeAndInformationLevels(t *testing.T) {
	// Constructed parameter/data blocks. MS-CIFS 2.2.1.1, 2.2.6.6.1, 2.2.8.4.1 and 2.2.8.3.8.
	oemWords := smb1Words(uint16(9), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(9), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	oemData := []byte{0, 0, 0, 0x07, 0x01, 0, 0, 0, 0, 'a', 'b', 0}
	oem := smb1Command(0x32, 0x08, 0, 2048, 1, 2048, 3, oemWords, oemData)
	unicodeData := []byte{0, 0, 0, 0x07, 0x01, 0, 0, 0, 0, 'a', 0, 'b', 0, 0, 0}
	unicodeWords := smb1Words(uint16(12), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(12), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	unicode := smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 4, unicodeWords, unicodeData)
	session := smb1Session(t)
	oemEvent := smb1FeedOK(t, session, 0, oem)
	require.Equal(t, "ab", oemEvent.Session["Body"].(map[string]any)["File Name"])
	require.Equal(t, []byte{'a', 'b', 0}, oemEvent.Session["Body"].(map[string]any)["File Name Raw Bytes"])
	require.Contains(t, oemEvent.Summary, "SMB1 SMB_COM_TRANSACTION2")
	require.Contains(t, oemEvent.Summary, "ab")
	masked := smb1Command(0x32, 0x08, 0, 2048, 1, 2048, 5, unicodeWords, unicodeData)
	maskedEvent := smb1FeedOK(t, session, 0, masked)
	require.NotContains(t, maskedEvent.Session["Body"], "File Name")
	require.Contains(t, maskedEvent.Session["Body"].(map[string]any)["File Name Status"], "OEM")
	unicodeEvent := smb1FeedOK(t, session, 0, unicode)
	require.Equal(t, "ab", unicodeEvent.Session["Body"].(map[string]any)["File Name"])

	record := make([]byte, 22)
	binary.LittleEndian.PutUint16(record[0:2], 0x0102)
	setWords := smb1Words(uint16(6), uint16(22), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(22), uint16(74), byte(1), byte(0), uint16(8))
	setData := append([]byte{0, 0, 0, 0x00, 0x40, 0x01, 0x00, 0, 0}, record...)
	setEvent := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 6, setWords, setData))
	info := setEvent.Session["Body"].(map[string]any)["Information Record"].(map[string]any)
	require.Equal(t, uint16(0x0102), info["CreationDate"])
	require.Equal(t, []any{uint64(78), uint64(80)}, info["Wire Field Ranges"].(map[string]any)["CreationDate"])
	require.Len(t, info["Reserved"].([]byte), 10)

	levelWords := smb1Words(uint16(10), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(10), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	levelData := []byte{0, 0, 0, 0x03, 0x01, 0, 0, 0, 0, 'a', 0, 0, 0}
	level := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 11, levelWords, levelData))
	require.Equal(t, uint16(0x0103), level.Session["Body"].(map[string]any)["Information Level"])
	responseWords := smb1Words(uint16(2), uint16(4), uint16(0), uint16(2), uint16(56), uint16(0), uint16(4), uint16(60), uint16(0), byte(0), byte(0))
	responseData := []byte{0, 0, 0, 0, 0, 7, 0, 0, 0}
	response := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 11, responseWords, responseData))
	ea := response.Session["Body"].(map[string]any)["Information Record"].(map[string]any)
	require.Equal(t, uint32(7), ea["EA Size"])
	require.Equal(t, "SMB_QUERY_FILE_EA_INFO", response.Session["Body"].(map[string]any)["Information Level Name"])
	shortData := []byte{0, 0, 0, 0, 0, 1, 2, 3}
	shortWords := smb1Words(uint16(2), uint16(3), uint16(0), uint16(2), uint16(56), uint16(0), uint16(3), uint16(60), uint16(0), byte(0), byte(0))
	other := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 12, levelWords, levelData))
	require.Equal(t, uint16(0x0103), other.Session["Body"].(map[string]any)["Information Level"])
	shortEvent := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 12, shortWords, shortData))
	require.NotContains(t, shortEvent.Session["Body"], "Information Record")
	require.Contains(t, shortEvent.Session["Body"].(map[string]any)["Data Status"], "opaque")
}

func TestSMB1ConstructedStandardAndDisposition(t *testing.T) {
	// MS-CIFS 2.2.8.3.7 is 22 bytes. MS-CIFS 2.2.8.4.4 is one byte.
	session := smb1Session(t)
	levelWords := smb1Words(uint16(10), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(10), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	levelData := []byte{0, 0, 0, 0x02, 0x01, 0, 0, 0, 0, 'a', 0, 0, 0}
	require.Nil(t, session.Feed(0, time.Unix(1, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 15, levelWords, levelData)).Err)
	data := make([]byte, 22)
	binary.LittleEndian.PutUint32(data[16:20], 3)
	data[21] = 1
	responseWords := smb1Words(uint16(2), uint16(22), uint16(0), uint16(2), uint16(56), uint16(0), uint16(22), uint16(60), uint16(0), byte(0), byte(0))
	responseData := append([]byte{0, 0, 0, 0, 0}, data...)
	event := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 15, responseWords, responseData))
	record := event.Session["Body"].(map[string]any)["Information Record"].(map[string]any)
	require.Equal(t, uint32(3), record["Number Of Links"])
	require.Equal(t, byte(1), record["Directory"])
	require.Equal(t, []any{uint64(64 + 16), uint64(64 + 20)}, record["Wire Field Ranges"].(map[string]any)["Number Of Links"])

	setWords := smb1Words(uint16(6), uint16(1), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(1), uint16(74), byte(1), byte(0), uint16(8))
	setData := []byte{0, 0, 0, 0x00, 0x40, 0x02, 0x01, 0, 0, 1}
	setEvent := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 16, setWords, setData))
	require.Equal(t, byte(1), setEvent.Session["Body"].(map[string]any)["Information Record"].(map[string]any)["Delete Pending"])
}

func TestSMBStateMatrixFragmentBudgetAndDisplay(t *testing.T) {
	pdu := string(smb2NegotiateReq(0x0311))
	split := len(pdu) / 2
	events := replaySMB(t, []tcpStep{
		{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
		{seq: 100, data: pdu[:split]},
		{seq: uint32(100 + split), data: pdu[split:]},
	})
	require.Equal(t, 1, smbCount(events, "NEGOTIATE"))

	retransmit := replaySMB(t, []tcpStep{
		{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
		{seq: 100, data: pdu}, {seq: 100, data: pdu},
	})
	require.Equal(t, 1, smbCount(retransmit, "NEGOTIATE"))

	var holeEvents []*ProtocolEvent
	holeErr := ReplayPcap(bytes.NewReader(binTestPcap(t, []tcpStep{
		{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
		{seq: 100, data: pdu[:8]}, {seq: 120, data: pdu[8:]},
	}, 445, false, true)), WithOnProtocolMessage(func(event *ProtocolEvent) {
		holeEvents = append(holeEvents, event)
	}))
	require.Error(t, holeErr)
	require.Contains(t, holeErr.Error(), "unfilled sequence gap")
	require.Equal(t, 0, smbCount(holeEvents, "NEGOTIATE"))

	session := smb1Session(t)
	require.Nil(t, session.Feed(0, time.Unix(1, 0), smb2NegotiateReq(0x0311)).Err)
	negotiated := smb1FeedOK(t, session, 1, smb2NegotiateResp(0x0311))
	require.Equal(t, uint16(0x0311), negotiated.Session["Dialect"].(uint16))
	require.Contains(t, negotiated.Summary, "NEGOTIATE")
	span := negotiated.Session["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Dialect"].([]any)
	require.Equal(t, []byte{0x11, 0x03}, negotiated.Raw[span[0].(uint64):span[1].(uint64)])
	before := negotiated.Session["Dialect"]
	negotiated.Session["Dialect"] = uint16(0)
	fresh, err := negotiated.GetFields()
	require.NoError(t, err)
	require.Equal(t, before, fresh["Dialect"])
	closePDU := smb2Close(9, 1, 1, bytes.Repeat([]byte{0xff}, 16))
	binary.LittleEndian.PutUint32(closePDU[4+16:4+20], smb2FlagRelated)
	refused := session.Feed(0, time.Unix(3, 0), closePDU)
	require.NotNil(t, refused.Err)
	require.Contains(t, refused.Err.Error(), "no predecessor")

	tight, err := NewProtocolSession(ParserBudget{MaxFrameBytes: len(pdu) - 1})
	require.NoError(t, err)
	defer tight.Close("test")
	limited := tight.Feed(0, time.Unix(1, 0), []byte(pdu))
	require.NotNil(t, limited.Err)
	still := tight.Feed(1, time.Unix(2, 0), smb2NegotiateResp(0x0311))
	require.NotNil(t, still.Err)
}

func replaySMB(t *testing.T, steps []tcpStep) []*ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 445, false, true)), WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	}))
	require.NoError(t, err)
	return events
}

func TestSMB2ConstructedQueryInfoAndDFSVersions(t *testing.T) {
	// Constructed outputs. The class comes from the same session's request, not from another VLAN.
	cases := []struct {
		typ, class byte
		wire       []byte
		key, text  string
	}{
		{1, 34, make([]byte, 56), "End Of File", ""},
		{1, 22, append(make([]byte, 24), ':', 0, '$', 0, 'D', 0, 'A', 0, 'T', 0, 'A', 0), "Stream Name", ":$DATA"},
		{2, 1, append(make([]byte, 18), 'C', 0, ':', 0), "Volume Label", "C:"},
		{2, 5, append(make([]byte, 12), 'N', 0, 'T', 0, 'F', 0, 'S', 0), "File System Name", "NTFS"},
		{3, 0, make([]byte, 20), "Revision", ""},
	}
	binary.LittleEndian.PutUint32(cases[0].wire[40:44], 7)
	binary.LittleEndian.PutUint32(cases[1].wire[4:8], 12)
	binary.LittleEndian.PutUint32(cases[2].wire[12:16], 4)
	binary.LittleEndian.PutUint32(cases[3].wire[8:12], 8)
	for _, tc := range cases {
		state := &binSMB2{client: 0}
		_, err := state.consumeFrom(0, smb2QueryRequest(21, 17, 7, 1, tc.class, nil), 64)
		if tc.typ != 1 {
			request := smb2QueryRequest(21, 17, 7, tc.typ, tc.class, nil)
			state = &binSMB2{client: 0}
			_, err = state.consumeFrom(0, request, 64)
		}
		require.NoError(t, err)
		info, err := state.consumeFrom(1, smb2QueryResponse(21, 17, 7, 0, tc.wire), 64)
		require.NoError(t, err)
		body := info["Body"].(map[string]any)
		typed := body[info["Output Information Type"].(string)].(map[string]any)
		if tc.text != "" {
			require.Equal(t, tc.text, typed[tc.key])
		}
		require.Contains(t, typed, "Wire Field Ranges")
		short := tc.wire[:len(tc.wire)-1]
		miss := &binSMB2{client: 0}
		_, err = miss.consumeFrom(0, smb2QueryRequest(22, 17, 7, tc.typ, tc.class, nil), 64)
		require.NoError(t, err)
		opaque, err := miss.consumeFrom(1, smb2QueryResponse(22, 17, 7, 0, short), 64)
		require.NoError(t, err)
		require.Contains(t, opaque["Output Content Status"], "opaque")
		require.NotContains(t, opaque, "Output Information Type")
	}
	share := []byte{'\\', 0, 's', 0, 0, 0}
	raw := make([]byte, 8+6+len(share))
	binary.LittleEndian.PutUint16(raw[2:4], 1)
	binary.LittleEndian.PutUint16(raw[8:10], 1)
	binary.LittleEndian.PutUint16(raw[10:12], uint16(6+len(share)))
	copy(raw[14:], share)
	referral, ok, err := smb2DecodeDFSReferralV1(raw, 4)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, `\s`, referral["Referrals"].([]map[string]any)[0]["Share Name"])
	raw[8] = 2
	_, ok, err = smb2DecodeDFSReferralV1(raw, 4)
	require.NoError(t, err)
	require.False(t, ok)
	body := map[string]any{}
	require.NoError(t, smb2DecodeIOCTLOutput(body, smb2FSCTLPipeTransceive, []byte{5, 0, 0, 3}, 80, 4))
	require.Contains(t, body["Control Payload Status"], "PIPE_TRANSCEIVE")
	require.NotContains(t, body["Control Payload Status"], "DCERPC")
}

func TestSMB1RemainingTrans2InformationLevels(t *testing.T) {
	// Constructed records. MS-CIFS 2.2.8.3.6, 2.2.8.3.9, 2.2.8.4.3 and 2.2.8.4.5.
	session := smb1Session(t)
	basicReq := smb1Words(uint16(10), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(10), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	basicParams := []byte{0, 0, 0, 0x01, 0x01, 0, 0, 0, 0, 'a', 0, 0, 0}
	require.Nil(t, session.Feed(0, time.Unix(1, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 21, basicReq, basicParams)).Err)
	data := make([]byte, 40)
	binary.LittleEndian.PutUint64(data[0:8], 0x0102030405060708)
	responseWords := smb1Words(uint16(2), uint16(40), uint16(0), uint16(2), uint16(56), uint16(0), uint16(40), uint16(60), uint16(0), byte(0), byte(0))
	response := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 21, responseWords, append([]byte{0, 0, 0, 0, 0}, data...)))
	record := response.Session["Body"].(map[string]any)["Information Record"].(map[string]any)
	require.Equal(t, "SMB_QUERY_FILE_BASIC_INFO", response.Session["Body"].(map[string]any)["Information Level Name"])
	require.Equal(t, uint64(0x0102030405060708), record["Creation Time"])
	require.Equal(t, []any{uint64(64), uint64(72)}, record["Wire Field Ranges"].(map[string]any)["Creation Time"])
	shortWords := smb1Words(uint16(2), uint16(39), uint16(0), uint16(2), uint16(56), uint16(0), uint16(39), uint16(60), uint16(0), byte(0), byte(0))
	require.Nil(t, session.Feed(0, time.Unix(2, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 22, basicReq, basicParams)).Err)
	short := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 22, shortWords, append([]byte{0, 0, 0, 0, 0}, data[:39]...)))
	require.NotContains(t, short.Session["Body"], "Information Record")
	require.Contains(t, short.Session["Body"].(map[string]any)["Data Status"], "opaque")

	nameReq := smb1Words(uint16(10), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(10), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	nameParams := []byte{0, 0, 0, 0x04, 0x01, 0, 0, 0, 0, 'a', 0, 0, 0}
	require.Nil(t, session.Feed(0, time.Unix(3, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 23, nameReq, nameParams)).Err)
	nameData := []byte{4, 0, 0, 0, 'a', 0, 'b', 0}
	nameWords := smb1Words(uint16(2), uint16(8), uint16(0), uint16(2), uint16(56), uint16(0), uint16(8), uint16(60), uint16(0), byte(0), byte(0))
	named := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 23, nameWords, append([]byte{0, 0, 0, 0, 0}, nameData...)))
	nameRecord := named.Session["Body"].(map[string]any)["Information Record"].(map[string]any)
	require.Equal(t, "ab", nameRecord["File Name"])
	require.Equal(t, []byte{'a', 0, 'b', 0}, nameRecord["File Name Raw Bytes"])
	require.Equal(t, []any{uint64(68), uint64(72)}, nameRecord["Wire Field Ranges"].(map[string]any)["File Name"])
	require.Nil(t, session.Feed(0, time.Unix(4, 0), smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 24, nameReq, nameParams)).Err)
	overflow := []byte{4, 0, 0, 0, 'a', 0}
	overflowWords := smb1Words(uint16(2), uint16(6), uint16(0), uint16(2), uint16(56), uint16(0), uint16(6), uint16(60), uint16(0), byte(0), byte(0))
	opaque := smb1FeedOK(t, session, 1, smb1Command(0x32, 0x80, 0x8000, 2048, 1, 2048, 24, overflowWords, append([]byte{0, 0, 0, 0, 0}, overflow...)))
	require.Contains(t, opaque.Session["Body"].(map[string]any)["Data Status"], "opaque")

	record40 := make([]byte, 40)
	binary.LittleEndian.PutUint64(record40[0:8], 9)
	setWords := smb1Words(uint16(6), uint16(40), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(40), uint16(74), byte(1), byte(0), uint16(8))
	setData := append([]byte{0, 0, 0, 0x00, 0x40, 0x01, 0x01, 0, 0}, record40...)
	setBasic := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 25, setWords, setData))
	setRecord := setBasic.Session["Body"].(map[string]any)["Information Record"].(map[string]any)
	require.Equal(t, "SMB_SET_FILE_BASIC_INFO", setBasic.Session["Body"].(map[string]any)["Information Level Name"])
	require.Equal(t, uint64(9), setRecord["Creation Time"])
	require.Equal(t, []any{uint64(78), uint64(86)}, setRecord["Wire Field Ranges"].(map[string]any)["Creation Time"])

	alloc := make([]byte, 8)
	binary.LittleEndian.PutUint64(alloc, 0x8000000000000000)
	allocWords := smb1Words(uint16(6), uint16(8), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(8), uint16(74), byte(1), byte(0), uint16(8))
	allocData := append([]byte{0, 0, 0, 0x00, 0x40, 0x03, 0x01, 0, 0}, alloc...)
	allocEvent := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 26, allocWords, allocData))
	require.Equal(t, int64(-9223372036854775808), allocEvent.Session["Body"].(map[string]any)["Information Record"].(map[string]any)["Allocation Size"])
	wide := append(allocData, 0)
	wideWords := smb1Words(uint16(6), uint16(9), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(9), uint16(74), byte(1), byte(0), uint16(8))
	wideEvent := smb1FeedOK(t, session, 0, smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 27, wideWords, wide))
	require.NotContains(t, wideEvent.Session["Body"], "Information Record")
}

func smbCount(events []*ProtocolEvent, name string) int {
	n := 0
	for _, event := range events {
		if event.Protocol == "smb2" && event.Session["Packet Name"] == name && (event.Status == "decoded" || event.Status == "deferred") {
			n++
		}
	}
	return n
}
