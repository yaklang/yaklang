package sharkcli

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func smb1SetupDisplaySource(response bool) []byte {
	base := 63
	if response {
		base = 47
	}
	w := make([]byte, base)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x73})
	w[36], w[37] = 12, 0xff
	binary.LittleEndian.PutUint16(w[14:16], 0xc803)
	binary.LittleEndian.PutUint16(w[30:32], 32719)
	binary.LittleEndian.PutUint16(w[34:36], 2)
	if response {
		w[13], w[36] = 0x80, 4
		binary.LittleEndian.PutUint32(w[9:13], 0xc0000016)
		binary.LittleEndian.PutUint16(w[32:34], 2048)
		binary.LittleEndian.PutUint16(w[39:41], 412)
		binary.LittleEndian.PutUint16(w[43:45], 3)
	} else {
		binary.LittleEndian.PutUint16(w[41:43], 12288)
		binary.LittleEndian.PutUint16(w[51:53], 3)
		binary.LittleEndian.PutUint32(w[57:61], 0x80000000)
	}
	w = append(w, 0x91, 0x82, 0x73)
	for _, s := range []string{"Windows source", "source manager"} {
		for _, b := range []byte(s) {
			w = append(w, b, 0)
		}
		w = append(w, 0, 0)
	}
	if !response {
		for i := 0; i < 20; i++ {
			w = append(w, 0xa7)
		}
	}
	binary.LittleEndian.PutUint16(w[base-2:base], uint16(len(w)-base))
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func TestSMB1SetupProgressiveTextAndRawSourcesAcrossWorkers(t *testing.T) {
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	for i, response := range []bool{false, true} {
		w := smb1SetupDisplaySource(response)
		r := s.Feed(i, time.Unix(int64(i+1), 0), w)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		e := r.Events[0]
		e.Fields, err = e.GetFields()
		require.NoError(t, err)
		before, err := json.Marshal(r.Events)
		require.NoError(t, err)
		row := presentEvents(r.Events).apply(packetSummary{Protocol: "TCP", Layers: []string{"Ethernet", "IPv4", "TCP"}})
		require.Equal(t, "SMB1", row.Protocol)
		require.Contains(t, row.Info, "SESSION_SETUP_ANDX")
		require.NotContains(t, row.Info, "NEGOTIATE")
		require.Contains(t, row.Info, "opaque security blob 3 B")
		fields, sources := protocolEventDetail(r.Events)
		for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, r.Events, false), workerMetadataDetail(t, r.Events, true)} {
			header, body, context := -1, -1, -1
			counts := map[string]int{}
			osStart := 66
			if response {
				osStart = 50
			}
			for j, f := range view.fields {
				switch {
				case f.branch && f.text == "Header":
					header = j
				case f.branch && f.text == "Body":
					body = j
					require.Greater(t, body, header)
				case f.branch && f.text == "Observed context":
					context = j
					require.Greater(t, context, body)
				case strings.HasPrefix(f.text, "Native OS:"):
					counts["decoded"]++
					require.Greater(t, j, body)
					require.True(t, f.hasRange)
					require.Equal(t, osStart, f.start)
					require.Equal(t, osStart+28, f.end)
					require.Contains(t, f.text, "Windows source")
				case strings.HasPrefix(f.text, "Native OS Raw Bytes:"):
					counts["raw"]++
					require.True(t, f.hasRange)
					require.Equal(t, osStart, f.start)
					require.Equal(t, osStart+30, f.end)
					require.Equal(t, []byte{0, 0}, view.sources[f.source-1].Data[f.end-2:f.end])
				case strings.HasPrefix(f.text, "Security Blob:"):
					counts["blob"]++
					require.True(t, f.hasRange)
					require.Equal(t, osStart-3, f.start)
					require.Equal(t, osStart, f.end)
				case strings.HasPrefix(f.text, "AndX Offset:"):
					counts["offset"]++
					require.True(t, f.hasRange)
					require.Equal(t, 39, f.start)
					require.Equal(t, 41, f.end)
				case strings.HasPrefix(f.text, "Trailing Extension Bytes:"):
					counts["tail"]++
					if !response {
						require.True(t, f.hasRange)
						require.Equal(t, len(w)-20, f.start)
						require.Equal(t, len(w), f.end)
					}
				case strings.HasPrefix(f.text, "Setup Phase:"):
					counts["phase"]++
					require.Greater(t, j, context)
					require.False(t, f.hasRange)
				case strings.HasPrefix(f.text, "Request Event ID:"):
					require.False(t, f.hasRange)
				}
			}
			for _, key := range []string{"decoded", "raw", "blob", "offset", "tail", "phase"} {
				require.Equal(t, 1, counts[key], key)
			}
			require.Equal(t, w, view.sources[0].Data)
		}
		after, err := json.Marshal(r.Events)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}
