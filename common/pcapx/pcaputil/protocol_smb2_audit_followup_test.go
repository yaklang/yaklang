package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMB2DialectValuesKeepNumericLiteralsAndRanges(t *testing.T) {
	// Constructed offers. Each value keeps its own two-byte slot in the captured negotiate layout.
	offers := []uint16{0x02ff, 0x0300, 0x0302, 0x02fe}
	request, err := (&binSMB2{client: 0}).consume(smb2NegotiateReq(offers...), 64)
	require.NoError(t, err)
	entries := request["Body"].(map[string]any)["Dialects"].([]map[string]any)
	require.Len(t, entries, len(offers))
	for i, dialect := range offers {
		require.IsType(t, uint16(0), entries[i]["Dialect"])
		require.Equal(t, dialect, entries[i]["Dialect"])
		require.NotEqual(t, "2/3", entries[i]["Dialect"])
		smb2LayoutSpan(t, entries[i], "Dialect", uint64(104+2*i), uint64(106+2*i))
	}

	for _, dialect := range offers {
		t.Run(smb2DialectName(dialect), func(t *testing.T) {
			wire := bytes.Clone(smb2LayoutFixture(t, "negotiate-response.bin"))
			binary.LittleEndian.PutUint16(wire[72:74], dialect)
			info, err := (&binSMB2{client: 0}).consume(wire, 64)
			require.NoError(t, err)
			require.IsType(t, uint16(0), info["Dialect"])
			require.Equal(t, dialect, info["Dialect"])
			require.NotEqual(t, "2/3", info["Dialect"])
			require.Equal(t, smb2DialectName(dialect), info["Version Name"])
			body := info["Body"].(map[string]any)
			require.Equal(t, dialect, body["Dialect"])
			require.IsType(t, uint16(0), body["Dialect"])
			smb2LayoutSpan(t, body, "Dialect", 72, 74)
			require.Equal(t, wire[72:74], []byte{byte(dialect), byte(dialect >> 8)})
		})
	}
}

func TestSMB2CreateContextNextChainOwnsEachPayload(t *testing.T) {
	first := []byte("FIRST111")
	second := []byte("SECOND22")
	fid := bytes.Repeat([]byte{0x41}, 16)
	payload := smb2CreateResp(9, 0x11, 1, fid)[4:]
	payload = append(payload, make([]byte, 64)...)
	binary.LittleEndian.PutUint32(payload[144:148], 152)
	binary.LittleEndian.PutUint32(payload[148:152], 64)
	smb2PutCreateContext(payload, 152, 32, "MxAc", first)
	smb2PutCreateContext(payload, 184, 0, "AAAA", second)
	payload = append(payload, bytes.Repeat([]byte{0xA5}, 8)...)
	wire := smb2TCP(payload)
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2Create(9, 0x11, 1, "owned.txt"), 64)
	require.NoError(t, err)
	info, err := state.consumeFrom(1, wire, 64)
	require.NoError(t, err)
	contexts := info["Body"].(map[string]any)["Create Contexts"].([]map[string]any)
	require.Len(t, contexts, 2)
	require.Equal(t, first, contexts[0]["Data"])
	require.Equal(t, second, contexts[1]["Data"])
	firstAt := bytes.Index(wire, first)
	secondAt := bytes.Index(wire, second)
	require.Greater(t, secondAt, firstAt+len(first))
	smb2LayoutSpan(t, contexts[0], "Data", uint64(firstAt), uint64(firstAt+len(first)))
	smb2LayoutSpan(t, contexts[1], "Data", uint64(secondAt), uint64(secondAt+len(second)))
	require.NotContains(t, contexts[1]["Data"], byte(0xA5))
	require.Len(t, state.files, 1)

	overrun := bytes.Clone(payload)
	binary.LittleEndian.PutUint32(overrun[184:188], 32)
	binary.LittleEndian.PutUint32(overrun[184+12:184+16], 16)
	copy(overrun[184+32:184+40], []byte("NOLEAK!!"))
	binary.LittleEndian.PutUint32(overrun[148:152], 96)
	overrun = append(overrun, make([]byte, 32)...)
	bad := &binSMB2{client: 0}
	_, err = bad.consumeFrom(0, smb2Create(9, 0x11, 1, "owned.txt"), 64)
	require.NoError(t, err)
	_, err = bad.consumeFrom(1, smb2TCP(overrun), 64)
	require.Error(t, err)
	require.Empty(t, bad.files)
	require.Len(t, bad.pending, 1)
}

