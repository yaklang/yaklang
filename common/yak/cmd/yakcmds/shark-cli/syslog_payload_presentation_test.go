package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestSyslogBinaryMSGHasOwnedByteSourceAcrossWorkerJSON(t *testing.T) {
	for _, workerJSON := range []bool{false, true} {
		payload := []byte{1, 2, 3, 8, 0, 0x45, 0xff, 0}
		session := map[string]any{"Message Bytes": payload, "Message Encoding": "opaque-octets", "App Name": nil, "Hostname": nil}
		if workerJSON {
			b, err := json.Marshal(session)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(b, &session))
		}
		e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "syslog", Status: "decoded", Completeness: "message", Session: session, Raw: append([]byte("<0>1 - - - - - - "), payload...)}
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		var labels []string
		for _, f := range fields {
			labels = append(labels, f.text)
		}
		require.Contains(t, labels, "App Name: not provided")
		require.Contains(t, labels, "Hostname: not provided")
		found := false
		for _, f := range fields {
			if !strings.HasPrefix(f.text, "Message Bytes:") {
				continue
			}
			found = true
			require.True(t, f.hasRange)
			require.Greater(t, f.source, 0)
			source := sources[f.source-1]
			require.Equal(t, "Syslog MSG · extracted payload", source.Name)
			require.Equal(t, payload, source.Data)
			require.Equal(t, 0, f.start)
			require.Equal(t, len(payload), f.end)
			require.Contains(t, f.text, "8 bytes")
			require.NotContains(t, f.text, "AQID")
			clear(payload)
			require.False(t, bytes.Equal(payload, source.Data), "retained byte sources own their data")
		}
		require.True(t, found)
	}
}
