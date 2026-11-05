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