func smb2PutCreateContext(payload []byte, at int, next uint32, name string, data []byte) {
	binary.LittleEndian.PutUint32(payload[at:], next)
	binary.LittleEndian.PutUint16(payload[at+4:], 16)
	binary.LittleEndian.PutUint16(payload[at+6:], uint16(len(name)))
	binary.LittleEndian.PutUint16(payload[at+10:], 24)
	binary.LittleEndian.PutUint32(payload[at+12:], uint32(len(data)))
	copy(payload[at+16:], name)
	copy(payload[at+24:], data)
}

func TestSMB2DeclaredDataLengthPastCommandDoesNotConsumeNext(t *testing.T) {
	fid := bytes.Repeat([]byte{0x11}, 16)
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2Create(4, 0x11, 1, "kept.txt"), 64)
	require.NoError(t, err)
	_, err = state.consumeFrom(1, smb2CreateResp(4, 0x11, 1, fid), 64)
	require.NoError(t, err)
	_, err = state.consumeFrom(0, smb2Read(5, 0x11, 1, fid, 8), 64)
	require.NoError(t, err)
	require.Len(t, state.files, 1)
	require.Len(t, state.pending, 1)

	read := smb2ReadResp(5, 0x11, 1, nil)[4:]
	read[66] = 80
	binary.LittleEndian.PutUint32(read[68:72], 8)
	binary.LittleEndian.PutUint32(read[20:24], 80)
	closeResp := smb2CloseResp(6, 0x11, 1, nil)[4:]
	_, err = state.consumeFrom(1, smb2TCP(append(read, closeResp...)), 64)
	require.Error(t, err)
	require.Len(t, state.files, 1, "the following CLOSE was not consumed")
	require.Len(t, state.pending, 1, "the READ is still outstanding")

	info, err := state.consumeFrom(1, smb2ReadResp(5, 0x11, 1, []byte("abcdefgh")), 64)
	require.NoError(t, err)
	require.Equal(t, "matched", info["Association Status"])
	require.Equal(t, []byte("abcdefgh"), info["Body"].(map[string]any)["Data"])
	require.Len(t, state.files, 1)
	require.Empty(t, state.pending)
}

func TestSMB2SessionSetupMoreProcessingKeepsSecurityBuffer(t *testing.T) {
	token := []byte{0x11, 0x22, 0x33, 0x44}
	body := make([]byte, 8+len(token))
	binary.LittleEndian.PutUint16(body[0:], 9)
	binary.LittleEndian.PutUint16(body[2:], 1)
	binary.LittleEndian.PutUint16(body[4:], 72)
	binary.LittleEndian.PutUint16(body[6:], uint16(len(token)))
	copy(body[8:], token)
	header := smb2Hdr(1, smb2FlagResp, 2, 0x11, 0)
	binary.LittleEndian.PutUint32(header[8:], 0xC0000016)
	info, err := (&binSMB2{client: 0}).consume(smb2TCP(append(header, body...)), 64)
	require.NoError(t, err)
	require.Equal(t, uint32(0xC0000016), info["Status"])
	require.NotEqual(t, true, info["Error Response"])
	got := info["Body"].(map[string]any)
	require.Equal(t, uint16(9), got["Structure Size"])
	require.Equal(t, uint16(1), got["Session Flags"])
	require.Equal(t, uint16(72), got["Security Buffer Offset"])
	require.Equal(t, uint16(4), got["Security Buffer Length"])
	require.Equal(t, token, got["Security Buffer"])
	require.NotContains(t, got, "Byte Count")
	require.NotContains(t, got, "Error Data")
	smb2LayoutSpan(t, got, "Session Flags", 70, 72)
	smb2LayoutSpan(t, got, "Security Buffer Offset", 72, 74)
	smb2LayoutSpan(t, got, "Security Buffer Length", 74, 76)
	smb2LayoutSpan(t, got, "Security Buffer", 76, 80)
}

