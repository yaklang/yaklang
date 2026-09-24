package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRTPAdmissionDoesNotClaimCassandra(t *testing.T) {
	var claimed []*ProtocolEvent
	var unsupported, decoded int
	err := ReplayPcap(bytes.NewReader(binCorpusBytes(t, "ndpi/ndpi-cassandra.pcap")),
		WithTCPReassemblyWorkers(1), WithBinParser(func(e *ProtocolEvent) {
			if e.Protocol == "rtp" {
				claimed = append(claimed, e)
			}
			if e.Protocol == "cassandra" && e.Status == "context-required" {
				unsupported++
			}
			if e.Protocol == "cassandra" && e.Status == "decoded" {
				decoded++
			}
		}))
	require.NoError(t, err)
	require.Positive(t, decoded, "Cassandra v4 messages must remain positively recognized")
	require.Positive(t, unsupported, "Cassandra v5 must remain explicitly unsupported")
	for _, e := range claimed {
		t.Errorf("Cassandra claimed as RTP: %s -> %s raw=%x", e.Source, e.Destination, e.Raw)
	}
}
