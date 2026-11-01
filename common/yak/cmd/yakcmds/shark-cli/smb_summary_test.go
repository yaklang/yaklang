package sharkcli

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestSMBQuerySummaryNamesOnlyConfirmedOutputAndPreservesIPCIntegers(t *testing.T) {
	for _, class := range []byte{5, 7, 34} {
		t.Run(fmt.Sprint(class), func(t *testing.T) {
			session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
			require.NoError(t, err)
			defer session.Close("test")
			request := smbQueryDisplayRequest()
			request[71] = class
			first := session.Feed(0, time.Unix(1, 0), request)
			require.Nil(t, first.Err)
			body := []byte{9, 0, 72, 0, 4, 0, 0, 0, 41, 0, 0, 0}
			want := "QUERY_INFO response · FileEaInformation · EA size=41 · MID=21"
			if class == 5 {
				body = make([]byte, 32)
				binary.LittleEndian.PutUint16(body, 9)
				binary.LittleEndian.PutUint16(body[2:], 72)
				binary.LittleEndian.PutUint32(body[4:], 24)
				binary.LittleEndian.PutUint64(body[8:], 8192)
				binary.LittleEndian.PutUint64(body[16:], 1<<60|1)
				binary.LittleEndian.PutUint32(body[24:], 2)
				body[29] = 255
				want = "QUERY_INFO response · FileStandardInformation · EOF=1152921504606846977 · allocation=8192 · links=2 · directory=true · MID=21"
			}
			result := session.Feed(1, time.Unix(2, 0), smbDetailTestDirect(smbDetailTestHeader(16, 1, 21, 17, 7, body)))
			require.Nil(t, result.Err)
			require.Len(t, result.Events, 1)
			event := result.Events[0]
			if class == 34 {
				_, ok := smbQueryInfoSummary(event)
				require.False(t, ok)
				require.NotContains(t, applicationEventSummary(event), "FileEaInformation")
				return
			}
			require.Equal(t, want, applicationEventSummary(event))
			require.Contains(t, applicationEventSummary(first.Events[0]), "QUERY_INFO request")
			row := presentEvents(result.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
			require.Equal(t, "SMB2", row.Protocol)
			require.Contains(t, row.Info, want)
			event.Fields, err = event.GetFields()
			require.NoError(t, err)
			fields, sources := protocolEventDetail(result.Events)
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, result.Events, false), workerMetadataDetail(t, result.Events, true)} {
				found := false
				for _, field := range view.fields {
					if field.text == want {
						found = true
					}
				}
				require.True(t, found, "local/packet/stream summary must preserve the exact named type and integer")
			}
			// No class inference when request context, selected type or owned
			// record identity is missing, even though output length is familiar.
			for _, change := range []string{"unmatched", "unselected", "wrong-raw"} {
				copyEvent := *event
				copyEvent.Session = map[string]any{}
				for key, value := range event.Session {
					copyEvent.Session[key] = value
				}
				if change == "unmatched" {
					copyEvent.Session["Association Status"] = "missing-or-ambiguous-request"
				}
				if change == "unselected" {
					delete(copyEvent.Session, "Output Information Type")
				}
				if change == "wrong-raw" {
					copyEvent.Raw = make([]byte, len(event.Raw))
				}
				_, ok := smbQueryInfoSummary(&copyEvent)
				require.False(t, ok, change)
			}
		})
	}
	for _, value := range []any{nil, float64(1 << 60), json.Number("1e20")} {
		_, ok := smbSummaryInteger(value)
		require.False(t, ok, "missing/rounded values cannot be rendered as precise file sizes")
	}
}