func TestSMB2StatusPendingReadKeepsCommandStructureSize(t *testing.T) {
	fid := bytes.Repeat([]byte{0x21}, 16)
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, smb2Read(7, 0x11, 1, fid, 4), 64)
	require.NoError(t, err)
	pending := smb2ReadResp(7, 0x11, 1, nil)
	binary.LittleEndian.PutUint32(pending[12:16], 0x00000103)
	binary.LittleEndian.PutUint32(pending[20:24], smb2FlagResp|smb2FlagAsync)
	binary.LittleEndian.PutUint64(pending[36:44], 0xfeed0001)
	info, err := state.consumeFrom(1, pending, 64)
	require.NoError(t, err)
	require.Equal(t, true, info["Operation Pending"])
	require.Equal(t, uint64(0xfeed0001), info["Async ID"])
	require.NotEqual(t, true, info["Error Response"])
	body := info["Body"].(map[string]any)
	require.Equal(t, uint16(17), body["Structure Size"])
	require.NotContains(t, body, "Byte Count")
	require.NotContains(t, body, "Error Data")
	smb2LayoutSpan(t, body, "Structure Size", 68, 70)
	require.Equal(t, []byte{17, 0}, pending[68:70])
	require.Len(t, state.pending, 1)
	for _, item := range state.pending {
		require.True(t, item.async)
		require.Equal(t, uint64(0xfeed0001), item.asyncID)
		require.Equal(t, "READ", item.name)
	}
}

func TestSMB2LogoffResponseRetiresSessionAndNextGeneration(t *testing.T) {
	createReq := smb2LayoutFixture(t, "create-request.bin")
	createResp := smb2LayoutFixture(t, "create-response.bin")
	logoffReq := smb2LayoutFixture(t, "logoff-request.bin")
	logoffResp := smb2LayoutFixture(t, "logoff-response.bin")
	fid := createResp[132:148]
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	require.Nil(t, session.Feed(0, time.Unix(1, 0), createReq).Err)
	opened := session.Feed(1, time.Unix(2, 0), createResp)
	require.Nil(t, opened.Err)
	require.Equal(t, "opened", opened.Events[0].Session["File Context Status"])
	flow := session.(*captureSession).f.smb2
	require.Len(t, flow.files, 1)
	require.Nil(t, session.Feed(0, time.Unix(3, 0), smb2Read(50, 0x11, 7, fid, 8)).Err)
	require.Len(t, flow.pending, 1)
	requested := session.Feed(0, time.Unix(4, 0), logoffReq)
	require.Nil(t, requested.Err)
	require.Equal(t, "LOGOFF", requested.Events[0].Session["Packet Name"])
	require.NotEqual(t, "logged-off", requested.Events[0].Session["Session Context Status"])
	require.Len(t, flow.files, 1)
	require.Len(t, flow.pending, 2)
	still := session.Feed(0, time.Unix(5, 0), smb2Read(51, 0x11, 7, fid, 4))
	require.Nil(t, still.Err)
	require.Equal(t, "file.txt", still.Events[0].Session["File Context"])
	require.Nil(t, session.Feed(1, time.Unix(6, 0), smb2ReadResp(50, 0x11, 7, []byte("alive"))).Err)
	accepted := session.Feed(1, time.Unix(7, 0), logoffResp)
	require.Nil(t, accepted.Err)
	require.Equal(t, "logged-off", accepted.Events[0].Session["Session Context Status"])
	require.Equal(t, uint16(4), accepted.Events[0].Session["Body"].(map[string]any)["Structure Size"])
	smb2LayoutSpan(t, accepted.Events[0].Session["Body"].(map[string]any), "Reserved", 70, 72)
	require.Empty(t, flow.files)
	require.Empty(t, flow.pending)
	late := session.Feed(1, time.Unix(8, 0), smb2ReadResp(51, 0x11, 7, []byte("gone")))
	require.Nil(t, late.Err)
	require.Equal(t, "missing-or-ambiguous-request", late.Events[0].Session["Association Status"])
	after := session.Feed(0, time.Unix(9, 0), smb2Read(52, 0x11, 7, fid, 4))
	require.Nil(t, after.Err)
	require.Equal(t, "missing-create", after.Events[0].Session["File Context Status"])
	require.NotEqual(t, "file.txt", after.Events[0].Session["File Context"])

	steps := smbTCPHandshake(100, 500)
	client, server := uint32(100), uint32(500)
	smbTCPWrite(&steps, &client, false, createReq)
	smbTCPWrite(&steps, &server, true, createResp)
	smbTCPWrite(&steps, &client, false, smb2Read(50, 0x11, 7, fid, 8))
	smbTCPWrite(&steps, &client, false, logoffReq)
	smbTCPWrite(&steps, &server, true, logoffResp)
	steps = append(steps,
		tcpStep{seq: client, fin: true}, tcpStep{seq: server, fin: true, reverse: true},
		tcpStep{seq: 8000, syn: true}, tcpStep{seq: 9000, ack: 8001, syn: true, synack: true, reverse: true}, tcpStep{seq: 8001, ack: 9001},
	)
	client, server = 8001, 9001
	smbTCPWrite(&steps, &client, false, smb2Read(60, 0x11, 7, fid, 4))
	var events []*ProtocolEvent
	err = ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 445, false, true)), WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	}))
	require.NoError(t, err)
	var first, retired *ProtocolEvent
	for _, event := range events {
		if event.Protocol != "smb2" || (event.Status != "decoded" && event.Status != "deferred") {
			continue
		}
		if event.Session["Packet Name"] == "CREATE" && event.Session["Response"] != true && first == nil {
			first = event
		}
		if event.Session["Message ID"] == uint64(60) {
			retired = event
		}
	}
	require.NotNil(t, first)
	require.NotNil(t, retired)
	require.NotEqual(t, first.FlowID, retired.FlowID)
	require.Equal(t, "missing-create", retired.Session["File Context Status"])
	require.NotEqual(t, "file.txt", retired.Session["File Context"])
}

