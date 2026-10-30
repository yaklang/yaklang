package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func smbQueryDisplayRequest() []byte {
	body := make([]byte, 40)
	binary.LittleEndian.PutUint16(body, 41)
	body[2], body[3] = 1, 0x22
	binary.LittleEndian.PutUint32(body[4:], 4096)
	for i := 24; i < 40; i++ {
		body[i] = byte(i)
	}
	return smbDetailTestDirect(smbDetailTestHeader(16, 0, 21, 17, 7, body))
}

func TestSMBQueryFileIDAliasRequiresExactOwnedOccurrence(t *testing.T) {
	for _, change := range []string{"equal", "wrong-value", "wrong-range", "raw-string", "header-type", "other-command"} {
		t.Run(change, func(t *testing.T) {
			event := smbDetailTestNativeEvents(t, smbQueryDisplayRequest())[0]
			body := event.Session["Body"].(map[string]any)
			// Native Fields and Session remain independent owners for this test.
			event.Fields = nil
			switch change {
			case "wrong-value":
				event.Session["FileId"] = strings.Repeat("00", 16)
			case "wrong-range":
				event.Session["Wire Field Ranges"].(map[string]any)["FileId"] = []any{uint64(91), uint64(107)}
			case "raw-string":
				body["File ID"] = hex.EncodeToString(body["File ID"].([]byte))
			case "header-type":
				event.Session["Header"].(map[string]any)["Command"] = uint32(16)
			case "other-command":
				event.Session["Header"].(map[string]any)["Command"] = uint16(5)
			}
			before, err := json.Marshal(event)
			require.NoError(t, err)
			projected := smbDisplayEvent(event)
			_, remains := projected.Session["FileId"]
			require.Equal(t, change != "equal", remains)
			after, err := json.Marshal(event)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSMBQueryResponseProgressiveMeaningAndSourcesAcrossWorkers(t *testing.T) {
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	request := session.Feed(0, time.Unix(1, 0), smbQueryDisplayRequest())
	require.Nil(t, request.Err)
	require.Len(t, request.Events, 1)
	body := make([]byte, 12)
	binary.LittleEndian.PutUint16(body, 9)
	binary.LittleEndian.PutUint16(body[2:], 72)
	binary.LittleEndian.PutUint32(body[4:], 4)
	copy(body[8:], []byte{9, 0, 0x34, 0x12}) // A prefix is not a request selector.
	wire := smbDetailTestDirect(smbDetailTestHeader(16, 1, 21, 17, 7, body))
	response := session.Feed(1, time.Unix(2, 0), wire)
	require.Nil(t, response.Err)
	require.Len(t, response.Events, 1)
	event := response.Events[0]
	event.Fields, err = event.GetFields()
	require.NoError(t, err)
	before, err := json.Marshal(event)
	require.NoError(t, err)
	fields, sources := protocolEventDetail(response.Events)
	for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, response.Events, false), workerMetadataDetail(t, response.Events, true)} {
		header, payload, context, selector := -1, -1, -1, -1
		foundOutput := false
		for index, field := range view.fields {
			switch {
			case field.branch && field.text == "Header":
				header = index
			case field.branch && field.text == "Body":
				payload = index
			case field.branch && field.text == "Observed context":
				context = index
			case strings.HasPrefix(field.text, "Request Info Type:"):
				selector = index
				require.False(t, field.hasRange)
				require.Zero(t, field.source)
			case strings.HasPrefix(field.text, "Output Buffer:"):
				foundOutput = true
				require.True(t, field.hasRange)
				require.Equal(t, 76, field.start)
				require.Equal(t, 80, field.end)
				require.Equal(t, []byte{9, 0, 0x34, 0x12}, view.sources[field.source-1].Data[field.start:field.end])
			}
		}
		require.GreaterOrEqual(t, header, 0)
		require.Greater(t, payload, header)
		require.Greater(t, context, payload)
		require.Greater(t, selector, context)
		require.True(t, foundOutput)
		require.True(t, bytes.Equal(wire, view.sources[0].Data))
	}
	after, err := json.Marshal(event)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestSMBQueryRequestFileIDAcrossWorkers(t *testing.T) {
	wire := smbQueryDisplayRequest()
	events := smbDetailTestNativeEvents(t, wire)
	before, err := json.Marshal(events)
	require.NoError(t, err)
	fields, sources := protocolEventDetail(events)
	for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, events, false), workerMetadataDetail(t, events, true)} {
		require.Equal(t, 1, smbDetailTestBranchCount(view.fields, "Body"))
		require.Zero(t, smbDetailTestLeafCount(view.fields, "FileId"), "only the proven compatibility alias is omitted")
		require.Equal(t, 1, smbDetailTestLeafCount(view.fields, "File ID"))
		for _, field := range view.fields {
			if strings.HasPrefix(field.text, "File ID:") {
				require.True(t, field.hasRange)
				require.Equal(t, 92, field.start)
				require.Equal(t, 108, field.end)
				require.Equal(t, wire[92:108], view.sources[field.source-1].Data[field.start:field.end])
			}
		}
	}
	after, err := json.Marshal(events)
	require.NoError(t, err)
	require.Equal(t, before, after, "SDK values and both original representations remain available")
}
