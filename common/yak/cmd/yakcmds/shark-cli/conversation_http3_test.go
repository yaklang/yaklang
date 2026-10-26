package sharkcli

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
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