func TestSMB2TreeDisconnectFromRequestResponseBytes(t *testing.T) {
	createReq := smb2LayoutFixture(t, "create-request.bin")
	createResp := smb2LayoutFixture(t, "create-response.bin")
	disconnectReq := smb2LayoutFixture(t, "tree-disconnect-request.bin")
	disconnectResp := smb2LayoutFixture(t, "tree-disconnect-response.bin")
	fid := createResp[132:148]
	other := bytes.Repeat([]byte{0x42}, 16)
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	require.Nil(t, session.Feed(0, time.Unix(1, 0), createReq).Err)
	require.Nil(t, session.Feed(1, time.Unix(2, 0), createResp).Err)
	require.Nil(t, session.Feed(0, time.Unix(3, 0), smb2Create(70, 0x11, 9, "other.txt")).Err)
	require.Nil(t, session.Feed(1, time.Unix(4, 0), smb2CreateResp(70, 0x11, 9, other)).Err)
	flow := session.(*captureSession).f.smb2
	require.Len(t, flow.files, 2)
	requested := session.Feed(0, time.Unix(5, 0), disconnectReq)
	require.Nil(t, requested.Err)
	require.Equal(t, "TREE_DISCONNECT", requested.Events[0].Session["Packet Name"])
	require.NotEqual(t, "disconnected", requested.Events[0].Session["Tree Context Status"])
	require.Len(t, flow.files, 2)
	require.Equal(t, "file.txt", session.Feed(0, time.Unix(6, 0), smb2Read(71, 0x11, 7, fid, 4)).Events[0].Session["File Context"])
	accepted := session.Feed(1, time.Unix(7, 0), disconnectResp)
	require.Nil(t, accepted.Err)
	require.Equal(t, "disconnected", accepted.Events[0].Session["Tree Context Status"])
	require.Equal(t, uint16(4), accepted.Events[0].Session["Body"].(map[string]any)["Structure Size"])
	smb2LayoutSpan(t, accepted.Events[0].Session["Body"].(map[string]any), "Reserved", 70, 72)
	require.Len(t, flow.files, 1)
	gone := session.Feed(0, time.Unix(8, 0), smb2Read(72, 0x11, 7, fid, 4))
	require.Equal(t, "missing-create", gone.Events[0].Session["File Context Status"])
	kept := session.Feed(0, time.Unix(9, 0), smb2Read(73, 0x11, 9, other, 4))
	require.Equal(t, "other.txt", kept.Events[0].Session["File Context"])
}