func TestSMBCompleteRefusedRecordKeepsSpecificIdentityAndFailureStatus(t *testing.T) {
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	body := make([]byte, 20)
	binary.LittleEndian.PutUint16(body, 17)
	binary.LittleEndian.PutUint16(body[12:], 80)
	binary.LittleEndian.PutUint16(body[14:], 4)
	wire := smbDetailTestDirect(smbDetailTestHeader(1, 0, 99, 0, 0, body))
	result := session.Feed(0, time.Unix(1, 0), wire)
	require.Len(t, result.Events, 1)
	event := result.Events[0]
	require.True(t, smbBodyDiagnostic(event))
	row := presentEvents(result.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp", payloadBytes: len(wire), packetProtocol: "TCP"})
	require.Equal(t, "SMB2", row.Protocol)
	require.Equal(t, "context-required", row.ProtocolStatus)
	require.Contains(t, row.ProtocolScope, "complete SMB2 record")
	require.Contains(t, row.Info, "SESSION_SETUP · unsupported body layout")
	row = applyApplication(row, "smb2", "validated protocol events", 1)
	row = applyApplication(row, "smb2", "validated protocol events", 1)
	require.Equal(t, "context-required", row.ProtocolStatus)
	require.NotContains(t, row.Info, "stream bytes")
	require.NotContains(t, row.Info, "no complete decoded message")
	require.Equal(t, 1, strings.Count(row.Info, "unsupported body layout"))
	_, err = event.GetFields()
	require.Error(t, err, "a display identity is not successful body decoding")
	for _, change := range []string{"port-hint", "incomplete", "wrong-raw", "missing-scope"} {
		copyEvent := *event
		copyEvent.Session = map[string]any{}
		for key, value := range event.Session {
			copyEvent.Session[key] = value
		}
		if change == "port-hint" {
			copyEvent.Profile = "port-hint"
		}
		if change == "incomplete" {
			copyEvent.Completeness = "incomplete"
		}
		if change == "wrong-raw" {
			copyEvent.Raw = make([]byte, len(wire))
		}
		if change == "missing-scope" {
			delete(copyEvent.Session, "Error Scope")
		}
		require.False(t, smbBodyDiagnostic(&copyEvent), change)
		require.Equal(t, "TCP", presentEvents([]*pcaputil.ProtocolEvent{&copyEvent}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"}).Protocol)
	}
}

func TestSMBRefusedAndDecodedCoalescedRecordsKeepBothOutcomes(t *testing.T) {
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	body := make([]byte, 20)
	binary.LittleEndian.PutUint16(body, 17)
	bad := smbDetailTestDirect(smbDetailTestHeader(1, 0, 99, 0, 0, body))
	request := smbQueryDisplayRequest()
	request[71] = 7
	result := session.Feed(0, time.Unix(1, 0), append(bad, request...))
	require.Len(t, result.Events, 2)
	for _, events := range [][]*pcaputil.ProtocolEvent{result.Events, {result.Events[1], result.Events[0]}} {
		row := presentEvents(events).apply(packetSummary{Protocol: "TCP", Transport: "tcp", packetProtocol: "TCP", payloadBytes: len(bad) + len(request)})
		require.Equal(t, "SMB2", row.Protocol)
		require.Equal(t, "partial", row.ProtocolStatus)
		require.Equal(t, "mixed-record-outcomes", row.Visibility)
		require.Contains(t, row.Info, "Mixed SMB2 records")
		require.Contains(t, row.Info, "FileEaInformation")
		require.Contains(t, row.Info, "unsupported body layout")
		row = applyApplication(row, "smb2", "validated protocol events", 1)
		require.Equal(t, "partial", row.ProtocolStatus)
		require.NotContains(t, row.Info, "stream bytes")
	}
}

func TestSMB2ListShowsVersionCommandAndKeepsLiteralDialect(t *testing.T) {
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	negotiate := make([]byte, 36+4)
	binary.LittleEndian.PutUint16(negotiate[0:], 36)
	binary.LittleEndian.PutUint16(negotiate[2:], 2)
	binary.LittleEndian.PutUint16(negotiate[4:], 1)
	binary.LittleEndian.PutUint16(negotiate[36:], 0x0202)
	binary.LittleEndian.PutUint16(negotiate[38:], 0x0210)
	request := session.Feed(0, time.Unix(1, 0), smbDetailTestDirect(smbDetailTestHeader(0, 0, 1, 0, 0, negotiate)))
	require.Nil(t, request.Err)
	require.Contains(t, applicationEventSummary(request.Events[0]), "SMB2 · NEGOTIATE · request")
	require.Contains(t, applicationEventSummary(request.Events[0]), "dialects=2")
	responseBody := make([]byte, 64)
	binary.LittleEndian.PutUint16(responseBody[0:], 65)
	binary.LittleEndian.PutUint16(responseBody[2:], 1)
	binary.LittleEndian.PutUint16(responseBody[4:], 0x0210)
	binary.LittleEndian.PutUint16(responseBody[56:], 128)
	response := session.Feed(1, time.Unix(2, 0), smbDetailTestDirect(smbDetailTestHeader(0, 1, 1, 0, 0, responseBody)))
	require.Nil(t, response.Err)
	require.Contains(t, applicationEventSummary(response.Events[0]), "SMB2 · NEGOTIATE · response")
	response.Events[0].Session["Connection Dialect"] = uint16(0x0311)
	require.Contains(t, applicationEventSummary(response.Events[0]), "dialect=0x0210")
	require.NotContains(t, applicationEventSummary(response.Events[0]), "0x0311")
	negotiateBody := response.Events[0].Session["Body"].(map[string]any)
	span := negotiateBody["Wire Field Ranges"].(map[string]any)["Dialect"].([]any)
	require.Equal(t, []byte{0x10, 0x02}, response.Events[0].Raw[span[0].(uint64):span[1].(uint64)])
	path := make([]byte, 22)
	for i, c := range `\\srv\share` {
		path[i*2] = byte(c)
	}
	treeBody := make([]byte, 8+len(path))
	binary.LittleEndian.PutUint16(treeBody[0:], 9)
	binary.LittleEndian.PutUint16(treeBody[4:], 72)
	binary.LittleEndian.PutUint16(treeBody[6:], uint16(len(path)))
	copy(treeBody[8:], path)
	tree := session.Feed(0, time.Unix(3, 0), smbDetailTestDirect(smbDetailTestHeader(3, 0, 3, 0x11, 0, treeBody)))
	require.Nil(t, tree.Err)
	summary := applicationEventSummary(tree.Events[0])
	require.Contains(t, summary, "SMB2 · TREE_CONNECT · request")
	require.Contains(t, summary, `\\srv\share`)
	require.NotContains(t, summary, "0x0210")
	body := tree.Events[0].Session["Body"].(map[string]any)
	require.NotContains(t, body, "Dialect")
	fields, sources := protocolEventDetail(tree.Events)
	require.NotEmpty(t, sources)
	var sawHeader, sawBody bool
	for _, field := range fields {
		if field.branch && field.text == "Header" {
			sawHeader = true
		}
		if field.branch && field.text == "Body" {
			sawBody = true
		}
	}
	require.True(t, sawHeader)
	require.True(t, sawBody)
	row := presentEvents(tree.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "SMB2", row.Protocol)
	require.Contains(t, row.Info, "TREE_CONNECT")
	for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, tree.Events, false)} {
		require.NotEmpty(t, view.sources)
		require.Equal(t, tree.Events[0].Raw, view.sources[0].Data)
	}
}

func TestSMBQuerySummaryNamesConstructedProfiles(t *testing.T) {
	// Constructed same-session outputs. MS-FSCC 2.4.34, 2.4.40, 2.5.9, 2.5.1 and MS-DTYP 2.4.6.
	stream := append(make([]byte, 24), ':', 0, '$', 0, 'D', 0, 'A', 0, 'T', 0, 'A', 0)
	binary.LittleEndian.PutUint32(stream[4:8], 12)
	volume := append(make([]byte, 18), 'C', 0, ':', 0)
	binary.LittleEndian.PutUint32(volume[12:16], 4)
	fs := append(make([]byte, 12), 'N', 0, 'T', 0, 'F', 0, 'S', 0)
	binary.LittleEndian.PutUint32(fs[8:12], 8)
	network := make([]byte, 56)
	binary.LittleEndian.PutUint64(network[40:48], 7)
	security := make([]byte, 20)
	security[0] = 1
	cases := []struct {
		typ, class byte
		output     []byte
		want       string
	}{
		{1, 34, network, "QUERY_INFO response · FileNetworkOpenInformation · EOF=7 · MID=21"},
		{1, 22, stream, "QUERY_INFO response · FileStreamInformation · stream=:$DATA · MID=21"},
		{2, 1, volume, "QUERY_INFO response · FileFsVolumeInformation · volume=C: · MID=21"},
		{2, 5, fs, "QUERY_INFO response · FileFsAttributeInformation · fs=NTFS · MID=21"},
		{3, 0, security, "QUERY_INFO response · SecurityDescriptor · revision=1 · opaque SID/ACL · MID=21"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
			require.NoError(t, err)
			defer session.Close("test")
			request := smbQueryDisplayRequest()
			request[70], request[71] = tc.typ, tc.class
			require.Nil(t, session.Feed(0, time.Unix(1, 0), request).Err)
			body := make([]byte, 8+len(tc.output))
			binary.LittleEndian.PutUint16(body, 9)
			binary.LittleEndian.PutUint16(body[2:], 72)
			binary.LittleEndian.PutUint32(body[4:], uint32(len(tc.output)))
			copy(body[8:], tc.output)
			result := session.Feed(1, time.Unix(2, 0), smbDetailTestDirect(smbDetailTestHeader(16, 1, 21, 17, 7, body)))
			require.Nil(t, result.Err)
			require.Equal(t, tc.want, applicationEventSummary(result.Events[0]))
			row := presentEvents(result.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
			require.Equal(t, "SMB2", row.Protocol)
			require.Contains(t, row.Info, tc.want)
			fields, sources := protocolEventDetail(result.Events)
			require.Equal(t, result.Events[0].Raw, sources[0].Data)
			header, bodyBranch := false, false
			for _, field := range fields {
				if field.branch && field.text == "Header" {
					header = true
				}
				if field.branch && field.text == "Body" {
					bodyBranch = true
				}
				if field.hasRange {
					require.GreaterOrEqual(t, field.start, 0)
					require.LessOrEqual(t, field.end, len(sources[0].Data))
				}
			}
			require.True(t, header)
			require.True(t, bodyBranch)
			copied := workerMetadataDetail(t, result.Events, false)
			require.Equal(t, sources[0].Data, copied.sources[0].Data)
		})
	}
}

func TestSMB1ListShowsCommandResultAndSeparatesHeader(t *testing.T) {
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	oem := smb1DisplayCommand(0x32, 0x08, 0, 2048, 1, 2048, 3, smb1DisplayWords(uint16(9), uint16(0), uint16(2), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(9), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5)), []byte{0, 0, 0, 0x07, 0x01, 0, 0, 0, 0, 'a', 'b', 0})
	event := session.Feed(0, time.Unix(1, 0), oem)
	require.Nil(t, event.Err)
	summary := applicationEventSummary(event.Events[0])
	require.Contains(t, summary, "SMB1")
	require.Contains(t, summary, "SMB_COM_TRANSACTION2")
	require.Contains(t, summary, "ab")
	row := presentEvents(event.Events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "SMB1", row.Protocol)
	require.Contains(t, row.Info, "ab")
	fields, sources := protocolEventDetail(event.Events)
	require.Equal(t, oem, sources[0].Data)
	header, bodyBranch, nameRange := false, false, false
	for _, field := range fields {
		if field.branch && field.text == "Header" {
			header = true
		}
		if field.branch && field.text == "Body" {
			bodyBranch = true
		}
		if strings.HasPrefix(field.text, "File Name:") && field.hasRange {
			nameRange = true
			require.Equal(t, []byte{'a', 'b'}, sources[0].Data[field.start:field.end])
		}
	}
	require.True(t, header)
	require.True(t, bodyBranch)
	require.True(t, nameRange)
	status := uint32(0xc0000022)
	reply := smb1DisplayCommand(0x06, 0x80, 0x8000, 2048, 1, 2048, 4, nil, nil)
	binary.LittleEndian.PutUint32(reply[9:13], status)
	refused := session.Feed(1, time.Unix(2, 0), reply)
	require.Nil(t, refused.Err)
	require.Contains(t, applicationEventSummary(refused.Events[0]), "SMB_COM_DELETE")
	require.Contains(t, applicationEventSummary(refused.Events[0]), "status=0xc0000022")
	require.Equal(t, false, refused.Events[0].Session["Success Layout"])
	require.NotContains(t, refused.Events[0].Session["Body"], "File Name")
}

func smb1DisplayWords(values ...any) []byte {
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

func smb1DisplayCommand(command, flags byte, flags2, tid, pid, uid, mid uint16, words, data []byte) []byte {
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
