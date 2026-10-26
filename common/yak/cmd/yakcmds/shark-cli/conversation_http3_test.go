package sharkcli

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestHTTP3ConversationProjectionKeepsHeadersAndOriginalPacketProof(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 7, Protocol: "http3", Status: "decoded", Session: map[string]any{"Authentication Verified": true, "HTTP3 Stream ID": uint64(6), "QPACK Resumed Streams": []map[string]any{{"HTTP3 Stream ID": uint64(0), "HTTP3 Frames": []map[string]any{{"Frame Type": "HEADERS", "Stream ID": uint64(0), "Headers": []map[string]any{{"Name": ":path", "Value": "/resumed"}}, "Byte Source": map[string]any{"Ranges": make([]any, 1000)}}}}}}}
	projected := http3ConversationEvent(e)
	require.Less(t, projected.RetainedBytes(), e.RetainedBytes())
	groups := mapList(projected.Session["HTTP3 Message Headers"])
	require.Len(t, groups, 1)
	require.Equal(t, uint64(0), groups[0]["Stream ID"])
	require.Equal(t, "/resumed", mapList(groups[0]["Headers"])[0]["Value"])
	mapList(groups[0]["Headers"])[0]["Value"] = "edited"
	original := mapList(mapList(e.Session["QPACK Resumed Streams"])[0]["HTTP3 Frames"])[0]
	require.Equal(t, "/resumed", mapList(original["Headers"])[0]["Value"])
	require.Len(t, original["Byte Source"].(map[string]any)["Ranges"], 1000, "full packet proof is retained")
}

func TestHTTP3ConversationShowsOtherStreamHeadersAlongsideFieldError(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 7, Protocol: "http3", Transport: "udp", Status: "malformed", Error: "bad field in stream 0", Session: map[string]any{"Frames": []map[string]any{
		{"Stream ID": uint64(0), "HTTP3 Frames": []map[string]any{{"Frame Type": "HEADERS", "Stream ID": uint64(0), "Header Kind": "malformed", "Header Conformance Error": "HTTP3 missing request pseudo-headers", "Headers": []map[string]any{{"Name": ":method", "Value": "GET"}}}}},
		{"Stream ID": uint64(4), "HTTP3 Frames": []map[string]any{{"Frame Type": "HEADERS", "Stream ID": uint64(4), "Header Kind": "request", "Headers": []map[string]any{{"Name": ":path", "Value": "/still-visible"}}}}},
	}}}
	v := &streamSnapshot{transport: "udp", events: []*pcaputil.ProtocolEvent{http3ConversationEvent(e)}}
	var rows []string
	for _, row := range applicationMessageRows(v, 0, 0, 180) {
		rows = append(rows, row.text)
	}
	text := strings.Join(rows, "\n")
	require.Contains(t, text, "bad field in stream 0")
	require.Contains(t, text, "HTTP/3 stream 0 · malformed")
	require.Contains(t, text, "HTTP/3 stream 4 · request")
	require.Contains(t, text, ":path: /still-visible")
}