func TestSMB2CompressionTransformHeaderStaysOpaque(t *testing.T) {
	payload := append([]byte{0xfe, 'S', 'M', 'B'}, 1, 2, 3, 4)
	wire := smb2CompressionWire(64, 2, 0, 0, payload)
	info, err := (&binSMB2{client: 0}).consume(wire, 64)
	require.NoError(t, err)
	require.Equal(t, "COMPRESSION_TRANSFORM", info["Packet Name"])
	require.Equal(t, false, info["Decompression Performed"])
	require.Equal(t, uint32(smb2CompressionTransform), info["Protocol ID"])
	require.Equal(t, uint32(64), info["Original Compressed Segment Size"])
	require.Equal(t, uint16(2), info["Compression Algorithm"])
	require.Equal(t, uint16(0), info["Flags"])
	require.Equal(t, uint32(0), info["Offset"])
	require.Equal(t, payload, info["Compressed Payload"])
	require.Contains(t, info["Compressed Payload Status"], "not decompressed")
	require.NotContains(t, info, "Commands")
	require.NotContains(t, info, "Dialect")
	smb2LayoutSpan(t, info, "Protocol ID", 4, 8)
	smb2LayoutSpan(t, info, "Original Compressed Segment Size", 8, 12)
	smb2LayoutSpan(t, info, "Compression Algorithm", 12, 14)
	smb2LayoutSpan(t, info, "Flags", 14, 16)
	smb2LayoutSpan(t, info, "Offset", 16, 20)
	smb2LayoutSpan(t, info, "Compressed Payload", 20, 28)
	require.Equal(t, []byte{0xfc, 0x53, 0x4d, 0x42}, wire[4:8])

	chained := smb2CompressionWire(32, 0, 1, 8, payload)
	info, err = (&binSMB2{}).consume(chained, 64)
	require.NoError(t, err)
	require.Equal(t, "COMPRESSION_TRANSFORM", info["Packet Name"])
	require.Contains(t, info["Compressed Payload Status"], "chained")
	require.Equal(t, false, info["Decompression Performed"])
	require.Equal(t, payload, info["Compressed Payload"])
	require.NotContains(t, info, "Commands")

	short := smb2TCP(bytes.Clone(wire[4:19]))
	_, err = (&binSMB2{}).consume(short, 64)
	require.Error(t, err)
	established, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer established.Close("test")
	require.Nil(t, established.Feed(0, time.Unix(1, 0), smb2NegotiateReq(0x0311)).Err)
	flow := established.(*captureSession).f
	consumed, spec, err := flow.frameSMB2(short)
	require.Error(t, err)
	require.Zero(t, consumed)
	require.Nil(t, spec)
	truncated := established.Feed(0, time.Unix(2, 0), short)
	for _, event := range truncated.Events {
		if event.Session != nil {
			require.NotEqual(t, "COMPRESSION_TRANSFORM", event.Session["Packet Name"])
			require.NotEqual(t, "NEGOTIATE", event.Session["Packet Name"])
		}
		require.NotEqual(t, "decoded", event.Status)
		require.NotEqual(t, "deferred", event.Status)
	}

	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	probe := session.Probe(wire)
	require.Equal(t, ProbeAccept, probe.Verdict)
	require.Equal(t, "smb2", probe.Protocol)
	accepted := session.Feed(0, time.Unix(3, 0), wire)
	require.Nil(t, accepted.Err)
	require.Equal(t, "COMPRESSION_TRANSFORM", accepted.Events[0].Session["Packet Name"])
	require.Equal(t, payload, accepted.Events[0].Session["Compressed Payload"])
}

