package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStratumMiningJSONAdmission(t *testing.T) {
	w := []byte("{\"id\":1,\"method\":\"mining.subscribe\",\"params\":[\"lab-stratum/0\"]}\n")
	require.Equal(t, ProbeAccept, probeStratum(w, 64).Verdict)
	require.Equal(t, ProbeNeedMore, probeStratum(w[:24], 64).Verdict)
	for _, bad := range [][]byte{
		[]byte("{\"id\":1,\"method\":\"generic.call\",\"params\":[]}\n"),
		[]byte("{\"id\":1,\"method\":\"mining.subscribe\",\"params\":[]}\n"),
		[]byte("{\"id\":1,\"method\":\"mining.subscribe\",\"params\":[\"a\"]"),
		bytes.Repeat([]byte{'{'}, 64),
	} {
		require.NotEqualf(t, ProbeAccept, probeStratum(bad, 64).Verdict, "%q", bad)
	}
	_, err := parseStratumLine([]byte("{\"id\":1,\"method\":\"mining.authorize\",\"params\":[\"worker\"]}\n"))
	require.Error(t, err)
}

func TestWinlab5013StratumMiningSession(t *testing.T) {
	events := replayWinlab5013Protocols(t, "blue-01-stratum.pcapng")
	var mining []*ProtocolEvent
	for _, event := range events {
		if event.Protocol == "stratum" {
			require.Equal(t, "decoded", event.Status, "%s", event.Error)
			mining = append(mining, event)
		}
	}
	require.Len(t, mining, 7) // subscribe/reply, authorize/reply, notify, submit/reply
	require.Equal(t, "mining.subscribe", mining[0].Fields["Packet Name"])
	require.Equal(t, "lab-stratum/0", mining[0].Fields["Agent"])
	require.Equal(t, "mining.authorize", mining[2].Fields["Packet Name"])
	require.Equal(t, "lab.win01", mining[2].Fields["Worker"])
	require.NotContains(t, mining[2].Fields, "Password")
	require.Equal(t, "mining.notify", mining[4].Fields["Packet Name"])
	require.Equal(t, "7f3a9c", mining[4].Fields["Job ID"])
	require.Equal(t, "mining.submit", mining[5].Fields["Packet Name"])
	require.Equal(t, "7f3a9c", mining[5].Fields["Job ID"])
}
