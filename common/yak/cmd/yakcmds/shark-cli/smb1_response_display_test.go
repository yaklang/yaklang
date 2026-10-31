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

func smb1ResponseDisplaySource() (request, response []byte) {
	request = make([]byte, 39)
	copy(request[4:], []byte{0xff, 'S', 'M', 'B', 0x72})
	request[13] = 0x18
	binary.LittleEndian.PutUint16(request[14:16], 0xc853)
	binary.LittleEndian.PutUint16(request[16:18], 0x1122)
	binary.LittleEndian.PutUint16(request[30:32], 0x7788)
	binary.LittleEndian.PutUint16(request[34:36], 7)
	for _, name := range []string{"SMB 2.002", "NT LM 0.12"} {
		request = append(request, 2)
		request = append(request, name...)
		request = append(request, 0)
	}
	binary.LittleEndian.PutUint16(request[37:39], uint16(len(request)-39))
	binary.BigEndian.PutUint32(request[:4], uint32(len(request)-4))
	response = make([]byte, 153)
	copy(response[4:36], request[4:36])
	response[13] |= 0x80
	response[36] = 17
	binary.LittleEndian.PutUint16(response[37:39], 1)
	response[39] = 3
	binary.LittleEndian.PutUint16(response[40:42], 50)
	binary.LittleEndian.PutUint16(response[42:44], 1)
	binary.LittleEndian.PutUint32(response[44:48], 65535)
	binary.LittleEndian.PutUint32(response[48:52], 65536)
	binary.LittleEndian.PutUint32(response[52:56], 0x11223344)
	binary.LittleEndian.PutUint32(response[56:60], 0x8000005c)
	binary.LittleEndian.PutUint64(response[60:68], 0xfedcba9876543210)
	binary.LittleEndian.PutUint16(response[68:70], 0xffc4)
	binary.LittleEndian.PutUint16(response[71:73], 80)
	for i := 73; i < len(response); i++ {
		response[i] = byte(i * 13)
	}
	binary.BigEndian.PutUint32(response[:4], uint32(len(response)-4))
	return
}

func TestSMB1ExtendedResponseProgressiveOwnedFieldsAcrossWorkers(t *testing.T) {
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	req, resp := smb1ResponseDisplaySource()
	r := s.Feed(0, time.Unix(1, 0), req)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	r = s.Feed(1, time.Unix(2, 0), resp)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.Fields, err = e.GetFields()
	require.NoError(t, err)
	require.NotContains(t, e.Fields["Wire Field Ranges"], "Selected Dialect Raw Name Bytes")
	before, err := json.Marshal(r.Events)
	require.NoError(t, err)
	row := presentEvents(r.Events).apply(packetSummary{Protocol: "TCP", Layers: []string{"Ethernet", "IPv4", "TCP"}})
	require.Equal(t, "SMB1", row.Protocol)
	require.Contains(t, row.Info, "NT LM 0.12")
	require.Contains(t, row.Info, "dialect index 1")
	require.Contains(t, row.Info, "opaque security blob 64 B")
	fields, sources := protocolEventDetail(r.Events)
	for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, r.Events, false), workerMetadataDetail(t, r.Events, true)} {
		lastHeader, lastBody, lastContext := -1, -1, -1
		counts := map[string]int{}
		for i, field := range view.fields {
			switch {
			case field.branch && field.text == "Header":
				lastHeader = i
			case field.branch && field.text == "Body":
				lastBody = i
				require.Greater(t, lastBody, lastHeader)
			case field.branch && field.text == "Observed context":
				lastContext = i
				require.Greater(t, lastContext, lastBody)
			case strings.HasPrefix(field.text, "System Time:"):
				counts["time"]++
				require.Greater(t, i, lastBody)
				require.True(t, field.hasRange)
				require.Equal(t, 60, field.start)
				require.Equal(t, 68, field.end)
				require.True(t, strings.HasPrefix(field.text, "System Time: 18364758544493064720"), field.text)
				require.Equal(t, uint64(0xfedcba9876543210), binary.LittleEndian.Uint64(view.sources[field.source-1].Data[field.start:field.end]))
			case strings.HasPrefix(field.text, "Server Time Zone:"):
				counts["timezone"]++
				require.Greater(t, i, lastBody)
				require.True(t, field.hasRange)
				require.Equal(t, 68, field.start)
				require.Equal(t, 70, field.end)
				require.True(t, strings.HasPrefix(field.text, "Server Time Zone: -60"), field.text)
			case strings.HasPrefix(field.text, "Server GUID:"):
				counts["guid"]++
				require.True(t, field.hasRange)
				require.Equal(t, 73, field.start)
				require.Equal(t, 89, field.end)
			case strings.HasPrefix(field.text, "Security Blob:"):
				counts["blob"]++
				require.True(t, field.hasRange)
				require.Equal(t, 89, field.start)
				require.Equal(t, 153, field.end)
			case strings.HasPrefix(field.text, "Selected Dialect Name:"):
				counts["selected"]++
				require.Greater(t, i, lastContext)
				require.False(t, field.hasRange)
				require.Contains(t, field.text, "NT LM 0.12")
			case strings.HasPrefix(field.text, "Selected Dialect Raw Name Bytes:"):
				// Context octets are inspectable in their own copied-value source,
				// never in the response PDU. This range describes that copy only.
				require.True(t, field.hasRange)
				require.Greater(t, field.source, 1)
				require.Equal(t, 0, field.start)
				require.Equal(t, 10, field.end)
				source := view.sources[field.source-1]
				require.Contains(t, source.Name, "request context bytes")
				require.Contains(t, source.Name, "response offsets unavailable")
				require.Equal(t, []byte("NT LM 0.12"), source.Data)
			case strings.HasPrefix(field.text, "Word Count:"):
				counts["wct"]++
			case strings.HasPrefix(field.text, "Byte Count:"):
				counts["bcc"]++
			}
		}
		for _, name := range []string{"time", "timezone", "guid", "blob", "selected", "wct", "bcc"} {
			require.Equal(t, 1, counts[name], fmt.Sprint(name, view.fields))
		}
		require.Len(t, view.sources, 2)
		require.Equal(t, resp, view.sources[0].Data)
	}
	after, err := json.Marshal(r.Events)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