func smb2CompressionWire(original uint32, algorithm, flags uint16, offset uint32, payload []byte) []byte {
	body := make([]byte, smb2CompressionHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(body[0:], smb2CompressionTransform)
	binary.LittleEndian.PutUint32(body[4:], original)
	binary.LittleEndian.PutUint16(body[8:], algorithm)
	binary.LittleEndian.PutUint16(body[10:], flags)
	binary.LittleEndian.PutUint32(body[12:], offset)
	copy(body[smb2CompressionHeaderSize:], payload)
	return smb2TCP(body)
}

func TestSMB2AuditCrossCuttingCases(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"zero-length-path", func(t *testing.T) {
			wire := bytes.Clone(smb2LayoutFixture(t, "tree-connect-request.bin"))
			binary.LittleEndian.PutUint16(wire[74:76], 0)
			_, err := (&binSMB2{}).consume(wire, 64)
			require.Error(t, err)
			require.Contains(t, err.Error(), "path is empty")
		}},
		{"unknown-dialect-literal", func(t *testing.T) {
			wire := bytes.Clone(smb2LayoutFixture(t, "negotiate-response.bin"))
			binary.LittleEndian.PutUint16(wire[72:74], 0x02fe)
			info, err := (&binSMB2{}).consume(wire, 64)
			require.NoError(t, err)
			require.Equal(t, uint16(0x02fe), info["Dialect"])
			require.IsType(t, uint16(0), info["Dialect"])
			require.Equal(t, "2/3", info["Version Name"])
			require.NotEqual(t, "2/3", info["Dialect"])
			smb2LayoutSpan(t, info["Body"].(map[string]any), "Dialect", 72, 74)
		}},
		{"directory-class-not-37", func(t *testing.T) {
			output := make([]byte, 106)
			binary.LittleEndian.PutUint32(output[60:64], 2)
			output[104] = 'Z'
			request := smb2QueryDirectory(21, 0x11, 7, 2, bytes.Repeat([]byte{0x7a}, 16))
			response := smb2QueryDirectoryResp(21, 0x11, 7, output)
			state := &binSMB2{client: 0}
			_, err := state.consumeFrom(0, request, 64)
			require.NoError(t, err)
			info, err := state.consumeFrom(1, response, 64)
			require.NoError(t, err)
			body := info["Body"].(map[string]any)
			require.Contains(t, body["Directory Entry Status"], "not selected")
			require.NotContains(t, body, "Directory Entries")
			require.NotContains(t, body, "File Name")
			control := &binSMB2{client: 0}
			_, err = control.consumeFrom(0, smb2QueryDirectory(21, 0x11, 7, 37, bytes.Repeat([]byte{0x7a}, 16)), 64)
			require.NoError(t, err)
			decoded, err := control.consumeFrom(1, response, 64)
			require.NoError(t, err)
			entries := decoded["Body"].(map[string]any)["Directory Entries"].([]map[string]any)
			require.Equal(t, "Z", entries[0]["File Name"])
		}},
		{"collection-budget-one-short", func(t *testing.T) {
			wire := smb2NegotiateReq(0x02ff, 0x0300, 0x0302, 0x02fe)[4:]
			_, err := smb2Walk(wire, 3)
			require.Error(t, err)
			require.Contains(t, err.Error(), "collection budget")
		}},
		{"collection-budget-one-over", func(t *testing.T) {
			wire := smb2NegotiateReq(0x02ff, 0x0300, 0x0302, 0x02fe)[4:]
			commands, err := smb2Walk(wire, 5)
			require.NoError(t, err)
			require.Len(t, commands[0]["Body"].(map[string]any)["Dialects"].([]map[string]any), 4)
		}},
		{"truncation", func(t *testing.T) {
			short := bytes.Clone(smb2LayoutFixture(t, "read-request.bin"))
			short = short[:len(short)-1]
			binary.BigEndian.PutUint16(short[2:], uint16(len(short)-4))
			_, err := (&binSMB2{}).consume(short, 64)
			require.Error(t, err)
		}},
		{"signed-negative-end-of-file", func(t *testing.T) {
			negative := bytes.Clone(smb2LayoutFixture(t, "create-response.bin"))
			binary.LittleEndian.PutUint64(negative[116:124], ^uint64(0))
			_, err := (&binSMB2{}).consume(negative, 64)
			require.Error(t, err)
			require.Contains(t, err.Error(), "negative")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.fn)
	}
}

func smb2QueryDirectory(mid, sid uint64, tid uint32, class byte, fid []byte) []byte {
	body := make([]byte, 32)
	binary.LittleEndian.PutUint16(body[0:], 33)
	body[2] = class
	copy(body[8:24], fid)
	return smb2TCP(append(smb2Hdr(14, 0, mid, sid, tid), body...))
}

func smb2QueryDirectoryResp(mid, sid uint64, tid uint32, output []byte) []byte {
	body := make([]byte, 8+len(output))
	binary.LittleEndian.PutUint16(body[0:], 9)
	binary.LittleEndian.PutUint16(body[2:], 72)
	binary.LittleEndian.PutUint32(body[4:], uint32(len(output)))
	copy(body[8:], output)
	return smb2TCP(append(smb2Hdr(14, smb2FlagResp, mid, sid, tid), body...))
}
