package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestSSHRoleLimitationAndOffersSurviveWorkerIPC(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 4, FlowID: 1, Protocol: "ssh", Transport: "tcp", Status: "decoded", Session: map[string]any{"Packet Name": "KEXINIT", "Context Level": "partial", "Negotiation Context": "client/server role not observed", "Offered Encryption C2S": "aes256-ctr,aes128-ctr", "Kex Algorithms": "diffie-hellman-group14-sha256,curve25519-sha256"}}
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var ipc pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &ipc))
	row := presentEvents([]*pcaputil.ProtocolEvent{&ipc}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"Loopback", "IPv4", "TCP"}})
	require.Equal(t, "SSH", row.Protocol)
	require.Contains(t, row.Info, "client/server role not observed")
	fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{&ipc})
	context, offered := -1, -1
	for i, f := range fields {
		if strings.Contains(f.text, "Negotiation Context: client/server role not observed") {
			context = i
		}
		if strings.Contains(f.text, "Offered Encryption C2S: aes256-ctr,aes128-ctr") {
			offered = i
		}
		require.False(t, strings.HasPrefix(f.text, "Encryption C2S:"), "an offer cannot become a selected cipher")
	}
	require.GreaterOrEqual(t, context, 0)
	require.Greater(t, offered, context, "explain missing role evidence before listing offers")
}
